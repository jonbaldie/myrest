package httpapi_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

func TestOrdinaryOrderWithoutNullOptionsStillSucceeds(t *testing.T) {
	t.Parallel()
	for _, option := range []string{"", ".asc", ".desc", ".ASC", ".DESC"} {
		t.Run(option, func(t *testing.T) {
			source := &reader{read: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}}
			response, body := get(t, serve(t, source, settings()), "/items?order=id"+option)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d; body = %s", response.StatusCode, body)
			}
			if len(source.query.Order) != 1 || source.query.Order[0].Column != "id" || source.query.Order[0].Desc != (option == ".desc" || option == ".DESC") {
				t.Fatalf("order = %#v", source.query.Order)
			}
		})
	}
}

// read-014: explicit null placement is not supported, including bare options.
func TestOrderNullsOptionsRefuse(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"id", "payload->>key"} {
		for _, option := range []string{"asc.nullsfirst", "asc.nullslast", "desc.nullsfirst", "desc.nullslast", "nullsfirst", "nullslast", "ASC.NULLSFIRST", "NULLSLAST"} {
			t.Run(field+"."+option, func(t *testing.T) {
				source := &reader{}
				response, body := get(t, serve(t, source, settings()), "/items?order="+url.QueryEscape(field+"."+option))
				failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
				if failure.Message != "Order nulls options are not available with MySQL" {
					t.Fatalf("message = %q", failure.Message)
				}
				if source.table.ID.Name != "" {
					t.Fatalf("reader ran for unsupported ordering: %#v", source.table.ID)
				}
			})
		}
	}
}
