package rpcexec_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/rpcexec"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

type spyCaller struct {
	called  bool
	role    schemacache.Role
	routine schemacache.RoutineFact
	args    map[string]any
	options rpcexec.CallOptions
	result  any
	err     error
}

func (c *spyCaller) Call(
	ctx context.Context,
	role schemacache.Role,
	routine schemacache.RoutineFact,
	args map[string]any,
	options rpcexec.CallOptions,
) (any, error) {
	if ctx == nil {
		panic("nil context passed to caller")
	}
	c.called = true
	c.role = role
	c.routine = routine
	c.args = args
	c.options = options
	if c.err != nil {
		return nil, c.err
	}
	if options.Validate != nil {
		if err := options.Validate(c.result); err != nil {
			return nil, err
		}
	}
	return c.result, nil
}

func testFunction() schemacache.RoutineFact {
	return schemacache.RoutineFact{
		ID:            schemacache.RoutineID{Database: "shop", Name: "add_them"},
		Kind:          "FUNCTION",
		ReturnType:    "bigint",
		SQLDataAccess: "NO SQL",
		Parameters: []schemacache.ParameterFact{
			{Ordinal: 0, DataType: "bigint"},
			{Name: "a", Mode: "IN", Ordinal: 1, DataType: "bigint"},
			{Name: "b", Mode: "IN", Ordinal: 2, DataType: "bigint"},
		},
	}
}

func TestExecuteMissingRequiredArgument(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(3)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := testFunction()
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{"a": float64(1)},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}

	var mismatch rpcexec.SignatureMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected SignatureMismatch error, got %T: %v", err, err)
	}
	if mismatch.Routine != routine.ID {
		t.Fatalf("mismatch.Routine = %v, want %v", mismatch.Routine, routine.ID)
	}
	if mismatch.Missing != "b" {
		t.Fatalf("mismatch.Missing = %q, want %q", mismatch.Missing, "b")
	}
	if mismatch.Error() == "" {
		t.Fatal("expected non-empty Error()")
	}
	if caller.called {
		t.Fatal("caller was invoked despite missing argument")
	}
}

func TestExecuteUnknownArgument(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(3)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := testFunction()
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{"a": float64(1), "b": float64(2), "c": float64(3)},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}

	var mismatch rpcexec.SignatureMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected SignatureMismatch error, got %T: %v", err, err)
	}
	if mismatch.Routine != routine.ID {
		t.Fatalf("mismatch.Routine = %v, want %v", mismatch.Routine, routine.ID)
	}
	if mismatch.Unknown != "c" {
		t.Fatalf("mismatch.Unknown = %q, want %q", mismatch.Unknown, "c")
	}
	if mismatch.Error() == "" {
		t.Fatal("expected non-empty Error()")
	}
	if caller.called {
		t.Fatal("caller was invoked despite unknown argument")
	}
}

func TestExecuteGetReadSafetyEnforced(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(1)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:            schemacache.RoutineID{Database: "shop", Name: "write_log"},
		Kind:          "PROCEDURE",
		SQLDataAccess: "MODIFIES SQL DATA",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModeGet,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected ReadSafetyViolation, got nil")
	}

	var violation rpcexec.ReadSafetyViolation
	if !errors.As(err, &violation) {
		t.Fatalf("expected ReadSafetyViolation, got %T: %v", err, err)
	}
	if caller.called {
		t.Fatal("caller was invoked despite read-safety violation")
	}
}

func TestExecuteGetReadSafeAllowed(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(42)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := testFunction() // NO SQL
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
		CallMode: rpcexec.CallModeGet,
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !caller.called {
		t.Fatal("caller was not invoked")
	}
	if outcome.Kind != rpcexec.ResultKindScalar {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindScalar)
	}
	if outcome.Data != int64(42) {
		t.Fatalf("outcome.Data = %v, want 42", outcome.Data)
	}
}

