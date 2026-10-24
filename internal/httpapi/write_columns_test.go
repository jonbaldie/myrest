package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// DELETE has no body column selection; keep its existing query behavior.
func TestDeleteColumnsDoesNotSelectBodyKeys(t *testing.T) {
	t.Parallel()
	sink := &writer{deleted: 1}
	service := serveWrite(t, &reader{}, sink)
	res, body := apitest.Do(t, http.MethodDelete, service.URL()+"/items?id=eq.1&columns=name", nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", res.StatusCode, body)
	}
	if sink.called != "delete" {
		t.Fatalf("writer called %q, want delete", sink.called)
	}
}

// write-017: columns must not silently permit writes of excluded body keys.
// Issue #265: until selection is supported, refuse before calling the writer.
func TestWriteColumnsRefusesBeforeWriter(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		for _, query := range []string{"columns=name", "columns=", "columns", "columns=name&columns=id"} {
			for _, payload := range []string{`{"name":"new","id":99}`, `{"name":"new","extra":1}`} {
				if method == http.MethodPost {
					payload = "[" + payload + "]"
				}
				for _, prefer := range []string{"", "return=representation", "return=headers-only"} {
					t.Run(method+"/"+query+"/"+payload+"/"+prefer, func(t *testing.T) {
						t.Parallel()
						sink := &writer{}
						service := serveWrite(t, &reader{}, sink)
						headers := http.Header{"Content-Type": {"application/json"}, "Prefer": {prefer}}
						res, body := apitest.DoBody(t, method, service.URL()+"/items?id=eq.1&"+query, headers, payload)
						failure := apitest.AssertEnvelope(t, res, body, http.StatusBadRequest, "MYREST001")
						if failure.Message != "PostgREST columns on POST and PATCH is not supported" {
							t.Fatalf("message = %q", failure.Message)
						}
						if sink.called != "" {
							t.Fatalf("writer must not run for columns; called %q", sink.called)
						}
					})
				}
			}
		}
	}
}
