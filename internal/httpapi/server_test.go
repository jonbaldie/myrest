package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary. These tests hold the wire contract
// of an anonymous read with a reader that answers in memory. The tests in
// ./test/acceptance hold the same contract over MySQL 8.

// reader answers a read with fixed rows, or with a failure.
type reader struct {
	read    []rows.Row
	failure error
	role    string
	table   schemacache.Table
}

func (r *reader) Read(_ context.Context, role string, table schemacache.Table) ([]rows.Row, error) {
	r.role, r.table = role, table
	if r.failure != nil {
		return nil, r.failure
	}
	return r.read, nil
}

// cache holds one readable table and one the anonymous role cannot select.
func cache() *schemacache.Cache {
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableFact{
			{Schema: "shop", Name: "items"},
			{Schema: "shop", Name: "secrets"},
		},
		Columns: []schemacache.ColumnFact{
			{Schema: "shop", Table: "items", Name: "id"},
			{Schema: "shop", Table: "items", Name: "name"},
			{Schema: "shop", Table: "secrets", Name: "payload"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Schema: "shop", Table: "items"},
		},
	})
}

func settings() config.Settings {
	resolved := config.Defaults()
	resolved.DB.URI = "mysql://authenticator@127.0.0.1:3306/"
	resolved.DB.Schemas = []string{"shop"}
	resolved.DB.AnonRole = "myrest_anon"
	return resolved
}

// serve starts a myrest listener over the given reader and settings.
func serve(t *testing.T, source httpapi.Reader, resolved config.Settings) *httpapi.Service {
	t.Helper()

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    cache(),
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})
	return service
}

// get reads a path from a running service.
func get(t *testing.T, service *httpapi.Service, path string) *http.Response {
	t.Helper()

	response, err := http.Get(service.URL() + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// body reads the answer of a response.
func body(t *testing.T, response *http.Response) []byte {
	t.Helper()

	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return read
}

// smoke-001 and repr-001: an anonymous read gives JSON rows.
func TestAnonymousReadAnswersWithJSONRows(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}}
	response := get(t, serve(t, source, settings()), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	want := `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`
	if read := string(body(t, response)); read != want+"\n" {
		t.Fatalf("body = %q, want %q", read, want)
	}
}

// The read runs as the anonymous database role, on the table of the cache.
func TestAnonymousReadRunsAsTheAnonymousDatabaseRole(t *testing.T) {
	t.Parallel()

	source := &reader{}
	get(t, serve(t, source, settings()), "/items")

	if source.role != "myrest_anon" {
		t.Errorf("read as role %q, want myrest_anon", source.role)
	}
	if source.table.Schema != "shop" || source.table.Name != "items" {
		t.Errorf("read table %s.%s, want shop.items", source.table.Schema, source.table.Name)
	}
}

// A resource with no rows gives an empty JSON array, not null.
func TestReadOfAnEmptyResourceAnswersWithAnEmptyArray(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, &reader{read: []rows.Row{}}, settings()), "/items")

	if read := string(body(t, response)); read != "[]\n" {
		t.Fatalf("body = %q, want an empty array", read)
	}
}

// cache-002 and err-001: a table the role cannot select is not a resource.
func TestTableTheRoleCannotSelectGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, &reader{}, settings()), "/secrets")

	failure := assertEnvelope(t, response, http.StatusNotFound, "PGRST205")
	if want := "Could not find the table 'shop.secrets' in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// A table outside db-schemas is not in the cache, so it is not reachable.
func TestTableOutsideTheConfiguredDatabasesIsNotReachable(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, &reader{}, settings()), "/outside_items")

	assertEnvelope(t, response, http.StatusNotFound, "PGRST205")
}

// A request without an anonymous database role is refused, not served.
func TestReadWithoutAnAnonymousDatabaseRoleIsRefused(t *testing.T) {
	t.Parallel()

	withoutAnon := settings()
	withoutAnon.DB.AnonRole = ""
	withoutAnon.JWT.Secret = "reallyreallyreallyreallyverysafe"

	response := get(t, serve(t, &reader{}, withoutAnon), "/items")

	assertEnvelope(t, response, http.StatusUnauthorized, "PGRST301")
}

// err-001: a failing read answers with the error envelope, not with rows.
func TestFailingReadGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	source := &reader{failure: errors.New("SELECT command denied")}
	response := get(t, serve(t, source, settings()), "/items")

	assertEnvelope(t, response, http.StatusInternalServerError, "PGRST000")
}

func TestRootPathNamesTheService(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, &reader{}, settings()), "/")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var named struct {
		Service string `json:"service"`
	}
	if err := json.Unmarshal(body(t, response), &named); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if named.Service != "myrest" {
		t.Fatalf("service = %q, want myrest", named.Service)
	}
}

func TestListenRefusesAnAddressThatIsTaken(t *testing.T) {
	t.Parallel()

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = taken.Close() })

	_, err = httpapi.Listen(httpapi.Options{Addr: taken.Addr().String()})
	if err == nil {
		t.Fatal("Listen took an address that is already bound")
	}
}

func TestCloseStopsTheService(t *testing.T) {
	t.Parallel()

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    cache(),
		Reader:   &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()

	base := service.URL()
	if err := service.Close(); err != nil {
		t.Fatalf("close service: %v", err)
	}
	if _, err := http.Get(base + "/items"); err == nil {
		t.Fatal("the service answered after Close")
	}
}

// envelope is the error body of the parity target.
type envelope struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Details *string `json:"details"`
	Hint    *string `json:"hint"`
}

// assertEnvelope holds the error contract: the status, the code, and all four
// fields of the envelope.
func assertEnvelope(t *testing.T, response *http.Response, status int, code string) envelope {
	t.Helper()

	if response.StatusCode != status {
		t.Fatalf("status = %d, want %d", response.StatusCode, status)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	read := body(t, response)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(read, &fields); err != nil {
		t.Fatalf("decode body %s: %v", read, err)
	}
	for _, field := range []string{"code", "message", "details", "hint"} {
		if _, held := fields[field]; !held {
			t.Fatalf("the envelope %s holds no %s", read, field)
		}
	}

	var failure envelope
	if err := json.Unmarshal(read, &failure); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if failure.Code != code {
		t.Fatalf("code = %q, want %q", failure.Code, code)
	}
	if failure.Message == "" {
		t.Fatalf("the envelope %s holds an empty message", read)
	}
	return failure
}
