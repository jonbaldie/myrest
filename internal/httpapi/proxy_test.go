package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// Seam under test: the HTTP API boundary. Proxy request headers must not
// change ordinary responses. Absolute URL selection is covered next to
// reportedBaseURL; OpenAPI emission of that URL is in discovery tests.

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
