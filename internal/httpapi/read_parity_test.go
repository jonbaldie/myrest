package httpapi_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
)

// read-009 and repr-003: Prefer count=planned refuses stably with a myrest gap
// code. Planner stats are not a PostgREST-shaped count on MySQL.
func TestPreferCountPlannedIsRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Prefer", "count=planned")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, &reader{}, settings()).URL()+"/items", headers,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "planned") &&
		!strings.Contains(strings.ToLower(failure.Message), "count") {
		t.Fatalf("message = %q, want a count=planned refusal", failure.Message)
	}
}

// read-009 and repr-003: Prefer count=estimated refuses the same way.
func TestPreferCountEstimatedIsRefused(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("Prefer", "count=estimated")
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, &reader{}, settings()).URL()+"/items", headers,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-008: a Postgres array or range operator refuses stably.
func TestPostgresArrayOperatorIsRefused(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?tags=cs.{a,b}")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "array") &&
		!strings.Contains(strings.ToLower(failure.Message), "range") {
		t.Fatalf("message = %q, want an array/range refusal", failure.Message)
	}
}

// read-008: range operators share the same refusal.
func TestPostgresRangeOperatorIsRefused(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), `/items?period=ov.[1,10]`)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// read-004: a Postgres POSIX regex text match refuses stably.
func TestPostgresMatchOperatorIsRefused(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?name=imatch.alpha")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "match") &&
		!strings.Contains(strings.ToLower(failure.Message), "regex") {
		t.Fatalf("message = %q, want a match/imatch refusal", failure.Message)
	}
}

// read-003: a case-insensitive text match inside the documented subset succeeds.
func TestILikeFilterInsideTextCaseSubsetSucceeds(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	response, body := get(t, serve(t, source, settings()), "/items?name=ilike.ALPHA&select=id,name")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if len(source.query.Filters) != 1 || source.query.Filters[0].Op != readquery.OpILike {
		t.Fatalf("filters = %#v, want ilike", source.query.Filters)
	}
	if want := `[{"id":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// read-004: ilike outside the *_ci collation subset refuses stably.
func TestILikeOutsideCollationSubsetIsRefused(t *testing.T) {
	t.Parallel()

	source := &reader{failure: readquery.UnsupportedFeature{
		Message: "ilike needs a MySQL Unicode case-insensitive (*_ci) column collation",
	}}
	response, body := get(t, serve(t, source, settings()), "/items?name=ilike.ALPHA")

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(failure.Message, "*_ci") {
		t.Fatalf("message = %q, want a collation subset refusal", failure.Message)
	}
}

// read-006: a Postgres-only JSON path form refuses stably.
func TestPostgresOnlyJSONPathFormIsRefused(t *testing.T) {
	t.Parallel()

	path := "/items?select=" + url.QueryEscape("meta#>>{blood_type}")
	response, body := get(t, serve(t, &reader{}, settings()), path)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(failure.Message, "#>") {
		t.Fatalf("message = %q, want a #> / #>> refusal", failure.Message)
	}
}

// read-006: quoted JSON path keys stay outside the named MySQL subset.
func TestQuotedJSONPathKeyIsRefused(t *testing.T) {
	t.Parallel()

	path := "/items?select=" + url.QueryEscape(`meta->"blood type"`)
	response, body := get(t, serve(t, &reader{}, settings()), path)

	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}
