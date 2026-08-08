package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for OPTIONS and OpenAPI discovery.
// Inputs come only from the schema cache and the privileges of the active
// database role. See docs/discovery.md for parity labels on the document body.

func discoveryCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	secrets := schemacache.TableID{Database: "shop", Name: "secrets"}
	count := schemacache.RoutineID{Database: "shop", Name: "item_count"}
	writeMarker := schemacache.RoutineID{Database: "shop", Name: "write_marker"}

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
			{Role: "myrest_user", Table: items, Privilege: "SELECT"},
			{Role: "myrest_user", Table: secrets, Privilege: "SELECT"},
			{Role: "myrest_user", Table: secrets, Privilege: "UPDATE"},
			{Role: "myrest_user", Table: secrets, Privilege: "DELETE"},
		},
		Routines: []schemacache.RoutineFact{
			{ID: count, Kind: "FUNCTION", SQLDataAccess: "READS SQL DATA", ReturnType: "int"},
			{ID: writeMarker, Kind: "PROCEDURE", SQLDataAccess: "MODIFIES SQL DATA"},
		},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: "myrest_anon", Routine: count, Privilege: "EXECUTE"},
			{Role: "myrest_user", Routine: count, Privilege: "EXECUTE"},
			{Role: "myrest_user", Routine: writeMarker, Privilege: "EXECUTE"},
		},
	})
}

