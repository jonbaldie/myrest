package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Seam under test: the HTTP API boundary for ordinary writes and RPC. An
// Accept header myrest cannot serve must refuse before the database work
// starts, so the refusal never leaves a committed mutation behind.

// send issues one request with headers and an optional body.
func send(t *testing.T, method, url string, headers map[string]string, body string) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new %s request for %s: %v", method, url, err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read the body of %s %s: %v", method, url, err)
	}
	return response, answer
}

// repr-007 / tx-001: a POST with an unserveable Accept refuses with PGRST107 and
// writes nothing.
func TestPostRefusesUnserveableAcceptBeforeInsert(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	response, body := send(t, http.MethodPost, serveWrite(t, &reader{}, sink).URL()+"/items",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "application/geo+json",
		},
		`{"name":"should-not-persist"}`,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")
	if sink.called != "" {
		t.Fatalf("writer called %q, want no database work before the Accept refusal", sink.called)
	}
}

// repr-007 / tx-001: a PATCH with an unserveable Accept refuses with PGRST107 and
// updates nothing.
func TestPatchRefusesUnserveableAcceptBeforeUpdate(t *testing.T) {
	t.Parallel()

	sink := &writer{updated: 1}
	response, body := send(t, http.MethodPatch, serveWrite(t, &reader{}, sink).URL()+"/items?id=eq.1",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "text/plain",
		},
		`{"name":"mutated-patch"}`,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")
	if sink.called != "" {
		t.Fatalf("writer called %q, want no database work before the Accept refusal", sink.called)
	}
}

// repr-007 / tx-001: a DELETE with an unserveable Accept refuses with PGRST107 and
// deletes nothing.
func TestDeleteRefusesUnserveableAcceptBeforeDelete(t *testing.T) {
	t.Parallel()

	sink := &writer{deleted: 1}
	response, body := send(t, http.MethodDelete, serveWrite(t, &reader{}, sink).URL()+"/items?name=eq.victim",
		map[string]string{
			"Prefer": "return=representation",
			"Accept": "application/geo+json",
		},
		"",
	)

	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")
	if sink.called != "" {
		t.Fatalf("writer called %q, want no database work before the Accept refusal", sink.called)
	}
}

// repr-007 / tx-001: a POST /rpc with an unserveable Accept refuses with PGRST107 and
// never runs the routine.
func TestRPCRefusesUnserveableAcceptBeforeCall(t *testing.T) {
	t.Parallel()

	sink := &caller{body: int64(1)}
	response, body := send(t, http.MethodPost, serveRPC(t, sink).URL()+"/rpc/write_marker",
		map[string]string{
			"Content-Type": "application/json",
			"Accept":       "text/plain",
		},
		`{}`,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")
	if sink.called {
		t.Fatalf("caller ran the routine before the Accept refusal")
	}
}

// repr-008 / tx-001: a singular Accept on a write that affects more than one row
// refuses with PGRST116 and asks the database layer to roll the write back.
func TestPostSingularAcceptAsksForRollback(t *testing.T) {
	t.Parallel()

	sink := &writer{
		resultRows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(9), "gamma"}},
			{Columns: []string{"id", "name"}, Values: []any{int64(10), "delta"}},
		},
	}
	response, body := send(t, http.MethodPost, serveWrite(t, &reader{}, sink).URL()+"/items",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "application/vnd.pgrst.object+json",
		},
		`[{"name":"gamma"},{"name":"delta"}]`,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")
	if !sink.options.SingularResult {
		t.Fatalf("options = %#v, want SingularResult so the write rolls back", sink.options)
	}
}
