package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/rows"
)

// rangeRPC sends GET /rpc/<path> with a Range header to a caller that
// answers two rows, or the given body.
func rangeRPC(t *testing.T, path, rangeHeader string, answer any) (*http.Response, []byte) {
	t.Helper()

	source := &caller{body: answer}
	headers := make(http.Header)
	headers.Set("Range", rangeHeader)
	return apitest.Do(t, http.MethodGet, serveRPC(t, source).URL()+path, headers)
}

func twoItems() []rows.Row {
	return []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
	}
}

// GET on a row-set routine bounds the result with the Range header, the way
// a table read does.
func TestGetRPCRowSetHonoursTheRangeHeader(t *testing.T) {
	t.Parallel()

	response, body := rangeRPC(t, "/rpc/list_items", "0-0", twoItems())

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha"}]` + "\n"; string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if got := response.Header.Get("Content-Range"); got != "0-0/*" {
		t.Fatalf("Content-Range = %q, want 0-0/*", got)
	}
}

// An unsatisfiable Range on GET /rpc is refused with 416 PGRST103.
func TestGetRPCCrossedRangeBoundsAreRefused(t *testing.T) {
	t.Parallel()

	response, body := rangeRPC(t, "/rpc/list_items", "5-3", twoItems())

	apitest.AssertEnvelope(t, response, body, http.StatusRequestedRangeNotSatisfiable, "PGRST103")
}

// A Range header on a scalar routine is pagination on a scalar result.
func TestGetRPCScalarRefusesTheRangeHeader(t *testing.T) {
	t.Parallel()

	response, body := rangeRPC(t, "/rpc/add_them?a=1&b=2", "0-0", int64(3))

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Filter, order, pagination, and embed need a row-set RPC result"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}