func TestExecutePostNonReadSafeAllowed(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(1)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:            schemacache.RoutineID{Database: "shop", Name: "write_log"},
		Kind:          "PROCEDURE",
		SQLDataAccess: "MODIFIES SQL DATA",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected POST to succeed for non-read-safe routine, got %v", err)
	}
	if !caller.called {
		t.Fatal("caller was not invoked for POST")
	}
}

func TestExecuteProcedureRowSet(t *testing.T) {
	t.Parallel()

	expectedRows := []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "apple"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "banana"}},
	}
	caller := &spyCaller{result: expectedRows}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "list_items"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if caller.options.Validate != nil {
		t.Fatal("expected Validate to be nil when no row-set features or singular representation requested")
	}
	if outcome.Kind != rpcexec.ResultKindRowSet {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindRowSet)
	}
	if len(outcome.Rows) != 2 {
		t.Fatalf("len(outcome.Rows) = %d, want 2", len(outcome.Rows))
	}
	if !outcome.TxOutcome.Committed || outcome.TxOutcome.PreferApplied {
		t.Fatalf("unexpected TxOutcome: %+v", outcome.TxOutcome)
	}
}

func TestExecuteProcedureOutputs(t *testing.T) {
	t.Parallel()

	outputRow := rows.Row{
		Columns: []string{"out_val"},
		Values:  []any{"echo"},
	}
	caller := &spyCaller{result: outputRow}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "echo"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if outcome.Kind != rpcexec.ResultKindObject {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindObject)
	}
	if !outcome.TxOutcome.Committed || outcome.TxOutcome.PreferApplied {
		t.Fatalf("unexpected TxOutcome: %+v", outcome.TxOutcome)
	}
	gotRow, ok := outcome.Data.(rows.Row)
	if !ok || !reflect.DeepEqual(gotRow, outputRow) {
		t.Fatalf("outcome.Data = %#v, want %#v", outcome.Data, outputRow)
	}
}

func TestExecuteUnknownRoutineKind(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(1)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "bad"},
		Kind: "TRIGGER",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected error for unknown routine kind, got nil")
	}
	if caller.called {
		t.Fatal("caller was invoked for unknown routine kind")
	}
}

func TestExecuteSingularObjectOneRowPasses(t *testing.T) {
	t.Parallel()

	expectedRows := []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
	}
	caller := &spyCaller{result: expectedRows}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "get_one"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:        routine,
		Role:           "myrest_anon",
		Args:           map[string]any{},
		CallMode:       rpcexec.CallModePost,
		Representation: rpcexec.RepresentationSingularObject,
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if outcome.Kind != rpcexec.ResultKindRowSet {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindRowSet)
	}
}

func TestExecuteSingularObjectZeroRowsRefused(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: []rows.Row{}}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "get_none"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:        routine,
		Role:           "myrest_anon",
		Args:           map[string]any{},
		CallMode:       rpcexec.CallModePost,
		Representation: rpcexec.RepresentationSingularObject,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected SingularObjectRefusal, got nil")
	}
	var refusal rpcexec.SingularObjectRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("expected SingularObjectRefusal, got %T: %v", err, err)
	}
	if refusal.RowCount != 0 {
		t.Fatalf("refusal.RowCount = %d, want 0", refusal.RowCount)
	}
}

func TestExecuteSingularObjectTwoRowsRefused(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
	}}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "get_two"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:        routine,
		Role:           "myrest_anon",
		Args:           map[string]any{},
		CallMode:       rpcexec.CallModePost,
		Representation: rpcexec.RepresentationSingularObject,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected SingularObjectRefusal, got nil")
	}
	var refusal rpcexec.SingularObjectRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("expected SingularObjectRefusal, got %T: %v", err, err)
	}
	if refusal.RowCount != 2 {
		t.Fatalf("refusal.RowCount = %d, want 2", refusal.RowCount)
	}
}

func TestExecuteRowSetFeaturesOnScalarRefused(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(10)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := testFunction()
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{"a": float64(1), "b": float64(2)},
		CallMode: rpcexec.CallModePost,
		Query: readquery.Query{
			Columns: []readquery.Column{{Name: "a"}},
		},
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected RowSetFeaturesRefusal, got nil")
	}
	var refusal rpcexec.RowSetFeaturesRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("expected RowSetFeaturesRefusal, got %T: %v", err, err)
	}
}

