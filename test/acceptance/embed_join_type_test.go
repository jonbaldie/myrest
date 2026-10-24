package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// embed-007: explicit join types refuse, not as missing relationships.
func TestEmbedJoinTypeRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")
	for _, path := range []string{
		"/items?select=id,orders!inner(id)&orders.id=eq.3",
		"/items?select=id,orders!left(id)",
		"/deliveries?select=id,addresses!deliveries_from!inner(label)",
		"/deliveries?select=id,billing:addresses!left!deliveries_from(label)",
		"/items?select=id,orders(id,items!inner(name))",
	} {
		t.Run(path, func(t *testing.T) {
			response, body := get(t, service, path)
			failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
			want := "PostgREST embed join types !inner and !left are not supported"
			if failure.Message != want {
				t.Fatalf("message = %q, want %q", failure.Message, want)
			}
		})
	}

	// The issue's control request still retains parents with no matching orders.
	response, body := get(t, service, "/items?select=id,orders(id)&orders.id=eq.3&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[]},{"id":2,"orders":[{"id":3}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}
