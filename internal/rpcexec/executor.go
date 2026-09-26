package rpcexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Service coordinates routine execution.
type Service struct {
	caller Caller
	txEnd  config.TxEnd
}

// New returns a new Service implementing Executor.
func New(caller Caller, txEnd config.TxEnd) *Service {
	return &Service{
		caller: caller,
		txEnd:  txEnd,
	}
}

// Execute validates and runs a routine call.
func (s *Service) Execute(ctx context.Context, intent Intent) (Outcome, error) {
	kind := strings.ToUpper(intent.Routine.Kind)
	if kind != "FUNCTION" && kind != "PROCEDURE" {
		return Outcome{}, fmt.Errorf("unknown routine kind %q", intent.Routine.Kind)
	}

	if intent.CallMode == CallModeGet && !intent.Routine.ReadSafe() {
		return Outcome{}, ReadSafetyViolation{}
	}

	if missing, ok := missingRequiredArgument(intent.Routine, intent.Args); ok {
		return Outcome{}, SignatureMismatch{
			Routine: intent.Routine.ID,
			Missing: missing,
		}
	}
	if unknown, ok := unknownArgument(intent.Routine, intent.Args); ok {
		return Outcome{}, SignatureMismatch{
			Routine: intent.Routine.ID,
			Unknown: unknown,
		}
	}

	options := CallOptions{
		PreferTx: intent.PreferTx,
		Validate: s.buildValidator(intent),
	}

	rawResult, err := s.caller.Call(ctx, intent.Role, intent.Routine, intent.Args, options)
	if err != nil {
		return Outcome{}, err
	}

	commit, applied := config.DecideTxEnd(s.txEnd, intent.PreferTx)
	txOutcome := TxOutcome{
		Committed:     commit,
		PreferApplied: applied,
	}

	return s.discriminateOutcome(intent.Routine, rawResult, txOutcome)
}

func (s *Service) buildValidator(intent Intent) func(any) error {
	needSingular := intent.Representation == RepresentationSingularObject
	needRowSet := readquery.HasRowSetFeatures(intent.Query)
	if !needSingular && !needRowSet {
		return nil
	}
	return func(result any) error {
		set, tabular := rowSetResult(result)
		if needRowSet && !tabular {
			return RowSetFeaturesRefusal{}
		}
		if !tabular {
			return nil
		}
		rowCount, err := validateRepresentation(set, intent.Query)
		if err != nil {
			return err
		}
		if needSingular && rowCount != 1 {
			return SingularObjectRefusal{RowCount: rowCount}
		}
		return nil
	}
}

func (s *Service) discriminateOutcome(
	routine schemacache.RoutineFact,
	rawResult any,
	txOutcome TxOutcome,
) (Outcome, error) {
	set, tabular := rowSetResult(rawResult)
	if tabular {
		return Outcome{
			Kind:      ResultKindRowSet,
			Data:      rawResult,
			Rows:      set,
			TxOutcome: txOutcome,
		}, nil
	}

	if strings.EqualFold(routine.Kind, "FUNCTION") {
		return Outcome{
			Kind:      ResultKindScalar,
			Data:      rawResult,
			TxOutcome: txOutcome,
		}, nil
	}

	return Outcome{
		Kind:      ResultKindObject,
		Data:      rawResult,
		TxOutcome: txOutcome,
	}, nil
}

func rowSetResult(result any) ([]rows.Row, bool) {
	set, ok := result.([]rows.Row)
	if !ok {
		return nil, false
	}
	return set, true
}

// validateRepresentation applies filters and pagination for the singular row
// count and checks projection before the routine transaction commits.
func validateRepresentation(set []rows.Row, query readquery.Query) (int, error) {
	shaped, err := readquery.Shape(set, query)
	if err != nil {
		return 0, err
	}
	if _, err := readquery.Project(set, query); err != nil {
		return 0, err
	}
	return len(shaped.Rows), nil
}

func missingRequiredArgument(routine schemacache.RoutineFact, args map[string]any) (string, bool) {
	for _, param := range routine.Parameters {
		if !inputParameter(param) {
			continue
		}
		if _, held := args[param.Name]; !held {
			return param.Name, true
		}
	}
	return "", false
}

func unknownArgument(routine schemacache.RoutineFact, args map[string]any) (string, bool) {
	allowed := map[string]struct{}{}
	for _, param := range routine.Parameters {
		if inputParameter(param) {
			allowed[param.Name] = struct{}{}
		}
	}
	for name := range args {
		if _, held := allowed[name]; !held {
			return name, true
		}
	}
	return "", false
}

func inputParameter(param schemacache.ParameterFact) bool {
	if param.Ordinal == 0 || param.Name == "" {
		return false
	}
	return !strings.EqualFold(param.Mode, "OUT")
}
