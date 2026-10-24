package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// write-017: columns on POST and PATCH refuses without changing stored rows.
func TestWriteColumnsRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")
	response, body := apitest.PostJSON(t, service.URL()+"/colors", `{"name":"columns-265","tone":"plain"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("seed status = %d; body = %s", response.StatusCode, body)
	}
	const path = "/colors?name=eq.columns-265"
	const readBack = path + "&select=name,tone"
	const want = "[{\"name\":\"columns-265\",\"tone\":\"plain\"}]\n"
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		for _, payload := range []string{`{"name":"columns-265","tone":"changed"}`, `{"name":"columns-265","extra":1}`} {
			if method == http.MethodPost {
				payload = "[" + payload + "]"
			}
			for _, prefer := range []string{"return=minimal", "return=representation", "return=headers-only"} {
				t.Run(method+"/"+payload+"/"+prefer, func(t *testing.T) {
					headers := http.Header{"Content-Type": {"application/json"}, "Prefer": {prefer}}
					response, body := apitest.DoBody(t, method, service.URL()+path+"&columns=name", headers, payload)
					apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
					_, body = get(t, service, readBack)
					if string(body) != want {
						t.Fatalf("rows changed after refusal: %s, want %s", body, want)
					}
				})
			}
		}
	}
}
