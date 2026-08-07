package acceptance_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// serveWithPreRequest starts myrest with db-pre-request set to the named
// routine after the role switch.
func serveWithPreRequest(t *testing.T, routine string) *httpapi.Service {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = []string{"myrest_fixture"}
	settings.DB.AnonRole = anonRole
	settings.DB.PreRequest = routine

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
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings,
		Cache:    schemacache.Build(catalog),
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

func clearPreRequestLog(t *testing.T) {
	t.Helper()

	if err := harness.Exec("DELETE FROM myrest_fixture.pre_request_log"); err != nil {
		t.Fatalf("clear pre_request_log: %v", err)
	}
}

func preRequestLogCount(t *testing.T) int {
	t.Helper()

	db, err := sql.Open("mysql", harness.RootDSN())
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM myrest_fixture.pre_request_log").Scan(&count); err != nil {
		t.Fatalf("count pre_request_log: %v", err)
	}
	return count
}

// With db-pre-request unset, myrest calls no hook.
func TestPreRequestUnsetCallsNoHook(t *testing.T) {
	clearPreRequestLog(t)

	response, body := get(t, serve(t, "myrest_fixture"), "/items")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if count := preRequestLogCount(t); count != 0 {
		t.Fatalf("pre_request_log rows = %d, want 0 when db-pre-request is unset", count)
	}
}

// With db-pre-request set, myrest calls the named routine after the role
// switch and before the main statement.
func TestPreRequestRunsBeforeTheMainStatement(t *testing.T) {
	clearPreRequestLog(t)

	response, body := get(t, serveWithPreRequest(t, "myrest_fixture.before_request"), "/items")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if count := preRequestLogCount(t); count != 1 {
		t.Fatalf("pre_request_log rows = %d, want 1 after the hook ran", count)
	}
}

// A failing hook returns the error envelope with a stable code, and the main
// statement does not run.
func TestPreRequestFailureBlocksTheMainStatement(t *testing.T) {
	clearPreRequestLog(t)

	service := serveWithPreRequest(t, "myrest_fixture.before_request_fail")
	response, body := apitest.PostJSON(t, service.URL()+"/items", `{"name":"hook-blocked"}`)
	apitest.AssertEnvelope(t, response, body, http.StatusInternalServerError, "MYREST002")

	db, err := sql.Open("mysql", harness.RootDSN())
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM myrest_fixture.items WHERE name = 'hook-blocked'",
	).Scan(&count); err != nil {
		t.Fatalf("count blocked insert: %v", err)
	}
	if count != 0 {
		t.Fatalf("items named hook-blocked = %d, want 0 when the hook fails", count)
	}
}

// The hook runs as the active database role and needs the EXECUTE grant.
func TestPreRequestNeedsExecuteGrant(t *testing.T) {
	clearPreRequestLog(t)

	service := serveWithPreRequest(t, "myrest_fixture.before_request_denied")
	response, body := get(t, service, "/items")
	apitest.AssertEnvelope(t, response, body, http.StatusForbidden, "MYREST002")
	if count := preRequestLogCount(t); count != 0 {
		t.Fatalf("pre_request_log rows = %d, want 0 when EXECUTE is missing", count)
	}
}

// myrest injects no claims or headers as GUCs for the hook.
func TestPreRequestDoesNotInjectClaimGUCs(t *testing.T) {
	service := serveWithPreRequest(t, "myrest_fixture.before_request")
	headers := make(http.Header)
	headers.Set("Prefer", "jwt-claims")
	response, body := apitest.Do(t, http.MethodGet, service.URL()+"/items", headers)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
