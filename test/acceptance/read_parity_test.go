package acceptance_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// read-003: case-insensitive text match inside the MySQL collation subset.
func TestILikeInsideTextCaseSubsetOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,name&name=ilike.ALPHA&order=id.asc",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// read-004: Postgres POSIX regex text match refuses over MySQL.
func TestIMatchOutsideTextCaseSubsetOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/items?name=imatch.alpha")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-005: JSON path read and filter inside the MySQL subset.
func TestJSONPathInsideSubsetOverMySQL(t *testing.T) {
	path := "/profiles?select=id,meta->>blood_type&" +
		url.QueryEscape("meta->>blood_type") + "=eq.A-&order=id.asc"
	response, body := get(t, serve(t, "myrest_fixture"), path)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"blood_type":"A-"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// read-006: Postgres-only JSON path form refuses over MySQL.
func TestPostgresOnlyJSONPathOverMySQL(t *testing.T) {
	path := "/profiles?select=" + url.QueryEscape("meta#>>{blood_type}")
	response, body := get(t, serve(t, "myrest_fixture"), path)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(failure.Message, "#>") {
		t.Fatalf("message = %q, want a #> refusal", failure.Message)
	}
}

// read-007: FTS family operators refuse over MySQL.
func TestFTSOperatorOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/items?name=fts.english.alpha")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-008: Postgres array/range operators refuse over MySQL.
func TestArrayOperatorOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture"), "/items?tags=cs.{a,b}")
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-009 and repr-003: Prefer count=planned refuses over MySQL.
func TestPreferCountPlannedOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "count=planned")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items", headers,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-009 and repr-003: Prefer count=estimated refuses over MySQL.
func TestPreferCountEstimatedOverMySQL(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Prefer", "count=estimated")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, "myrest_fixture").URL()+"/items", headers,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
