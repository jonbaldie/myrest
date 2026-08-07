package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/rows"
)

const jwtSecret = "reallyreallyreallyreallyverysafe"

func jwtSettings() config.Settings {
	resolved := settings()
	resolved.JWT.Secret = jwtSecret
	return resolved
}

func bearer(t *testing.T, claims gojwt.MapClaims) http.Header {
	t.Helper()

	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+signed)
	return header
}

// auth-001 and smoke-002: a valid Bearer JWT reads under the role of the claim.
func TestValidBearerJWTReadsAsTheClaimRole(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"payload"}, Values: []any{"top-secret"}},
	}}
	headers := bearer(t, gojwt.MapClaims{"role": "myrest_user"})
	response, body := apitest.Do(t, http.MethodGet, serve(t, source, jwtSettings()).URL()+"/secrets", headers)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if source.role != "myrest_user" {
		t.Fatalf("read as role %q, want myrest_user", source.role)
	}
	if want := `[{"payload":"top-secret"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// auth-002: no usable JWT reads as the anonymous database role.
func TestMissingJWTReadsAsTheAnonymousRole(t *testing.T) {
	t.Parallel()

	source := &reader{}
	get(t, serve(t, source, jwtSettings()), "/items")

	if source.role != "myrest_anon" {
		t.Fatalf("read as role %q, want myrest_anon", source.role)
	}
}

// auth-003: an invalid JWT gives PGRST301.
func TestInvalidJWTGivesPGRST301(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Authorization", "Bearer not-a-jwt")
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/items", headers)

	apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST301")
}

// auth-003: an expired JWT gives PGRST303.
func TestExpiredJWTGivesPGRST303(t *testing.T) {
	t.Parallel()

	headers := bearer(t, gojwt.MapClaims{
		"role": "myrest_user",
		"exp":  time.Now().Add(-time.Hour).Unix(),
	})
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/items", headers)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusUnauthorized, "PGRST303")
	if failure.Message != "JWT expired" {
		t.Fatalf("message = %q, want JWT expired", failure.Message)
	}
}

// auth-007: a non-Bearer credential scheme is refused.
func TestNonBearerCredentialsAreRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Authorization", "Basic dXNlcjpwYXNz")
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/items", headers)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if failure.Message == "" {
		t.Fatal("empty refusal message")
	}
}

// auth-005: a Prefer that asks for Postgres RLS is refused. myrest offers no
// fake row policy layer.
func TestRowSecurityPreferIsRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Prefer", "row-security")
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/items", headers)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Postgres row-level security is not available with MySQL"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// auth-006: a Prefer that asks for request GUCs / jwt claims injection is
// refused.
func TestRequestGUCPreferIsRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Prefer", "jwt-claims")
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/items", headers)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Request GUCs and request.jwt.claims are not available with MySQL"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// lowercase bearer still works, as the parity target documents.
func TestLowercaseBearerIsAccepted(t *testing.T) {
	t.Parallel()

	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"role": "myrest_user"})
	signed, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	headers := make(http.Header)
	headers.Set("Authorization", "bearer "+signed)
	source := &reader{}
	response, body := apitest.Do(t, http.MethodGet, serve(t, source, jwtSettings()).URL()+"/items", headers)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if source.role != "myrest_user" {
		t.Fatalf("read as role %q, want myrest_user", source.role)
	}
}

// A JWT role without SELECT on a table does not see it as a resource.
func TestJWTRoleWithoutSelectDoesNotSeeTheTable(t *testing.T) {
	t.Parallel()

	headers := bearer(t, gojwt.MapClaims{"role": "myrest_anon"})
	response, body := apitest.Do(t, http.MethodGet, serve(t, &reader{}, jwtSettings()).URL()+"/secrets", headers)

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}
