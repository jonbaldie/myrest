package readexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readexec"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

const anon schemacache.Role = "anon"

// memoryReader is a fake database: it holds rows per table and answers a read
// with the filter, order, and select of the query, as the MySQL reader does.
type memoryReader struct {
	tables map[string][]rows.Row
	fail   map[string]error
	reads  []string
	seen   []readquery.Query
}

func (m *memoryReader) Read(
	ctx context.Context,
	_ schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	m.reads = append(m.reads, table.ID.Name)
	m.seen = append(m.seen, query)
	if err := ctx.Err(); err != nil {
		return readquery.Result{}, err
	}
	if err := m.fail[table.ID.Name]; err != nil {
		return readquery.Result{}, err
	}
	shaped, err := readquery.Shape(m.tables[table.ID.Name], query)
	if err != nil {
		return readquery.Result{}, err
	}
	projected, err := readquery.Project(shaped.Rows, query)
	if err != nil {
		return readquery.Result{}, err
	}
	shaped.Rows = projected
	return shaped, nil
}

func row(columns []string, values ...any) rows.Row {
	return rows.Row{Columns: columns, Values: values}
}

func parse(t *testing.T, raw string) readquery.Query {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse %q: %v", raw, err)
	}
	return query
}

// toJSON renders rows the way a client sees them, nested embeds included.
func toJSON(t *testing.T, set []rows.Row) string {
	t.Helper()
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(body)
}

func resource(t *testing.T, cache *schemacache.Cache, name string) schemacache.Table {
	t.Helper()
	found, ok := cache.Resource(anon, schemacache.TableID{Database: "shop", Name: name})
	if !ok {
		t.Fatalf("no resource %q", name)
	}
	return found
}

func execute(t *testing.T, cache *schemacache.Cache, source *memoryReader, name, raw string) string {
	t.Helper()
	result, err := readexec.New(cache, source).Execute(
		context.Background(), anon, resource(t, cache, name), parse(t, raw),
	)
	if err != nil {
		t.Fatalf("Execute %s?%s: %v", name, raw, err)
	}
	return toJSON(t, result.Rows)
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("rows = %s\nwant   %s", got, want)
	}
}

// shopCache: orders.item_id refers to items.id; tags and items meet through
// the item_tags join table.
func shopCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	tags := schemacache.TableID{Database: "shop", Name: "tags"}
	itemTags := schemacache.TableID{Database: "shop", Name: "item_tags"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders, tags, itemTags},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "item_id"},
			{Table: tags, Name: "id"},
			{Table: tags, Name: "label"},
			{Table: itemTags, Name: "item_id"},
			{Table: itemTags, Name: "tag_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: tags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: itemTags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"item_id", "tag_id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{
				Name: "orders_item", Table: orders, Columns: []string{"item_id"},
				ReferencedTable: items, ReferencedColumns: []string{"id"},
			},
			{
				Name: "item_tags_item", Table: itemTags, Columns: []string{"item_id"},
				ReferencedTable: items, ReferencedColumns: []string{"id"},
			},
			{
				Name: "item_tags_tag", Table: itemTags, Columns: []string{"tag_id"},
				ReferencedTable: tags, ReferencedColumns: []string{"id"},
			},
		},
		Selects: []schemacache.SelectFact{
			{Role: anon, Table: items},
			{Role: anon, Table: orders},
			{Role: anon, Table: tags},
			{Role: anon, Table: itemTags},
		},
	})
}

func shopRows() *memoryReader {
	item := []string{"id", "name"}
	order := []string{"id", "item_id"}
	tag := []string{"id", "label"}
	link := []string{"item_id", "tag_id"}
	return &memoryReader{tables: map[string][]rows.Row{
		"items": {row(item, int64(1), "alpha"), row(item, int64(2), "beta")},
		"orders": {
			row(order, int64(10), int64(1)),
			row(order, int64(11), int64(1)),
			row(order, int64(12), int64(2)),
			row(order, int64(13), nil),
		},
		"tags": {row(tag, int64(7), "red"), row(tag, int64(8), "blue")},
		"item_tags": {
			row(link, int64(1), int64(7)),
			row(link, int64(1), int64(8)),
			row(link, int64(2), int64(8)),
		},
	}}
}

