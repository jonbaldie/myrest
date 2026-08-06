package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary. These tests hold the wire contract
// of an anonymous read with a reader that answers in memory. The tests in
// ./test/acceptance hold the same contract over MySQL 8.

// reader answers a read with fixed rows, or with a failure, and keeps what
// the service asked of it.
type reader struct {
	read []rows.Row
	// failure is what the database says when it refuses the read.
	failure error
	// stoppable records whether the read carried a context a request can stop.
	stoppable bool
	role      schemacache.Role
	table     schemacache.Table
}

func (r *reader) Read(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
) ([]rows.Row, error) {
	r.stoppable = ctx != nil && ctx.Done() != nil
	r.role, r.table = role, table
	if r.failure != nil {
		return nil, r.failure
	}
	return r.read, nil
}

// cache holds one readable table and one the anonymous role cannot select.
func cache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	secrets := schemacache.TableID{Database: "shop", Name: "secrets"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, secrets},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
			{Table: secrets, Name: "payload"},
		},
		Selects: []schemacache.SelectFact{{Role: "myrest_anon", Table: items}},
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
func get(t *testing.T, service *httpapi.Service, path string) (*http.Response, []byte) {
	t.Helper()

	return apitest.Get(t, service.URL()+path)
}

// smoke-001 and repr-001: an anonymous read gives JSON rows.
func TestAnonymousReadAnswersWithJSONRows(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}}
	response, body := get(t, serve(t, source, settings()), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	want := `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %q, want %q", body, want)
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
	if want := (schemacache.TableID{Database: "shop", Name: "items"}); source.table.ID != want {
		t.Errorf("read table %v, want %v", source.table.ID, want)
	}
	// The read carries the context of the request, so that a client that
	// goes away stops the work in the database.
	if !source.stoppable {
		t.Error("the read carries no context a request can stop")
	}
}

// A resource with no rows gives an empty JSON array, not null.
func TestReadOfAnEmptyResourceAnswersWithAnEmptyArray(t *testing.T) {
	t.Parallel()

	_, body := get(t, serve(t, &reader{read: []rows.Row{}}, settings()), "/items")

	if string(body) != "[]\n" {
		t.Fatalf("body = %q, want an empty array", body)
	}
}

// cache-002 and err-001: a table the role cannot select is not a resource.
func TestTableTheRoleCannotSelectGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/secrets")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if want := "Could not find the table 'shop.secrets' in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// A table outside db-schemas is not in the cache, so it is not reachable.
func TestTableOutsideTheConfiguredDatabasesIsNotReachable(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/outside_items")

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// A request without an anonymous database role is refused, not served.
func TestReadWithoutAnAnonymousDatabaseRoleIsRefused(t *testing.T) {
	t.Parallel()

	withoutAnon := settings()
	withoutAnon.DB.AnonRole = ""
	withoutAnon.JWT.Secret = "reallyreallyreallyreallyverysafe"

	response, body := get(t, serve(t, &reader{}, withoutAnon), "/items")

	apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST301")
}

// err-001: a failing read answers with the error envelope, not with rows. The
// message is what myrest says; what the database said goes to details.
func TestFailingReadGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	source := &reader{failure: errors.New("SELECT command denied")}
	response, body := get(t, serve(t, source, settings()), "/items")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusInternalServerError, "PGRST000")
	if strings.Contains(failure.Message, "SELECT command denied") {
		t.Errorf("message %q holds the words of the database", failure.Message)
	}
	if failure.Details == nil || *failure.Details != "SELECT command denied" {
		t.Errorf("details = %v, want what the database said", failure.Details)
	}
}

func TestRootPathNamesTheService(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var named struct {
		Service string `json:"service"`
	}
	if err := json.Unmarshal(body, &named); err != nil {
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
