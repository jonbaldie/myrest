package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// writeJSON sends a write request with a JSON body and gives back the answer.
func writeJSON(t *testing.T, method, url, body string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new %s: %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
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

// write-013: a nested JSON object in a POST body stores as JSON when the
// column holds JSON. See issue #119.
func TestPostNestedJSONStoresAsJSONOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := writeJSON(t, http.MethodPost, service.URL()+"/profiles", `{"meta":{"blood_type":"A-"}}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	created, body := get(t, service, "/profiles?select=meta&order=id.desc&limit=1")
	if created.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", created.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1; body = %s", len(rows), body)
	}
	meta, ok := rows[0]["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a JSON object; body = %s", rows[0]["meta"], body)
	}
	if meta["blood_type"] != "A-" {
		t.Fatalf("blood_type = %#v, want A-; body = %s", meta["blood_type"], body)
	}
}

// write-013: a nested JSON object in a PATCH body stores as JSON when the
// column holds JSON. See issue #119.
func TestPatchNestedJSONStoresAsJSONOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := writeJSON(t, http.MethodPatch, service.URL()+"/profiles?id=eq.1",
		`{"meta":{"blood_type":"B+","tag":"Alpha"}}`)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}

	updated, body := get(t, service, "/profiles?select=meta&id=eq.1")
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", updated.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	meta, ok := rows[0]["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a JSON object; body = %s", rows[0]["meta"], body)
	}
	if meta["blood_type"] != "B+" || meta["tag"] != "Alpha" {
		t.Fatalf("meta = %#v, want the stored JSON object; body = %s", meta, body)
	}
}

// write-013: a nested JSON object in a PUT body stores as JSON when the
// column holds JSON. See issue #119.
func TestPutNestedJSONStoresAsJSONOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := writeJSON(t, http.MethodPut, service.URL()+"/profiles?id=eq.2",
		`{"id":2,"meta":{"blood_type":"O-","tag":"Beta"}}`)
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 or 204; body = %s", response.StatusCode, body)
	}

	updated, body := get(t, service, "/profiles?select=meta&id=eq.2")
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", updated.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	meta, ok := rows[0]["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a JSON object; body = %s", rows[0]["meta"], body)
	}
	if meta["blood_type"] != "O-" || meta["tag"] != "Beta" {
		t.Fatalf("meta = %#v, want the stored JSON object; body = %s", meta, body)
	}
}

// write-014: a nested JSON object for a non-JSON column refuses with a
// client error and never reaches MySQL. See issue #119.
func TestNestedJSONObjectOnNonJSONColumnRefuses(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := writeJSON(t, http.MethodPost, service.URL()+"/loose_notes", `{"body":{"a":1}}`)
	envelope := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if envelope.Message != "Cannot write a JSON object into column body: the column does not hold JSON" {
		t.Fatalf("message = %q, want the column refusal", envelope.Message)
	}
}

// write-013: scalar, string, and null bindings keep their shapes.
func TestScalarWriteBindingsUnchangedOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := apitest.PostJSON(t, service.URL()+"/loose_notes", `{"body":"plain text"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	created, body := get(t, service, "/loose_notes?select=body&order=body.desc&limit=1")
	if created.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", created.StatusCode, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if rows[0]["body"] != "plain text" {
		t.Fatalf("body = %#v, want %q; rows = %s", rows[0]["body"], "plain text", body)
	}
}