func TestExecuteManyToOneNestsParentAndDropsJoinKey(t *testing.T) {
	t.Parallel()

	got := execute(t, shopCache(), shopRows(), "orders", "select=id,items(name)&order=id")
	assertJSON(t, got, `[{"id":10,"items":{"name":"alpha"}},{"id":11,"items":{"name":"alpha"}},`+
		`{"id":12,"items":{"name":"beta"}},{"id":13,"items":null}]`)
}

// Child limit and order apply per parent after the batch read groups the
// children, not to the batch read itself.
func TestExecuteOneToManyPagesChildrenPerParent(t *testing.T) {
	t.Parallel()

	source := shopRows()
	got := execute(t, shopCache(), source, "items", "select=name,orders(id)&order=id&orders.order=id.desc&orders.limit=1")
	assertJSON(t, got, `[{"name":"alpha","orders":[{"id":11}]},{"name":"beta","orders":[{"id":12}]}]`)
	child := source.seen[1]
	if child.Limit != nil || child.Offset != 0 {
		t.Fatalf("child batch read paged: limit=%v offset=%d", child.Limit, child.Offset)
	}
}

func TestExecuteManyToManyNestsThroughJoinTable(t *testing.T) {
	t.Parallel()

	source := shopRows()
	got := execute(t, shopCache(), source, "items", "select=name,tags(label)&order=id")
	assertJSON(t, got, `[{"name":"alpha","tags":[{"label":"red"},{"label":"blue"}]},`+
		`{"name":"beta","tags":[{"label":"blue"}]}]`)
	if len(source.reads) != 3 || source.reads[1] != "item_tags" || source.reads[2] != "tags" {
		t.Fatalf("reads = %v, want items, item_tags, tags", source.reads)
	}
}

// Nested embeds read one batch per level and keep the keys each level needs.
func TestExecuteNestedEmbedReadsOneBatchPerLevel(t *testing.T) {
	t.Parallel()

	source := shopRows()
	got := execute(t, shopCache(), source, "orders", "select=id,items(name,tags(label))&id=eq.12")
	assertJSON(t, got, `[{"id":12,"items":{"name":"beta","tags":[{"label":"blue"}]}}]`)
	if len(source.reads) != 4 {
		t.Fatalf("reads = %v, want one read per level", source.reads)
	}
}

func TestExecuteSelectAllKeepsJoinColumns(t *testing.T) {
	t.Parallel()

	got := execute(t, shopCache(), shopRows(), "orders", "select=*,items(name)&id=eq.10")
	assertJSON(t, got, `[{"id":10,"item_id":1,"items":{"name":"alpha"}}]`)
}

func TestExecuteAliasedEmbedUsesAliasKey(t *testing.T) {
	t.Parallel()

	got := execute(t, shopCache(), shopRows(), "orders", "select=id,product:items(name)&id=eq.10")
	assertJSON(t, got, `[{"id":10,"product":{"name":"alpha"}}]`)
}

func TestExecuteWithoutRelationshipReadsNothing(t *testing.T) {
	t.Parallel()

	source := shopRows()
	cache := shopCache()
	_, err := readexec.New(cache, source).Execute(
		context.Background(), anon, resource(t, cache, "tags"), parse(t, "select=id,orders(id)"),
	)
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want RelationshipMissing", err)
	}
	if len(source.reads) != 0 {
		t.Fatalf("reads = %v, want none", source.reads)
	}
}

func TestExecutePassesReaderFailureThrough(t *testing.T) {
	t.Parallel()

	failure := errors.New("database down")
	cache := shopCache()
	_, err := readexec.New(cache, failingReader{err: failure}).Execute(
		context.Background(), anon, resource(t, cache, "orders"), parse(t, "select=id,items(name)"),
	)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want the reader failure", err)
	}
}

type failingReader struct{ err error }

func (f failingReader) Read(
	context.Context, schemacache.Role, schemacache.Table, readquery.Query,
) (readquery.Result, error) {
	return readquery.Result{}, f.err
}

// compositeCache: orders(tenant_id, item_id) refers to items(tenant_id, id).
func compositeCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "tenant_id"},
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
			{Table: orders, Name: "tenant_id"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "item_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"tenant_id", "id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"tenant_id", "id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_item", Table: orders,
			Columns:         []string{"tenant_id", "item_id"},
			ReferencedTable: items, ReferencedColumns: []string{"tenant_id", "id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: anon, Table: items},
			{Role: anon, Table: orders},
		},
	})
}

