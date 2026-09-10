// Double-quoted scalar filter values decode to their literal over MySQL.
// Issue #145.
package acceptance_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestQuotedScalarFilterValueOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	cases := []struct {
		name  string
		query string
		body  string
	}{
		{
			name:  "quoted value matches the same rows as the bare form",
			query: `/quoted_values?select=id,value&value=eq."alpha"&order=id.asc`,
			body:  `[{"id":1,"value":"alpha"}]`,
		},
		{
			name:  "quoted comma is value data",
			query: `/quoted_values?select=id,value&value=eq."a,b"&order=id.asc`,
			body:  `[{"id":2,"value":"a,b"}]`,
		},
		{
			name:  "doubled quote is an escaped quote",
			query: `/quoted_values?select=id,value&value=eq.%22say%20%22%22hi%22%22%22&order=id.asc`,
			body:  `[{"id":3,"value":"say \"hi\""}]`,
		},
		{
			name:  "neq with a quoted value",
			query: `/quoted_values?select=id,value&value=neq."a,b"&order=id.asc`,
			body:  `[{"id":1,"value":"alpha"},{"id":3,"value":"say \"hi\""}]`,
		},
		{
			name:  "like with a quoted pattern",
			query: `/quoted_values?select=id,value&value=like."a,*"&order=id.asc`,
			body:  `[{"id":2,"value":"a,b"}]`,
		},
		{
			name:  "quoted value in a logical group",
			query: `/quoted_values?select=id,value&or=(value.eq."a,b",value.eq.alpha)&order=id.asc`,
			body:  `[{"id":1,"value":"alpha"},{"id":2,"value":"a,b"}]`,
		},
		{
			name:  "quoted element in an in list stays a literal",
			query: `/quoted_values?select=id,value&value=in.("a,b",alpha)&order=id.asc`,
			body:  `[{"id":1,"value":"alpha"},{"id":2,"value":"a,b"}]`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response, body := get(t, service, test.query)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
			}
			if want := test.body + "\n"; string(body) != want {
				t.Fatalf("body = %s, want %s", body, want)
			}
		})
	}
}

func TestMalformedQuotedScalarFilterRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, `/quoted_values?value=eq."a,b`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusBadRequest, body)
	}
	if !strings.Contains(string(body), "PGRST100") {
		t.Fatalf("body = %s, want a PGRST100 envelope", body)
	}
}