func TestExecuteRowSetFeaturesOnProcedureOutputsRefused(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: rows.Row{
		Columns: []string{"out_val"},
		Values:  []any{"echo"},
	}}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "echo"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
		Query: readquery.Query{
			Columns: []readquery.Column{{Name: "out_val"}},
		},
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected RowSetFeaturesRefusal, got nil")
	}
	var refusal rpcexec.RowSetFeaturesRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("expected RowSetFeaturesRefusal, got %T: %v", err, err)
	}
}

func TestExecuteTxOutcome(t *testing.T) {
	t.Parallel()

	t.Run("default commit", func(t *testing.T) {
		caller := &spyCaller{result: int64(1)}
		exec := rpcexec.New(caller, config.TxEndCommit)
		routine := testFunction()
		intent := rpcexec.Intent{
			Routine:  routine,
			Role:     "myrest_anon",
			Args:     map[string]any{"a": float64(1), "b": float64(2)},
			CallMode: rpcexec.CallModePost,
		}
		outcome, err := exec.Execute(context.Background(), intent)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		if caller.options.PreferTx != "" {
			t.Fatalf("caller.options.PreferTx = %q, want empty", caller.options.PreferTx)
		}
		if !outcome.TxOutcome.Committed {
			t.Fatal("expected Committed = true")
		}
		if outcome.TxOutcome.PreferApplied {
			t.Fatal("expected PreferApplied = false")
		}
	})

	t.Run("override rollback applied", func(t *testing.T) {
		caller := &spyCaller{result: int64(1)}
		exec := rpcexec.New(caller, config.TxEndCommitAllowOverride)
		routine := testFunction()
		intent := rpcexec.Intent{
			Routine:  routine,
			Role:     "myrest_anon",
			Args:     map[string]any{"a": float64(1), "b": float64(2)},
			CallMode: rpcexec.CallModePost,
			PreferTx: "rollback",
		}
		outcome, err := exec.Execute(context.Background(), intent)
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		if caller.options.PreferTx != "rollback" {
			t.Fatalf("caller.options.PreferTx = %q, want rollback", caller.options.PreferTx)
		}
		if outcome.TxOutcome.Committed {
			t.Fatal("expected Committed = false")
		}
		if !outcome.TxOutcome.PreferApplied {
			t.Fatal("expected PreferApplied = true")
		}
	})
}

func TestExecuteOutParameterInArgsRejected(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(1)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "echo"},
		Kind: "PROCEDURE",
		Parameters: []schemacache.ParameterFact{
			{Name: "in_val", Mode: "IN", Ordinal: 1},
			{Name: "out_val", Mode: "OUT", Ordinal: 2},
		},
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{"in_val": "hello", "out_val": "world"},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected error for passing OUT parameter in args, got nil")
	}

	var mismatch rpcexec.SignatureMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected SignatureMismatch, got %T: %v", err, err)
	}
	if mismatch.Routine != routine.ID {
		t.Fatalf("mismatch.Routine = %v, want %v", mismatch.Routine, routine.ID)
	}
	if mismatch.Unknown != "out_val" {
		t.Fatalf("mismatch.Unknown = %q, want out_val", mismatch.Unknown)
	}
	if caller.called {
		t.Fatal("caller was invoked despite unknown argument")
	}
}

func TestExecuteRowSetFeaturesValid(t *testing.T) {
	t.Parallel()

	rawRows := []rows.Row{
		{Columns: []string{"id", "title"}, Values: []any{int64(2), "two"}},
		{Columns: []string{"id", "title"}, Values: []any{int64(1), "one"}},
	}
	caller := &spyCaller{result: rawRows}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "get_items"},
		Kind: "PROCEDURE",
	}

	limit := uint64(1)
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
		Query: readquery.Query{
			Limit: &limit,
		},
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if caller.options.Validate == nil {
		t.Fatal("expected non-nil Validate")
	}
	if outcome.Kind != rpcexec.ResultKindRowSet {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindRowSet)
	}
}

