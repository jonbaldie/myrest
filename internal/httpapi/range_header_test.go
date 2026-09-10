package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
)

// rangeGet sends a GET with the item range headers of the ticket.
func rangeGet(
	t *testing.T,
	service *httpapi.Service,
	path, unit, window string,
) (*http.Response, []byte) {
	t.Helper()

	headers := make(http.Header)
	if unit != "" {
		headers.Set("Range-Unit", unit)
	}
	headers.Set("Range", window)
	return apitest.Do(t, http.MethodGet, service.URL()+path, headers)
}

// read-001: Range: 3-7 with Range-Unit: items pages the read the same way as
// limit=5&offset=3, filters and order included.
func TestRangeHeaderPagesLikeLimitAndOffset(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(4), "delta"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(5), "echo"}},
	}}
	response, body := rangeGet(
		t,
		serve(t, source, settings()),
		"/items?select=id,name&name=like.*e*&order=id.asc",
		"items",
		"3-7",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if source.query.Limit == nil || *source.query.Limit != 5 || source.query.Offset != 3 {
		t.Fatalf("page = %v offset %d, want limit 5 offset 3", source.query.Limit, source.query.Offset)
	}
	if len(source.query.Filters) != 1 || len(source.query.Order) != 1 {
		t.Fatalf("filters = %#v order = %#v", source.query.Filters, source.query.Order)
	}
	if response.Header.Get("Content-Range") != "3-4/*" {
		t.Fatalf("Content-Range = %q, want 3-4/*", response.Header.Get("Content-Range"))
	}
	if response.Header.Get("Range-Unit") != "items" {
		t.Fatalf("Range-Unit = %q", response.Header.Get("Range-Unit"))
	}
}

// A Range header without a Range-Unit header still means items.
func TestRangeHeaderWithoutUnitPagesTheRead(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
	}}
	response, body := rangeGet(t, serve(t, source, settings()), "/items", "", "0-2")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if source.query.Limit == nil || *source.query.Limit != 3 || source.query.Offset != 0 {
		t.Fatalf("page = %v offset %d, want limit 3 offset 0", source.query.Limit, source.query.Offset)
	}
}

// An open-ended range skips rows and does not cap them.
func TestOpenEndedRangeHeaderSetsOffsetOnly(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(6)}},
	}}
	rangeGet(t, serve(t, source, settings()), "/items", "items", "5-")

	if source.query.Limit != nil || source.query.Offset != 5 {
		t.Fatalf("page = %v offset %d, want no limit and offset 5", source.query.Limit, source.query.Offset)
	}
}

// A page after the last row gives an empty body and the empty Content-Range.
func TestRangeHeaderPastTheEndReturnsAnEmptyPage(t *testing.T) {
	t.Parallel()

	total := int64(20)
	source := &reader{read: []rows.Row{}, total: &total}
	response, body := rangeGet(t, serve(t, source, settings()), "/items?order=id.asc", "items", "50-59")

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusPartialContent, body)
	}
	if string(body) != "[]\n" {
		t.Fatalf("body = %s, want an empty array", body)
	}
	if response.Header.Get("Content-Range") != "*/20" {
		t.Fatalf("Content-Range = %q, want */20", response.Header.Get("Content-Range"))
	}
	if source.query.Offset != 50 || source.query.Limit == nil || *source.query.Limit != 10 {
		t.Fatalf("page = %v offset %d", source.query.Limit, source.query.Offset)
	}
}

// HEAD carries the same range metadata as GET and no body.
func TestHeadWithRangeHeaderHasTheSameMetadata(t *testing.T) {
	t.Parallel()

	total := int64(20)
	source := &reader{
		read:  []rows.Row{{Columns: []string{"id"}, Values: []any{int64(4)}}},
		total: &total,
	}
	headers := make(http.Header)
	headers.Set("Range-Unit", "items")
	headers.Set("Range", "3-3")
	headers.Set("Prefer", "count=exact")
	response, body := apitest.Do(
		t, http.MethodHead, serve(t, source, settings()).URL()+"/items?order=id.asc", headers,
	)

	if response.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusPartialContent)
	}
	if len(body) != 0 {
		t.Fatalf("HEAD body = %q, want empty", body)
	}
	if response.Header.Get("Content-Range") != "3-3/20" {
		t.Fatalf("Content-Range = %q, want 3-3/20", response.Header.Get("Content-Range"))
	}
	if !source.query.ExactCount {
		t.Fatal("the exact count was lost with a range header")
	}
}

// db-max-rows keeps its hard cap when a range header is present.
func TestRangeHeaderKeepsMaxRows(t *testing.T) {
	t.Parallel()

	set := settings()
	set.DB.MaxRows = config.RowLimit{Rows: 2, Capped: true}
	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(4)}},
	}}
	rangeGet(t, serve(t, source, set), "/items", "items", "3-7")

	if source.query.MaxRows == nil || *source.query.MaxRows != 2 {
		t.Fatalf("MaxRows = %v, want 2", source.query.MaxRows)
	}
}

// limit and offset in the query string own the page.
func TestQueryPageWinsOverTheRangeHeader(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
	}}
	rangeGet(t, serve(t, source, settings()), "/items?limit=1&offset=0", "items", "3-7")

	if source.query.Limit == nil || *source.query.Limit != 1 || source.query.Offset != 0 {
		t.Fatalf("page = %v offset %d, want limit 1 offset 0", source.query.Limit, source.query.Offset)
	}
}

// A request with no range header keeps the unpaged behaviour.
func TestNoRangeHeaderKeepsTheWholeRead(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
	}}
	get(t, serve(t, source, settings()), "/items")

	if source.query.Limit != nil || source.query.Offset != 0 {
		t.Fatalf("page = %v offset %d, want no page", source.query.Limit, source.query.Offset)
	}
}

// A bad range refuses as PGRST100 and does not run an unbounded read.
func TestBadRangeHeaderRefuses(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		unit   string
		window string
	}{
		"reversed":         {unit: "items", window: "7-3"},
		"not a number":     {unit: "items", window: "a-b"},
		"no separator":     {unit: "items", window: "3"},
		"negative suffix":  {unit: "items", window: "-5"},
		"unsupported unit": {unit: "bytes", window: "0-9"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			source := &reader{read: []rows.Row{
				{Columns: []string{"id"}, Values: []any{int64(1)}},
			}}
			response, body := rangeGet(t, serve(t, source, settings()), "/items", tc.unit, tc.window)

			apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
			if source.calls != 0 {
				t.Fatalf("the service ran %d reads, want none", source.calls)
			}
		})
	}
}