// Item 3 exists for two tenants; every key column takes part in the match.
func compositeRows() *memoryReader {
	item := []string{"tenant_id", "id", "name"}
	order := []string{"tenant_id", "id", "item_id"}
	return &memoryReader{tables: map[string][]rows.Row{
		"items": {row(item, int64(7), int64(3), "alpha"), row(item, int64(8), int64(3), "beta")},
		"orders": {
			row(order, int64(7), int64(1), int64(3)),
			row(order, int64(8), int64(2), int64(3)),
			row(order, int64(8), int64(4), int64(3)),
		},
	}}
}

func TestExecuteCompositeManyToOneMatchesEveryKeyColumn(t *testing.T) {
	t.Parallel()

	got := execute(t, compositeCache(), compositeRows(), "orders", "select=id,items(name)&order=id")
	assertJSON(t, got, `[{"id":1,"items":{"name":"alpha"}},{"id":2,"items":{"name":"beta"}},`+
		`{"id":4,"items":{"name":"beta"}}]`)
}

func TestExecuteCompositeOneToManyGroupsByEveryKeyColumn(t *testing.T) {
	t.Parallel()

	got := execute(t, compositeCache(), compositeRows(), "items", "select=name,orders(id)&order=tenant_id")
	assertJSON(t, got, `[{"name":"alpha","orders":[{"id":1}]},{"name":"beta","orders":[{"id":2},{"id":4}]}]`)
}

// employeesCache: employees.manager_id refers to employees.id.
func employeesCache() *schemacache.Cache {
	employees := schemacache.TableID{Database: "shop", Name: "employees"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{employees},
		Columns: []schemacache.ColumnFact{
			{Table: employees, Name: "id"},
			{Table: employees, Name: "name"},
			{Table: employees, Name: "manager_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: employees, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "employees_manager", Table: employees, Columns: []string{"manager_id"},
			ReferencedTable: employees, ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{{Role: anon, Table: employees}},
	})
}

func employeesRows() *memoryReader {
	employee := []string{"id", "name", "manager_id"}
	return &memoryReader{tables: map[string][]rows.Row{
		"employees": {
			row(employee, int64(1), "ada", nil),
			row(employee, int64(2), "bob", int64(1)),
			row(employee, int64(3), "cy", int64(1)),
		},
	}}
}

func TestExecuteSelfReferentialEmbedsBothDirections(t *testing.T) {
	t.Parallel()

	got := execute(t, employeesCache(), employeesRows(), "employees",
		"select=name,manager:employees!employees_manager(name),reports:employees!manager_id(name)&order=id")
	assertJSON(t, got, `[{"name":"ada","manager":null,"reports":[{"name":"bob"},{"name":"cy"}]},`+
		`{"name":"bob","manager":{"name":"ada"},"reports":[]},`+
		`{"name":"cy","manager":{"name":"ada"},"reports":[]}]`)
}

func shape(t *testing.T, name string, set []rows.Row, raw string) (readquery.Result, *memoryReader) {
	t.Helper()
	source := shopRows()
	cache := shopCache()
	result, err := readexec.New(cache, source).Shape(
		context.Background(), anon, resource(t, cache, name), set, parse(t, raw),
	)
	if err != nil {
		t.Fatalf("Shape %s?%s: %v", name, raw, err)
	}
	return result, source
}

// Shape filters, orders, and pages held rows before it nests, then keeps the
// selected columns, so a join column the client did not select leaves.
func TestShapeFiltersNestsAndProjectsHeldRows(t *testing.T) {
	t.Parallel()

	order := []string{"id", "item_id"}
	held := []rows.Row{
		row(order, int64(10), int64(1)),
		row(order, int64(12), int64(2)),
		row(order, int64(11), int64(1)),
	}
	result, source := shape(t, "orders", held, "select=id,items(name)&item_id=eq.1&order=id.desc&limit=1")
	assertJSON(t, toJSON(t, result.Rows), `[{"id":11,"items":{"name":"alpha"}}]`)
	if len(source.reads) != 1 || source.reads[0] != "items" {
		t.Fatalf("reads = %v, want one items read", source.reads)
	}
}

func TestShapeCountsFilteredRowsBeforePaging(t *testing.T) {
	t.Parallel()

	order := []string{"id", "item_id"}
	held := []rows.Row{
		row(order, int64(10), int64(1)),
		row(order, int64(11), int64(1)),
		row(order, int64(12), int64(2)),
	}
	cache := shopCache()
	query := parse(t, "select=id&item_id=eq.1&limit=1")
	query.ExactCount = true
	result, err := readexec.New(cache, shopRows()).Shape(
		context.Background(), anon, resource(t, cache, "orders"), held, query,
	)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	assertJSON(t, toJSON(t, result.Rows), `[{"id":10}]`)
	if result.Total == nil || *result.Total != 2 {
		t.Fatalf("total = %v, want 2", result.Total)
	}
}

// Shape adds no join columns: held rows without the origin key nest nothing.
func TestShapeDoesNotInjectJoinColumns(t *testing.T) {
	t.Parallel()

	held := []rows.Row{row([]string{"id"}, int64(10))}
	result, source := shape(t, "orders", held, "select=id,items(name)")
	assertJSON(t, toJSON(t, result.Rows), `[{"id":10,"items":null}]`)
	if len(source.reads) != 0 {
		t.Fatalf("reads = %v, want none", source.reads)
	}
}

func TestShapeWithoutEmbedsReadsNothing(t *testing.T) {
	t.Parallel()

	result, source := shape(t, "orders", nil, "select=id")
	if result.Rows == nil || len(result.Rows) != 0 {
		t.Fatalf("rows = %#v, want an empty set", result.Rows)
	}
	if len(source.reads) != 0 {
		t.Fatalf("reads = %v, want none", source.reads)
	}
}

func TestShapeRefusesUnknownEmbed(t *testing.T) {
	t.Parallel()

	cache := shopCache()
	_, err := readexec.New(cache, shopRows()).Shape(
		context.Background(), anon, resource(t, cache, "tags"),
		[]rows.Row{row([]string{"id"}, int64(7))}, parse(t, "select=id,orders(id)"),
	)
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want RelationshipMissing", err)
	}
}