func TestExecuteRowSetFeaturesInvalidProjection(t *testing.T) {
	t.Parallel()

	rawRows := []rows.Row{
		{Columns: []string{"id", "title"}, Values: []any{int64(1), "item"}},
	}
	caller := &spyCaller{result: rawRows}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "get_items"},
		Kind: "PROCEDURE",
	}

	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
		Query: readquery.Query{
			Columns: []readquery.Column{{Name: "nonexistent_field"}},
		},
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected error from validation of nonexistent field projection, got nil")
	}
}

func TestExecuteSingularObjectNonTabularResult(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(100)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := testFunction()
	intent := rpcexec.Intent{
		Routine:        routine,
		Role:           "myrest_anon",
		Args:           map[string]any{"a": float64(1), "b": float64(2)},
		CallMode:       rpcexec.CallModePost,
		Representation: rpcexec.RepresentationSingularObject,
	}

	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if caller.options.Validate == nil {
		t.Fatal("expected non-nil Validate when singular representation requested")
	}
	if outcome.Kind != rpcexec.ResultKindScalar {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindScalar)
	}
}

func TestExecuteParameterModesAndOrdinals(t *testing.T) {
	t.Parallel()

	caller := &spyCaller{result: int64(1)}
	exec := rpcexec.New(caller, config.TxEndCommit)

	routine := schemacache.RoutineFact{
		ID:   schemacache.RoutineID{Database: "shop", Name: "mixed_params"},
		Kind: "PROCEDURE",
		Parameters: []schemacache.ParameterFact{
			{Ordinal: 0, DataType: "bigint"}, // return param, ignored
			{Name: "", Mode: "IN", Ordinal: 1}, // empty name, ignored
			{Name: "out_p", Mode: "OUT", Ordinal: 2}, // OUT param, ignored
			{Name: "inout_p", Mode: "INOUT", Ordinal: 3}, // INOUT param, required
		},
	}

	// Missing inout_p
	intent := rpcexec.Intent{
		Routine:  routine,
		Role:     "myrest_anon",
		Args:     map[string]any{},
		CallMode: rpcexec.CallModePost,
	}

	_, err := exec.Execute(context.Background(), intent)
	if err == nil {
		t.Fatal("expected error for missing INOUT param, got nil")
	}
	var mismatch rpcexec.SignatureMismatch
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected SignatureMismatch, got %T: %v", err, err)
	}
	if mismatch.Missing != "inout_p" {
		t.Fatalf("mismatch.Missing = %q, want inout_p", mismatch.Missing)
	}

	// With inout_p provided
	intent.Args = map[string]any{"inout_p": "value"}
	outcome, err := exec.Execute(context.Background(), intent)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if outcome.Kind != rpcexec.ResultKindObject {
		t.Fatalf("outcome.Kind = %v, want %v", outcome.Kind, rpcexec.ResultKindObject)
	}
}

func TestErrorStrings(t *testing.T) {
	t.Parallel()

	smEmpty := rpcexec.SignatureMismatch{Routine: schemacache.RoutineID{Database: "db", Name: "proc"}}
	if smEmpty.Error() != "signature mismatch for db.proc" {
		t.Fatalf("unexpected error string: %s", smEmpty.Error())
	}

	rsv := rpcexec.ReadSafetyViolation{}
	if rsv.Error() != "Only a read-safe routine can be called with GET" {
		t.Fatalf("unexpected error string: %s", rsv.Error())
	}

	sor := rpcexec.SingularObjectRefusal{RowCount: 5}
	if sor.Error() != "The result contains 5 rows, while 1 was expected" {
		t.Fatalf("unexpected error string: %s", sor.Error())
	}

	rsf := rpcexec.RowSetFeaturesRefusal{}
	if rsf.Error() != "Filter, order, pagination, and embed need a row-set RPC result" {
		t.Fatalf("unexpected error string: %s", rsf.Error())
	}
}
