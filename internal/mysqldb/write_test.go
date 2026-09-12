package mysqldb

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

func TestBuildInsertSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildInsert(table, []string{"name"}, []map[string]any{
		{"name": "gamma"},
		{"name": "delta"},
	}, false)
	if err != nil {
		t.Fatalf("buildInsert: %v", err)
	}
	want := "INSERT INTO `shop`.`items` (`name`) VALUES (?), (?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != "gamma" || parts.args[1] != "delta" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestOnDuplicateInsertSQL(t *testing.T) {
	t.Parallel()

	plain := sqlParts{statement: "INSERT INTO `shop`.`items` (`id`, `name`) VALUES (?, ?)"}
	cases := []struct {
		name string
		mode writequery.OnDuplicate
		want string
	}{
		{name: "fails", mode: writequery.DuplicateFails, want: plain.statement},
		{
			name: "ignored",
			mode: writequery.DuplicateIgnored,
			want: "INSERT IGNORE INTO `shop`.`items` (`id`, `name`) VALUES (?, ?)",
		},
		{
			name: "merges",
			mode: writequery.DuplicateMerges,
			want: "INSERT INTO `shop`.`items` (`id`, `name`) VALUES (?, ?)" +
				" AS `new` ON DUPLICATE KEY UPDATE `name` = `new`.`name`",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parts, err := onDuplicateInsert(plain, []string{"id", "name"}, writequery.Options{
				PrimaryKey:  []string{"id"},
				OnDuplicate: tc.mode,
			})
			if err != nil {
				t.Fatalf("onDuplicateInsert: %v", err)
			}
			if parts.statement != tc.want {
				t.Fatalf("statement = %q, want %q", parts.statement, tc.want)
			}
		})
	}

	_, err := onDuplicateInsert(plain, []string{"id", "name"}, writequery.Options{
		OnDuplicate: writequery.DuplicateMerges,
	})
	var gap readquery.UnsupportedFeature
	if !errors.As(err, &gap) {
		t.Fatalf("merge without primary key err = %v, want UnsupportedFeature", err)
	}
}

func TestBuildUpdateSQLWithFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpdate(
		table,
		map[string]any{"name": "alpha2"},
		readquery.Query{Filters: []readquery.Filter{{
			Column: "name", Op: readquery.OpEq, Value: "alpha",
		}}},
	)
	if err != nil {
		t.Fatalf("buildUpdate: %v", err)
	}
	want := "UPDATE `shop`.`items` SET `name` = ? WHERE `name` = ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != "alpha2" || parts.args[1] != "alpha" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildInsertSQLMissingDefault(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "colors"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
			{Name: "tone"},
		},
	}
	parts, err := buildInsert(table, []string{"name", "tone"}, []map[string]any{
		{"name": "red"},
		{"name": "blue", "tone": "bright"},
	}, true)
	if err != nil {
		t.Fatalf("buildInsert: %v", err)
	}
	want := "INSERT INTO `shop`.`colors` (`name`, `tone`) VALUES (?, DEFAULT), (?, ?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 3 || parts.args[0] != "red" || parts.args[1] != "blue" || parts.args[2] != "bright" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildDeleteSQLWithFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildDelete(table, readquery.Query{Filters: []readquery.Filter{{
		Column: "name", Op: readquery.OpEq, Value: "beta",
	}}})
	if err != nil {
		t.Fatalf("buildDelete: %v", err)
	}
	want := "DELETE FROM `shop`.`items` WHERE `name` = ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "beta" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildUpsertMergeSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpsert(
		table,
		map[string]any{"id": 1, "name": "alpha2"},
		[]string{"id"},
		httpapi.UpsertMergeDuplicates,
	)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	want := "INSERT INTO `shop`.`items` (`id`, `name`) VALUES (?,?) AS `new` ON DUPLICATE KEY UPDATE `name` = `new`.`name`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != 1 || parts.args[1] != "alpha2" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildUpsertIgnoreSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpsert(
		table,
		map[string]any{"id": 99, "name": "fresh"},
		[]string{"id"},
		httpapi.UpsertIgnoreDuplicates,
	)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	want := "INSERT IGNORE INTO `shop`.`items` (`id`, `name`) VALUES (?,?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}

// A nested JSON object or array payload binds as a JSON document string on a
// JSON column, so the driver can encode it. See issue #119.
func TestBuildInsertBindsNestedJSONOnJSONColumn(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "profiles"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "meta", DataType: "JSON"},
		},
	}
	parts, err := buildInsert(table, []string{"meta"}, []map[string]any{
		{"meta": map[string]any{"blood_type": "A-", "count": float64(1)}},
		{"meta": []any{"first", "second"}},
	}, false)
	if err != nil {
		t.Fatalf("buildInsert: %v", err)
	}
	if len(parts.args) != 2 {
		t.Fatalf("args = %#v, want two JSON documents", parts.args)
	}
	if parts.args[0] != `{"blood_type":"A-","count":1}` {
		t.Fatalf("object arg = %#v, want the JSON document", parts.args[0])
	}
	if parts.args[1] != `["first","second"]` {
		t.Fatalf("array arg = %#v, want the JSON document", parts.args[1])
	}
}

