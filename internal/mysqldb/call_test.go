package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestFunctionCallStatementUsesNamedPlaceholders(t *testing.T) {
	t.Parallel()

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "add_them"},
		Kind: "FUNCTION",
	}
	got := functionCallStatement(routine, 2)
	want := "SELECT `shop`.`add_them`(?, ?)"
	if got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
}

func TestProcedureCallStatementUsesPlaceholders(t *testing.T) {
	t.Parallel()

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "echo_name"},
		Kind: "PROCEDURE",
	}
	got := procedureCallStatement(routine, []string{"?", "@myrest_out_0"})
	want := "CALL `shop`.`echo_name`(?, @myrest_out_0)"
	if got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
}

func TestBindArgumentsFollowsParameterOrder(t *testing.T) {
	t.Parallel()

	params := []schemacache.ParameterFact{
		{Name: "a", Mode: "IN", Ordinal: 1},
		{Name: "b", Mode: "IN", Ordinal: 2},
	}
	values, err := bindArguments(params, map[string]any{"b": float64(2), "a": float64(1)})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(values) != 2 || values[0] != float64(1) || values[1] != float64(2) {
		t.Fatalf("values = %#v, want [1, 2]", values)
	}
}

func TestBindArgumentsNeedsEveryNamedArgument(t *testing.T) {
	t.Parallel()

	_, err := bindArguments(
		[]schemacache.ParameterFact{{Name: "a", Mode: "IN", Ordinal: 1}},
		map[string]any{},
	)
	if err == nil {
		t.Fatal("missing argument was accepted")
	}
}

func TestInputParametersSkipTheReturnSlot(t *testing.T) {
	t.Parallel()

	routine := schemacache.RoutineFact{
		Parameters: []schemacache.ParameterFact{
			{Ordinal: 0, DataType: "bigint"},
			{Name: "a", Mode: "IN", Ordinal: 1, DataType: "bigint"},
		},
	}
	got := inputParameters(routine)
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("params = %#v, want [a]", got)
	}
}
