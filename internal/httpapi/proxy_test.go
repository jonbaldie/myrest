package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
)

// Seam under test: how myrest chooses the absolute URLs it reports. The
// function takes no request headers, so X-Forwarded-* cannot change the
// answer. The HTTP check below proves those headers also do not leak into
// the current response surface.

func TestReportedBaseURLUsesOpenAPIServerProxyURI(t *testing.T) {
	t.Parallel()

	resolved := settings()
	resolved.OpenAPI.ServerProxyURI = "https://api.example/v1/"

	got := httpapi.ReportedBaseURL(resolved, "http://127.0.0.1:3000")
	if got != "https://api.example/v1" {
		t.Fatalf("ReportedBaseURL = %q, want https://api.example/v1", got)
	}
}

func TestReportedBaseURLFallsBackToListenURL(t *testing.T) {
	t.Parallel()

	got := httpapi.ReportedBaseURL(settings(), "http://127.0.0.1:3000/")
	if got != "http://127.0.0.1:3000" {
		t.Fatalf("ReportedBaseURL = %q, want http://127.0.0.1:3000", got)
	}
}

// Proxy request headers must not change the ordinary response of a process
// that has no openapi-server-proxy-uri. The parity target also ignores them.
func TestForwardedHeadersDoNotChangeOrdinaryResponses(t *testing.T) {
	t.Parallel()

	service := serve(t, &reader{}, settings())
	response, body := apitest.Do(
		t,
		http.MethodGet,
		service.URL()+"/",
		http.Header{
			"X-Forwarded-Host":  {"public.example"},
			"X-Forwarded-Proto": {"https"},
			"Forwarded":         {"host=public.example;proto=https"},
		},
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := httpapi.ReportedBaseURL(config.Defaults(), service.URL()); want != service.URL() {
		t.Fatalf("ReportedBaseURL = %q, want the listen URL %q", want, service.URL())
	}
	for _, leak := range []string{"public.example", "https://"} {
		if strings.Contains(string(body), leak) {
			t.Fatalf("body %s holds proxy header data %q", body, leak)
		}
		for name, values := range response.Header {
			for _, value := range values {
				if strings.Contains(value, leak) {
					t.Fatalf("header %s: %q holds proxy header data %q", name, value, leak)
				}
			}
		}
	}
}
