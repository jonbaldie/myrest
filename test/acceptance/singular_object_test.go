package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
)

// Issue #175: a mutating request that claims the singular representation
// (Accept: application/vnd.pgrst.object+json) but yields zero or several rows
// refuses with 406 PGRST116, and the refused write unit leaves the database
// unchanged.

// singularHeaders holds the Prefer and Accept headers of a representation
// write that claims one JSON object.
func singularHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Prefer", "return=representation")
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	return headers
}

// rpcObjectHeaders holds the headers of an RPC call that claims one JSON
// object.
func rpcObjectHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	return headers
}

// doWrite sends a write request with headers and a JSON body.
func doWrite(t *testing.T, method, url, body string, headers http.Header) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new %s: %v", method, err)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send %s: %v", method, err)
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s body: %v", method, err)
	}
	_ = response.Body.Close()
	return response, responseBody
}

// assertSingularRefusal checks the 406 PGRST116 envelope.
func assertSingularRefusal(t *testing.T, response *http.Response, body []byte) {
	t.Helper()

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")
	if !strings.Contains(failure.Message, "single JSON object") {
		t.Fatalf("message = %q, want the singular-object refusal", failure.Message)
	}
}

// assertSingularRefusalCount checks the row count in a PGRST116 response.
func assertSingularRefusalCount(t *testing.T, response *http.Response, body []byte, rowCount int) {
	t.Helper()

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")
	if want := fmt.Sprintf("The result contains %d rows", rowCount); failure.Details != want {
		t.Fatalf("details = %v, want %q", failure.Details, want)
	}
}

// decodeRows decodes a JSON array body into row maps.
func decodeRows(t *testing.T, body []byte) []map[string]any {
	t.Helper()

	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	return rows
}

// readNamedRows reads the rows one filter selects, as id and name.
func readNamedRows(t *testing.T, service *httpapi.Service, path string) []map[string]any {
	t.Helper()

	response, body := get(t, service, path+"&select=id,name&order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", response.StatusCode, body)
	}
	return decodeRows(t, body)
}

// POST of several rows with the singular Accept refuses 406 PGRST116 and
// inserts nothing.
func TestPostSingularRefusalRollsBackTheInsert(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodPost, service.URL()+"/items",
		`[{"name":"gamma-175"},{"name":"delta-175"}]`, singularHeaders(),
	)
	assertSingularRefusal(t, response, body)

	rows := readNamedRows(t, service, "/items?name=in.(gamma-175,delta-175)")
	if len(rows) != 0 {
		t.Fatalf("the refused POST kept its rows: %v", rows)
	}
}

// PATCH that matches several rows with the singular Accept refuses 406
// PGRST116 and leaves every matched row unmutated.
func TestPatchSingularRefusalRollsBackTheUpdate(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"alpha-175"}`)
	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"beta-175"}`)

	response, body := doWrite(
		t, http.MethodPatch, service.URL()+"/items?name=in.(alpha-175,beta-175)",
		`{"name":"changed-175"}`, singularHeaders(),
	)
	assertSingularRefusal(t, response, body)

	rows := readNamedRows(t, service, "/items?name=in.(alpha-175,beta-175)")
	if len(rows) != 2 || rows[0]["name"] != "alpha-175" || rows[1]["name"] != "beta-175" {
		t.Fatalf("items = %v, want both rows unmutated", rows)
	}
}

// PATCH that matches no row with the singular Accept refuses 406 PGRST116.
func TestPatchSingularRefusalOnZeroRows(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodPatch, service.URL()+"/items?name=eq.nope-175",
		`{"name":"changed-175"}`, singularHeaders(),
	)
	assertSingularRefusal(t, response, body)
}

// DELETE that matches several rows with the singular Accept refuses 406
// PGRST116 and keeps every row.
func TestDeleteSingularRefusalRollsBackTheDelete(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/colors", `{"name":"red-175"}`)
	_, _ = apitest.PostJSON(t, service.URL()+"/colors", `{"name":"blue-175"}`)

	response, body := doWrite(
		t, http.MethodDelete, service.URL()+"/colors?name=in.(red-175,blue-175)",
		``, singularHeaders(),
	)
	assertSingularRefusal(t, response, body)

	rows := readNamedRows(t, service, "/colors?name=in.(red-175,blue-175)")
	if len(rows) != 2 {
		t.Fatalf("colors = %v, want both rows kept", rows)
	}
}

// DELETE that matches no row with the singular Accept refuses 406 PGRST116.
func TestDeleteSingularRefusalOnZeroRows(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodDelete, service.URL()+"/colors?name=eq.nope-175",
		``, singularHeaders(),
	)
	assertSingularRefusal(t, response, body)
}

