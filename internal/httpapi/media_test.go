package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Seam under test: the HTTP API boundary for Accept media types (repr media
// matrix) and Prefer timezone. Proof lives here and in test/acceptance.

// repr-004: application/json, array+json, and */* claim the JSON array representation.
func TestAcceptJSONAndWildcardKeepJSONArray(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	service := serve(t, source, settings())

	cases := []struct {
		accept      string
		contentType string
	}{
		{accept: "", contentType: "application/json"},
		{accept: "application/json", contentType: "application/json"},
		{accept: "*/*", contentType: "application/json"},
		{accept: "application/vnd.pgrst.array+json", contentType: "application/vnd.pgrst.array+json"},
	}
	for _, tc := range cases {
		headers := make(http.Header)
		if tc.accept != "" {
			headers.Set("Accept", tc.accept)
		}
		response, body := apitest.Do(t, http.MethodGet, service.URL()+"/items", headers)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("Accept %q: status = %d; body = %s", tc.accept, response.StatusCode, body)
		}
		if got := response.Header.Get("Content-Type"); got != tc.contentType {
			t.Fatalf("Accept %q: Content-Type = %q, want %q", tc.accept, got, tc.contentType)
		}
		if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
			t.Fatalf("Accept %q: body = %s, want %s", tc.accept, body, want)
		}
	}
}

// repr-005: application/vnd.pgrst.object+json returns one object.
func TestAcceptSingularObjectReturnsOneObject(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	headers := make(http.Header)
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, source, settings()).URL()+"/items", headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Type"); got != "application/vnd.pgrst.object+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if want := `{"id":1,"name":"alpha"}`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// repr-008: singular Accept refuses when the result is not exactly one row.
func TestAcceptSingularObjectRefusesWrongCardinality(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}}
	headers := make(http.Header)
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, source, settings()).URL()+"/items", headers,
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST116")
	if !strings.Contains(failure.Message, "single JSON object") {
		t.Fatalf("message = %q", failure.Message)
	}
}

// repr-006: text/csv returns CSV for an ordinary read.
func TestAcceptCSVReturnsCSVRows(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}}
	headers := make(http.Header)
	headers.Set("Accept", "text/csv")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, source, settings()).URL()+"/items", headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if got := response.Header.Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	want := "id,name\n1,alpha\n2,beta\n"
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

// repr-006: empty CSV with select still emits the header row.
func TestAcceptCSVEmptyResultKeepsHeader(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Accept", "text/csv")
	response, body := apitest.Do(
		t, http.MethodGet,
		serve(t, &reader{read: []rows.Row{}}, settings()).URL()+"/items?select=id,name",
		headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if want := "id,name\n"; string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

// A claimed media type on a scalar RPC body refuses with PGRST107.
func TestCSVAcceptOnScalarRPCRefuses(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(
		http.MethodPost,
		serveRPC(t, &caller{body: int64(3)}).URL()+"/rpc/add_them",
		strings.NewReader(`{"a":1,"b":2}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/csv")
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = answer.Body.Close() })
	payload, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	apitest.AssertEnvelope(t, answer, payload, http.StatusUnsupportedMediaType, "PGRST107")
}

// repr-007: geo+json, plan media, and unknown Accept refuse with PGRST107.
func TestUnclaimedAcceptMediaTypesRefuse(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	service := serve(t, source, settings())
	for _, accept := range []string{
		"application/geo+json",
		"application/vnd.pgrst.plan+json",
		"application/vnd.pgrst.plan",
		"application/vnd.custom.handler",
		"unknown/unknown",
	} {
		headers := make(http.Header)
		headers.Set("Accept", accept)
		response, body := apitest.Do(t, http.MethodGet, service.URL()+"/items", headers)
		failure := apitest.AssertEnvelope(
			t, response, body, http.StatusUnsupportedMediaType, "PGRST107",
		)
		if !strings.Contains(failure.Message, accept) {
			t.Fatalf("Accept %q: message = %q", accept, failure.Message)
		}
	}
}

// repr-009 and repr-010: CSV and form write bodies refuse at the HTTP seam.
func TestUnclaimedWriteBodyMediaTypesRefuse(t *testing.T) {
	t.Parallel()

	service := serveWrite(t, &reader{}, &writer{})
	for _, contentType := range []string{"text/csv", "application/x-www-form-urlencoded"} {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/items",
			strings.NewReader("name,alpha"),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", contentType)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		apitest.AssertEnvelope(t, response, payload, http.StatusBadRequest, "PGRST102")
	}
}

// prefer-001: Prefer timezone is not supported and refuses stably.
func TestPreferTimezoneIsRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Prefer", "timezone=America/Los_Angeles")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, &reader{}, settings()).URL()+"/items", headers,
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "timezone") {
		t.Fatalf("message = %q, want a timezone refusal", failure.Message)
	}
}
