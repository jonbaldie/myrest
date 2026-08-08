package acceptance_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// serveWithTxEnd starts myrest with the given db-tx-end value.
func serveWithTxEnd(t *testing.T, txEnd config.TxEnd) *httpapi.Service {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = []string{"myrest_fixture"}
	settings.DB.AnonRole = anonRole
	settings.DB.TxEnd = txEnd

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	pool.SetPreRequest(settings.DB.PreRequest)
	pool.SetTxEnd(settings.DB.TxEnd)

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

func postJSONWithPrefer(t *testing.T, url, body, prefer string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if prefer != "" {
		request.Header.Set("Prefer", prefer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response, answer
}

// A write under db-tx-end=rollback answers successfully and then rolls back
// as one unit: a later read does not see the row.
func TestWriteRollsBackAsOneUnitWhenTxEndIsRollback(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndRollback)

	response, body := postJSONWithPrefer(
		t,
		service.URL()+"/items",
		`{"name":"tx-rollback-unit"}`,
		"return=representation",
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if !strings.Contains(string(body), `"name":"tx-rollback-unit"`) {
		t.Fatalf("representation body missing inserted name: %s", body)
	}

	_, body = get(t, service, "/items?select=name&name=eq.tx-rollback-unit")
	if string(body) != "[]\n" {
		t.Fatalf("rolled-back write still visible: %s", body)
	}
}

// Prefer: tx=rollback under commit-allow-override rolls the write back and
// sets Preference-Applied.
func TestPreferTxRollbackUnderCommitAllowOverride(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndCommitAllowOverride)

	response, body := postJSONWithPrefer(
		t,
		service.URL()+"/items",
		`{"name":"tx-prefer-rollback"}`,
		"tx=rollback, return=representation",
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	applied := response.Header.Get("Preference-Applied")
	if !strings.Contains(applied, "tx=rollback") {
		t.Fatalf("Preference-Applied = %q, want tx=rollback", applied)
	}

	_, body = get(t, service, "/items?select=name&name=eq.tx-prefer-rollback")
	if string(body) != "[]\n" {
		t.Fatalf("prefer-rollback write still visible: %s", body)
	}
}

// Prefer: tx=commit under rollback-allow-override keeps the write.
func TestPreferTxCommitUnderRollbackAllowOverride(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndRollbackAllowOverride)

	response, body := postJSONWithPrefer(
		t,
		service.URL()+"/items",
		`{"name":"tx-prefer-commit"}`,
		"tx=commit, return=representation",
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	applied := response.Header.Get("Preference-Applied")
	if !strings.Contains(applied, "tx=commit") {
		t.Fatalf("Preference-Applied = %q, want tx=commit", applied)
	}

	_, body = get(t, service, "/items?select=name&name=eq.tx-prefer-commit")
	if !strings.Contains(string(body), `"name":"tx-prefer-commit"`) {
		t.Fatalf("prefer-commit write missing: %s", body)
	}
}

// An RPC call under db-tx-end=rollback runs as one unit and does not persist
// side effects.
func TestRPCRollsBackAsOneUnitWhenTxEndIsRollback(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndRollback)
	clearRPCWriteMarker(t, service)

	response, body := apitest.PostJSON(t, service.URL()+"/rpc/write_marker", `{}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rpc status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}

	_, body = get(t, service, "/addresses?select=label&label=eq.rpc-write")
	if string(body) != "[]\n" {
		t.Fatalf("rolled-back RPC write still visible: %s", body)
	}
}

// Prefer: tx=rollback under commit-allow-override rolls an RPC side effect back.
func TestRPCPreferTxRollbackUnderCommitAllowOverride(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndCommitAllowOverride)
	clearRPCWriteMarker(t, service)

	response, body := postJSONWithPrefer(
		t,
		service.URL()+"/rpc/write_marker",
		`{}`,
		"tx=rollback",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rpc status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Preference-Applied"); got != "tx=rollback" {
		t.Fatalf("Preference-Applied = %q, want tx=rollback", got)
	}

	_, body = get(t, service, "/addresses?select=label&label=eq.rpc-write")
	if string(body) != "[]\n" {
		t.Fatalf("prefer-rollback RPC write still visible: %s", body)
	}
}

func clearRPCWriteMarker(t *testing.T, service *httpapi.Service) {
	t.Helper()

	headers := http.Header{}
	headers.Set("Prefer", "all-rows")
	_, _ = apitest.Do(t, http.MethodDelete, service.URL()+"/addresses?label=eq.rpc-write", headers)
}

// Default db-tx-end=commit keeps a write (control case for the tx-end gate).
func TestWriteCommitsWhenTxEndIsCommit(t *testing.T) {
	service := serveWithTxEnd(t, config.TxEndCommit)

	response, body := postJSONWithPrefer(
		t,
		service.URL()+"/items",
		`{"name":"tx-commit-keep"}`,
		"return=representation",
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	_, body = get(t, service, "/items?select=name&name=eq.tx-commit-keep")
	if !strings.Contains(string(body), `"name":"tx-commit-keep"`) {
		t.Fatalf("committed write missing: %s", body)
	}
}