// A write whose representation shaping fails (a select column the rows do
// not hold) refuses with 400 PGRST204 and leaves the write undone.
func TestWriteShapingFailureRollsBackTheWrite(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/colors", `{"name":"shape-175"}`)

	response, body := doWrite(
		t, http.MethodPatch, service.URL()+"/colors?name=eq.shape-175&select=bogus-175",
		`{"name":"changed-175"}`, singularHeaders(),
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST204")

	rows := readNamedRows(t, service, "/colors?name=eq.shape-175")
	if len(rows) != 1 || rows[0]["name"] != "shape-175" {
		t.Fatalf("colors = %v, want the row unmutated", rows)
	}
}

// A tabular RPC that writes and yields several rows with the singular Accept
// refuses 406 PGRST116 and rolls every side effect of the routine back.
func TestRPCSingularRefusalRollsBackTheRoutine(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodPost, service.URL()+"/rpc/mark_and_list",
		`{}`, rpcObjectHeaders(),
	)
	assertSingularRefusal(t, response, body)

	markers, body := get(t, service, "/addresses?label=eq.rpc-rollback")
	if markers.StatusCode != http.StatusOK {
		t.Fatalf("marker read-back status = %d; body = %s", markers.StatusCode, body)
	}
	rows := decodeRows(t, body)
	if len(rows) != 0 {
		t.Fatalf("the refused routine kept its side effect: %v", rows)
	}
}

// A tabular RPC that yields no rows with the singular Accept refuses 406
// PGRST116.
func TestRPCSingularRefusalOnZeroRows(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodPost, service.URL()+"/rpc/list_missing_items",
		`{}`, rpcObjectHeaders(),
	)
	assertSingularRefusal(t, response, body)
}

// A row-set RPC applies its filter before checking a singular representation.
func TestRPCSingularUsesFilteredRowSet(t *testing.T) {
	service := serve(t, "myrest_fixture")

	headers := http.Header{}
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	response, body := apitest.Do(
		t, http.MethodGet,
		service.URL()+"/rpc/list_items?id=eq.1",
		headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Content-Type"); got != "application/vnd.pgrst.object+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if want := `{"id":1,"name":"alpha"}` + "\n"; string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// A row-set RPC applies its limit and offset before checking a singular representation.
func TestRPCSingularUsesPagination(t *testing.T) {
	service := serve(t, "myrest_fixture")

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "limit", path: "/rpc/list_items?limit=1", want: `{"id":1,"name":"alpha"}` + "\n"},
		{name: "offset and limit", path: "/rpc/list_items?offset=1&limit=1", want: `{"id":2,"name":"beta"}` + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, body := doWrite(
				t, http.MethodPost, service.URL()+test.path,
				`{}`, rpcObjectHeaders(),
			)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
			}
			if string(body) != test.want {
				t.Fatalf("body = %s, want %s", body, test.want)
			}
		})
	}
}

// Singular refusals report the row count after the RPC filter has run.
func TestRPCSingularRefusalReportsFilteredRowCount(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"rpc-singular-198"}`)

	response, body := doWrite(
		t, http.MethodPost, service.URL()+"/rpc/list_items?id=lte.2",
		`{}`, rpcObjectHeaders(),
	)
	assertSingularRefusalCount(t, response, body, 2)

	response, body = doWrite(
		t, http.MethodPost, service.URL()+"/rpc/list_items?id=eq.999999",
		`{}`, rpcObjectHeaders(),
	)
	assertSingularRefusalCount(t, response, body, 0)
}

// A write or tabular RPC that yields exactly one row with the singular Accept
// commits and answers the single JSON object.
func TestSingularWriteCommitsOneRow(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := doWrite(
		t, http.MethodPost, service.URL()+"/colors?select=id,name",
		`{"name":"solo-175"}`, singularHeaders(),
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if got := response.Header.Get("Content-Type"); got != "application/vnd.pgrst.object+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	var created map[string]any
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if created["name"] != "solo-175" {
		t.Fatalf("body = %s, want one committed object", body)
	}

	response, body = doWrite(
		t, http.MethodPatch, service.URL()+"/colors?name=eq.solo-175&select=id,name",
		`{"name":"solo-2-175"}`, singularHeaders(),
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), `"name":"solo-2-175"`) {
		t.Fatalf("PATCH body = %s, want one committed object", body)
	}

	response, body = doWrite(
		t, http.MethodDelete, service.URL()+"/colors?name=eq.solo-2-175&select=id,name",
		``, singularHeaders(),
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), `"name":"solo-2-175"`) {
		t.Fatalf("DELETE body = %s, want one committed object", body)
	}

	after, body := get(t, service, "/colors?name=eq.solo-2-175")
	if after.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", after.StatusCode, body)
	}
	if string(body) != "[]\n" {
		t.Fatalf("the committed DELETE left the row: %s", body)
	}

	response, body = doWrite(
		t, http.MethodPost, service.URL()+"/rpc/list_one_item",
		`{}`, rpcObjectHeaders(),
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("RPC status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `{"id":1,"name":"alpha"}`; string(body) != want+"\n" {
		t.Fatalf("RPC body = %s, want %s", body, want)
	}
}
