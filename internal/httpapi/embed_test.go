package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func embedCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	profiles := schemacache.TableID{Database: "shop", Name: "profiles"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders, profiles},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "item_id"},
			{Table: profiles, Name: "id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_item", Table: orders, Columns: []string{"item_id"},
			ReferencedTable: items, ReferencedColumns: []string{"id"},
		}},
		Routines: []schemacache.RoutineFact{{
			ID: schemacache.RoutineID{Database: "shop", Name: "item_count"}, Kind: "FUNCTION",
		}},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: orders},
			{Role: "myrest_anon", Table: profiles},
		},
	})
}

func serveEmbed(t *testing.T, source httpapi.Reader) *httpapi.Service {
	t.Helper()
	resolved := settings()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    embedCache(),
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestEmbedWithoutRelationshipRefusesAtHTTP(t *testing.T) {
	t.Parallel()
	response, body := get(t, serveEmbed(t, &reader{}), "/items?select=id,profiles(id)")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST200")
}

func TestComputedRelationshipEmbedRefusesAtHTTP(t *testing.T) {
	t.Parallel()
	response, body := get(t, serveEmbed(t, &reader{}), "/items?select=id,item_count(*)")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "computed relationship") {
		t.Fatalf("message = %q", failure.Message)
	}
}

func TestEmbedManyToOneUsesReaderRows(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
	}}
	response, body := get(t, serveEmbed(t, source), "/orders?select=id,items(id,name)&id=eq.1")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"id":1,"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestEmbedOneToManyUsesReaderRows(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(2), int64(1)}},
			{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}},
		}},
	}}
	response, body := get(
		t,
		serveEmbed(t, source),
		"/items?select=id,orders(id)&id=eq.1&orders.order=id.desc&orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestEmbedAmbiguousRelationshipAtHTTP(t *testing.T) {
	t.Parallel()

	addresses := schemacache.TableID{Database: "shop", Name: "addresses"}
	deliveries := schemacache.TableID{Database: "shop", Name: "deliveries"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{addresses, deliveries},
		Columns: []schemacache.ColumnFact{
			{Table: addresses, Name: "id"},
			{Table: addresses, Name: "label"},
			{Table: deliveries, Name: "id"},
			{Table: deliveries, Name: "from_address_id"},
			{Table: deliveries, Name: "to_address_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: addresses, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: deliveries, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "deliveries_from", Table: deliveries, Columns: []string{"from_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
			{Name: "deliveries_to", Table: deliveries, Columns: []string{"to_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: addresses},
			{Role: "myrest_anon", Table: deliveries},
		},
	})
	resolved := settings()
	service, err := httpapi.Listen(httpapi.Options{
		Addr: "127.0.0.1:0", Settings: resolved, Cache: cache, Reader: &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/deliveries?select=id,addresses(label)")
	apitest.AssertEnvelope(t, response, body, http.StatusMultipleChoices, "PGRST201")

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "from_address_id"}, Values: []any{int64(1), int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"id", "label"}, Values: []any{int64(1), "from-here"}}}},
	}}
	service2, err := httpapi.Listen(httpapi.Options{
		Addr: "127.0.0.1:0", Settings: resolved, Cache: cache, Reader: source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service2.Serve() }()
	t.Cleanup(func() { _ = service2.Close() })
	response, body = get(t, service2, "/deliveries?select=id,addresses!deliveries_from(label)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"addresses":{"label":"from-here"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

type readAnswer struct {
	rows  []rows.Row
	total *int64
	err   error
}

// multiReader answers successive Read calls with fixed results.
type multiReader struct {
	answers []readAnswer
	calls   int
	seen    []readquery.Query
}

func (r *multiReader) Read(
	_ context.Context,
	_ schemacache.Role,
	_ schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	r.seen = append(r.seen, query)
	if r.calls >= len(r.answers) {
		return readquery.Result{Rows: []rows.Row{}}, nil
	}
	answer := r.answers[r.calls]
	r.calls++
	if answer.err != nil {
		return readquery.Result{}, answer.err
	}
	return readquery.Result{Rows: answer.rows, Total: answer.total}, nil
}
