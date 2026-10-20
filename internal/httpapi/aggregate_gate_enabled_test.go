package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
)

// This fix changes only the disabled gate. Keep the existing enabled behavior
// on RPC and writes; aggregate execution on those surfaces is a separate task.
func TestEnabledAggregateGateKeepsRPCAndWriteBehavior(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"GET", "POST"} {
		t.Run("RPC/"+method, func(t *testing.T) {
			source := &caller{body: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}}
			service := serveRPCWithSettings(t, source, aggregatesOn())
			response, body := apitest.DoBody(t, method, service.URL()+"/rpc/list_items?select=id.sum()", http.Header{"Content-Type": {"application/json"}}, "{}")
			if response.StatusCode != http.StatusOK || string(body) != "[{\"id\":1}]\n" || !source.committed {
				t.Fatalf("status=%d body=%s committed=%v", response.StatusCode, body, source.committed)
			}
		})
	}
	for _, method := range []string{"POST", "PATCH", "DELETE", "PUT"} {
		t.Run("write/"+method, func(t *testing.T) {
			sink := &writer{}
			service := serveWrite(t, &reader{}, sink, httpapi.Options{Settings: aggregatesOn()})
			response, body := apitest.DoBody(t, method, service.URL()+"/items?id=eq.1&select=id.sum()",
				http.Header{"Content-Type": {"application/json"}, "Prefer": {"return=minimal"}}, `{"id":1,"name":"new"}`)
			if response.StatusCode < 200 || response.StatusCode >= 300 || sink.called == "" {
				t.Fatalf("status=%d body=%s write=%q", response.StatusCode, body, sink.called)
			}
		})
	}
}
