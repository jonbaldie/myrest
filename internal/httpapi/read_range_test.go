package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// threeRows holds the fixture of the Range header tests: three rows that the
// reader returns whole unless the query bounds them.
func threeRows() []rows.Row {
	return []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
		{Columns: []string{"id"}, Values: []any{int64(3)}},
	}
}

// rangeRead sends a GET with the given headers and answers the response, the
// body, and what the reader was asked.
func rangeRead(
	t *testing.T,
	path string,
	headers map[string]string,
) (*http.Response, []byte, *reader) {
	t.Helper()

	source := &reader{read: threeRows()}
	sent := make(http.Header)
	for name, value := range headers {
		sent.Set(name, value)
	}
	response, body := apitest.Do(
		t, http.MethodGet, serve(t, source, settings()).URL()+path, sent,
	)
	return response, body, source
}

// A client that bounds a read with Range: first-last gets the same window as
// limit/offset would give: #146.
func TestRangeHeaderBoundsTheRead(t *testing.T) {
	t.Parallel()

	response, body, source := rangeRead(t, "/items?order=id.asc", map[string]string{
		"Range-Unit": "items",
		"Range":      "0-2",
	})

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if source.query.Limit == nil || *source.query.Limit != 3 || source.query.Offset != 0 {
		t.Fatalf("page = limit %#v offset %d, want limit 3 offset 0", source.query.Limit, source.query.Offset)
	}
	if got := response.Header.Get("Content-Range"); got != "0-2/*" {
		t.Fatalf("Content-Range = %q, want 0-2/*", got)
	}
	if got := response.Header.Get("Range-Unit"); got != "items" {
		t.Fatalf("Range-Unit = %q, want items", got)
	}
}

// Range-Unit is a response unit on the parity target: the request needs no
// Range-Unit header beside Range.
func TestRangeHeaderBoundsTheReadWithoutRangeUnit(t *testing.T) {
	t.Parallel()

	response, _, source := rangeRead(t, "/items?order=id.asc", map[string]string{
		"Range": "3-7",
	})

	if source.query.Limit == nil || *source.query.Limit != 5 || source.query.Offset != 3 {
		t.Fatalf("page = limit %#v offset %d, want limit 5 offset 3", source.query.Limit, source.query.Offset)
	}
	if got := response.Header.Get("Content-Range"); got != "3-5/*" {
		t.Fatalf("Content-Range = %q, want 3-5/*", got)
	}
}

// An open-ended Range has no row cap: it only skips rows.
func TestOpenEndedRangeHeaderSkipsRows(t *testing.T) {
	t.Parallel()

	_, _, source := rangeRead(t, "/items?order=id.asc", map[string]string{
		"Range": "1-",
	})

	if source.query.Limit != nil {
		t.Fatalf("limit = %d, want no limit", *source.query.Limit)
	}
	if source.query.Offset != 1 {
		t.Fatalf("offset = %d, want 1", source.query.Offset)
	}
}

// A Range header the parity target cannot parse is ignored, not refused.
func TestMalformedRangeHeaderIsIgnored(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"no lower":     "-2",
		"two dashes":   "1-2-3",
		"no dash":      "5",
		"not digits":   "a-b",
		"leading text": "items 0-2",
	} {
		response, _, source := rangeRead(t, "/items?order=id.asc", map[string]string{"Range": raw})
		if source.query.Limit != nil || source.query.Offset != 0 {
			t.Errorf("%s (%q): page = limit %#v offset %d, want no window", name, raw, source.query.Limit, source.query.Offset)
		}
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s (%q): status = %d, want %d", name, raw, response.StatusCode, http.StatusOK)
		}
	}
}

// HEAD reads with the same intent but ignores the Range header, the way the
// RFC and the parity target read methods other than GET.
func TestHeadReadIgnoresTheRangeHeader(t *testing.T) {
	t.Parallel()

	source := &reader{read: threeRows()}
	headers := make(http.Header)
	headers.Set("Range", "0-2")
	response, _ := apitest.Do(
		t, http.MethodHead, serve(t, source, settings()).URL()+"/items?order=id.asc", headers,
	)

	if source.query.Limit != nil || source.query.Offset != 0 {
		t.Fatalf("page = limit %#v offset %d, want no window", source.query.Limit, source.query.Offset)
	}
	if got := response.Header.Get("Content-Range"); got != "0-2/*" {
		t.Fatalf("Content-Range = %q, want 0-2/*", got)
	}
}

// A Range window whose bounds cross is an unsatisfiable range: 416 PGRST103.
func TestCrossedRangeBoundsAreRefused(t *testing.T) {
	t.Parallel()

	response, body, source := rangeRead(t, "/items", map[string]string{
		"Range": "5-3",
	})

	failure := apitest.AssertEnvelope(t, response, body, http.StatusRequestedRangeNotSatisfiable, "PGRST103")
	if want := "Requested range not satisfiable"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
	if want := "The lower boundary must be lower than or equal to the upper boundary in the Range header."; failure.Details != want {
		t.Fatalf("details = %v, want %q", failure.Details, want)
	}
	if source.query.Limit != nil || source.query.Offset != 0 {
		t.Fatalf("page = limit %#v offset %d, want no window", source.query.Limit, source.query.Offset)
	}
}

// The Range window and limit/offset must meet: a window past the limit end is
// an unsatisfiable range.
func TestRangeWindowPastTheLimitEndIsRefused(t *testing.T) {
	t.Parallel()

	response, body, _ := rangeRead(t, "/items?limit=3", map[string]string{
		"Range": "5-",
	})

	failure := apitest.AssertEnvelope(t, response, body, http.StatusRequestedRangeNotSatisfiable, "PGRST103")
	if want := "Limit should be greater than or equal to zero."; failure.Details != want {
		t.Fatalf("details = %v, want %q", failure.Details, want)
	}
}

// A Range window and limit/offset both bound the read: their meeting window
// is the answer.
func TestRangeHeaderAndLimitOffsetMeet(t *testing.T) {
	t.Parallel()

	response, _, source := rangeRead(t, "/items?limit=3&offset=1", map[string]string{
		"Range": "2-9",
	})

	if source.query.Limit == nil || *source.query.Limit != 2 || source.query.Offset != 2 {
		t.Fatalf("page = limit %#v offset %d, want limit 2 offset 2", source.query.Limit, source.query.Offset)
	}
	if got := response.Header.Get("Content-Range"); got != "2-4/*" {
		t.Fatalf("Content-Range = %q, want 2-4/*", got)
	}
}

// limit=0 keeps its meaning of "no rows" and bypasses the Range window.
func TestLimitZeroBypassesTheRangeWindow(t *testing.T) {
	t.Parallel()

	_, _, source := rangeRead(t, "/items?limit=0", map[string]string{
		"Range": "5-",
	})

	if source.query.Limit == nil || *source.query.Limit != 0 {
		t.Fatalf("limit = %#v, want 0", source.query.Limit)
	}
	if source.query.Offset != 0 {
		t.Fatalf("offset = %d, want 0", source.query.Offset)
	}
}
