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
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

// Seam under test: the HTTP API boundary for ordinary writes. The Writer
// answers in memory. Acceptance tests hold the same contracts over MySQL 8.

// writer records insert, update, and delete calls.
type writer struct {
	role       schemacache.Role
	table      schemacache.Table
	rows       []map[string]any
	patch      map[string]any
	row        map[string]any
	primaryKey []string
	resolution httpapi.UpsertResolution
	filters    []readquery.Filter
	groups     []readquery.Group
	inserted   int
	updated    int64
	deleted    int64
	upserted   bool
	failure    error
	called     string
	stoppable  bool
	options    writequery.Options
	resultRows []rows.Row
	resultKeys []map[string]any
}

func (w *writer) Insert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	rows []map[string]any,
	options writequery.Options,
) (writequery.Result, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "insert"
	w.role, w.table, w.rows = role, table, rows
	w.options = options
	if w.failure != nil {
		return writequery.Result{}, w.failure
	}
	result := writequery.Result{Affected: int64(len(rows)), Rows: w.resultRows, Keys: w.resultKeys}
	if w.inserted != 0 {
		result.Affected = int64(w.inserted)
	}
	return result, nil
}

func (w *writer) Update(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "update"
	w.role, w.table, w.patch = role, table, patch
	w.filters, w.groups = query.Filters, query.Groups
	w.options = options
	if w.failure != nil {
		return writequery.Result{}, w.failure
	}
	return writequery.Result{Affected: w.updated, Rows: w.resultRows, Keys: w.resultKeys}, nil
}

func (w *writer) Delete(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "delete"
	w.role, w.table = role, table
	w.filters, w.groups = query.Filters, query.Groups
	w.options = options
	if w.failure != nil {
		return writequery.Result{}, w.failure
	}
	return writequery.Result{Affected: w.deleted, Rows: w.resultRows, Keys: w.resultKeys}, nil
}

func (w *writer) Upsert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	row map[string]any,
	primaryKey []string,
	resolution httpapi.UpsertResolution,
) (bool, error) {
	w.stoppable = ctx != nil && ctx.Done() != nil
	w.called = "upsert"
	w.role, w.table = role, table
	w.row, w.primaryKey, w.resolution = row, primaryKey, resolution
	if w.failure != nil {
		return false, w.failure
	}
	return w.upserted, nil
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
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	profiles := schemacache.TableID{Database: "shop", Name: "profiles"}
	secrets := schemacache.TableID{Database: "shop", Name: "secrets"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders, profiles, secrets},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "item_id"},
			{Table: profiles, Name: "id"},
			{Table: secrets, Name: "payload"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_item", Table: orders, Columns: []string{"item_id"},
			ReferencedTable: items, ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: orders},
			{Role: "myrest_anon", Table: profiles},
			{Role: "myrest_user", Table: items},
			{Role: "myrest_user", Table: secrets},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: items, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: items, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: items, Privilege: "UPDATE"},
			{Role: "myrest_anon", Table: items, Privilege: "DELETE"},
			{Role: "myrest_anon", Table: orders, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: orders, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: profiles, Privilege: "SELECT"},
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

// Grant denial wins over the unbounded-write gate.
func TestWriteWithoutGrantBeatsUnboundedGate(t *testing.T) {
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

	response, body := apitest.Do(t, http.MethodDelete, service.URL()+"/items", nil)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if sink.called != "" {
		t.Fatalf("writer must not run without DELETE; called %q", sink.called)
	}
}

