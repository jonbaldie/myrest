package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// serveWithAggregates starts myrest with db-aggregates-enabled on.
func serveWithAggregates(t *testing.T, databases ...string) *httpapi.Service {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = databases
	settings.DB.AnonRole = anonRole
	settings.DB.AggregatesEnabled = true

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	pool.SetTxEnd(settings.DB.TxEnd)

	catalog, err := pool.Catalog(t.Context(), settings.DB.Schemas)
	if err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	cache := schemacache.Build(catalog)

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings,
		Cache:    cache,
		Reader:   pool,
		Writer:   pool,
		Caller:   pool,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

// read-011: with aggregates off, an aggregate select refuses stably over MySQL 8.
func TestAggregateSelectRefusesWhenDisabledOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/items?select=count()")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST123")
	if failure.Message != "Use of aggregate functions is not allowed" {
		t.Fatalf("message = %q", failure.Message)
	}
}

// read-010: with aggregates enabled, bare count and auto group succeed over MySQL 8.
func TestAggregateSelectWithAutoGroupOverMySQL(t *testing.T) {
	service := serveWithAggregates(t, "myrest_fixture")

	response, body := get(t, service, "/items?select=count()")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if want := `[{"count":2}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	response, body = get(t, service, "/orders?select=count(),item_id&order=item_id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if want := `[{"count":2,"item_id":1},{"count":1,"item_id":2}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	response, body = get(t, service, "/items?select=id.sum(),id.min(),id.max()")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if want := `[{"sum":3,"min":1,"max":2}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// read-012: allowed aggregate + embed combination with a cache relationship.
func TestAggregateInsideEmbedOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serveWithAggregates(t, "myrest_fixture"),
		"/items?select=name,orders(count())&order=name.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"name":"alpha","orders":[{"count":2}]},{"name":"beta","orders":[{"count":1}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// read-012 also covers grouping by an embedded resource.
func TestAggregateGroupedByEmbedOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serveWithAggregates(t, "myrest_fixture"),
		"/orders?select=count(),items(name)",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	// Two groups: item 1 (alpha) count 2, item 2 (beta) count 1. Order is not fixed without order.
	got := string(body)
	if got != `[{"count":2,"items":{"name":"alpha"}},{"count":1,"items":{"name":"beta"}}]`+"\n" &&
		got != `[{"count":1,"items":{"name":"beta"}},{"count":2,"items":{"name":"alpha"}}]`+"\n" {
		t.Fatalf("body = %s", body)
	}
}

// read-013: aggregate + to-many spread combo the parity target refuses.
func TestAggregateInToManySpreadRefusesOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serveWithAggregates(t, "myrest_fixture"),
		"/items?select=id,...orders(count())",
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST127")
	if failure.Message != "Feature not implemented" {
		t.Fatalf("message = %q", failure.Message)
	}
	if failure.Details != "Aggregates are not implemented for one-to-many or many-to-many spreads." {
		t.Fatalf("details = %#v", failure.Details)
	}
}
