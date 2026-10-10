package httpapi_test

import (
	"bytes"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for GET / with Accept-Profile
// (issue #234). The OpenAPI document describes only the request database.

// profileDiscoveryCache holds items in both databases with different grants,
// plus one table and one routine that only one database holds.
func profileDiscoveryCache() *schemacache.Cache {
	shopItems := schemacache.TableID{Database: "shop", Name: "items"}
	shopOrders := schemacache.TableID{Database: "shop", Name: "orders"}
	warehouseItems := schemacache.TableID{Database: "warehouse", Name: "items"}
	warehouseOutside := schemacache.TableID{Database: "warehouse", Name: "outside_items"}
	shopCount := schemacache.RoutineID{Database: "shop", Name: "shop_count"}
	stockCount := schemacache.RoutineID{Database: "warehouse", Name: "stock_count"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{shopItems, shopOrders, warehouseItems, warehouseOutside},
		Columns: []schemacache.ColumnFact{
			{Table: shopItems, Name: "id"},
			{Table: shopOrders, Name: "id"},
			{Table: warehouseItems, Name: "id"},
			{Table: warehouseOutside, Name: "id"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: shopItems},
			{Role: "myrest_anon", Table: shopOrders},
			{Role: "myrest_anon", Table: warehouseItems},
			{Role: "myrest_anon", Table: warehouseOutside},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: shopItems, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: shopItems, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: shopItems, Privilege: "UPDATE"},
			{Role: "myrest_anon", Table: shopItems, Privilege: "DELETE"},
			{Role: "myrest_anon", Table: shopOrders, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: warehouseItems, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: warehouseOutside, Privilege: "SELECT"},
		},
		Routines: []schemacache.RoutineFact{
			{ID: shopCount, Kind: "FUNCTION", ReturnType: "bigint", SQLDataAccess: "NO SQL"},
			{ID: stockCount, Kind: "FUNCTION", ReturnType: "bigint", SQLDataAccess: "NO SQL"},
		},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: "myrest_anon", Routine: shopCount, Privilege: "EXECUTE"},
			{Role: "myrest_anon", Routine: stockCount, Privilege: "EXECUTE"},
		},
	})
}

func serveProfileDiscovery(t *testing.T, mode config.OpenAPIMode) *httpapi.Service {
	t.Helper()

	resolved := multiSchemaSettings()
	resolved.OpenAPI.Mode = mode
	return serveDiscoveryCache(t, resolved, profileDiscoveryCache())
}

func getRootWithProfile(t *testing.T, service *httpapi.Service, profile string) (*http.Response, []byte) {
	t.Helper()

	headers := http.Header{}
	if profile != "" {
		headers.Set("Accept-Profile", profile)
	}
	return apitest.Do(t, http.MethodGet, service.URL()+"/", headers)
}

func openAPIPathsFor(t *testing.T, service *httpapi.Service, profile string) map[string]any {
	t.Helper()

	response, body := getRootWithProfile(t, service, profile)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET / profile %q status = %d, want %d; body = %s",
			profile, response.StatusCode, http.StatusOK, body)
	}
	paths, _ := decodeOpenAPI(t, body)["paths"].(map[string]any)
	return paths
}

