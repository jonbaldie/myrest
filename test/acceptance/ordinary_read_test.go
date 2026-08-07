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

// read-001: GET with select, a common filter, order, and limit/offset succeeds
// over MySQL 8.
func TestOrdinaryReadWithSelectFilterOrderAndPageOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,name&name=eq.alpha&order=id.asc&limit=1&offset=0",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if response.Header.Get("Content-Range") != "0-0/*" {
		t.Fatalf("Content-Range = %q", response.Header.Get("Content-Range"))
	}
}

// read-002: Prefer count=exact returns the exact total over MySQL 8.
func TestPreferCountExactOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "count=exact")
	response, body := apitest.Do(
		t,
		http.MethodGet,
		serve(t, "myrest_fixture").URL()+"/items?select=id&limit=1&order=id.asc",
		headers,
	)

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusPartialContent, body)
	}
	if want := `[{"id":1}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if response.Header.Get("Content-Range") != "0-0/2" {
		t.Fatalf("Content-Range = %q, want 0-0/2", response.Header.Get("Content-Range"))
	}
}

// HEAD follows the same read intent and returns no body over MySQL 8.
func TestHeadOrdinaryReadOverMySQL(t *testing.T) {
	response, body := apitest.Do(
		t,
		http.MethodHead,
		serve(t, "myrest_fixture").URL()+"/items?select=id&name=eq.beta",
		nil,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if len(body) != 0 {
		t.Fatalf("HEAD body = %q, want empty", body)
	}
	if response.Header.Get("Content-Range") != "0-0/*" {
		t.Fatalf("Content-Range = %q", response.Header.Get("Content-Range"))
	}
}

// db-max-rows bounds the returned row count over MySQL 8.
func TestDBMaxRowsBoundsRowsOverMySQL(t *testing.T) {
	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = []string{"myrest_fixture"}
	settings.DB.AnonRole = anonRole
	settings.DB.MaxRows = config.RowLimit{Rows: 1, Capped: true}

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	catalog, err := pool.Catalog(t.Context(), settings.DB.Schemas)
	if err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings,
		Cache:    schemacache.Build(catalog),
		Reader:   pool,
		Caller:   pool,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/items?order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha","name_len":5}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want one row under db-max-rows", body)
	}
}

// Privilege filtering still hides a table without SELECT.
func TestOrdinaryReadWithoutSelectGrantOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/secrets?select=payload&limit=1")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}
