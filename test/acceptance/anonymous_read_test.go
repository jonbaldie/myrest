// Package acceptance_test holds the normative scenarios of myrest at the HTTP
// API boundary, over a MySQL 8 database. The tests in the internal packages
// hold the same contracts with a reader that answers in memory; these tests
// prove that MySQL 8 gives the same answers.
package acceptance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/mysqltest"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// anonRole is the anonymous database role of the fixtures.
const anonRole = "myrest_anon"

// harness is the MySQL 8 database every test in this package reads.
var harness *mysqltest.Harness

func TestMain(m *testing.M) {
	started, err := mysqltest.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start MySQL: %v\n", err)
		os.Exit(1)
	}
	harness = started

	fixture := filepath.Join("..", "..", "testdata", "fixtures", "schema.sql")
	if err := harness.LoadSQL(fixture); err != nil {
		fmt.Fprintf(os.Stderr, "load the fixtures: %v\n", err)
		_ = harness.Stop()
		os.Exit(1)
	}

	code := m.Run()
	_ = harness.Stop()
	os.Exit(code)
}

// serve starts myrest over the fixture database, exposing the given databases.
func serve(t *testing.T, databases ...string) *httpapi.Service {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = databases
	settings.DB.AnonRole = anonRole

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
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func get(t *testing.T, service *httpapi.Service, path string) (*http.Response, []byte) {
	t.Helper()

	response, err := http.Get(service.URL() + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the body: %v", err)
	}
	return response, body
}

// smoke-001, cache-001 and repr-001: a client that sends no JWT reads the rows
// of an exposed table as the anonymous database role, in JSON.
func TestAnonymousReadOfAnExposedTable(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if want := `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// cache-002 and err-001: a table the anonymous role holds no SELECT on is not
// a resource, and the refusal carries the error envelope.
func TestTableWithoutSelectForTheAnonymousRoleIsNotAResource(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/secrets")

	assertEnvelope(t, response, body, http.StatusNotFound)
	if string(body) == "" {
		t.Fatal("the refusal carries no body")
	}
}

// A table outside db-schemas is not reachable, even with SELECT on it.
func TestTableOutsideTheConfiguredDatabasesIsNotReachable(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/outside_items")

	assertEnvelope(t, response, body, http.StatusNotFound)
}

// The pool logs in as the authenticator, which holds no privilege of its own:
// only the role switch of the request opens the read. A read of the same
// table with the role switch turned off must therefore fail.
func TestTheAuthenticatorAloneCannotReadTheResource(t *testing.T) {
	service := serve(t, "myrest_fixture")

	pool, err := mysqldb.Open(harness.URI("authenticator", "secret"))
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	table := schemacache.Table{
		Schema:  "myrest_fixture",
		Name:    "items",
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	if _, err := pool.Read(context.Background(), "", table); err == nil {
		t.Fatal("the authenticator read the table without a role switch")
	}

	if response, body := get(t, service, "/items"); response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d after the failed read; body = %s", response.StatusCode, body)
	}
}

// MySQL still enforces the grants after the role switch: a grant the schema
// cache still holds gives a database error, not rows.
func TestMySQLEnforcesTheGrantsAfterTheRoleSwitch(t *testing.T) {
	service := serve(t, "myrest_fixture")

	if err := harness.Exec("REVOKE SELECT ON myrest_fixture.items FROM 'myrest_anon'"); err != nil {
		t.Fatalf("revoke SELECT: %v", err)
	}
	t.Cleanup(func() {
		if err := harness.Exec("GRANT SELECT ON myrest_fixture.items TO 'myrest_anon'"); err != nil {
			t.Errorf("grant SELECT back: %v", err)
		}
	})

	response, body := get(t, service, "/items")
	if response.StatusCode == http.StatusOK {
		t.Fatalf("the read gave rows after the grant was taken away: %s", body)
	}
	assertEnvelope(t, response, body, http.StatusInternalServerError)
}

// assertEnvelope holds the error contract at the HTTP boundary.
func assertEnvelope(t *testing.T, response *http.Response, body []byte, status int) {
	t.Helper()

	if response.StatusCode != status {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, status, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode the body %s: %v", body, err)
	}
	for _, field := range []string{"code", "message", "details", "hint"} {
		if _, held := fields[field]; !held {
			t.Fatalf("the envelope %s holds no %s", body, field)
		}
	}
}
