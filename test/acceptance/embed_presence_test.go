package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// embed-008: embed presence filters refuse before column lookup.
func TestEmbedPresenceFilterRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")
	for _, path := range []string{
		"/items?select=id,orders(id)&orders.id=eq.3&orders=not.is.null",
		"/items?select=id,orders(id)&orders=not.is.null",
		"/items?select=id,orders(id)&orders=is.null",
		"/items?select=id,purchases:orders(id)&purchases=not.is.null",
		"/items?select=id,orders(id,items(id))&orders.items=is.null",
		"/items?select=id,orders(id)&or=(id.eq.1,orders.not.is.null)",
	} {
		t.Run(path, func(t *testing.T) {
			response, body := get(t, service, path)
			failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
			if want := "PostgREST embed presence filters are not supported"; failure.Message != want {
				t.Fatalf("message = %q, want %q", failure.Message, want)
			}
		})
	}

	response, body := get(t, service, "/items?select=id,orders(id)&orders.id=eq.3&id=not.is.null&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if want := "[{\"id\":1,\"orders\":[]},{\"id\":2,\"orders\":[{\"id\":3}]}]\n"; string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	response, body = get(t, service, "/items?select=id,orders(id)&missing=is.null")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST204")
}
