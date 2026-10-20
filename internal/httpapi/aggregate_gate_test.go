package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
)

// The HTTP response and executor call records show that a disabled aggregate
// is refused before any database work, even without a representation body.
func TestDisabledAggregatesRefuseBeforeExecution(t *testing.T) {
	t.Parallel()
	surfaces := []struct {
		method, path, body string
		rpc                bool
	}{
		{"GET", "/rpc/list_items?", "", true},
		{"POST", "/rpc/list_items?", "{}", true},
		{"POST", "/rpc/write_marker?", "{}", true},
		{"POST", "/items?", `{"id":1,"name":"new"}`, false},
		{"PATCH", "/items?id=eq.1&", `{"name":"new"}`, false},
		{"DELETE", "/items?id=eq.1&", "", false},
		{"PUT", "/items?id=eq.1&", `{"id":1,"name":"new"}`, false},
	}
	for _, surface := range surfaces {
		for _, selectList := range []string{"count()", "id.sum()", "orders(count())"} {
			for _, mode := range []string{"minimal", "representation"} {
				t.Run(surface.method+surface.path+selectList+"/"+mode, func(t *testing.T) {
					sink := &writer{}
					source := &caller{body: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}}
					var service *httpapi.Service
					if surface.rpc {
						service = serveRPC(t, source)
					} else {
						service = serveWrite(t, &reader{}, sink)
					}
					response, body := apitest.DoBody(t, surface.method,
						service.URL()+surface.path+"select="+selectList,
						http.Header{"Content-Type": {"application/json"}, "Prefer": {"return=" + mode}}, surface.body)
					failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST123")
					if failure.Message != "Use of aggregate functions is not allowed" {
						t.Errorf("message = %q", failure.Message)
					}
					if source.role != "" || sink.called != "" {
						t.Fatalf("database work ran: routine role=%q write=%q", source.role, sink.called)
					}
				})
			}
		}
	}
}
