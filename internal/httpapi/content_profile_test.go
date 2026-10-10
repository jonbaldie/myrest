package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for the Content-Profile response
// header (repr-002, issue #235). The parity target names the request database
// in Content-Profile with the Content-Type of a successful body, when the
// database was negotiated by profile: the client sent a profile header, or
// db-schemas lists more than one database.

// contentProfileCache holds items, item_count, and list_items in two databases,
// with every grant the anonymous database role needs to read, write, and call
// them.
func contentProfileCache() *schemacache.Cache {
	catalog := schemacache.Catalog{}
	for _, database := range []string{"shop", "warehouse"} {
		items := schemacache.TableID{Database: database, Name: "items"}
		count := schemacache.RoutineID{Database: database, Name: "item_count"}
		list := schemacache.RoutineID{Database: database, Name: "list_items"}
		catalog.Tables = append(catalog.Tables, items)
		catalog.Columns = append(catalog.Columns, schemacache.ColumnFact{Table: items, Name: "id"})
		catalog.Keys = append(catalog.Keys, schemacache.KeyFact{
			Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"},
		})
		catalog.Selects = append(catalog.Selects, schemacache.SelectFact{Role: "myrest_anon", Table: items})
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			catalog.TablePrivileges = append(catalog.TablePrivileges, schemacache.TablePrivilegeFact{
				Role: "myrest_anon", Table: items, Privilege: privilege,
			})
		}
		catalog.Routines = append(catalog.Routines,
			schemacache.RoutineFact{ID: count, Kind: "FUNCTION", ReturnType: "bigint", SQLDataAccess: "NO SQL"},
			schemacache.RoutineFact{ID: list, Kind: "PROCEDURE", SQLDataAccess: "READS SQL DATA"},
		)
		for _, routine := range []schemacache.RoutineID{count, list} {
			catalog.RoutinePrivileges = append(catalog.RoutinePrivileges, schemacache.RoutinePrivilegeFact{
				Role: "myrest_anon", Routine: routine, Privilege: "EXECUTE",
			})
		}
	}
	return schemacache.Build(catalog)
}

// procedureCaller answers a procedure with a row set and a function with a
// scalar.
type procedureCaller struct{}

func (procedureCaller) Call(
	_ context.Context,
	_ schemacache.Role,
	routine schemacache.RoutineFact,
	_ map[string]any,
	options httpapi.CallOptions,
) (any, error) {
	var body any = int64(3)
	if routine.Kind == "PROCEDURE" {
		body = []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}
	}
	if options.Validate != nil {
		if err := options.Validate(body); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func serveContentProfile(t *testing.T, databases []string, source *reader) *httpapi.Service {
	t.Helper()

	resolved := settings()
	resolved.DB.Schemas = databases
	resolved.OpenAPI.Mode = config.OpenAPIModeFollowPrivileges
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    contentProfileCache(),
		Reader:   source,
		Writer:   &writer{},
		Caller:   procedureCaller{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

type contentProfileCase struct {
	name    string
	method  string
	path    string
	headers map[string]string
	body    string
	status  int
	want    string
}

func (test contentProfileCase) run(t *testing.T, databases []string, source *reader) {
	t.Helper()

	headers := http.Header{}
	for name, value := range test.headers {
		headers.Set(name, value)
	}
	if test.body != "" {
		headers.Set("Content-Type", "application/json")
	}
	response, body := apitest.DoBody(
		t,
		test.method,
		serveContentProfile(t, databases, source).URL()+test.path,
		headers,
		test.body,
	)

	if response.StatusCode != test.status {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, test.status, body)
	}
	got, sent := response.Header[http.CanonicalHeaderKey("Content-Profile")]
	if test.want == "" {
		if sent {
			t.Fatalf("Content-Profile = %q, want no header", got)
		}
		return
	}
	if len(got) != 1 || got[0] != test.want {
		t.Fatalf("Content-Profile = %q, want %q", got, test.want)
	}
}

// repr-002: with two configured databases, every successful body names the
// request database, also when the client sent no profile header.
func TestSuccessfulBodyNamesTheRequestDatabase(t *testing.T) {
	t.Parallel()

	accept := map[string]string{"Accept-Profile": "warehouse"}
	content := map[string]string{"Content-Profile": "warehouse"}
	represent := map[string]string{"Content-Profile": "warehouse", "Prefer": "return=representation"}
	for _, test := range []contentProfileCase{
		{name: "GET default", method: http.MethodGet, path: "/items", status: http.StatusOK, want: "shop"},
		{name: "GET Accept-Profile", method: http.MethodGet, path: "/items", headers: accept,
			status: http.StatusOK, want: "warehouse"},
		{name: "HEAD Accept-Profile", method: http.MethodHead, path: "/items", headers: accept,
			status: http.StatusOK, want: "warehouse"},
		{name: "POST representation default", method: http.MethodPost, path: "/items",
			headers: map[string]string{"Prefer": "return=representation"}, body: `{"id":1}`,
			status: http.StatusCreated, want: "shop"},
		{name: "POST representation", method: http.MethodPost, path: "/items", headers: represent,
			body: `{"id":1}`, status: http.StatusCreated, want: "warehouse"},
		{name: "PATCH representation", method: http.MethodPatch, path: "/items?id=eq.1", headers: represent,
			body: `{"id":2}`, status: http.StatusOK, want: "warehouse"},
		{name: "DELETE representation", method: http.MethodDelete, path: "/items?id=eq.1", headers: represent,
			status: http.StatusOK, want: "warehouse"},
		{name: "GET rpc default", method: http.MethodGet, path: "/rpc/item_count", status: http.StatusOK, want: "shop"},
		{name: "GET rpc Accept-Profile", method: http.MethodGet, path: "/rpc/item_count", headers: accept,
			status: http.StatusOK, want: "warehouse"},
		{name: "POST rpc Content-Profile", method: http.MethodPost, path: "/rpc/item_count", headers: content,
			body: `{}`, status: http.StatusOK, want: "warehouse"},
		{name: "POST row-set rpc Content-Profile", method: http.MethodPost, path: "/rpc/list_items",
			headers: content, body: `{}`, status: http.StatusOK, want: "warehouse"},
		{name: "OpenAPI default", method: http.MethodGet, path: "/", status: http.StatusOK, want: "shop"},
		{name: "OpenAPI Accept-Profile", method: http.MethodGet, path: "/", headers: accept,
			status: http.StatusOK, want: "warehouse"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, []string{"shop", "warehouse"}, &reader{})
		})
	}
}

