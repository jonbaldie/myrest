package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Call runs a routine as the database role with named JSON arguments.
// Functions return the scalar value. Procedures return the stable object of
// OUT and INOUT parameter values (an empty object when there are none).
func (p *Pool) Call(
	ctx context.Context,
	role schemacache.Role,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return nil, err
	}

	var result any
	err = p.onConnection(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var callErr error
		result, callErr = callRoutine(ctx, conn, routine, args)
		return callErr
	})
	return result, err
}

func callRoutine(
	ctx context.Context,
	conn *sql.Conn,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	switch strings.ToUpper(routine.Kind) {
	case "FUNCTION":
		return callFunction(ctx, conn, routine, args)
	case "PROCEDURE":
		return callProcedure(ctx, conn, routine, args)
	default:
		return nil, fmt.Errorf("unknown routine kind %q", routine.Kind)
	}
}

func callFunction(
	ctx context.Context,
	conn *sql.Conn,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	params := inputParameters(routine)
	values, err := bindArguments(params, args)
	if err != nil {
		return nil, err
	}

	row := conn.QueryRowContext(ctx, functionCallStatement(routine, len(params)), values...)
	var value any
	if err := row.Scan(&value); err != nil {
		return nil, err
	}
	return jsonValue(value), nil
}

type procedureCall struct {
	placeholders []string
	bound        []any
	outNames     []string
	outVars      []string
}

func callProcedure(
	ctx context.Context,
	conn *sql.Conn,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	built, err := buildProcedureCall(ctx, conn, callableParameters(routine), args)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, procedureCallStatement(routine, built.placeholders), built.bound...); err != nil {
		return nil, err
	}
	return readProcedureOutputs(ctx, conn, built.outNames, built.outVars)
}

func buildProcedureCall(
	ctx context.Context,
	conn *sql.Conn,
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
			if err := bindOutParameter(ctx, conn, &built, i, param, args); err != nil {
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
	conn *sql.Conn,
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
		if _, err := conn.ExecContext(ctx, "SET "+name+" = ?", value); err != nil {
			return err
		}
	} else if _, err := conn.ExecContext(ctx, "SET "+name+" = NULL"); err != nil {
		return err
	}
	built.placeholders[index] = name
	built.outNames = append(built.outNames, param.Name)
	built.outVars = append(built.outVars, name)
	return nil
}

func readProcedureOutputs(
	ctx context.Context,
	conn *sql.Conn,
	outNames, outVars []string,
) (map[string]any, error) {
	result := map[string]any{}
	for i, name := range outNames {
		var value any
		if err := conn.QueryRowContext(ctx, "SELECT "+outVars[i]).Scan(&value); err != nil {
			return nil, err
		}
		result[name] = jsonValue(value)
	}
	return result, nil
}

func inputParameters(routine schemacache.RoutineFact) []schemacache.ParameterFact {
	params := make([]schemacache.ParameterFact, 0, len(routine.Parameters))
	for _, param := range routine.Parameters {
		if param.Ordinal == 0 {
			continue
		}
		params = append(params, param)
	}
	return params
}

func callableParameters(routine schemacache.RoutineFact) []schemacache.ParameterFact {
	params := make([]schemacache.ParameterFact, 0, len(routine.Parameters))
	for _, param := range routine.Parameters {
		if param.Ordinal == 0 || param.Name == "" {
			continue
		}
		params = append(params, param)
	}
	return params
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
		return nil, fmt.Errorf("missing argument %q", name)
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
