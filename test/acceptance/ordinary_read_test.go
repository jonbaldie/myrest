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

// isdistinct operator uses MySQL <=> NULL-safe equality.
func TestIsDistinctOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/items?select=id,name&name=isdistinct.beta&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	notResponse, notBody := get(t, service, "/items?select=id,name&name=not.isdistinct.alpha&order=id.asc")
	if notResponse.StatusCode != http.StatusOK {
		t.Fatalf("not status = %d, want %d; body = %s", notResponse.StatusCode, http.StatusOK, notBody)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(notBody) != want+"\n" {
		t.Fatalf("not body = %s, want %s", notBody, want)
	}

	nullResponse, nullBody := get(t, service, "/items?select=id,name&name=isdistinct.null&order=id.asc")
	if nullResponse.StatusCode != http.StatusOK {
		t.Fatalf("null status = %d, want %d; body = %s", nullResponse.StatusCode, http.StatusOK, nullBody)
	}
	if want := `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`; string(nullBody) != want+"\n" {
		t.Fatalf("null body = %s, want %s", nullBody, want)
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

// An unknown is filter value is a query parse failure: 400 PGRST100,
// rejected before any SQL is built. Issue #130.
func TestUnknownIsFilterValueIsAParseFailureOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	for _, path := range []string{"/items?id=is.bogus", "/items?id=not.is.bogus"} {
		response, body := get(t, service, path)
		apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
	}
}

// The documented is filter values keep their answers over MySQL 8, and case
// handling matches the other operators: a value is matched as written.
func TestIsFilterValuesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	cases := []struct {
		filter string
		rows   string
	}{
		{"manager_id=is.null", `[{"id":1,"name":"ada"}]`},
		{"manager_id=is.not_null", `[{"id":2,"name":"bob"},{"id":3,"name":"carl"},{"id":4,"name":"dee"}]`},
		{"manager_id=not.is.not_null", `[{"id":1,"name":"ada"}]`},
		{"manager_id=is.true", `[{"id":2,"name":"bob"},{"id":3,"name":"carl"},{"id":4,"name":"dee"}]`},
		{"id=is.false", `[]`},
		{"manager_id=is.unknown", `[{"id":1,"name":"ada"}]`},
	}
	for _, c := range cases {
		response, body := get(t, service, "/employees?select=id,name&"+c.filter+"&order=id.asc")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d; body = %s", c.filter, response.StatusCode, http.StatusOK, body)
		}
		if want := c.rows + "\n"; string(body) != want {
			t.Fatalf("%s: body = %s, want %s", c.filter, body, want)
		}
	}
}

// read-001: a Range header window reads the same page as the equivalent limit
// and offset query over MySQL 8.
func TestRangeHeaderMatchesLimitAndOffsetOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	headers := make(http.Header)
	headers.Set("Range-Unit", "items")
	headers.Set("Range", "1-2")
	ranged, rangedBody := apitest.Do(
		t, http.MethodGet, service.URL()+"/items?select=id,name&order=id.asc", headers,
	)
	paged, pagedBody := get(t, service, "/items?select=id,name&order=id.asc&offset=1&limit=2")

	if ranged.StatusCode != paged.StatusCode {
		t.Fatalf("status = %d, want %d; body = %s", ranged.StatusCode, paged.StatusCode, rangedBody)
	}
	if string(rangedBody) != string(pagedBody) {
		t.Fatalf("range body = %s, want %s", rangedBody, pagedBody)
	}
	if ranged.Header.Get("Content-Range") != paged.Header.Get("Content-Range") {
		t.Fatalf(
			"Content-Range = %q, want %q",
			ranged.Header.Get("Content-Range"), paged.Header.Get("Content-Range"),
		)
	}
}

// A reversed Range header refuses as PGRST100 over MySQL 8.
func TestReversedRangeHeaderRefusesOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Range-Unit", "items")
	headers.Set("Range", "7-3")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items?select=id", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
}
