package writeexec_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writeexec"
	"github.com/jonbaldie/myrest/internal/writequery"
)

type database struct {
	options  writequery.Options
	calls    int
	commits  int
	rows     []rows.Row
	affected int64
	err      error
}

func (d *database) WithinTransaction(ctx context.Context, _ schemacache.Role, tx string, work func(context.Context, writeexec.Transaction) error) error {
	d.calls++
	if err := work(ctx, d); err != nil {
		return err
	}
	if tx != "rollback" {
		d.commits++
	}
	return nil
}
func (d *database) ReadDelete(context.Context, writeexec.Intent) ([]rows.Row, error) {
	return d.rows, d.err
}
func (d *database) Mutate(_ context.Context, in writeexec.Intent, options writequery.Options) (writeexec.Outcome, error) {
	d.options = options
	if d.err != nil {
		return writeexec.Outcome{}, d.err
	}
	if options.MaxAffected != nil && d.affected > *options.MaxAffected {
		return writeexec.Outcome{}, writequery.MaxAffectedExceeded{Affected: d.affected, Max: *options.MaxAffected}
	}
	return writeexec.Outcome{Affected: d.affected, Rows: d.rows, Keys: []map[string]any{{"id": 1}}, Inserted: in.Action == writeexec.Upsert}, nil
}
func intent(action writeexec.Action, header string) writeexec.Intent {
	return writeexec.Intent{Action: action, Payload: []map[string]any{{"id": 1}}, Filter: readquery.Query{SelectAll: true, Filters: []readquery.Filter{{Column: "id", Op: readquery.OpEq, Value: "1"}}}, Preferences: prefer.Parse([]string{header}), PrimaryKey: []string{"id"}, CanSelect: true}
}
func TestActionsAndResolutionDefaults(t *testing.T) {
	for _, tc := range []struct {
		action   writeexec.Action
		header   string
		mode     writequery.OnDuplicate
		inserted bool
	}{
		{writeexec.Insert, "", writequery.DuplicateFails, false},
		{writeexec.Insert, "resolution=merge-duplicates", writequery.DuplicateMerges, false},
		{writeexec.Insert, "resolution=ignore-duplicates", writequery.DuplicateIgnored, false},
		{writeexec.Update, "", writequery.DuplicateFails, false},
		{writeexec.Delete, "", writequery.DuplicateFails, false},
		{writeexec.Upsert, "", writequery.DuplicateMerges, true},
		{writeexec.Upsert, "resolution=ignore-duplicates", writequery.DuplicateIgnored, true},
	} {
		d := &database{affected: 1}
		out, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), intent(tc.action, tc.header))
		if err != nil || out.Affected != 1 || out.Inserted != tc.inserted || d.options.OnDuplicate != tc.mode || d.commits != 1 {
			t.Fatalf("action %v header %q: outcome %+v, options %+v, commits %d, error %v", tc.action, tc.header, out, d.options, d.commits, err)
		}
	}
}
func TestPrechecksDoNotOpenTransaction(t *testing.T) {
	for _, tc := range []struct {
		action    writeexec.Action
		header    string
		unbounded bool
		canSelect bool
	}{
		{writeexec.Update, "", true, true}, {writeexec.Delete, "", true, true},
		{writeexec.Upsert, "return=representation", false, true},
		{writeexec.Insert, "return=representation", false, false},
		{writeexec.Insert, "return=representation,resolution=ignore-duplicates", false, true},
		{writeexec.Insert, "handling=strict,unknown=value", false, true},
		{writeexec.Upsert, "resolution=wrong", false, true},
	} {
		d := &database{}
		in := intent(tc.action, tc.header)
		in.CanSelect = tc.canSelect
		if tc.unbounded {
			in.Filter = readquery.Query{}
		}
		_, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), in)
		if err == nil || d.calls != 0 {
			t.Fatalf("action %v header %q: error %v, calls %d", tc.action, tc.header, err, d.calls)
		}
	}
}
func TestValidationAndFailureRollBack(t *testing.T) {
	for _, action := range []writeexec.Action{writeexec.Insert, writeexec.Update, writeexec.Delete} {
		for _, projection := range []bool{false, true} {
			d := &database{affected: 1, rows: []rows.Row{{Columns: []string{"id"}, Values: []any{1}}, {Columns: []string{"id"}, Values: []any{2}}}}
			in := intent(action, "return=representation")
			if projection {
				in.Filter.SelectAll = false
				in.Filter.Columns = []readquery.Column{{Name: "absent"}}
			} else {
				in.Representation.Kind = representation.KindSingularObject
			}
			_, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), in)
			if err == nil || d.commits != 0 {
				t.Fatalf("action %v projection %t: error %v, commits %d", action, projection, err, d.commits)
			}
		}
	}
	failure := errors.New("database failure")
	d := &database{err: failure}
	_, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), intent(writeexec.Insert, ""))
	if !errors.Is(err, failure) || d.commits != 0 {
		t.Fatalf("error %v, commits %d", err, d.commits)
	}
}
func TestPreferencesAndLimits(t *testing.T) {
	for _, n := range []int64{1, 2} {
		d := &database{affected: n}
		in := intent(writeexec.Update, "handling=strict,return=minimal,max-affected=1,tx=rollback")
		out, err := writeexec.New(d, config.TxEndCommitAllowOverride).Execute(context.Background(), in)
		if n == 2 {
			var max writequery.MaxAffectedExceeded
			if !errors.As(err, &max) || d.commits != 0 {
				t.Fatalf("limit error %v", err)
			}
			continue
		}
		want := []string{"handling=strict", "return=minimal", "max-affected=1", "tx=rollback"}
		if err != nil || !reflect.DeepEqual(out.Applied, want) || d.commits != 0 || out.Affected != 1 {
			t.Fatalf("outcome %+v, error %v, commits %d", out, err, d.commits)
		}
	}
	d := &database{affected: 2}
	_, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), intent(writeexec.Update, "max-affected=1"))
	if err != nil || d.commits != 1 {
		t.Fatalf("lenient error %v", err)
	}
}

func TestActionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   writeexec.Intent
		want string
	}{
		{"negative action", intent(-1, ""), "unknown write action -1"},
		{"large action", intent(4, ""), "unknown write action 4"},
		{"insert without payload", func() writeexec.Intent { i := intent(writeexec.Insert, ""); i.Payload = nil; return i }(), "write action 0 needs a payload"},
		{"update without payload", func() writeexec.Intent { i := intent(writeexec.Update, ""); i.Payload = nil; return i }(), "write action 1 needs a payload"},
		{"upsert without payload", func() writeexec.Intent { i := intent(writeexec.Upsert, ""); i.Payload = nil; return i }(), "write action 3 needs a payload"},
		{"insert bad resolution", intent(writeexec.Insert, "resolution=wrong"), "Prefer resolution must be merge-duplicates or ignore-duplicates"},
		{"update without key", func() writeexec.Intent {
			i := intent(writeexec.Update, "return=representation")
			i.PrimaryKey = nil
			return i
		}(), "Prefer return=representation needs a primary key to return updated rows honestly"},
		{"insert without key", func() writeexec.Intent {
			i := intent(writeexec.Insert, "return=representation")
			i.PrimaryKey = nil
			return i
		}(), "Prefer return=representation needs a primary key to return inserted rows honestly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &database{}
			_, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), tc.in)
			if err == nil || err.Error() != tc.want || d.calls != 0 {
				t.Fatalf("error %v, transactions %d; want %q and no transaction", err, d.calls, tc.want)
			}
		})
	}
	for _, action := range []writeexec.Action{writeexec.Update, writeexec.Delete} {
		for _, grouped := range []bool{false, true} {
			in := intent(action, "all-rows")
			in.Filter = readquery.Query{}
			if grouped {
				in.Preferences = prefer.Preferences{}
				in.Filter.Groups = []readquery.Group{{Or: true}}
			}
			d := &database{affected: 3}
			out, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), in)
			if err != nil || out.Affected != 3 || d.commits != 1 {
				t.Fatalf("action %v grouped %t: %+v %v", action, grouped, out, err)
			}
		}
	}
}

