package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

func TestEmbedJoinTypeRefusesAtHTTP(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"/items?select=id,orders!inner(id)&orders.id=eq.3",
		"/items?select=id,orders!left(id)",
		"/items?select=id,orders!orders_item!inner(id)",
		"/items?select=orders!inner(id)",
	} {
		t.Run(path, func(t *testing.T) {
			response, body := get(t, serveEmbed(t, &reader{}), path)
			failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
			if !strings.Contains(failure.Message, "join types") {
				t.Fatalf("message = %q, want join type refusal", failure.Message)
			}
		})
	}
}
