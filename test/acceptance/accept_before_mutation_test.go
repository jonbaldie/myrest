package acceptance_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// Seam under test: the whole service over MySQL 8. A refusal that a client
// reads as "the server did nothing" must leave the database unchanged.

// sendOverMySQL issues one request with headers and an optional body.
func sendOverMySQL(t *testing.T, method, url string, headers map[string]string, body string) (*http.Response, []byte) {
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

// repr-007 / tx-001: a POST that refuses on Accept inserts no row.
func TestPostAcceptRefusalWritesNothingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := sendOverMySQL(t, http.MethodPost, service.URL()+"/items",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "application/geo+json",
		},
		`{"name":"should-not-persist"}`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")

	after, afterBody := apitest.Get(t, service.URL()+"/items?name=eq.should-not-persist")
	if after.StatusCode != http.StatusOK {
		t.Fatalf("read back status = %d; body = %s", after.StatusCode, afterBody)
	}
	if strings.TrimSpace(string(afterBody)) != "[]" {
		t.Fatalf("read back = %s, want no row after the 415", afterBody)
	}
}

// repr-007 / tx-001: a PATCH that refuses on Accept updates no row.
func TestPatchAcceptRefusalWritesNothingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := sendOverMySQL(t, http.MethodPatch, service.URL()+"/items?id=eq.1",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "text/plain",
		},
		`{"name":"mutated-patch"}`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")

	after, afterBody := apitest.Get(t, service.URL()+"/items?id=eq.1")
	if strings.Contains(string(afterBody), "mutated-patch") {
		t.Fatalf("read back = %s (status %d), want the row unchanged after the 415", afterBody, after.StatusCode)
	}
}

// repr-007 / tx-001: a DELETE that refuses on Accept removes no row.
func TestDeleteAcceptRefusalWritesNothingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	created, createdBody := apitest.PostJSON(t, service.URL()+"/items", `{"name":"accept-victim"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("seed status = %d; body = %s", created.StatusCode, createdBody)
	}
	// The tests of this package share one fixture database, so the seed row goes
	// away again and the row counts other tests read stay true.
	t.Cleanup(func() {
		_, _ = apitest.Do(t, http.MethodDelete, service.URL()+"/items?name=eq.accept-victim", nil)
	})

	response, body := sendOverMySQL(t, http.MethodDelete, service.URL()+"/items?name=eq.accept-victim",
		map[string]string{
			"Prefer": "return=representation",
			"Accept": "application/geo+json",
		},
		"",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")

	after, afterBody := apitest.Get(t, service.URL()+"/items?name=eq.accept-victim")
	if after.StatusCode != http.StatusOK {
		t.Fatalf("read back status = %d; body = %s", after.StatusCode, afterBody)
	}
	if !strings.Contains(string(afterBody), "accept-victim") {
		t.Fatalf("read back = %s, want the row still there after the 415", afterBody)
	}
}

// repr-007 / tx-001: an RPC that refuses on Accept never runs the routine.
func TestRPCAcceptRefusalRunsNothingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	before, beforeBody := apitest.Get(t, service.URL()+"/addresses?label=eq.rpc-write")
	if before.StatusCode != http.StatusOK {
		t.Fatalf("read back status = %d; body = %s", before.StatusCode, beforeBody)
	}

	response, body := sendOverMySQL(t, http.MethodPost, service.URL()+"/rpc/write_marker",
		map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/geo+json",
		},
		`{}`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusUnsupportedMediaType, "PGRST107")

	after, afterBody := apitest.Get(t, service.URL()+"/addresses?label=eq.rpc-write")
	if after.StatusCode != http.StatusOK {
		t.Fatalf("read back status = %d; body = %s", after.StatusCode, afterBody)
	}
	if string(afterBody) != string(beforeBody) {
		t.Fatalf("addresses = %s, want %s: the routine ran despite the 415", afterBody, beforeBody)
	}
}

// repr-008 / tx-001: a singular Accept on a write that affects more than one row
// refuses with PGRST116 and rolls the write back.
func TestSingularAcceptRollsWriteBackOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := sendOverMySQL(t, http.MethodPost, service.URL()+"/items",
		map[string]string{
			"Content-Type": "application/json",
			"Prefer":       "return=representation",
			"Accept":       "application/vnd.pgrst.object+json",
		},
		`[{"name":"singular-a"},{"name":"singular-b"}]`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")

	after, afterBody := apitest.Get(t, service.URL()+"/items?name=like.singular-*")
	if after.StatusCode != http.StatusOK {
		t.Fatalf("read back status = %d; body = %s", after.StatusCode, afterBody)
	}
	if strings.TrimSpace(string(afterBody)) != "[]" {
		t.Fatalf("read back = %s, want no rows after the 406 rollback", afterBody)
	}
}