func TestWriteOptionsAndAppliedPreferences(t *testing.T) {
	for _, tc := range []struct {
		name        string
		action      writeexec.Action
		header      string
		wantOptions writequery.Options
		wantApplied []string
	}{
		{"insert defaults", writeexec.Insert, "missing=default,return=headers-only,resolution=merge-duplicates,tx=rollback", writequery.Options{PrimaryKey: []string{"id"}, MissingDefault: true, PreferTx: "rollback", ReturnKeys: true, OnDuplicate: writequery.DuplicateMerges}, []string{"return=headers-only", "missing=default", "tx=rollback", "resolution=merge-duplicates"}},
		{"update representation", writeexec.Update, "missing=default,return=representation", writequery.Options{PrimaryKey: []string{"id"}, MissingDefault: true, ReturnKeys: true, ReturnRepresentation: true}, []string{"return=representation"}},
		{"delete headers", writeexec.Delete, "return=headers-only", writequery.Options{PrimaryKey: []string{"id"}, ReturnKeys: true}, []string{"return=headers-only"}},
		{"upsert default", writeexec.Upsert, "", writequery.Options{PrimaryKey: []string{"id"}, OnDuplicate: writequery.DuplicateMerges}, nil},
		{"upsert ignore", writeexec.Upsert, "resolution=ignore-duplicates", writequery.Options{PrimaryKey: []string{"id"}, OnDuplicate: writequery.DuplicateIgnored}, []string{"resolution=ignore-duplicates"}},
		{"update ignores resolution", writeexec.Update, "resolution=merge-duplicates", writequery.Options{PrimaryKey: []string{"id"}, OnDuplicate: writequery.DuplicateMerges}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &database{affected: 1, rows: []rows.Row{{Columns: []string{"id"}, Values: []any{1}}}}
			out, err := writeexec.New(d, config.TxEndCommitAllowOverride).Execute(context.Background(), intent(tc.action, tc.header))
			if err != nil || out.Affected != 1 || !reflect.DeepEqual(d.options, tc.wantOptions) || !reflect.DeepEqual(out.Applied, tc.wantApplied) {
				t.Fatalf("outcome %+v, options %+v, error %v; want options %+v and applied %v", out, d.options, err, tc.wantOptions, tc.wantApplied)
			}
		})
	}
}

func TestDeleteRepresentationReturnsRowsBeforeMutation(t *testing.T) {
	d := &deleteDatabase{}
	in := intent(writeexec.Delete, "return=representation")
	out, err := writeexec.New(d, config.TxEndCommit).Execute(context.Background(), in)
	want := writeexec.Outcome{Affected: 1, Rows: []rows.Row{{Columns: []string{"id"}, Values: []any{7}}}, Applied: []string{"return=representation"}}
	if err != nil || !reflect.DeepEqual(out, want) || d.options.ReturnRepresentation || !d.options.ReturnKeys {
		t.Fatalf("outcome %+v, options %+v, error %v; want %+v", out, d.options, err, want)
	}
}

type deleteDatabase struct{ options writequery.Options }

func (d *deleteDatabase) WithinTransaction(ctx context.Context, _ schemacache.Role, _ string, work func(context.Context, writeexec.Transaction) error) error {
	return work(ctx, d)
}
func (d *deleteDatabase) ReadDelete(context.Context, writeexec.Intent) ([]rows.Row, error) {
	return []rows.Row{{Columns: []string{"id"}, Values: []any{7}}}, nil
}
func (d *deleteDatabase) Mutate(_ context.Context, _ writeexec.Intent, options writequery.Options) (writeexec.Outcome, error) {
	d.options = options
	return writeexec.Outcome{Affected: 1}, nil
}
