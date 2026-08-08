package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Call runs a routine as the database role with named JSON arguments.
// Functions return the scalar value. Procedures return []rows.Row when CALL
// yields a SELECT result set; otherwise they return the stable object of OUT
// and INOUT parameter values (an empty object when there are none).
func (p *Pool) Call(
	ctx context.Context,
	role schemacache.Role,
	routine schemacache.RoutineFact,
	args map[string]any,
	options httpapi.CallOptions,
) (any, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return nil, err
	}

	var result any
	err = p.withRequestTx(ctx, statement, options.PreferTx, func(ctx context.Context, tx *sql.Tx) error {
		var callErr error
		result, callErr = callRoutine(ctx, tx, routine, args)
		return callErr
	})
	return result, err
}

func callRoutine(
	ctx context.Context,
	tx *sql.Tx,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	switch strings.ToUpper(routine.Kind) {
	case "FUNCTION":
		return callFunction(ctx, tx, routine, args)
	case "PROCEDURE":
		return callProcedure(ctx, tx, routine, args)
	default:
		return nil, fmt.Errorf("unknown routine kind %q", routine.Kind)
	}
}

func callFunction(
	ctx context.Context,
	tx *sql.Tx,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	params := inputParameters(routine)
	values, err := bindArguments(params, args)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRowContext(ctx, functionCallStatement(routine, len(params)), values...)
	var value any
	if err := row.Scan(&value); err != nil {
		return nil, err
	}
	return jsonValue(value, nil), nil
}

type procedureCall struct {
	placeholders []string
	bound        []any
	outNames     []string
	outVars      []string
}

func callProcedure(
	ctx context.Context,
	tx *sql.Tx,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	built, err := buildProcedureCall(ctx, tx, callableParameters(routine), args)
	if err != nil {
		return nil, err
	}
	result, err := tx.QueryContext(
		ctx,
		procedureCallStatement(routine, built.placeholders),
		built.bound...,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	set, tabular, err := readFirstResultSet(result)
	if err != nil {
		return nil, err
	}
	// Drain further result sets so the connection can read OUT variables.
	for result.NextResultSet() {
	}
	if tabular {
		return set, nil
	}
	return readProcedureOutputs(ctx, tx, built.outNames, built.outVars)
}

// readFirstResultSet reads the first CALL result set. A set with column
// metadata is tabular even when it holds zero rows.
func readFirstResultSet(result *sql.Rows) ([]rows.Row, bool, error) {
	columns, err := result.Columns()
	if err != nil {
		return nil, false, err
	}
	if len(columns) == 0 {
		return nil, false, nil
	}
	set := []rows.Row{}
	for result.Next() {
		values, err := scanValues(result, len(columns))
		if err != nil {
			return nil, false, err
		}
		set = append(set, rows.Row{
			Columns: append([]string(nil), columns...),
			Values:  values,
		})
	}
	if err := result.Err(); err != nil {
		return nil, false, err
	}
	return set, true, nil
}

func buildProcedureCall(
	ctx context.Context,
	tx *sql.Tx,
	params []schemacache.ParameterFact,
	args map[string]any,
) (procedureCall, error) {
	built := procedureCall{
		placeholders: make([]string, len(params)),
		bound:        make([]any, 0, len(params)),
	}
	for i, param := range params {
		switch strings.ToUpper(param.Mode) {
		case "IN":
			value, err := argumentValue(param.Name, args)
			if err != nil {
				return procedureCall{}, err
			}
			built.placeholders[i] = "?"
			built.bound = append(built.bound, value)
		case "OUT", "INOUT":
			if err := bindOutParameter(ctx, tx, &built, i, param, args); err != nil {
				return procedureCall{}, err
			}
		default:
			return procedureCall{}, fmt.Errorf("unsupported parameter mode %q", param.Mode)
		}
	}
	return built, nil
}

func bindOutParameter(
	ctx context.Context,
	tx *sql.Tx,
	built *procedureCall,
	index int,
	param schemacache.ParameterFact,
	args map[string]any,
) error {
	name := userVarName(len(built.outVars))
	if strings.EqualFold(param.Mode, "INOUT") {
		value, err := argumentValue(param.Name, args)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "SET "+name+" = ?", value); err != nil {
			return err
		}
	} else if _, err := tx.ExecContext(ctx, "SET "+name+" = NULL"); err != nil {
		return err
	}
	built.placeholders[index] = name
	built.outNames = append(built.outNames, param.Name)
	built.outVars = append(built.outVars, name)
	return nil
}

func readProcedureOutputs(
	ctx context.Context,
	tx *sql.Tx,
	outNames, outVars []string,
) (rows.Row, error) {
	values := make([]any, len(outNames))
	for i := range outNames {
		var value any
		if err := tx.QueryRowContext(ctx, "SELECT "+outVars[i]).Scan(&value); err != nil {
			return rows.Row{}, err
		}
		values[i] = jsonValue(value, nil)
	}
	return rows.Row{Columns: append([]string(nil), outNames...), Values: values}, nil
}

func inputParameters(routine schemacache.RoutineFact) []schemacache.ParameterFact {
	return filterParameters(routine, func(param schemacache.ParameterFact) bool {
		return param.Ordinal != 0
	})
}

func callableParameters(routine schemacache.RoutineFact) []schemacache.ParameterFact {
	return filterParameters(routine, func(param schemacache.ParameterFact) bool {
		return param.Ordinal != 0 && param.Name != ""
	})
}

func filterParameters(
	routine schemacache.RoutineFact,
	keep func(schemacache.ParameterFact) bool,
) []schemacache.ParameterFact {
	params := make([]schemacache.ParameterFact, 0, len(routine.Parameters))
	for _, param := range routine.Parameters {
		if keep(param) {
			params = append(params, param)
		}
	}
	return params
}

// MissingArgument says the JSON body did not name a required argument.
type MissingArgument struct {
	Name string
}

func (e MissingArgument) Error() string {
	return fmt.Sprintf("missing argument %q", e.Name)
}

func bindArguments(params []schemacache.ParameterFact, args map[string]any) ([]any, error) {
	values := make([]any, len(params))
	for i, param := range params {
		value, err := argumentValue(param.Name, args)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

func argumentValue(name string, args map[string]any) (any, error) {
	value, held := args[name]
	if !held {
		return nil, MissingArgument{Name: name}
	}
	return value, nil
}

func functionCallStatement(routine schemacache.RoutineFact, argc int) string {
	marks := make([]string, argc)
	for i := range marks {
		marks[i] = "?"
	}
	return fmt.Sprintf(
		"SELECT %s.%s(%s)",
		quoteIdentifier(routine.ID.Database),
		quoteIdentifier(routine.ID.Name),
		strings.Join(marks, ", "),
	)
}

func procedureCallStatement(routine schemacache.RoutineFact, placeholders []string) string {
	return fmt.Sprintf(
		"CALL %s.%s(%s)",
		quoteIdentifier(routine.ID.Database),
		quoteIdentifier(routine.ID.Name),
		strings.Join(placeholders, ", "),
	)
}

func userVarName(index int) string {
	return fmt.Sprintf("@myrest_out_%d", index)
}
