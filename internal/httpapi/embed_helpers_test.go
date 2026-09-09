package httpapi

import (
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestPageRowsOffsetAndLimit(t *testing.T) {
	t.Parallel()

	children := []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
		{Columns: []string{"id"}, Values: []any{int64(3)}},
	}
	limit := uint64(1)
	got := pageRows(children, readquery.Embed{Offset: 1, Limit: &limit})
	if len(got) != 1 || got[0].Values[0] != int64(2) {
		t.Fatalf("page = %#v", got)
	}
	got = pageRows(children, readquery.Embed{Offset: 10})
	if len(got) != 0 {
		t.Fatalf("empty page = %#v", got)
	}
	got = pageRows(nil, readquery.Embed{})
	if got == nil || len(got) != 0 {
		t.Fatalf("nil page = %#v", got)
	}
}

func TestProjectEmbedRowKeepsAskedColumnsAndNestedKeys(t *testing.T) {
	t.Parallel()

	row := rows.Row{
		Columns: []string{"id", "item_id", "orders"},
		Values:  []any{int64(1), int64(9), []rows.Row{}},
	}
	got := projectEmbedRow(row, readquery.Embed{
		Columns: []readquery.Column{{Name: "id"}},
		Embeds:  []readquery.Embed{{Resource: "orders"}},
	})
	if len(got.Columns) != 2 || got.Columns[0] != "id" || got.Columns[1] != "orders" {
		t.Fatalf("projected = %#v", got)
	}
}

func TestStringifyValueAndRowKey(t *testing.T) {
	t.Parallel()

	if stringifyValue(nil) != "" || stringifyValue(int64(7)) != "7" {
		t.Fatal("stringifyValue failed")
	}
	if stringifyValue(uint64(3)) != "3" || stringifyValue([]byte("x")) != "x" {
		t.Fatal("stringifyValue variants failed")
	}
	row := rows.Row{Columns: []string{"id"}, Values: []any{int64(4)}}
	if rowKey(row, []string{"id"}) != "4" {
		t.Fatal("rowKey failed")
	}
	if rowKey(row, []string{"missing"}) != "" {
		t.Fatal("missing column should empty the key")
	}
}

func TestExceptNamesAndAmbiguousMessages(t *testing.T) {
	t.Parallel()

	got := exceptNames([]string{"a", "b", "c"}, []string{"b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("exceptNames = %#v", got)
	}
	failure := schemacache.RelationshipAmbiguous{
		Origin: schemacache.TableID{Name: "deliveries"},
		Target: "addresses",
		Options: []schemacache.Relationship{
			{Name: "deliveries_from", Cardinality: schemacache.ManyToOne},
			{Name: "deliveries_to", Cardinality: schemacache.ManyToOne},
		},
	}
	hint := ambiguousHint(failure)
	if hint == "" || cardinalityName(schemacache.ManyToMany) != "many-to-many" {
		t.Fatalf("hint = %q", hint)
	}
	details := ambiguousDetails(failure)
	if len(details) != 2 {
		t.Fatalf("details = %#v", details)
	}
	// The hint of a self one-to-many option names its key column, so the
	// suggestion resolves instead of repeating the shared constraint name.
	employees := schemacache.TableID{Name: "employees"}
	self := schemacache.RelationshipAmbiguous{
		Origin: employees,
		Target: "employees",
		Options: []schemacache.Relationship{
			{Name: "employees_manager", Cardinality: schemacache.ManyToOne, Origin: employees, Target: employees},
			{Name: "employees_manager", Cardinality: schemacache.OneToMany, Origin: employees, Target: employees, TargetColumns: []string{"manager_id"}},
		},
	}
	selfHint := ambiguousHint(self)
	if !strings.Contains(selfHint, "'employees!employees_manager'") ||
		!strings.Contains(selfHint, "'employees!manager_id'") {
		t.Fatalf("self hint = %q", selfHint)
	}
}

func TestUniqueKeyTuplesAndGroups(t *testing.T) {
	t.Parallel()

	read := []rows.Row{
		{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(9)}},
		{Columns: []string{"id", "item_id"}, Values: []any{int64(2), int64(9)}},
		{Columns: []string{"id", "item_id"}, Values: []any{int64(3), int64(9)}},
	}
	keys := uniqueKeyTuples(read, []string{"item_id"})
	if len(keys) != 1 || stringifyValue(keys[0][0]) != "9" {
		t.Fatalf("keys = %#v", keys)
	}
	grouped := groupRows(read, []string{"item_id"})
	if len(grouped["9"]) != 3 {
		t.Fatalf("grouped = %#v", grouped)
	}
	indexed := indexRows(read, []string{"id"})
	if indexed["2"].Values[0] != int64(2) {
		t.Fatalf("indexed = %#v", indexed)
	}
}

func TestWithJoinColumnsInjectsOriginKeys(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "orders"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "item_id"}},
	}
	query := readquery.Query{
		Columns: []readquery.Column{{Name: "id"}},
	}
	plan := []plannedEmbed{{
		relationship: schemacache.Relationship{
			OriginColumns: []string{"item_id"},
		},
	}}
	got, injected := withJoinColumns(table, query, plan)
	if len(injected) != 1 || injected[0] != "item_id" || len(got.Columns) != 2 {
		t.Fatalf("withJoinColumns = %#v injected %#v", got.Columns, injected)
	}
	all := readquery.Query{SelectAll: true}
	got, injected = withJoinColumns(table, all, plan)
	if injected != nil || !got.SelectAll {
		t.Fatalf("select all should skip inject: %#v %#v", got, injected)
	}
}

