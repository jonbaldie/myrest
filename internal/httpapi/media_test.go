package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Seam under test: the HTTP API boundary for Accept media types (repr media
// matrix) and Prefer timezone. Proof lives here and in test/acceptance.

// repr-004: application/json and */* stay the claimed JSON array representation.
func TestAcceptJSONAndWildcardKeepJSONArray(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	service := serve(t, source, settings())

	for _, accept := range []string{"", "application/json", "*/*"} {
		headers := make(http.Header)
		if accept != "" {
			headers.Set("Accept", accept)
		}
		response, body := apitest.Do(t, http.MethodGet, service.URL()+"/items", headers)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("Accept %q: status = %d; body = %s", accept, response.StatusCode, body)
		}
		if got := response.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Accept %q: Content-Type = %q", accept, got)
		}
		if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
			t.Fatalf("Accept %q: body = %s, want %s", accept, body, want)
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

// repr-005: singular Accept refuses when the result is not exactly one row.
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
