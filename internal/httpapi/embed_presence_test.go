package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

func TestEmbedPresenceFilterRefusesAtHTTP(t *testing.T) {
	response, body := get(t, serveEmbed(t, &reader{}), "/items?select=id,orders(id)&orders.id=eq.3&orders=not.is.null")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
