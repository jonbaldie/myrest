package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
)

// Seam under test: the HTTP API boundary for CORS origins and preflight.
// Expected header values come from the PostgREST v14.16 wire contract.

func withCORSOrigins(origins ...string) config.Settings {
	resolved := settings()
	resolved.Server.CORSAllowedOrigins = origins
	return resolved
}

// An allowed origin on a GET receives the documented CORS response headers.
func TestAllowedOriginGetsCORSResponseHeaders(t *testing.T) {
	t.Parallel()

	origin := "http://example.com"
	response, _ := apitest.Do(
		t,
		http.MethodGet,
		serve(t, &reader{}, withCORSOrigins(origin, "http://example2.com")).URL()+"/items",
		http.Header{"Origin": {origin}},
	)

	if got := response.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
	if got := response.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	wantExpose := "Content-Encoding, Content-Location, Content-Range, Content-Type, Date, Location, Server, Transfer-Encoding, Range-Unit"
	if got := response.Header.Get("Access-Control-Expose-Headers"); got != wantExpose {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %q", got, wantExpose)
	}
}

// An origin outside server-cors-allowed-origins gets no Allow-Origin header.
// The request still receives its ordinary answer.
func TestDisallowedOriginOmitsCORSAllowOrigin(t *testing.T) {
	t.Parallel()

	response, body := apitest.Do(
		t,
		http.MethodGet,
		serve(t, &reader{}, withCORSOrigins("http://example.com")).URL()+"/items",
		http.Header{"Origin": {"http://invalid.com"}},
	)

	if _, held := response.Header["Access-Control-Allow-Origin"]; held {
		t.Fatalf("Access-Control-Allow-Origin = %q, want it omitted", response.Header.Get("Access-Control-Allow-Origin"))
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
}

// A preflight OPTIONS request from an allowed origin gets the documented
// CORS preflight headers and an empty body.
func TestAllowedOriginPreflightGetsCORSHeaders(t *testing.T) {
	t.Parallel()

	origin := "http://example.com"
	response, body := apitest.Do(
		t,
		http.MethodOptions,
		serve(t, &reader{}, withCORSOrigins(origin, "http://example2.com")).URL()+"/items",
		http.Header{
			"Origin":                         {origin},
			"Access-Control-Request-Method":  {"POST"},
			"Access-Control-Request-Headers": {"Content-Type"},
		},
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if len(body) != 0 {
		t.Fatalf("preflight body = %q, want empty", body)
	}
	if got := response.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
	if got := response.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
	if got := response.Header.Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD" {
		t.Fatalf("Access-Control-Allow-Methods = %q", got)
	}
	wantHeaders := "Authorization, Content-Type, Accept, Accept-Language, Content-Language"
	if got := response.Header.Get("Access-Control-Allow-Headers"); got != wantHeaders {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %q", got, wantHeaders)
	}
	if got := response.Header.Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("Access-Control-Max-Age = %q, want 86400", got)
	}
}

// An empty server-cors-allowed-origins list accepts every origin as "*".
func TestEmptyCORSOriginsAllowEveryOrigin(t *testing.T) {
	t.Parallel()

	response, _ := apitest.Do(
		t,
		http.MethodOptions,
		serve(t, &reader{}, settings()).URL()+"/items",
		http.Header{
			"Origin":                         {"http://anyorigin.com"},
			"Access-Control-Request-Method":  {"POST"},
			"Access-Control-Request-Headers": {"Content-Type"},
		},
	)

	if got := response.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if _, held := response.Header["Access-Control-Allow-Credentials"]; held {
		t.Fatalf("Access-Control-Allow-Credentials must stay unset with *")
	}
	if got := response.Header.Get("Access-Control-Allow-Methods"); !containsToken(got, "POST") {
		t.Fatalf("Access-Control-Allow-Methods = %q, want it to hold POST", got)
	}
}

// A preflight from a refused origin gets no Allow-Origin header.
func TestDisallowedOriginPreflightOmitsCORSAllowOrigin(t *testing.T) {
	t.Parallel()

	response, _ := apitest.Do(
		t,
		http.MethodOptions,
		serve(t, &reader{}, withCORSOrigins("http://example.com")).URL()+"/items",
		http.Header{
			"Origin":                        {"http://invalid.com"},
			"Access-Control-Request-Method": {"POST"},
		},
	)

	if _, held := response.Header["Access-Control-Allow-Origin"]; held {
		t.Fatalf("Access-Control-Allow-Origin = %q, want it omitted", response.Header.Get("Access-Control-Allow-Origin"))
	}
}

// A request with no Origin header gets no CORS headers.
func TestRequestWithoutOriginGetsNoCORSHeaders(t *testing.T) {
	t.Parallel()

	response, _ := get(t, serve(t, &reader{}, withCORSOrigins("http://example.com")), "/items")

	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Expose-Headers",
	} {
		if _, held := response.Header[name]; held {
			t.Fatalf("%s = %q, want it omitted", name, response.Header.Get(name))
		}
	}
}

func containsToken(list, token string) bool {
	for _, part := range strings.Split(list, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