// write-006: a write through a writable view succeeds under the view grants.
func TestWriteThroughWritableViewSucceeds(t *testing.T) {
	t.Parallel()

	itemsView := schemacache.TableID{Database: "shop", Name: "items_view"}
	cache := schemacache.Build(schemacache.Catalog{
		Views:          []schemacache.TableID{itemsView},
		UpdatableViews: []schemacache.TableID{itemsView},
		Columns: []schemacache.ColumnFact{
			{Table: itemsView, Name: "id"},
			{Table: itemsView, Name: "name"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: itemsView},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: itemsView, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: itemsView, Privilege: "INSERT"},
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

	response, body := apitest.PostJSON(t, service.URL()+"/items_view", `{"name":"via-view"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if sink.called != "insert" || sink.table.ID.Name != "items_view" {
		t.Fatalf("writer called %q on %#v", sink.called, sink.table.ID)
	}
}

// write-006: a write through a non-updatable view refuses stably.
func TestWriteThroughNonWritableViewRefuses(t *testing.T) {
	t.Parallel()

	stats := schemacache.TableID{Database: "shop", Name: "items_stats"}
	cache := schemacache.Build(schemacache.Catalog{
		Views: []schemacache.TableID{stats},
		Columns: []schemacache.ColumnFact{
			{Table: stats, Name: "total"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: stats},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: stats, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: stats, Privilege: "INSERT"},
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

	response, body := apitest.PostJSON(t, service.URL()+"/items_stats", `{"total":1}`)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if sink.called != "" {
		t.Fatalf("writer must not run on a non-updatable view; called %q", sink.called)
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

func TestPreferReturnMinimalPost(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"gamma"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=minimal")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(body) != 0 {
		t.Fatalf("body = %s, want empty", body)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=minimal" {
		t.Fatalf("Preference-Applied = %q", got)
	}
}

// write-007: Prefer return=headers-only returns Location and no body.
func TestPreferReturnHeadersOnly(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultKeys: []map[string]any{{"id": int64(9)}},
	}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"gamma"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=headers-only")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(body) != 0 {
		t.Fatalf("body = %s, want empty", body)
	}
	if got := response.Header.Get("Location"); got != "/items?id=eq.9" {
		t.Fatalf("Location = %q", got)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=headers-only" {
		t.Fatalf("Preference-Applied = %q", got)
	}
	if !sink.options.ReturnKeys {
		t.Fatalf("options = %#v, want ReturnKeys", sink.options)
	}
}

// write-008 / smoke-003: Prefer return=representation returns the affected rows.
func TestPreferReturnRepresentation(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultRows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(9), "gamma"}},
		},
		resultKeys: []map[string]any{{"id": int64(9)}},
	}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"gamma"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if want := `[{"id":9,"name":"gamma"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=representation" {
		t.Fatalf("Preference-Applied = %q", got)
	}
	if !sink.options.ReturnRepresentation {
		t.Fatalf("options = %#v", sink.options)
	}
}

// write-011: return=representation with a nested select over a cache
// relationship nests the related rows in the write body.
func TestPreferReturnRepresentationWithEmbed(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultRows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(9), int64(1)}},
		},
		resultKeys: []map[string]any{{"id": int64(9)}},
	}
	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
	}}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, source, sink).URL()+"/orders?select=id,items(id,name)",
		strings.NewReader(`{"item_id":1}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	want := `[{"id":9,"items":{"id":1,"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if sink.called != "insert" {
		t.Fatalf("called = %q, want insert", sink.called)
	}
}

// Nested filter and order on a write representation follow embed read rules.
func TestPreferReturnRepresentationEmbedNestedFilterOrder(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultRows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
		},
		resultKeys: []map[string]any{{"id": int64(1)}},
		updated:    1,
	}
	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(2), int64(1)}},
			{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}},
		}},
	}}
	request, err := http.NewRequest(
		http.MethodPatch,
		serveWrite(t, source, sink).URL()+
			"/items?id=eq.1&select=id,orders(id)&orders.order=id.desc&orders.limit=1&orders.id=gt.1",
		strings.NewReader(`{"name":"alpha"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// write-012: return=representation with a nested select and no cache
// relationship refuses stably and does not write.
func TestPreferReturnRepresentationEmbedWithoutRelationshipRefuses(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items?select=id,profiles(id)",
		strings.NewReader(`{"name":"gamma"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST200")
	if sink.called != "" {
		t.Fatalf("called = %q, want no write", sink.called)
	}
}

// write-009: Prefer return=representation refuses when no primary key can
// identify inserted rows honestly.
func TestPreferReturnRepresentationWithoutPrimaryKeyRefuses(t *testing.T) {
	t.Parallel()

	notes := schemacache.TableID{Database: "shop", Name: "notes"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{notes},
		Columns: []schemacache.ColumnFact{
			{Table: notes, Name: "body"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: notes},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: notes, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: notes, Privilege: "INSERT"},
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

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/notes",
		strings.NewReader(`{"body":"x"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if sink.called != "" {
		t.Fatalf("writer must not run; called %q", sink.called)
	}
}

// write-010: Prefer missing=default reaches the writer.
func TestPreferMissingDefault(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"gamma"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "missing=default")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if !sink.options.MissingDefault {
		t.Fatalf("options = %#v", sink.options)
	}
	if got := response.Header.Get("Preference-Applied"); got != "missing=default" {
		t.Fatalf("Preference-Applied = %q", got)
	}
}

// write-010: Prefer max-affected with handling=strict refuses when too many rows change.
func TestPreferMaxAffectedStrict(t *testing.T) {
	t.Parallel()

	sink := &writer{failure: writequery.MaxAffectedExceeded{Affected: 5, Max: 2}}
	headers := http.Header{}
	headers.Set("Prefer", "handling=strict, max-affected=2")
	response, body := apitest.Do(
		t,
		http.MethodDelete,
		serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.alpha",
		headers,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST124")
	if sink.options.MaxAffected == nil || *sink.options.MaxAffected != 2 {
		t.Fatalf("options = %#v", sink.options)
	}
}

// write-010: Prefer handling=strict refuses unknown preference tokens.
func TestPreferHandlingStrictRejectsUnknown(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	headers := http.Header{}
	headers.Set("Prefer", "handling=strict, foo")
	headers.Set("Content-Type", "application/json")
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"x"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header = headers
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	envelope := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST122")
	if envelope.Details == nil {
		t.Fatalf("details = %#v", envelope.Details)
	}
	if sink.called != "" {
		t.Fatalf("writer must not run; called %q", sink.called)
	}
}

// write-010: Prefer handling=lenient ignores unknown tokens.
func TestPreferHandlingLenientIgnoresUnknown(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	request, err := http.NewRequest(
		http.MethodPost,
		serveWrite(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"name":"x"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "handling=lenient, foo")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if sink.called != "insert" {
		t.Fatalf("writer called %q", sink.called)
	}
}

func TestPutUpsertByPrimaryKeyMergeDuplicates(t *testing.T) {
	t.Parallel()

	sink := &writer{upserted: true}
	response, body := putJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items?id=eq.1",
		`{"id":1,"name":"alpha2"}`,
		"resolution=merge-duplicates",
	)

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(body) != 0 {
		t.Fatalf("body = %s, want empty for Prefer return=minimal default", body)
	}
	if sink.called != "upsert" {
		t.Fatalf("writer called %q, want upsert", sink.called)
	}
	if sink.resolution != httpapi.UpsertMergeDuplicates {
		t.Fatalf("resolution = %v, want merge-duplicates", sink.resolution)
	}
	if len(sink.primaryKey) != 1 || sink.primaryKey[0] != "id" {
		t.Fatalf("primaryKey = %#v", sink.primaryKey)
	}
	if sink.row["name"] != "alpha2" {
		t.Fatalf("row = %#v", sink.row)
	}
}

// write-004: a PUT by primary key with resolution=ignore-duplicates succeeds.
func TestPutUpsertByPrimaryKeyIgnoreDuplicates(t *testing.T) {
	t.Parallel()

	sink := &writer{upserted: true}
	response, body := putJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items?id=eq.99",
		`{"id":99,"name":"fresh"}`,
		"resolution=ignore-duplicates",
	)

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if sink.called != "upsert" {
		t.Fatalf("writer called %q, want upsert", sink.called)
	}
	if sink.resolution != httpapi.UpsertIgnoreDuplicates {
		t.Fatalf("resolution = %v, want ignore-duplicates", sink.resolution)
	}
}

// A PUT that does not target the primary key refuses stably.
func TestPutWithoutPrimaryKeyTargetRefuses(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	response, body := putJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.alpha",
		`{"id":1,"name":"alpha"}`,
		"resolution=merge-duplicates",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST105")
	if sink.called != "" {
		t.Fatalf("writer must not run for a non-PK PUT; called %q", sink.called)
	}
}

// A PUT without the matching grant is denied.
func TestPutWithoutGrantIsDenied(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
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

	response, body := putJSON(
		t,
		service.URL()+"/items?id=eq.1",
		`{"id":1,"name":"nope"}`,
		"resolution=merge-duplicates",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
	if sink.called != "" {
		t.Fatalf("writer must not run without INSERT; called %q", sink.called)
	}
}

// An updated row under merge-duplicates answers 204 with the minimal default.
func TestPutMergeUpdateAnswersNoContent(t *testing.T) {
	t.Parallel()

	sink := &writer{upserted: false}
	response, body := putJSON(
		t,
		serveWrite(t, &reader{}, sink).URL()+"/items?id=eq.1",
		`{"id":1,"name":"alpha2"}`,
		"resolution=merge-duplicates",
	)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}
}

func putJSON(t *testing.T, url, body, prefer string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if prefer != "" {
		request.Header.Set("Prefer", prefer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response, answer
}