func TestPlanNamesOriginJoinColumns(t *testing.T) {
	t.Parallel()

	cache := shopCache()
	plan, err := readexec.New(cache, shopRows()).Plan(
		anon, schemacache.TableID{Database: "shop", Name: "orders"}, parse(t, "select=id,items(name)").Embeds,
	)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	got := plan.OriginColumns()
	if len(got) != 1 || got[0] != "item_id" {
		t.Fatalf("origin columns = %v, want [item_id]", got)
	}
}

func TestPlanRefusesSpreadEmbeds(t *testing.T) {
	t.Parallel()

	executor := readexec.New(shopCache(), shopRows())
	items := schemacache.TableID{Database: "shop", Name: "items"}
	_, err := executor.Plan(anon, items, parse(t, "select=id,...orders(id.count())").Embeds)
	if !errors.As(err, &readexec.SpreadAggregateRefused{}) {
		t.Fatalf("to-many spread aggregate err = %v", err)
	}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	_, err = executor.Plan(anon, orders, parse(t, "select=id,...items(name)").Embeds)
	if !errors.As(err, &readexec.SpreadNotSupported{}) {
		t.Fatalf("spread err = %v", err)
	}
}

// codesCache: orders.code refers to codes.code, a text key.
func codesCache() *schemacache.Cache {
	codes := schemacache.TableID{Database: "shop", Name: "codes"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{codes, orders},
		Columns: []schemacache.ColumnFact{
			{Table: codes, Name: "code"},
			{Table: codes, Name: "part"},
			{Table: codes, Name: "label"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "code"},
			{Table: orders, Name: "part"},
		},
		Keys: []schemacache.KeyFact{
			{Table: codes, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"code", "part"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_code", Table: orders, Columns: []string{"code", "part"},
			ReferencedTable: codes, ReferencedColumns: []string{"code", "part"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: anon, Table: codes},
			{Role: anon, Table: orders},
		},
	})
}

// Keys that a naive join of the key parts would merge stay apart, and an
// empty text key is a real key, not a missing one.
func TestExecuteKeepsDistinctTextKeysApart(t *testing.T) {
	t.Parallel()

	code := []string{"code", "part", "label"}
	order := []string{"id", "code", "part"}
	source := &memoryReader{tables: map[string][]rows.Row{
		"codes": {
			row(code, "a", "b\x1fc", "first"),
			row(code, "a\x1fb", "c", "second"),
			row(code, "", "", "blank"),
			row(code, "0:", "", "colon"),
		},
		"orders": {
			row(order, int64(1), "a", "b\x1fc"),
			row(order, int64(2), "a\x1fb", "c"),
			row(order, int64(3), "", ""),
			row(order, int64(4), "0:", ""),
		},
	}}
	got := execute(t, codesCache(), source, "orders", "select=id,codes(label)&order=id")
	assertJSON(t, got, `[{"id":1,"codes":{"label":"first"}},{"id":2,"codes":{"label":"second"}},`+
		`{"id":3,"codes":{"label":"blank"}},{"id":4,"codes":{"label":"colon"}}]`)
}

