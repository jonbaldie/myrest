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
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items?id=eq.1", headers,
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