// A nested JSON payload for a column that does not hold JSON refuses before
// MySQL sees the statement. See issue #119.
func TestBuildInsertRefusesNestedJSONOnNonJSONColumn(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "name"},
		},
	}
	_, err := buildInsert(table, []string{"name"}, []map[string]any{
		{"name": map[string]any{"a": float64(1)}},
	}, false)
	var gap readquery.UnsupportedFeature
	if !errors.As(err, &gap) {
		t.Fatalf("buildInsert: %v, want UnsupportedFeature", err)
	}
	if gap.Message != "Cannot write a JSON object into column name: the column does not hold JSON" {
		t.Fatalf("message = %q, want the object refusal", gap.Message)
	}

	_, err = buildInsert(table, []string{"name"}, []map[string]any{
		{"name": []any{"a"}},
	}, false)
	if !errors.As(err, &gap) {
		t.Fatalf("buildInsert array: %v, want UnsupportedFeature", err)
	}
	if gap.Message != "Cannot write a JSON array into column name: the column does not hold JSON" {
		t.Fatalf("message = %q, want the array refusal", gap.Message)
	}
}

// A nested JSON payload in an update binds as a JSON document string on a JSON
// column and refuses on any other column. See issue #119.
func TestBuildUpdateNestedJSON(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "profiles"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "meta", DataType: "JSON"},
			{Name: "name"},
		},
	}
	parts, err := buildUpdate(
		table,
		map[string]any{"meta": map[string]any{"tag": "Alpha"}},
		readquery.Query{Filters: []readquery.Filter{{
			Column: "id", Op: readquery.OpEq, Value: "1",
		}}},
	)
	if err != nil {
		t.Fatalf("buildUpdate: %v", err)
	}
	if parts.args[0] != `{"tag":"Alpha"}` {
		t.Fatalf("args = %#v, want the JSON document first", parts.args)
	}

	_, err = buildUpdate(
		table,
		map[string]any{"name": []any{"a"}},
		readquery.Query{},
	)
	var gap readquery.UnsupportedFeature
	if !errors.As(err, &gap) {
		t.Fatalf("buildUpdate: %v, want UnsupportedFeature", err)
	}
}

// A nested JSON payload in an upsert body binds as a JSON document string.
// See issue #119.
func TestBuildUpsertBindsNestedJSONOnJSONColumn(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "profiles"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "meta", DataType: "JSON"},
		},
	}
	parts, err := buildUpsert(
		table,
		map[string]any{"id": 2, "meta": map[string]any{"tag": "Beta"}},
		[]string{"id"},
		httpapi.UpsertMergeDuplicates,
	)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	if len(parts.args) != 2 || parts.args[0] != 2 || parts.args[1] != `{"tag":"Beta"}` {
		t.Fatalf("args = %#v, want the JSON document last", parts.args)
	}
}

// A nested JSON payload never takes the SQL DEFAULT path, and scalar, string,
// and null values bind as the body sent them. See issue #119.
func TestBuildInsertKeepsScalarBindings(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "profiles"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "meta", DataType: "JSON"},
		},
	}
	parts, err := buildInsert(table, []string{"id", "meta"}, []map[string]any{
		{"id": 1, "meta": "plain"},
		{"id": 2, "meta": nil},
	}, true)
	if err != nil {
		t.Fatalf("buildInsert: %v", err)
	}
	if parts.args[0] != 1 || parts.args[1] != "plain" || parts.args[2] != 2 {
		t.Fatalf("args = %#v, want the scalar bindings", parts.args)
	}
	if _, held := parts.args[3].(map[string]any); held || parts.args[3] != nil {
		t.Fatalf("null arg = %#v, want nil", parts.args[3])
	}
}

func TestKeyFilterValueFormatsJSONNumbers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"large json integer", float64(1000000), "1000000"},
		{"small json integer", float64(123), "123"},
		{"fractional json number", float64(1.5), "1.5"},
		{"auto-increment int64", int64(9), "9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := keyFilterValue(tc.value); got != tc.want {
				t.Fatalf("keyFilterValue(%#v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestCheckMaxAffected(t *testing.T) {
	t.Parallel()

	zero := int64(0)
	one := int64(1)
	two := int64(2)

	cases := []struct {
		name     string
		affected int64
		max      *int64
		wantErr  bool
	}{
		{"nil max allows any", 10, nil, false},
		{"affected equals max", 1, &one, false},
		{"affected less than max", 1, &two, false},
		{"zero affected zero max", 0, &zero, false},
		{"affected exceeds zero max", 1, &zero, true},
		{"affected exceeds positive max", 2, &one, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkMaxAffected(tc.affected, tc.max)
			if tc.wantErr {
				var maxErr writequery.MaxAffectedExceeded
				if !errors.As(err, &maxErr) {
					t.Fatalf("checkMaxAffected(%d, %v) = %v, want MaxAffectedExceeded", tc.affected, tc.max, err)
				}
				if maxErr.Affected != tc.affected || maxErr.Max != *tc.max {
					t.Fatalf("maxErr = %#v, want Affected=%d, Max=%d", maxErr, tc.affected, *tc.max)
				}
			} else if err != nil {
				t.Fatalf("checkMaxAffected(%d, %v) unexpected error: %v", tc.affected, tc.max, err)
			}
		})
	}
}
