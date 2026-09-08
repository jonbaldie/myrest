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

func TestChainedJSONPathOverMySQL(t *testing.T) {
	server := serve(t, "myrest_fixture")

	// 1. Chained JSON path selection in select.
	path := "/profiles?select=id,meta->phones->0->>number&order=id.asc"
	response, body := get(t, server, path)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"number":"917-929-5745"},{"id":2,"number":"512-446-4988"}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	// 2. Filter on chained JSON path returns only matching rows.
	filterPath := "/profiles?select=id,meta->phones->0->>number&" +
		url.QueryEscape("meta->phones->0->>number") + "=eq.917-929-5745"
	response, body = get(t, server, filterPath)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	wantFiltered := `[{"id":1,"number":"917-929-5745"}]`
	if string(body) != wantFiltered+"\n" {
		t.Fatalf("body = %s, want %s", body, wantFiltered)
	}

	// 3. Ordering on chained JSON path orders result rows correctly.
	orderPath := "/profiles?select=id,meta->phones->0->>number&order=" +
		url.QueryEscape("meta->phones->0->>number.desc")
	response, body = get(t, server, orderPath)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	wantOrdered := `[{"id":1,"number":"917-929-5745"},{"id":2,"number":"512-446-4988"}]`
	if string(body) != wantOrdered+"\n" {
		t.Fatalf("body = %s, want %s", body, wantOrdered)
	}

	// 4. Intermediate ->> operator refuses with MYREST001.
	invalidPath := "/profiles?select=" + url.QueryEscape("meta->>phones->0")
	response, body = get(t, server, invalidPath)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
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

// read-007 and smoke-006: FTS family operators refuse over MySQL.
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