func serveDiscovery(t *testing.T, resolved config.Settings, caller ...httpapi.Caller) *httpapi.Service {
	t.Helper()

	options := httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    discoveryCache(),
		Reader:   &reader{},
	}
	if len(caller) == 1 {
		options.Caller = caller[0]
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

// OPTIONS on a table reports only methods the active role can use.
func TestOptionsReportsMethodsFromPrivileges(t *testing.T) {
	t.Parallel()

	response, body := apitest.Do(
		t, http.MethodOptions, serveDiscovery(t, settings()).URL()+"/items", nil,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want empty", body)
	}
	if got := response.Header.Get("Allow"); got != "OPTIONS,GET,HEAD,POST,PUT" {
		t.Fatalf("Allow = %q, want OPTIONS,GET,HEAD,POST,PUT", got)
	}
}

// OPTIONS omits methods the active role cannot use.
func TestOptionsOmitsMethodsWithoutPrivilege(t *testing.T) {
	t.Parallel()

	headers := bearer(t, gojwt.MapClaims{"role": "myrest_user"})
	response, body := apitest.Do(
		t, http.MethodOptions, serveDiscovery(t, jwtSettings()).URL()+"/secrets", headers,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Allow"); got != "OPTIONS,GET,HEAD,PATCH,DELETE" {
		t.Fatalf("Allow = %q, want OPTIONS,GET,HEAD,PATCH,DELETE", got)
	}
}

// OPTIONS on a table the role cannot use is not a resource.
func TestOptionsOnHiddenTableGivesPGRST205(t *testing.T) {
	t.Parallel()

	response, body := apitest.Do(
		t, http.MethodOptions, serveDiscovery(t, settings()).URL()+"/secrets", nil,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// OPTIONS on a view name is not a table resource, even when a privilege fact
// names that view.
func TestOptionsOnViewIsNotATableResource(t *testing.T) {
	t.Parallel()

	itemsView := schemacache.TableID{Database: "shop", Name: "items_view"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{{Database: "shop", Name: "items"}},
		Views:  []schemacache.TableID{itemsView},
		Columns: []schemacache.ColumnFact{
			{Table: schemacache.TableID{Database: "shop", Name: "items"}, Name: "id"},
			{Table: itemsView, Name: "id"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: schemacache.TableID{Database: "shop", Name: "items"}},
			{Role: "myrest_anon", Table: itemsView},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: schemacache.TableID{Database: "shop", Name: "items"}, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: itemsView, Privilege: "SELECT"},
		},
	})
	resolved := settings()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    cache,
		Reader:   &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := apitest.Do(t, http.MethodOptions, service.URL()+"/items_view", nil)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// OPTIONS on a routine reports methods from EXECUTE and read-safety.
func TestOptionsOnRoutineReportsMethods(t *testing.T) {
	t.Parallel()

	response, body := apitest.Do(
		t, http.MethodOptions, serveDiscovery(t, settings()).URL()+"/rpc/item_count", nil,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Allow"); got != "OPTIONS,POST,GET,HEAD" {
		t.Fatalf("Allow = %q, want OPTIONS,POST,GET,HEAD", got)
	}

	headers := bearer(t, gojwt.MapClaims{"role": "myrest_user"})
	response, body = apitest.Do(
		t, http.MethodOptions, serveDiscovery(t, jwtSettings()).URL()+"/rpc/write_marker", headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Allow"); got != "OPTIONS,POST" {
		t.Fatalf("Allow = %q, want OPTIONS,POST", got)
	}
}

// The OpenAPI document lists only resources the active role can use.
func TestOpenAPIListsOnlyPrivilegedResources(t *testing.T) {
	t.Parallel()

	response, body := get(t, serveDiscovery(t, settings()), "/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/openapi+json" {
		t.Fatalf("Content-Type = %q, want application/openapi+json", contentType)
	}

	doc := decodeOpenAPI(t, body)
	if doc["swagger"] != "2.0" {
		t.Fatalf("swagger = %v, want 2.0", doc["swagger"])
	}
	paths, _ := doc["paths"].(map[string]any)
	if _, held := paths["/items"]; !held {
		t.Fatalf("paths = %v, want /items", paths)
	}
	if _, held := paths["/rpc/item_count"]; !held {
		t.Fatalf("paths = %v, want /rpc/item_count", paths)
	}
	if _, held := paths["/secrets"]; held {
		t.Fatalf("paths held /secrets for anonymous role")
	}
	if _, held := paths["/rpc/write_marker"]; held {
		t.Fatalf("paths held /rpc/write_marker for anonymous role")
	}

	item, _ := paths["/items"].(map[string]any)
	for _, method := range []string{"get", "post", "put"} {
		if _, held := item[method]; !held {
			t.Fatalf("/items missing %s", method)
		}
	}
	for _, method := range []string{"patch", "delete"} {
		if _, held := item[method]; held {
			t.Fatalf("/items must not advertise %s without the grant", method)
		}
	}
}

func decodeOpenAPI(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode OpenAPI: %v; body = %s", err, body)
	}
	return doc
}

// openapi-mode=disabled refuses the root OpenAPI document.
func TestOpenAPIModeDisabledRefusesRoot(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.OpenAPI.Mode = config.OpenAPIModeDisabled
	response, body := get(t, serveDiscovery(t, resolved), "/")
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "MYREST003")
}

// openapi-mode=ignore-privileges lists resources outside the active role.
func TestOpenAPIModeIgnorePrivilegesListsAllResources(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.OpenAPI.Mode = config.OpenAPIModeIgnorePrivileges
	_, body := get(t, serveDiscovery(t, resolved), "/")
	doc := decodeOpenAPI(t, body)
	paths, _ := doc["paths"].(map[string]any)
	for _, path := range []string{"/items", "/secrets", "/rpc/item_count", "/rpc/write_marker"} {
		if _, held := paths[path]; !held {
			t.Fatalf("paths = %v, want %s under ignore-privileges", paths, path)
		}
	}
	secrets, _ := paths["/secrets"].(map[string]any)
	for _, method := range []string{"get", "post", "put", "patch", "delete"} {
		if _, held := secrets[method]; !held {
			t.Fatalf("/secrets missing %s under ignore-privileges", method)
		}
	}
}

// openapi-security-active adds JWT security to the document.
func TestOpenAPISecurityActiveAddsDefinitions(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.OpenAPI.SecurityActive = true
	_, body := get(t, serveDiscovery(t, resolved), "/")
	doc := decodeOpenAPI(t, body)
	if _, held := doc["securityDefinitions"]; !held {
		t.Fatalf("document holds no securityDefinitions: %v", doc)
	}
	security, _ := doc["security"].([]any)
	if len(security) == 0 {
		t.Fatalf("document holds no security: %v", doc)
	}

	_, body = get(t, serveDiscovery(t, settings()), "/")
	without := decodeOpenAPI(t, body)
	if _, held := without["securityDefinitions"]; held {
		t.Fatalf("default document must omit securityDefinitions")
	}
}

// openapi-server-proxy-uri sets host, schemes, and basePath on the document.
func TestOpenAPIServerProxyURISetsBase(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.OpenAPI.ServerProxyURI = "https://api.example/v1/"
	_, body := get(t, serveDiscovery(t, resolved), "/")
	doc := decodeOpenAPI(t, body)
	if doc["host"] != "api.example" {
		t.Fatalf("host = %v, want api.example", doc["host"])
	}
	if doc["basePath"] != "/v1" && doc["basePath"] != "/v1/" {
		// reportedBaseURL trims trailing slash from the whole URI, so path is /v1
		if got := doc["basePath"]; got != "/v1" {
			t.Fatalf("basePath = %v, want /v1", got)
		}
	}
	schemes, _ := doc["schemes"].([]any)
	if len(schemes) != 1 || schemes[0] != "https" {
		t.Fatalf("schemes = %v, want [https]", schemes)
	}
}

type rootSpecCaller struct {
	role    schemacache.Role
	routine schemacache.RoutineFact
	result  any
	err     error
}

func (c *rootSpecCaller) Call(
	_ context.Context,
	role schemacache.Role,
	routine schemacache.RoutineFact,
	_ map[string]any,
) (any, error) {
	c.role, c.routine = role, routine
	return c.result, c.err
}

// db-root-spec replaces the OpenAPI body with the routine result.
func TestRootSpecReplacesOpenAPIBody(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.DB.RootSpec = "shop.item_count"
	caller := &rootSpecCaller{result: map[string]any{
		"swagger": "2.0",
		"info":    map[string]any{"title": "Overridden"},
	}}
	response, body := get(t, serveDiscovery(t, resolved, caller), "/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if caller.role != "myrest_anon" {
		t.Fatalf("root-spec role = %q, want myrest_anon", caller.role)
	}
	if caller.routine.ID.Name != "item_count" {
		t.Fatalf("root-spec routine = %q, want item_count", caller.routine.ID.Name)
	}
	doc := decodeOpenAPI(t, body)
	info, _ := doc["info"].(map[string]any)
	if info["title"] != "Overridden" {
		t.Fatalf("info.title = %v, want Overridden", info["title"])
	}
}