func TestStringifyValueMoreKinds(t *testing.T) {
	t.Parallel()

	if stringifyValue(float64(8)) != "8" {
		t.Fatal("float64")
	}
	if stringifyValue(true) != "true" {
		t.Fatal("default sprint")
	}
	if stringifyKeys([][]any{{int64(1)}, {int64(2)}})[1] != "2" {
		t.Fatal("stringifyKeys")
	}
}

func TestEmbedKeysPreserveEmptyAndCompositeValues(t *testing.T) {
	t.Parallel()

	empty := []rows.Row{{Columns: []string{"id"}, Values: []any{""}}}
	if len(uniqueKeyTuples(empty, []string{"id"})) != 1 {
		t.Fatal("empty string key was dropped")
	}
	if len(uniqueKeyTuples(append(empty, rows.Row{
		Columns: []string{"id"}, Values: []any{"0:"},
	}), []string{"id"})) != 2 {
		t.Fatal("empty string key collided with a real key")
	}

	composite := []rows.Row{
		{Columns: []string{"first", "second"}, Values: []any{"a", "b\x1fc"}},
		{Columns: []string{"first", "second"}, Values: []any{"a\x1fb", "c"}},
	}
	if rowKey(composite[0], []string{"first", "second"}) == rowKey(composite[1], []string{"first", "second"}) {
		t.Fatal("distinct composite keys collided")
	}
	if len(uniqueKeyTuples(composite, []string{"first", "second"})) != 2 {
		t.Fatal("distinct composite keys collapsed")
	}
}

func TestStringifyValuePreservesFractionalFloat(t *testing.T) {
	t.Parallel()

	if got := stringifyValue(float64(1.5)); got != "1.5" {
		t.Fatalf("float64 key = %q, want 1.5", got)
	}
}

func TestColumnListAndSelectedNames(t *testing.T) {
	t.Parallel()

	cols := columnList([]string{"a", "b"})
	if len(cols) != 2 || cols[1].Name != "b" {
		t.Fatalf("columnList = %#v", cols)
	}
	have := selectedNames([]readquery.Column{{Name: "a"}})
	if !have["a"] || have["b"] {
		t.Fatalf("selectedNames = %#v", have)
	}
	needed := originColumnsNeeded([]plannedEmbed{{
		relationship: schemacache.Relationship{OriginColumns: []string{"x", "y"}},
	}})
	if !needed["x"] || !needed["y"] {
		t.Fatalf("needed = %#v", needed)
	}
}

func TestEnsureColumnsAndColumnOf(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	query, injected := ensureColumns(table, readquery.Query{
		Columns: []readquery.Column{{Name: "name"}},
	}, []string{"id", "missing"})
	if len(injected) != 1 || injected[0] != "id" || len(query.Columns) != 2 {
		t.Fatalf("ensureColumns = %#v %#v", query.Columns, injected)
	}
	if _, ok := columnOf(table, "nope"); ok {
		t.Fatal("missing column")
	}
}

func TestCardinalityNameUnknown(t *testing.T) {
	t.Parallel()

	if cardinalityName(schemacache.Cardinality(99)) != "unknown" {
		t.Fatal("unknown cardinality")
	}
}

func TestAppendColumn(t *testing.T) {
	t.Parallel()

	row := appendColumn(
		rows.Row{Columns: []string{"id"}, Values: []any{int64(1)}},
		"orders",
		[]rows.Row{},
	)
	if len(row.Columns) != 2 || row.Columns[1] != "orders" {
		t.Fatalf("appendColumn = %#v", row)
	}
}

func TestGroupManyToMany(t *testing.T) {
	t.Parallel()

	related := []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
	}
	parents := map[string][]string{"1": {"p"}, "2": {"p"}}
	grouped := groupManyToMany(related, parents, []string{"id"})
	if len(grouped["p"]) != 2 {
		t.Fatalf("grouped = %#v", grouped)
	}
}

func TestAttachGroupedEmbedsEmptyChildren(t *testing.T) {
	t.Parallel()

	parent := []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}
	embed := plannedEmbed{
		ask: readquery.Embed{Resource: "orders"},
		relationship: schemacache.Relationship{
			OriginColumns: []string{"id"},
		},
	}
	got := attachGroupedEmbeds(parent, embed, map[string][]rows.Row{})
	if len(got) != 1 || got[0].Columns[1] != "orders" {
		t.Fatalf("attach = %#v", got)
	}
	orders, _ := got[0].Values[1].([]rows.Row)
	if orders == nil || len(orders) != 0 {
		t.Fatalf("empty orders = %#v", got[0].Values[1])
	}
}

func TestKeyConditionShapesSingleAndCompositeKeys(t *testing.T) {
	t.Parallel()

	keys := [][]any{{int64(7), int64(3)}, {int64(8), int64(3)}}
	filters, groups, ok := keyCondition([]string{"tenant_id"}, [][]any{{int64(7)}})
	if !ok || len(filters) != 1 || filters[0].Op != readquery.OpIn || groups != nil {
		t.Fatalf("single key = %#v %#v %v", filters, groups, ok)
	}
	filters, groups, ok = keyCondition([]string{"tenant_id", "id"}, keys)
	if !ok || filters != nil || len(groups) != 1 || !groups[0].Or || len(groups[0].Groups) != 2 {
		t.Fatalf("composite key = %#v %#v %v", filters, groups, ok)
	}
	tuple := groups[0].Groups[0].Filters
	if len(tuple) != 2 || tuple[0].Value != "7" || tuple[1].Value != "3" {
		t.Fatalf("tuple = %#v", tuple)
	}
	if _, _, ok = keyCondition(nil, keys); ok {
		t.Fatal("no key columns must report no condition")
	}
	if _, _, ok = keyCondition([]string{"id"}, nil); ok {
		t.Fatal("no keys must report no condition")
	}
}
