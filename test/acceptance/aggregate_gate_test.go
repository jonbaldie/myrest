package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

func TestDisabledAggregatesDoNotChangeDataOverMySQL(t *testing.T) {
	if err := harness.Exec("INSERT INTO myrest_fixture.colors (id, name) VALUES (212000, 'aggregate-gate')"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := harness.Exec("DELETE FROM myrest_fixture.colors WHERE id IN (212000, 212001)"); err != nil {
			t.Error(err)
		}
	})
	service := serve(t, "myrest_fixture")
	_, beforeColors := get(t, service, "/colors?id=in.(212000,212001)&order=id")
	_, beforeMarkers := get(t, service, "/addresses?label=eq.rpc-write&order=id")
	cases := []struct{ method, path, body string }{
		{"GET", "/rpc/list_items?", ""},
		{"POST", "/rpc/list_items?", "{}"},
		{"POST", "/rpc/write_marker?", "{}"},
		{"POST", "/colors?", `{"id":212001,"name":"should-not-exist"}`},
		{"PATCH", "/colors?id=eq.212000&", `{"tone":"changed"}`},
		{"DELETE", "/colors?id=eq.212000&", ""},
		{"PUT", "/colors?id=eq.212000&", `{"id":212000,"name":"changed"}`},
	}
	for _, tc := range cases {
		for _, selectList := range []string{"count()", "id.sum()"} {
			for _, mode := range []string{"minimal", "representation"} {
				t.Run(tc.method+tc.path+selectList+"/"+mode, func(t *testing.T) {
					response, body := apitest.DoBody(t, tc.method, service.URL()+tc.path+"select="+selectList,
						http.Header{"Content-Type": {"application/json"}, "Prefer": {"return=" + mode}}, tc.body)
					apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST123")
					response, colors := get(t, service, "/colors?id=in.(212000,212001)&order=id")
					if response.StatusCode != http.StatusOK || string(colors) != string(beforeColors) {
						t.Fatalf("colors changed: before=%s after=%s", beforeColors, colors)
					}
					response, markers := get(t, service, "/addresses?label=eq.rpc-write&order=id")
					if response.StatusCode != http.StatusOK || string(markers) != string(beforeMarkers) {
						t.Fatalf("RPC markers changed: before=%s after=%s", beforeMarkers, markers)
					}
				})
			}
		}
	}
}
