package mysqldb

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
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

func TestBindArgumentsSerializesJSONForJSONParameter(t *testing.T) {
	t.Parallel()

	params := []schemacache.ParameterFact{
		{Name: "doc", Mode: "IN", Ordinal: 1, DataType: "json"},
		{Name: "items", Mode: "IN", Ordinal: 2, DataType: "JSON"},
	}
	values, err := bindArguments(params, map[string]any{
		"doc":   map[string]any{"k": float64(1)},
		"items": []any{float64(1), float64(2)},
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("values len = %d, want 2", len(values))
	}
	if values[0] != `{"k":1}` {
		t.Fatalf("values[0] = %#v, want {\"k\":1}", values[0])
	}
	if values[1] != `[1,2]` {
		t.Fatalf("values[1] = %#v, want [1,2]", values[1])
	}
}

func TestBindArgumentsRefusesObjectForNonJSONParameter(t *testing.T) {
	t.Parallel()

	params := []schemacache.ParameterFact{
		{Name: "src", Mode: "IN", Ordinal: 1, DataType: "varchar"},
	}
	_, err := bindArguments(params, map[string]any{
		"src": map[string]any{"k": float64(1)},
	})
	var gap readquery.UnsupportedFeature
	if !errors.As(err, &gap) {
		t.Fatalf("err = %v, want UnsupportedFeature", err)
	}
	want := "Cannot pass a JSON object to parameter src: the parameter does not hold JSON"
	if gap.Message != want {
		t.Fatalf("message = %q, want %q", gap.Message, want)
	}
}

func TestBindArgumentsRefusesArrayForNonJSONParameter(t *testing.T) {
	t.Parallel()

	params := []schemacache.ParameterFact{
		{Name: "src", Mode: "IN", Ordinal: 1, DataType: "varchar"},
	}
	_, err := bindArguments(params, map[string]any{
		"src": []any{"a", "b"},
	})
	var gap readquery.UnsupportedFeature
	if !errors.As(err, &gap) {
		t.Fatalf("err = %v, want UnsupportedFeature", err)
	}
	want := "Cannot pass a JSON array to parameter src: the parameter does not hold JSON"
	if gap.Message != want {
		t.Fatalf("message = %q, want %q", gap.Message, want)
	}
}

func TestBindArgumentsKeepsScalarsStringsAndNulls(t *testing.T) {
	t.Parallel()

	params := []schemacache.ParameterFact{
		{Name: "name", Mode: "IN", Ordinal: 1, DataType: "varchar"},
		{Name: "count", Mode: "IN", Ordinal: 2, DataType: "int"},
		{Name: "doc", Mode: "IN", Ordinal: 3, DataType: "json"},
	}
	values, err := bindArguments(params, map[string]any{
		"name":  "hello",
		"count": float64(42),
		"doc":   nil,
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("values len = %d, want 3", len(values))
	}
	if values[0] != "hello" {
		t.Fatalf("values[0] = %#v, want hello", values[0])
	}
	if values[1] != float64(42) {
		t.Fatalf("values[1] = %#v, want 42", values[1])
	}
	if values[2] != nil {
		t.Fatalf("values[2] = %#v, want nil", values[2])
	}
}
