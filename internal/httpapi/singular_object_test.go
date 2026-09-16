package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Issue #175: a write or RPC unit that claims the singular representation
// (Accept: application/vnd.pgrst.object+json) but yields zero or several rows
// refuses with 406 PGRST116, and the refusal comes from the in-unit
// validation the database layer runs before commit.

// doWriteWithHeaders sends a request with headers and a body.
func doWriteWithHeaders(
	t *testing.T,
	method, url, body string,
	headers map[string]string,
) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new %s: %v", method, err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send %s: %v", method, err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response, answer
}

// singularWriteHeaders claim a representation of one JSON object.
func singularWriteHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
		"Prefer":       "return=representation",
		"Accept":       "application/vnd.pgrst.object+json",
	}
}

func twoResultRows() []rows.Row {
	return []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}
}

func assertSingularRefusalEnvelope(t *testing.T, response *http.Response, body []byte) {
	t.Helper()

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")
	if !strings.Contains(failure.Message, "single JSON object") {
		t.Fatalf("message = %q, want the singular-object refusal", failure.Message)
	}
}

// POST of several rows with the singular Accept refuses 406 PGRST116 and the
// unit runs the validator.
func TestPostSingularObjectRefusal(t *testing.T) {
	t.Parallel()

	sink := &writer{resultRows: twoResultRows()}
	response, body := doWriteWithHeaders(
		t, http.MethodPost, serveWrite(t, &reader{}, sink).URL()+"/items",
		`[{"name":"alpha"},{"name":"beta"}]`, singularWriteHeaders(),
	)
	assertSingularRefusalEnvelope(t, response, body)
	if sink.options.Validate == nil {
		t.Fatalf("the unit carried no representation validation")
	}
}

// PATCH that matches several rows with the singular Accept refuses 406
// PGRST116.
func TestPatchSingularObjectRefusal(t *testing.T) {
	t.Parallel()

	sink := &writer{updated: 2, resultRows: twoResultRows()}
	response, body := doWriteWithHeaders(
		t, http.MethodPatch, serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.alpha",
		`{"name":"changed"}`, singularWriteHeaders(),
	)
	assertSingularRefusalEnvelope(t, response, body)
}

// DELETE that matches several rows with the singular Accept refuses 406
// PGRST116.
func TestDeleteSingularObjectRefusal(t *testing.T) {
	t.Parallel()

	sink := &writer{deleted: 2, resultRows: twoResultRows()}
	response, body := doWriteWithHeaders(
		t, http.MethodDelete, serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.alpha",
		``, singularWriteHeaders(),
	)
	assertSingularRefusalEnvelope(t, response, body)
}

// A tabular RPC that yields several rows with the singular Accept refuses 406
// PGRST116, and the unit runs the validator.
func TestRPCSingularObjectRefusal(t *testing.T) {
	t.Parallel()

	source := &caller{body: twoResultRows()}
	response, body := doWriteWithHeaders(
		t, http.MethodPost, serveRPC(t, source).URL()+"/rpc/list_items",
		`{}`, map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/vnd.pgrst.object+json",
		},
	)
	assertSingularRefusalEnvelope(t, response, body)
	if source.options.Validate == nil {
		t.Fatalf("the unit carried no representation validation")
	}
}

// A scalar RPC result stays outside the singular refusal: the scalar refusal
// of the singular Accept keeps its own contract.
func TestRPCSingularAcceptOnScalarResult(t *testing.T) {
	t.Parallel()

	source := &caller{body: int64(2)}
	response, body := doWriteWithHeaders(
		t, http.MethodPost, serveRPC(t, source).URL()+"/rpc/add_them",
		`{"a":1,"b":1}`, map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/vnd.pgrst.object+json",
		},
	)
	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")
}

// A singular write that yields exactly one row answers the single JSON
// object, and a non-singular representation write carries no validator.
func TestSingularWriteValidatorScope(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultRows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(9), "gamma"}},
		},
		resultKeys: []map[string]any{{"id": int64(9)}},
	}
	response, body := doWriteWithHeaders(
		t, http.MethodPost, serveWrite(t, &reader{}, sink).URL()+"/items?select=id,name",
		`{"name":"gamma"}`, singularWriteHeaders(),
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if want := `{"id":9,"name":"gamma"}`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	plain := &writer{
		resultRows: twoResultRows(),
		resultKeys: []map[string]any{{"id": int64(1)}, {"id": int64(2)}},
	}
	response, body = doWriteWithHeaders(
		t, http.MethodPost, serveWrite(t, &reader{}, plain).URL()+"/items",
		`[{"name":"alpha"},{"name":"beta"}]`,
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
		},
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("array status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if plain.options.Validate != nil {
		t.Fatalf("a non-singular representation write carried a validator")
	}
}
