// Package acceptance_test holds the normative scenarios of myrest at the HTTP
// API boundary, over a MySQL 8 database. The tests in the internal packages
// hold the same contracts with a reader that answers in memory; these tests
// prove that MySQL 8 gives the same answers.
package acceptance_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/mysqltest"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// anonRole is the anonymous database role of the fixtures.
const anonRole = "myrest_anon"

// harness is the MySQL 8 database every test in this package reads.
var harness *mysqltest.Harness

func TestMain(m *testing.M) {
	fixtures := []string{mysqltest.FixtureSchema(filepath.Join("..", ".."))}
	os.Exit(mysqltest.RunTests(fixtures, func(started *mysqltest.Harness) int {
		harness = started
		return m.Run()
	}))
}

// serve starts myrest over the fixture database, exposing the given databases
// to the anonymous database role of the fixtures.
func serve(t *testing.T, databases ...string) *httpapi.Service {
	t.Helper()

	_, _, service := serveWithPoolAs(t, anonRole, databases...)
	return service
}

// serveWithPool starts myrest and keeps the pool and schema cache, so a test
// can reload the schema cache from the live catalog.
func serveWithPool(t *testing.T, databases ...string) (*mysqldb.Pool, *schemacache.Cache, *httpapi.Service) {
	t.Helper()

	return serveWithPoolAs(t, anonRole, databases...)
}

// serveAs starts myrest with the given anonymous database role.
func serveAs(t *testing.T, role string, databases ...string) *httpapi.Service {
	t.Helper()

	_, _, service := serveWithPoolAs(t, role, databases...)
	return service
}

// serveWithPoolAs starts myrest with the given anonymous database role and
// keeps the authenticator pool and schema cache.
func serveWithPoolAs(t *testing.T, role string, databases ...string) (*mysqldb.Pool, *schemacache.Cache, *httpapi.Service) {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = databases
	settings.DB.AnonRole = role

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	pool.SetPreRequest(settings.DB.PreRequest)

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
	return pool, cache, service
}

func get(t *testing.T, service *httpapi.Service, path string) (*http.Response, []byte) {
	t.Helper()

	return apitest.Get(t, service.URL()+path)
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
	if want := `[{"id":1,"name":"alpha","name_len":5},{"id":2,"name":"beta","name_len":4}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// cache-002 and err-001: a table the anonymous role holds no SELECT on is not
// a resource, and the refusal carries the error envelope.
func TestTableWithoutSelectForTheAnonymousRoleIsNotAResource(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/secrets")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if want := "Could not find the table 'myrest_fixture.secrets' in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// A table outside db-schemas is not reachable, even with SELECT on it.
func TestTableOutsideTheConfiguredDatabasesIsNotReachable(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/outside_items")

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// A request names no database, so the read goes to the default database: the
// first of db-schemas. A table of the second database is not reachable without
// Accept-Profile, even with SELECT on it.
func TestReadGoesToTheDefaultDatabaseOnly(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture", "myrest_hidden"), "/outside_items")

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")

	// The same table answers when its database comes first.
	response, body = get(t, serve(t, "myrest_hidden", "myrest_fixture"), "/outside_items")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
}

// smoke-001 for a role name MySQL takes but a simple name check would refuse.
func TestAnonymousReadWithADashInTheRoleName(t *testing.T) {
	response, body := get(t, serveAs(t, "web-anon", "myrest_fixture"), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha","name_len":5},{"id":2,"name":"beta","name_len":4}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// cache-001 through a second role: MySQL reads with the privileges of the
// roles granted to the active role, so those tables are resources too.
func TestGrantThroughAnotherRoleIsAResource(t *testing.T) {
	for _, statement := range []string{
		"CREATE ROLE IF NOT EXISTS 'nested_reader'",
		"GRANT SELECT ON myrest_fixture.secrets TO 'nested_reader'",
		"GRANT 'nested_reader' TO 'myrest_anon'",
	} {
		if err := harness.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		if err := harness.Exec("DROP ROLE IF EXISTS 'nested_reader'"); err != nil {
			t.Errorf("drop the nested role: %v", err)
		}
	})

	response, body := get(t, serve(t, "myrest_fixture"), "/secrets")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"payload":"top-secret"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
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
		ID:      schemacache.TableID{Database: "myrest_fixture", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}, {Name: "name_len"}},
	}
	if _, err := pool.Read(context.Background(), "", table, readquery.Query{}); err == nil {
		t.Fatal("the authenticator read the table without a role switch")
	}

	if response, body := get(t, service, "/items"); response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d after the failed read; body = %s", response.StatusCode, body)
	}
}

// cache-003: after a catalog or grant change and an explicit reload, the new
// exposure is visible over HTTP. A restart of the process is not required.
func TestExplicitReloadShowsNewExposure(t *testing.T) {
	pool, cache, service := serveWithPool(t, "myrest_fixture")

	for _, statement := range []string{
		`CREATE TABLE myrest_fixture.reloaded (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			label VARCHAR(255) NOT NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB`,
		`INSERT INTO myrest_fixture.reloaded (label) VALUES ('fresh')`,
		`GRANT SELECT ON myrest_fixture.reloaded TO 'myrest_anon'`,
	} {
		if err := harness.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		_ = harness.Exec("DROP TABLE IF EXISTS myrest_fixture.reloaded")
	})

	response, body := get(t, service, "/reloaded")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")

	catalog, err := pool.Catalog(t.Context(), []string{"myrest_fixture"})
	if err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	cache.Replace(catalog)

	response, body = get(t, service, "/reloaded")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"label":"fresh"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// A read through an exposed view uses the ordinary read surface.
func TestReadThroughViewOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/items_view?select=id,name&name=eq.alpha&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// A view the active role cannot privilege-use is not a usable resource.
func TestViewWithoutSelectIsNotAResourceOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/locked_view")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
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

	apitest.AssertEnvelope(t, response, body, http.StatusForbidden, "MYREST002")

	// What MySQL says names the accounts of the deployment, so it goes to
	// the log of the operator and not to the client.
	if strings.Contains(string(body), "authenticator") {
		t.Fatalf("the client reads the words of MySQL: %s", body)
	}
}
