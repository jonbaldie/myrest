// Package apitest holds the checks that every test of the myrest HTTP API
// boundary makes, so that the wire contract has one definition. The unit tests
// of internal/httpapi and the acceptance tests over MySQL 8 both use it.
package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Envelope is the error body of the parity target.
type Envelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
	Hint    any    `json:"hint"`
}

// Get reads a URL and gives back the answer with its body.
func Get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()

	return Do(t, http.MethodGet, url, nil)
}

// PostJSON sends a POST with a JSON body and gives back the answer with its body.
func PostJSON(t *testing.T, url, body string) (*http.Response, []byte) {
	t.Helper()

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	return do(t, http.MethodPost, url, headers, body)
}

// Do sends a request with the given method and headers and gives back the
// answer with its body.
func Do(t *testing.T, method, url string, headers http.Header) (*http.Response, []byte) {
	t.Helper()

	return do(t, method, url, headers, "")
}

func do(t *testing.T, method, url string, headers http.Header, body string) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new %s request for %s: %v", method, url, err)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
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

// AssertEnvelope holds the error contract of err-001: the status, the JSON
// content type, the code, a message, and all four fields of the envelope. It
// gives back the envelope, so that a test can read what it says.
func AssertEnvelope(
	t *testing.T,
	response *http.Response,
	body []byte,
	status int,
	code string,
) Envelope {
	t.Helper()

	if response.StatusCode != status {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, status, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode the body %s: %v", body, err)
	}
	for _, field := range []string{"code", "message", "details", "hint"} {
		if _, held := fields[field]; !held {
			t.Fatalf("the envelope %s holds no %s", body, field)
		}
	}

	var failure Envelope
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("decode the envelope %s: %v", body, err)
	}
	if failure.Code != code {
		t.Fatalf("code = %q, want %q", failure.Code, code)
	}
	if failure.Message == "" {
		t.Fatalf("the envelope %s holds an empty message", body)
	}
	return failure
}
