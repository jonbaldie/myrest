package acceptance_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/mysqldb"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

const jwtSecret = "reallyreallyreallyreallyverysafe"

const userRole = "myrest_user"

// serveWithJWT starts myrest with a JWT secret and the anonymous role.
func serveWithJWT(t *testing.T, databases ...string) *httpapi.Service {
	t.Helper()

	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = databases
	settings.DB.AnonRole = anonRole
	settings.JWT.Secret = jwtSecret
	settings.JWT.CacheMaxEntries = 1000

	pool, err := mysqldb.Open(settings.DB.URI)
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

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
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func bearerToken(t *testing.T, claims gojwt.MapClaims) string {
	t.Helper()

	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return signed
}

func getBearer(t *testing.T, service *httpapi.Service, path, token string) (*http.Response, []byte) {
	t.Helper()

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	return apitest.Do(t, http.MethodGet, service.URL()+path, headers)
}

// auth-001 and smoke-002: a valid Bearer JWT for a granted role reads under
// that role's grants.
func TestJWTReadOfAnExposedTable(t *testing.T) {
	service := serveWithJWT(t, "myrest_fixture")
	token := bearerToken(t, gojwt.MapClaims{"role": userRole})

	response, body := getBearer(t, service, "/secrets", token)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"payload":"top-secret"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// auth-002: no usable JWT reads as the anonymous database role.
func TestAnonymousReadStillWorksWithJWTConfigured(t *testing.T) {
	response, body := get(t, serveWithJWT(t, "myrest_fixture"), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
}

// auth-003: an invalid JWT gives PGRST301.
func TestInvalidJWTIsRefusedWithPGRST301(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer not.a.jwt")
	response, body := apitest.Do(
		t, http.MethodGet, serveWithJWT(t, "myrest_fixture").URL()+"/items", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST301")
}

// auth-003: an expired JWT gives PGRST303.
func TestExpiredJWTIsRefusedWithPGRST303(t *testing.T) {
	token := bearerToken(t, gojwt.MapClaims{
		"role": userRole,
		"exp":  time.Now().Add(-time.Hour).Unix(),
	})
	response, body := getBearer(t, serveWithJWT(t, "myrest_fixture"), "/items", token)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST303")
	if failure.Message != "JWT expired" {
		t.Fatalf("message = %q, want JWT expired", failure.Message)
	}
}

// A request with no usable JWT and no db-anon-role refuses with PGRST302.
func TestNoJWTAndNoAnonymousRoleIsRefusedWithPGRST302(t *testing.T) {
	settings := config.Defaults()
	settings.DB.URI = harness.URI("authenticator", "secret")
	settings.DB.Schemas = []string{"myrest_fixture"}
	settings.DB.AnonRole = ""
	settings.JWT.Secret = jwtSecret

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

	response, body := get(t, service, "/items")
	apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST302")
}

// auth-004: after role switch, grants follow the role while CURRENT_USER stays
// the authenticator.
func TestRoleSwitchKeepsAuthenticatorAsCurrentUser(t *testing.T) {
	pool, err := mysqldb.Open(harness.URI("authenticator", "secret"))
	if err != nil {
		t.Fatalf("open the authenticator pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	identity, err := pool.IdentityAfterRoleSwitch(t.Context(), userRole)
	if err != nil {
		t.Fatalf("IdentityAfterRoleSwitch: %v", err)
	}
	if !strings.HasPrefix(identity.CurrentUser, "authenticator@") {
		t.Fatalf("CURRENT_USER = %q, want authenticator@…", identity.CurrentUser)
	}
	if !strings.Contains(identity.CurrentRole, userRole) {
		t.Fatalf("CURRENT_ROLE = %q, want it to hold %s", identity.CurrentRole, userRole)
	}

	// Grants follow the switched role: secrets is readable as myrest_user.
	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "myrest_fixture", Name: "secrets"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "payload"}},
	}
	rows, err := pool.Read(t.Context(), userRole, table)
	if err != nil {
		t.Fatalf("read secrets as %s: %v", userRole, err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
}

// auth-005: a Prefer that asks for Postgres RLS is refused.
func TestRowSecurityPreferIsRefusedOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "row-security")
	response, body := apitest.Do(
		t, http.MethodGet, serveWithJWT(t, "myrest_fixture").URL()+"/items", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// auth-006: a Prefer that asks for request GUCs is refused.
func TestRequestGUCPreferIsRefusedOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "jwt-claims")
	response, body := apitest.Do(
		t, http.MethodGet, serveWithJWT(t, "myrest_fixture").URL()+"/items", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// auth-007: a non-Bearer credential scheme is refused.
func TestNonBearerCredentialsAreRefusedOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Authorization", "Basic dXNlcjpwYXNz")
	response, body := apitest.Do(
		t, http.MethodGet, serveWithJWT(t, "myrest_fixture").URL()+"/items", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