// The parity target sends Content-Profile only with the Content-Type of a
// successful body: an answer with no body and every error envelope name no
// profile.
func TestResponseWithoutBodyOrWithFailureNamesNoProfile(t *testing.T) {
	t.Parallel()

	content := map[string]string{"Content-Profile": "warehouse"}
	minimal := map[string]string{"Content-Profile": "warehouse", "Prefer": "return=minimal"}
	for _, test := range []contentProfileCase{
		{name: "POST minimal", method: http.MethodPost, path: "/items", headers: minimal,
			body: `{"id":1}`, status: http.StatusCreated},
		{name: "PATCH minimal", method: http.MethodPatch, path: "/items?id=eq.1", headers: minimal,
			body: `{"id":2}`, status: http.StatusNoContent},
		{name: "DELETE minimal", method: http.MethodDelete, path: "/items?id=eq.1", headers: content,
			status: http.StatusNoContent},
		{name: "OPTIONS", method: http.MethodOptions, path: "/items",
			headers: map[string]string{"Accept-Profile": "warehouse"}, status: http.StatusOK},
		{name: "PGRST106", method: http.MethodGet, path: "/items",
			headers: map[string]string{"Accept-Profile": "tenant3"}, status: http.StatusNotAcceptable},
		{name: "missing table", method: http.MethodGet, path: "/nothing", status: http.StatusNotFound},
		{name: "singular object refusal", method: http.MethodGet, path: "/items",
			headers: map[string]string{"Accept": "application/vnd.pgrst.object+json"},
			status:  http.StatusNotAcceptable},
		{name: "scalar rpc refuses CSV", method: http.MethodGet, path: "/rpc/item_count",
			headers: map[string]string{"Accept": "text/csv"}, status: http.StatusUnsupportedMediaType},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, []string{"shop", "warehouse"}, &reader{})
		})
	}
}

// With one configured database the default database is not negotiated by
// profile, so it is not named; an explicit profile header still is, as in the
// parity target.
func TestOneDatabaseNamesOnlyAnExplicitProfile(t *testing.T) {
	t.Parallel()

	for _, test := range []contentProfileCase{
		{name: "GET default", method: http.MethodGet, path: "/items", status: http.StatusOK},
		{name: "OpenAPI default", method: http.MethodGet, path: "/", status: http.StatusOK},
		{name: "GET rpc default", method: http.MethodGet, path: "/rpc/item_count", status: http.StatusOK},
		{name: "POST representation default", method: http.MethodPost, path: "/items",
			headers: map[string]string{"Prefer": "return=representation"}, body: `{"id":1}`,
			status: http.StatusCreated},
		{name: "GET Accept-Profile", method: http.MethodGet, path: "/items",
			headers: map[string]string{"Accept-Profile": "shop"}, status: http.StatusOK, want: "shop"},
		{name: "POST Content-Profile", method: http.MethodPost, path: "/items",
			headers: map[string]string{"Content-Profile": "shop", "Prefer": "return=representation"},
			body:    `{"id":1}`, status: http.StatusCreated, want: "shop"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, []string{"shop"}, &reader{read: []rows.Row{}})
		})
	}
}
