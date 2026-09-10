package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// read-001: GET with select, a common filter, order, and limit/offset succeeds.
func TestOrdinaryReadWithSelectFilterOrderAndPage(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	response, body := get(
		t,
		serve(t, source, settings()),
		"/items?select=id,name&name=eq.alpha&order=id.asc&limit=1&offset=0",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.query.Columns) != 2 || source.query.Columns[0].Name != "id" {
		t.Fatalf("select = %#v", source.query.Columns)
	}
	if len(source.query.Filters) != 1 || source.query.Filters[0].Op != readquery.OpEq {
		t.Fatalf("filters = %#v", source.query.Filters)
	}
	if len(source.query.Order) != 1 || source.query.Order[0].Column != "id" {
		t.Fatalf("order = %#v", source.query.Order)
	}
	if source.query.Limit == nil || *source.query.Limit != 1 || source.query.Offset != 0 {
		t.Fatalf("page = %#v offset %d", source.query.Limit, source.query.Offset)
	}
	if response.Header.Get("Content-Range") != "0-0/*" {
		t.Fatalf("Content-Range = %q", response.Header.Get("Content-Range"))
	}
}

// read-002: Prefer count=exact returns the exact total row count.
func TestPreferCountExactSetsContentRangeTotal(t *testing.T) {
	t.Parallel()

	total := int64(2)
	source := &reader{
		read: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
		},
		total: &total,
	}
	headers := make(http.Header)
	headers.Set("Prefer", "count=exact")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, source, settings()).URL()+"/items?limit=1", headers,
	)

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusPartialContent, body)
	}
	if !source.query.ExactCount {
		t.Fatal("ExactCount was not set on the reader query")
	}
	if response.Header.Get("Content-Range") != "0-0/2" {
		t.Fatalf("Content-Range = %q, want 0-0/2", response.Header.Get("Content-Range"))
	}
}

// HEAD follows the same read intent and returns no body.
func TestHeadOrdinaryReadReturnsNoBody(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
	}}
	response, body := apitest.Do(
		t, http.MethodHead, serve(t, source, settings()).URL()+"/items?select=id&order=id.asc", nil,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if len(body) != 0 {
		t.Fatalf("HEAD body = %q, want empty", body)
	}
	if response.Header.Get("Content-Range") != "0-1/*" {
		t.Fatalf("Content-Range = %q", response.Header.Get("Content-Range"))
	}
	if len(source.query.Columns) != 1 || source.query.Columns[0].Name != "id" {
		t.Fatalf("HEAD did not pass the select: %#v", source.query.Columns)
	}
}

// db-max-rows bounds the returned row count.
func TestDBMaxRowsCapsTheRead(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
	}}
	resolved := settings()
	resolved.DB.MaxRows = config.RowLimit{Rows: 1, Capped: true}
	get(t, serve(t, source, resolved), "/items")

	if source.query.MaxRows == nil || *source.query.MaxRows != 1 {
		t.Fatalf("MaxRows = %#v, want 1", source.query.MaxRows)
	}
	effective := source.query.EffectiveLimit()
	if effective == nil || *effective != 1 {
		t.Fatalf("EffectiveLimit = %#v, want 1", effective)
	}
}

// A read on a resource without SELECT is still denied with privilege filtering.
func TestOrdinaryReadWithoutSelectGrantIsDenied(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/secrets?select=payload")

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// A read through an exposed view uses the ordinary read surface.
func TestOrdinaryReadThroughViewSucceeds(t *testing.T) {
	t.Parallel()

	itemsView := schemacache.TableID{Database: "shop", Name: "items_view"}
	cache := schemacache.Build(schemacache.Catalog{
		Views: []schemacache.TableID{itemsView},
		Columns: []schemacache.ColumnFact{
			{Table: itemsView, Name: "id"},
			{Table: itemsView, Name: "name"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: itemsView},
		},
	})
	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    cache,
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/items_view?select=id,name&name=eq.alpha&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if source.table.ID.Name != "items_view" {
		t.Fatalf("reader table = %#v", source.table.ID)
	}
	if len(source.query.Filters) != 1 || source.query.Filters[0].Op != readquery.OpEq {
		t.Fatalf("filters = %#v", source.query.Filters)
	}
}

// A malformed percent escape in a GET or HEAD query pair is a query parse
// failure: 400 PGRST100, and the reader does not run. Issue #144.
func TestMalformedPercentEscapeInReadQueryRefuses(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}}
	service := serve(t, source, settings())
	url := service.URL() + "/items?name=eq.a%b"

	response, body := apitest.Do(t, http.MethodGet, url, nil)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")

	response, body = apitest.Do(t, http.MethodHead, url, nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("HEAD status = %d, want %d; body = %s", response.StatusCode, http.StatusBadRequest, body)
	}
	if source.table.ID.Name != "" {
		t.Fatalf("reader must not run for a malformed query; table = %#v", source.table.ID)
	}
}

// A malformed percent escape inside a logical filter is rejected the same way.
func TestMalformedPercentEscapeInLogicalFilterRefuses(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	response, body := get(t, serve(t, source, settings()), "/items?or=(name.eq.a%b)")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
	if source.table.ID.Name != "" {
		t.Fatalf("reader must not run for a malformed logical filter; table = %#v", source.table.ID)
	}
}

// Valid percent encodings and a repeated query key keep their current answers.
func TestValidPercentEncodingsInReadQueryKeepCurrentBehavior(t *testing.T) {
	t.Parallel()

	cases := []struct {
		query string
		value string
		count int
	}{
		{query: "name=eq.a%25b", value: "a%b", count: 1},
		{query: "name=eq.a%20b", value: "a b", count: 1},
		{query: "name=eq.%C3%A9", value: "é", count: 1},
		{query: "name=eq.alpha&name=eq.beta", value: "alpha", count: 2},
	}
	for _, c := range cases {
		source := &reader{read: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
		}}
		response, body := get(t, serve(t, source, settings()), "/items?"+c.query)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("query %s status = %d, want %d; body = %s", c.query, response.StatusCode, http.StatusOK, body)
		}
		if len(source.query.Filters) != c.count {
			t.Fatalf("query %s filters = %#v, want %d", c.query, source.query.Filters, c.count)
		}
		if source.query.Filters[0].Value != c.value {
			t.Fatalf("query %s value = %q, want %q", c.query, source.query.Filters[0].Value, c.value)
		}
	}
}

// A view the active role cannot select from is not a usable resource.
func TestViewWithoutSelectIsNotAUsableResource(t *testing.T) {
	t.Parallel()

	locked := schemacache.TableID{Database: "shop", Name: "locked_view"}
	cache := schemacache.Build(schemacache.Catalog{
		Views:   []schemacache.TableID{locked},
		Columns: []schemacache.ColumnFact{{Table: locked, Name: "id"}},
	})
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    cache,
		Reader:   &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/locked_view")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}
