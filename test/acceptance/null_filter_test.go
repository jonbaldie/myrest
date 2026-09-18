package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// Issue #180: SQL queries and in-memory row-set filters give identical results
// for eq.null, eq."null", in.(null), is.null, and is.not_null.
func TestNullFilterParityTableReadAndRPCRowSet(t *testing.T) {
	service := serve(t, "myrest_fixture")

	cases := []struct {
		name        string
		filterQuery string
		wantBody    string
	}{
		{
			name:        "eq.null matches literal string null and excludes NULL",
			filterQuery: "label=eq.null",
			wantBody:    `[{"id":1,"label":"null"}]` + "\n",
		},
		{
			name:        "eq quoted null matches literal string null and excludes NULL",
			filterQuery: `label=eq."null"`,
			wantBody:    `[{"id":1,"label":"null"}]` + "\n",
		},
		{
			name:        "in.(null) matches literal string null and excludes NULL",
			filterQuery: "label=in.(null)",
			wantBody:    `[{"id":1,"label":"null"}]` + "\n",
		},
		{
			name:        "in quoted null matches literal string null and excludes NULL",
			filterQuery: `label=in.("null")`,
			wantBody:    `[{"id":1,"label":"null"}]` + "\n",
		},
		{
			name:        "in with multiple values including null matches literal string null",
			filterQuery: "label=in.(null,other)",
			wantBody:    `[{"id":1,"label":"null"},{"id":3,"label":"other"}]` + "\n",
		},
		{
			name:        "is.null matches SQL NULL and excludes literal string null",
			filterQuery: "label=is.null",
			wantBody:    `[{"id":2,"label":null}]` + "\n",
		},
		{
			name:        "is.not_null matches non-NULL values and excludes SQL NULL",
			filterQuery: "label=is.not_null",
			wantBody:    `[{"id":1,"label":"null"},{"id":3,"label":"other"}]` + "\n",
		},
	}

	for _, tc := range cases {
		t.Run("table read "+tc.name, func(t *testing.T) {
			resp, body := get(t, service, "/null_records?select=id,label&order=id.asc&"+tc.filterQuery)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("table read status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, body)
			}
			if string(body) != tc.wantBody {
				t.Fatalf("table read body = %s, want %s", body, tc.wantBody)
			}
		})

		t.Run("rpc row-set "+tc.name, func(t *testing.T) {
			resp, body := apitest.PostJSON(
				t,
				service.URL()+"/rpc/list_null_records?select=id,label&order=id.asc&"+tc.filterQuery,
				`{}`,
			)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("rpc row-set status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, body)
			}
			if string(body) != tc.wantBody {
				t.Fatalf("rpc row-set body = %s, want %s", body, tc.wantBody)
			}
		})
	}
}
