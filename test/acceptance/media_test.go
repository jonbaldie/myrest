package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// repr-005: singular Accept returns one object over MySQL 8.
func TestAcceptSingularObjectOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Accept", "application/vnd.pgrst.object+json")
	response, body := apitest.Do(
		t, http.MethodGet,
		serve(t, "myrest_fixture").URL()+"/items?select=id,name&id=eq.1",
		headers,
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

// repr-006: CSV Accept returns CSV over MySQL 8.
func TestAcceptCSVOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Accept", "text/csv")
	response, body := apitest.Do(
		t, http.MethodGet,
		serve(t, "myrest_fixture").URL()+"/items?select=id,name&order=id.asc",
		headers,
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

// Accept quality values select the highest acceptable claimed representation.
func TestAcceptQualityValuesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")
	for _, tc := range []struct {
		name        string
		accept      string
		status      int
		contentType string
		code        string
	}{
		{
			name:   "refuses a zero quality representation",
			accept: "application/json;q=0",
			status: http.StatusUnsupportedMediaType,
			code:   "PGRST107",
		},
		{
			name:        "selects the highest quality representation",
			accept:      "application/json;q=0.1, text/csv;q=1",
			status:      http.StatusOK,
			contentType: "text/csv; charset=utf-8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("Accept", tc.accept)
			response, body := apitest.Do(
				t, http.MethodGet, service.URL()+"/items?select=id,name&order=id.asc", headers,
			)
			if tc.code != "" {
				apitest.AssertEnvelope(t, response, body, tc.status, tc.code)
				return
			}
			if response.StatusCode != tc.status {
				t.Fatalf("status = %d; body = %s", response.StatusCode, body)
			}
			if got := response.Header.Get("Content-Type"); got != tc.contentType {
				t.Fatalf("Content-Type = %q, want %q", got, tc.contentType)
			}
		})
	}
}

// repr-007: unclaimed Accept media types refuse over MySQL 8.
func TestUnclaimedAcceptMediaTypesOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Accept", "application/geo+json")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items", headers,
	)
	failure := apitest.AssertEnvelope(
		t, response, body, http.StatusUnsupportedMediaType, "PGRST107",
	)
	if !strings.Contains(failure.Message, "application/geo+json") {
		t.Fatalf("message = %q", failure.Message)
	}
}

// prefer-001: Prefer timezone refuses over MySQL 8.
func TestPreferTimezoneOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "timezone=UTC")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items", headers,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