// A fractional key reaches the child read with its fraction.
func TestExecuteMatchesFractionalKeys(t *testing.T) {
	t.Parallel()

	item := []string{"id", "name"}
	order := []string{"id", "item_id"}
	source := &memoryReader{tables: map[string][]rows.Row{
		"items":  {row(item, 1.5, "half"), row(item, float64(1), "one")},
		"orders": {row(order, int64(10), 1.5)},
	}}
	got := execute(t, shopCache(), source, "orders", "select=id,items(name)")
	assertJSON(t, got, `[{"id":10,"items":{"name":"half"}}]`)
}

// The context of the call reaches every read, the child reads too.
func TestExecuteAndShapePassTheContextToEveryRead(t *testing.T) {
	t.Parallel()

	cache := shopCache()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readexec.New(cache, shopRows()).Execute(
		ctx, anon, resource(t, cache, "orders"), parse(t, "select=id,items(name)"),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute err = %v, want context.Canceled", err)
	}
	held := []rows.Row{row([]string{"id", "item_id"}, int64(10), int64(1))}
	_, err = readexec.New(cache, shopRows()).Shape(
		ctx, anon, resource(t, cache, "orders"), held, parse(t, "select=id,items(name)"),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shape err = %v, want context.Canceled", err)
	}
}

func TestExecuteAndShapePassChildReadFailureThrough(t *testing.T) {
	t.Parallel()

	failure := errors.New("items table gone")
	cache := shopCache()
	source := shopRows()
	source.fail = map[string]error{"items": failure}
	_, err := readexec.New(cache, source).Execute(
		context.Background(), anon, resource(t, cache, "orders"), parse(t, "select=id,items(name)"),
	)
	if !errors.Is(err, failure) {
		t.Fatalf("Execute err = %v, want the child read failure", err)
	}
	held := []rows.Row{row([]string{"id", "item_id"}, int64(10), int64(1))}
	_, err = readexec.New(cache, source).Shape(
		context.Background(), anon, resource(t, cache, "orders"), held, parse(t, "select=id,items(name)"),
	)
	if !errors.Is(err, failure) {
		t.Fatalf("Shape err = %v, want the child read failure", err)
	}
}

// Shape refuses a filter or a select on a column the held rows do not have.
func TestShapeRefusesUnknownColumns(t *testing.T) {
	t.Parallel()

	cache := shopCache()
	held := []rows.Row{row([]string{"id"}, int64(10))}
	for _, raw := range []string{"select=id&nope=eq.1", "select=nope"} {
		_, err := readexec.New(cache, shopRows()).Shape(
			context.Background(), anon, resource(t, cache, "orders"), held, parse(t, raw),
		)
		var missing readquery.ColumnNotFound
		if !errors.As(err, &missing) {
			t.Fatalf("%s: err = %v, want ColumnNotFound", raw, err)
		}
	}
}

// ShapePlanned nests with a plan the caller made before, as a write does
// before its mutation, and does not plan again.
func TestShapePlannedNestsWithTheGivenPlan(t *testing.T) {
	t.Parallel()

	cache := shopCache()
	source := shopRows()
	executor := readexec.New(cache, source)
	query := parse(t, "select=id,items(name)")
	plan, err := executor.Plan(anon, resource(t, cache, "orders").ID, query.Embeds)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	held := []rows.Row{row([]string{"id", "item_id"}, int64(10), int64(1))}
	result, err := executor.ShapePlanned(context.Background(), anon, plan, held, query)
	if err != nil {
		t.Fatalf("ShapePlanned: %v", err)
	}
	assertJSON(t, toJSON(t, result.Rows), `[{"id":10,"items":{"name":"alpha"}}]`)

	// The zero Plan nests nothing, so the embed key is not in the rows.
	result, err = executor.ShapePlanned(context.Background(), anon, readexec.Plan{}, held, parse(t, "select=id"))
	if err != nil {
		t.Fatalf("ShapePlanned zero plan: %v", err)
	}
	assertJSON(t, toJSON(t, result.Rows), `[{"id":10}]`)
}
