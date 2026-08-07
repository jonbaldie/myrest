package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for ordinary writes. The Writer
// answers in memory. Acceptance tests hold the same contracts over MySQL 8.

// writer records insert, update, and delete calls.
type writer struct {
	role      schemacache.Role
	table     schemacache.Table
	rows      []map[string]any
	patch     map[string]any
	filters   []readquery.Filter
	groups    []readquery.Group
	inserted  int
	updated   int64
	deleted   int64
	failure   error
	called    string
	stoppable bool
}

func (w *writer) Insert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	rows []map[string]any,
) (int, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "insert"
	w.role, w.table, w.rows = role, table, rows
	if w.failure != nil {
		return 0, w.failure
	}
	if w.inserted != 0 {
		return w.inserted, nil
	}
	return len(rows), nil
}

func (w *writer) Update(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
) (int64, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "update"
	w.role, w.table, w.patch = role, table, patch
	w.filters, w.groups = query.Filters, query.Groups
	if w.failure != nil {
		return 0, w.failure
	}
	return w.updated, nil
}

func (w *writer) Delete(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (int64, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "delete"
	w.role, w.table = role, table
	w.filters, w.groups = query.Filters, query.Groups
	if w.failure != nil {
		return 0, w.failure
	}
	return w.deleted, nil
}

func serveWrite(
	t *testing.T,
	source httpapi.Reader,
	sink httpapi.Writer,
	resolved ...httpapi.Options,
) *httpapi.Service {
	t.Helper()

	settings := settings()
	options := httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings,
		Cache:    writeCache(),
		Reader:   source,
		Writer:   sink,
	}
	if len(resolved) == 1 {
		if resolved[0].Settings.DB.URI != "" {
			options.Settings = resolved[0].Settings
		}
		if resolved[0].Cache != nil {
			options.Cache = resolved[0].Cache
		}
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

// writeCache grants INSERT, UPDATE, and DELETE on items for the anonymous role.
func writeCache() *schemacache.Cache {
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
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: items, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: items, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: items, Privilege: "UPDATE"},
			{Role: "myrest_anon", Table: items, Privilege: "DELETE"},
			{Role: "myrest_user", Table: items, Privilege: "SELECT"},
			{Role: "myrest_user", Table: secrets, Privilege: "SELECT"},
		},
	})
}

// write-001: a POST of one object inserts a row and returns the default
// minimal response of the parity target.
func TestPostInsertsOneObject(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	response, body := apitest.PostJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		`{"name":"gamma"}`,
	)

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(body) != 0 {
		t.Fatalf("body = %s, want empty for Prefer return=minimal default", body)
	}
	if sink.called != "insert" {
		t.Fatalf("writer called %q, want insert", sink.called)
	}
	if sink.role != "myrest_anon" {
		t.Fatalf("role = %q, want myrest_anon", sink.role)
	}
	if sink.table.ID.Name != "items" {
		t.Fatalf("table = %#v", sink.table.ID)
	}
	if len(sink.rows) != 1 || sink.rows[0]["name"] != "gamma" {
		t.Fatalf("rows = %#v", sink.rows)
	}
}

// write-001: a POST of a JSON array inserts each row.
func TestPostInsertsJSONArray(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	response, body := apitest.PostJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		`[{"name":"delta"},{"name":"epsilon"}]`,
	)

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(sink.rows) != 2 {
		t.Fatalf("rows = %#v", sink.rows)
	}
	if sink.rows[0]["name"] != "delta" || sink.rows[1]["name"] != "epsilon" {
		t.Fatalf("rows = %#v", sink.rows)
	}
}

// write-002: a PATCH with a filter updates only through that filter.
func TestPatchUpdatesByFilter(t *testing.T) {
	t.Parallel()

	sink := &writer{updated: 1}
	request, err := http.NewRequest(
		http.MethodPatch,
		serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.alpha",
		strings.NewReader(`{"name":"alpha2"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}
	if sink.called != "update" {
		t.Fatalf("writer called %q, want update", sink.called)
	}
	if sink.patch["name"] != "alpha2" {
		t.Fatalf("patch = %#v", sink.patch)
	}
	if len(sink.filters) != 1 || sink.filters[0].Op != readquery.OpEq || sink.filters[0].Value != "alpha" {
		t.Fatalf("filters = %#v", sink.filters)
	}
}

// write-003: a DELETE with a filter removes only through that filter.
func TestDeleteRemovesByFilter(t *testing.T) {
	t.Parallel()

	sink := &writer{deleted: 1}
	response, body := apitest.Do(
		t,
		http.MethodDelete,
		serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.beta",
		nil,
	)

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}
	if sink.called != "delete" {
		t.Fatalf("writer called %q, want delete", sink.called)
	}
	if len(sink.filters) != 1 || sink.filters[0].Column != "name" || sink.filters[0].Value != "beta" {
		t.Fatalf("filters = %#v", sink.filters)
	}
}

// write-005: a PATCH or DELETE with no filter and no Prefer: all-rows refuses.
func TestUnboundedPatchAndDeleteRefuse(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	service := serveWrite(t, &reader{}, sink)

	request, err := http.NewRequest(
		http.MethodPatch,
		service.URL()+"/items",
		strings.NewReader(`{"name":"nope"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
	if sink.called != "" {
		t.Fatalf("writer must not run for an unbounded PATCH; called %q", sink.called)
	}

	response, body = apitest.Do(t, http.MethodDelete, service.URL()+"/items", nil)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
}

// Prefer: all-rows unlocks an otherwise unbounded DELETE.
func TestPreferAllRowsAllowsUnboundedDelete(t *testing.T) {
	t.Parallel()

	sink := &writer{deleted: 2}
	headers := http.Header{}
	headers.Set("Prefer", "all-rows")
	response, body := apitest.Do(
		t,
		http.MethodDelete,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		headers,
	)

	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}
	if sink.called != "delete" {
		t.Fatalf("writer called %q, want delete", sink.called)
	}
}

// A write without the matching grant is denied.
func TestWriteWithoutGrantIsDenied(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: items, Privilege: "SELECT"},
		},
	})
	sink := &writer{}
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    cache,
		Reader:   &reader{},
		Writer:   sink,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := apitest.PostJSON(t, service.URL()+"/items", `{"name":"nope"}`)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if sink.called != "" {
		t.Fatalf("writer must not run without INSERT; called %q", sink.called)
	}
}
