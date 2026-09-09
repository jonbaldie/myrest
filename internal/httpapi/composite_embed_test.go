package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// compositeCache declares one composite foreign key: orders(tenant_id, item_id)
// refers to items(tenant_id, id).
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
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: orders},
		},
	})
}

func serveComposite(t *testing.T, source httpapi.Reader) *httpapi.Service {
	t.Helper()
	service, err := httpapi.Listen(httpapi.Options{
		Addr: "127.0.0.1:0", Settings: settings(), Cache: compositeCache(), Reader: source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestCompositeManyToOneEmbedNestsParent(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{
			Columns: []string{"id", "tenant_id", "item_id"},
			Values:  []any{int64(1), int64(7), int64(3)},
		}}},
		{rows: []rows.Row{{
			Columns: []string{"tenant_id", "id", "name"},
			Values:  []any{int64(7), int64(3), "alpha"},
		}}},
	}}
	response, body := get(t, serveComposite(t, source), "/orders?select=id,items(name)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"name":"alpha"}}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// compositeGroup returns the OR-of-AND key condition of one recorded read.
func compositeGroup(t *testing.T, query readquery.Query) readquery.Group {
	t.Helper()
	if len(query.Groups) == 0 {
		t.Fatalf("no key group in query %#v", query)
	}
	group := query.Groups[0]
	if !group.Or {
		t.Fatalf("key group is not an OR: %#v", group)
	}
	return group
}

func TestCompositeManyToOneEmbedMatchesEveryKeyColumn(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(1), int64(7), int64(3)}},
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(2), int64(8), int64(3)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"tenant_id", "id", "name"}, Values: []any{int64(7), int64(3), "alpha"}},
			{Columns: []string{"tenant_id", "id", "name"}, Values: []any{int64(8), int64(3), "beta"}},
		}},
	}}
	response, body := get(t, serveComposite(t, source), "/orders?select=id,items(name)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"name":"alpha"}},{"id":2,"items":{"name":"beta"}}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	group := compositeGroup(t, source.seen[1])
	if len(group.Groups) != 2 {
		t.Fatalf("key tuples = %#v", group.Groups)
	}
	first := group.Groups[0].Filters
	if len(first) != 2 || first[0].Column != "tenant_id" || first[1].Column != "id" {
		t.Fatalf("tuple filters = %#v, want the declared key column order", first)
	}
	if first[0].Op != readquery.OpEq || first[0].Value != "7" || first[1].Value != "3" {
		t.Fatalf("tuple filters = %#v", first)
	}
}

func TestCompositeOneToManyEmbedNestsChildren(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"name", "tenant_id", "id"}, Values: []any{"alpha", int64(7), int64(3)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(1), int64(7), int64(3)}},
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(2), int64(7), int64(3)}},
		}},
	}}
	response, body := get(t, serveComposite(t, source), "/items?select=name,orders(id)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"name":"alpha","orders":[{"id":1},{"id":2}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	group := compositeGroup(t, source.seen[1])
	first := group.Groups[0].Filters
	if len(first) != 2 || first[0].Column != "tenant_id" || first[1].Column != "item_id" {
		t.Fatalf("tuple filters = %#v", first)
	}
}

// The hint names the composite constraint, and the injected key columns stay
// out of the response body.
func TestCompositeEmbedHintByConstraintNameHidesKeyColumns(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(1), int64(7), int64(3)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"name", "tenant_id", "id"}, Values: []any{"alpha", int64(7), int64(3)}},
		}},
	}}
	response, body := get(t, serveComposite(t, source), "/orders?select=id,items!orders_item(name)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"name":"alpha"}}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if strings.Contains(string(body), "tenant_id") {
		t.Fatalf("injected key column leaked into %s", body)
	}
}