func pathKeys(paths map[string]any) []string {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// pathMethods lists the OpenAPI verbs of one path item in Allow order.
func pathMethods(t *testing.T, paths map[string]any, path string) string {
	t.Helper()

	item, held := paths[path].(map[string]any)
	if !held {
		t.Fatalf("paths = %v, want %s", pathKeys(paths), path)
	}
	var methods []string
	for _, method := range []string{"get", "post", "put", "patch", "delete"} {
		if _, held := item[method]; held {
			methods = append(methods, method)
		}
	}
	return strings.Join(methods, ",")
}

// issue #234: GET / lists only resources of the request database. With no
// Accept-Profile that is the default database; Accept-Profile selects another.
func TestOpenAPIDescribesOnlyTheRequestDatabase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mode    config.OpenAPIMode
		profile string
		want    []string
	}{
		{
			name: "follow-privileges default",
			mode: config.OpenAPIModeFollowPrivileges,
			want: []string{"/", "/items", "/orders", "/rpc/shop_count"},
		},
		{
			name:    "follow-privileges warehouse",
			mode:    config.OpenAPIModeFollowPrivileges,
			profile: "warehouse",
			want:    []string{"/", "/items", "/outside_items", "/rpc/stock_count"},
		},
		{
			name: "ignore-privileges default",
			mode: config.OpenAPIModeIgnorePrivileges,
			want: []string{"/", "/items", "/orders", "/rpc/shop_count"},
		},
		{
			name:    "ignore-privileges warehouse",
			mode:    config.OpenAPIModeIgnorePrivileges,
			profile: "warehouse",
			want:    []string{"/", "/items", "/outside_items", "/rpc/stock_count"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			paths := openAPIPathsFor(t, serveProfileDiscovery(t, tc.mode), tc.profile)
			if got := strings.Join(pathKeys(paths), " "); got != strings.Join(tc.want, " ") {
				t.Fatalf("paths = %s, want %s", got, strings.Join(tc.want, " "))
			}
		})
	}
}

// issue #234: a table name held by two databases gets, in each document, the
// methods OPTIONS reports for that database and role.
func TestOpenAPITableMethodsMatchOptionsPerProfile(t *testing.T) {
	t.Parallel()

	service := serveProfileDiscovery(t, config.OpenAPIModeFollowPrivileges)
	cases := []struct {
		profile string
		allow   string
		methods string
	}{
		{profile: "", allow: "OPTIONS,GET,HEAD,POST,PUT,PATCH,DELETE", methods: "get,post,put,patch,delete"},
		{profile: "shop", allow: "OPTIONS,GET,HEAD,POST,PUT,PATCH,DELETE", methods: "get,post,put,patch,delete"},
		{profile: "warehouse", allow: "OPTIONS,GET,HEAD", methods: "get"},
	}
	for _, tc := range cases {
		headers := http.Header{}
		if tc.profile != "" {
			headers.Set("Accept-Profile", tc.profile)
		}
		response, body := apitest.Do(t, http.MethodOptions, service.URL()+"/items", headers)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("OPTIONS profile %q status = %d; body = %s", tc.profile, response.StatusCode, body)
		}
		if got := response.Header.Get("Allow"); got != tc.allow {
			t.Fatalf("OPTIONS profile %q Allow = %q, want %q", tc.profile, got, tc.allow)
		}
		if got := pathMethods(t, openAPIPathsFor(t, service, tc.profile), "/items"); got != tc.methods {
			t.Fatalf("GET / profile %q /items methods = %q, want %q", tc.profile, got, tc.methods)
		}
	}
}

// issue #234: the same schema cache, role, and profile always give the same
// document.
func TestOpenAPIDocumentIsDeterministic(t *testing.T) {
	t.Parallel()

	for _, mode := range []config.OpenAPIMode{config.OpenAPIModeFollowPrivileges, config.OpenAPIModeIgnorePrivileges} {
		service := serveProfileDiscovery(t, mode)
		for _, profile := range []string{"", "warehouse"} {
			_, first := getRootWithProfile(t, service, profile)
			for range 20 {
				_, body := getRootWithProfile(t, service, profile)
				if !bytes.Equal(body, first) {
					t.Fatalf("mode %q profile %q: document changed between requests:\n%s\n%s",
						mode, profile, first, body)
				}
			}
		}
	}
}

// issue #234: an Accept-Profile outside db-schemas refuses GET / in the
// PostgREST shape.
func TestOpenAPIProfileOutsideDbSchemasRefuses(t *testing.T) {
	t.Parallel()

	response, body := getRootWithProfile(t, serveProfileDiscovery(t, config.OpenAPIModeFollowPrivileges), "tenant3")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST106")
	if want := "The schema must be one of the following: shop, warehouse"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}
