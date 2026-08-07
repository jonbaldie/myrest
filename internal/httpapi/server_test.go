package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
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
	// total is the exact count when Prefer count=exact was asked.
	total *int64
	// failure is what the database says when it refuses the read.
	failure error
	// stoppable records whether the read carried a context a request can stop.
	stoppable bool
	role      schemacache.Role
	table     schemacache.Table
	query     readquery.Query
}

func (r *reader) Read(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	r.stoppable = ctx != nil && ctx.Done() != nil
	r.role, r.table, r.query = role, table, query
	if r.failure != nil {
		return readquery.Result{}, r.failure
	}
	return readquery.Result{Rows: r.read, Total: r.total}, nil
}

// cache holds one readable table and one the anonymous role cannot select.
// myrest_user can read secrets, so a JWT for that role proves grant switching.
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
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_user", Table: items},
			{Role: "myrest_user", Table: secrets},
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

// serve starts a myrest listener over the given reader and settings, with a
// log the test can read.
func serve(
	t *testing.T,
	source httpapi.Reader,
	resolved config.Settings,
	logged ...*bytes.Buffer,
) *httpapi.Service {
	t.Helper()

	options := httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    cache(),
		Reader:   source,
	}
	if len(logged) == 1 {
		options.Log = log.New(logged[0], "", 0)
	}
	service, err := httpapi.Listen(options)
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

// A request names no database, so the read goes to the default database: the
// first of db-schemas. A table of another configured database cannot answer
// in its place.
func TestReadGoesToTheDefaultDatabase(t *testing.T) {
	t.Parallel()

	// The cache holds shop.items only, and warehouse comes first.
	otherDefault := settings()
	otherDefault.DB.Schemas = []string{"warehouse", "shop"}

	response, body := get(t, serve(t, &reader{}, otherDefault), "/items")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if want := "Could not find the table 'warehouse.items' in the schema cache"; failure.Message != want {
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

	apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST302")
}

// err-001: a failing read answers with the error envelope, not with rows.
func TestFailingReadGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	source := &reader{failure: errors.New("SELECT command denied")}
	response, body := get(t, serve(t, source, settings()), "/items")

	apitest.AssertEnvelope(t, response, body, http.StatusInternalServerError, "MYREST002")
}

// err-004: MySQL access errors have the published SQLSTATE status and the
// error envelope. MYREST002 is a myrest gap code because a MySQL error cannot
// honestly claim a PostgreSQL SQLSTATE.
func TestMySQLAccessErrorGivesThePublishedStatusAndGapCode(t *testing.T) {
	t.Parallel()

	source := &reader{failure: mysqlError(1142, "42000", "SELECT command denied")}
	response, body := get(t, serve(t, source, settings()), "/items")

	apitest.AssertEnvelope(t, response, body, http.StatusForbidden, "MYREST002")
}

// Missing EXECUTE on a routine (including db-pre-request) is access denied.
func TestMySQLExecuteDeniedGivesForbiddenAndGapCode(t *testing.T) {
	t.Parallel()

	source := &reader{failure: mysqlError(1370, "42000", "execute command denied")}
	response, body := get(t, serve(t, source, settings()), "/items")

	apitest.AssertEnvelope(t, response, body, http.StatusForbidden, "MYREST002")
}

// err-005: a MySQL SQLSTATE outside the published table has the documented
// fallback status and keeps the same client error envelope.
func TestUnmappedMySQLErrorGivesTheFallbackStatusAndGapCode(t *testing.T) {
	t.Parallel()

	source := &reader{failure: mysqlError(0, "HY000", "general error")}
	response, body := get(t, serve(t, source, settings()), "/items")

	apitest.AssertEnvelope(t, response, body, http.StatusInternalServerError, "MYREST002")
}

// err-003: a PostgREST full-text search operator is refused with a myrest gap
// code. MySQL full-text search does not have the same semantics.
func TestPostgRESTFullTextSearchIsRefusedWithAMyrestGapCode(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?name=fts.english.alpha")

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// Negated PostgREST full-text search needs the same refusal.
func TestNegatedPostgRESTFullTextSearchIsRefusedWithAMyrestGapCode(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?name=not.fts.english.alpha")

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// cache-005: PostgREST domain and cast syntax is refused.
func TestPostgRESTCastSyntaxIsRefused(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?select=name::text")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(failure.Message, "domain and cast") {
		t.Fatalf("message = %q, want a domain and cast refusal", failure.Message)
	}
}

// cache-005: PostgREST row computed-field syntax is refused.
func TestPostgRESTComputedFieldSyntaxIsRefused(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?select=total()")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(failure.Message, "computed-field") {
		t.Fatalf("message = %q, want a computed-field refusal", failure.Message)
	}
}

// err-001: paths and methods outside the current service surface still have
// the client error envelope.
func TestUnhandledRequestGivesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(http.MethodPut, serve(t, &reader{}, settings()).URL()+"/items", nil)
	if err != nil {
		t.Fatalf("new PUT request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT /items: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read PUT /items body: %v", err)
	}

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "MYREST003")
}

// The words of the database name the accounts of the deployment: the operator
// reads them in the log, and the client reads none of them.
func TestFailingReadTellsTheOperatorAndNotTheClient(t *testing.T) {
	t.Parallel()

	said := "SELECT command denied to user 'authenticator'@'10.0.0.1'"
	var logged bytes.Buffer
	source := &reader{failure: errors.New(said)}

	_, body := get(t, serve(t, source, settings(), &logged), "/items")

	if strings.Contains(string(body), "authenticator") {
		t.Errorf("the client reads the words of the database: %s", body)
	}
	if !strings.Contains(logged.String(), said) {
		t.Errorf("the log %q does not hold what the database said", logged.String())
	}
	if !strings.Contains(logged.String(), "shop.items") {
		t.Errorf("the log %q does not name the table of the read", logged.String())
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

func mysqlError(number uint16, state, message string) error {
	var sqlState [5]byte
	copy(sqlState[:], state)

	return &mysql.MySQLError{Number: number, SQLState: sqlState, Message: message}
}
