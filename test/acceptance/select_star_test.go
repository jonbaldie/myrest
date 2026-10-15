package acceptance_test

import (
	"net/http"
	"testing"
)

// read-001: a * part beside named columns still projects every column.
func TestSelectStarBesideColumnsOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")
	cases := map[string]string{
		"/items?select=id,*&order=id.asc&limit=1":              `[{"id":1,"name":"alpha","name_len":5}]`,
		"/items?select=name,*,orders(id)&order=id.asc&limit=1": `[{"name":"alpha","id":1,"name_len":5,"orders":[{"id":1},{"id":2}]}]`,
		"/items?select=id,orders(id,*)&order=id.asc&limit=1":   `[{"id":1,"orders":[{"id":1,"item_id":1},{"id":2,"item_id":1}]}]`,
		"/orders?select=id,*,items(name)&order=id.asc&limit=1": `[{"id":1,"item_id":1,"items":{"name":"alpha"}}]`,
		"/items?select=id,tags(name,*)&order=id.asc&limit=1":   `[{"id":1,"tags":[{"name":"hot","id":1},{"name":"cold","id":2}]}]`,
	}
	for path, want := range cases {
		response, body := get(t, service, path)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d; body = %s", path, response.StatusCode, body)
		}
		if string(body) != want+"\n" {
			t.Errorf("%s: body = %s, want %s", path, body, want)
		}
	}
}
