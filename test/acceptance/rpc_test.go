package acceptance_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// rpc-001 and smoke-004: POST /rpc/<function> with named JSON arguments
// succeeds over MySQL 8.
func TestPostRPCFunctionWithNamedJSONArgs(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		`{"a":1,"b":2}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if string(body) != "3\n" {
		t.Fatalf("body = %s, want 3", body)
	}
}

// A no-argument function also answers over POST /rpc.
func TestPostRPCFunctionWithNoArguments(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/item_count",
		`{}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != "2\n" {
		t.Fatalf("body = %s, want 2", body)
	}
}

// rpc-002: POST /rpc/<procedure> succeeds with the stable OUT/INOUT object.
func TestPostRPCProcedureAnswersWithTheStableObject(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/ping",
		`{}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != "{}\n" {
		t.Fatalf("body = %s, want {}", body)
	}
}

// rpc-002 with OUT parameters: the stable object holds the OUT values.
func TestPostRPCProcedureAnswersWithOUTParameters(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_name",
		`{"src":"alpha"}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != `{"dst":"alpha"}`+"\n" {
		t.Fatalf("body = %s, want {\"dst\":\"alpha\"}", body)
	}
}

// Unknown named arguments are a signature mismatch: not a found routine.
func TestPostRPCWithAnUnknownArgumentIsNotAFoundRoutine(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		`{"a":1,"b":2,"c":3}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function myrest_fixture.add_them in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// An unexpected argument on a no-parameter routine is also not a found routine.
func TestPostRPCWithAnUnexpectedArgumentOnNoParamRoutineIsNotAFoundRoutine(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/ping",
		`{"unexpected":1}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function myrest_fixture.ping in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// A supplied OUT parameter is not an IN or INOUT argument: not a found routine.
func TestPostRPCWithAnOUTArgumentIsNotAFoundRoutine(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_name",
		`{"src":"hi","dst":"val"}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function myrest_fixture.echo_name in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// Missing named arguments continue to be a signature mismatch.
func TestPostRPCWithAMissingArgumentIsNotAFoundRoutine(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		`{"a":1}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function myrest_fixture.add_them in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// A routine without EXECUTE is not usable as a resource.
func TestRoutineWithoutExecuteIsNotAResource(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/secret_count",
		`{}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function myrest_fixture.secret_count in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-002 with an INOUT parameter: the stable object holds the final value.
func TestPostRPCProcedureAnswersWithINOUTParameters(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/bump_label",
		`{"label":"alpha"}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != `{"label":"alpha!"}`+"\n" {
		t.Fatalf("body = %s, want label alpha!", body)
	}
}

// rpc-003: GET /rpc/<read-safe routine> with named query-string arguments
// succeeds over MySQL 8.
func TestGetRPCReadSafeRoutineWithNamedQueryArgs(t *testing.T) {
	response, body := apitest.Get(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them?a=1&b=2",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if string(body) != "3\n" {
		t.Fatalf("body = %s, want 3", body)
	}
}

// A read-safe function that reads SQL data also answers over GET /rpc.
func TestGetRPCReadSafeRoutineThatReadsSQLData(t *testing.T) {
	response, body := apitest.Get(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/item_count",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != "2\n" {
		t.Fatalf("body = %s, want 2", body)
	}
}

// rpc-004: GET /rpc/<routine that is not read-safe> refuses stably.
func TestGetRPCNonReadSafeRoutineRefuses(t *testing.T) {
	response, body := apitest.Get(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/write_marker",
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Only a read-safe routine can be called with GET"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-007: a single unnamed JSON/jsonb whole-body argument refuses stably.
func TestPostRPCWholeBodyJSONArgumentRefuses(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		`[1, 2]`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "A single unnamed JSON RPC argument is not supported"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-008: a single unnamed bytea whole-body argument refuses stably.
func TestPostRPCWholeBodyByteaArgumentRefuses(t *testing.T) {
	response, body := apitest.Post(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		"application/octet-stream",
		"\x00\x01\x02",
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "A single unnamed bytea RPC argument is not supported"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-009: a single unnamed text whole-body argument refuses stably.
func TestPostRPCWholeBodyTextArgumentRefuses(t *testing.T) {
	response, body := apitest.Post(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		"text/plain",
		"hello",
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "A single unnamed text RPC argument is not supported"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-010: a single unnamed xml whole-body argument refuses stably.
func TestPostRPCWholeBodyXMLArgumentRefuses(t *testing.T) {
	response, body := apitest.Post(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/add_them",
		"text/xml",
		"<a/>",
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "A single unnamed xml RPC argument is not supported"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// rpc-005: filter, order, and pagination on a row-set RPC result succeed, and
// embed succeeds when the relationship is in the schema cache.
func TestRPCRowSetSupportsFilterOrderPageAndEmbed(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := apitest.PostJSON(
		t,
		service.URL()+"/rpc/list_items?id=eq.1&order=name.asc&limit=1",
		`{}`,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != `[{"id":1,"name":"alpha"}]`+"\n" {
		t.Fatalf("body = %s, want alpha only", body)
	}

	response, body = apitest.PostJSON(
		t,
		service.URL()+"/rpc/list_items?select=id,name,orders(id)&id=eq.1&orders.order=id.asc",
		`{}`,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("embed status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"name":"alpha","orders":[{"id":1},{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("embed body = %s, want %s", body, want)
	}
}

// rpc-006: filter, order, pagination, or embed on a scalar RPC result refuses.
func TestRPCScalarRefusesRowSetFeatures(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := apitest.PostJSON(
		t,
		service.URL()+"/rpc/add_them?limit=1",
		`{"a":1,"b":2}`,
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Filter, order, pagination, and embed need a row-set RPC result"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}

	response, body = apitest.PostJSON(
		t,
		service.URL()+"/rpc/ping?order=id.asc",
		`{}`,
	)
	failure = apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Filter, order, pagination, and embed need a row-set RPC result"; failure.Message != want {
		t.Fatalf("procedure message = %q, want %q", failure.Message, want)
	}
}

// Issue #178: a mutating non-tabular RPC that refuses row-set query features
// rolls the routine side effects back.
func TestRPCNonTabularRowSetFeaturesRollBackTheRoutine(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, beforeBody := get(t, service, "/addresses?select=label&label=eq.rpc-write")
	before := decodeRows(t, beforeBody)

	cases := []string{
		"/rpc/write_marker?limit=1",
		"/rpc/write_marker?offset=1",
		"/rpc/write_marker?order=label.desc",
		"/rpc/write_marker?label=eq.rpc-write",
		"/rpc/write_marker?select=*,items(id)",
	}
	for _, path := range cases {
		response, body := apitest.PostJSON(t, service.URL()+path, `{}`)
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
		if want := "Filter, order, pagination, and embed need a row-set RPC result"; failure.Message != want {
			t.Fatalf("path %s: message = %q, want %q", path, failure.Message, want)
		}
	}

	markers, afterBody := get(t, service, "/addresses?select=label&label=eq.rpc-write")
	if markers.StatusCode != http.StatusOK {
		t.Fatalf("marker read-back status = %d; body = %s", markers.StatusCode, afterBody)
	}
	after := decodeRows(t, afterBody)
	if len(after) != len(before) {
		t.Fatalf("the refused routine kept its side effect: before %d after %d (%v)", len(before), len(after), after)
	}
}

// Issue #178: a tabular mutating RPC whose representation shaping fails
// rolls the routine side effects back.
func TestRPCShapingFailureRollsBackTheRoutine(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, beforeBody := get(t, service, "/addresses?select=label&label=eq.rpc-rollback")
	before := decodeRows(t, beforeBody)

	response, body := apitest.PostJSON(
		t,
		service.URL()+"/rpc/mark_and_list?select=nonexistent",
		`{}`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST204")

	markers, afterBody := get(t, service, "/addresses?select=label&label=eq.rpc-rollback")
	if markers.StatusCode != http.StatusOK {
		t.Fatalf("marker read-back status = %d; body = %s", markers.StatusCode, afterBody)
	}
	after := decodeRows(t, afterBody)
	if len(after) != len(before) {
		t.Fatalf("the refused routine kept its side effect: before %d after %d (%v)", len(before), len(after), after)
	}
}

// Issue #196: POST /rpc with a JSON object for a non-JSON parameter refuses with 400 MYREST001.
func TestPostRPCNonJSONParamRefusesObject(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_name",
		`{"src":{"k":1}}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	want := "Cannot pass a JSON object to parameter src: the parameter does not hold JSON"
	if failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// Issue #196: POST /rpc with a JSON array for a non-JSON parameter refuses with 400 MYREST001.
func TestPostRPCNonJSONParamRefusesArray(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_name",
		`{"src":[1,2]}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	want := "Cannot pass a JSON array to parameter src: the parameter does not hold JSON"
	if failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// Issue #196: POST /rpc with an object for a JSON IN parameter returns a JSON value.
// A JSON OUT parameter is encoded as a JSON value, not a quoted string.
func TestPostRPCProcedureAnswersWithJSONObject(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_json",
		`{"doc":{"k":1}}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	doc, ok := result["back"].(map[string]any)
	if !ok {
		t.Fatalf("back = %#v (%T), want a JSON object; body = %s", result["back"], result["back"], body)
	}
	if doc["k"] != float64(1) {
		t.Fatalf("doc[k] = %#v, want 1", doc["k"])
	}
}

// Issue #196: POST /rpc with an array for a JSON IN parameter returns a JSON value.
func TestPostRPCProcedureAnswersWithJSONArray(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_json",
		`{"doc":[1,2]}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	items, ok := result["back"].([]any)
	if !ok {
		t.Fatalf("back = %#v (%T), want a JSON array; body = %s", result["back"], result["back"], body)
	}
	if len(items) != 2 || items[0] != float64(1) || items[1] != float64(2) {
		t.Fatalf("items = %#v, want [1, 2]", items)
	}
}

// Issue #196: POST /rpc function with a JSON parameter returns a JSON value.
func TestPostRPCFunctionAnswersWithJSONObject(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_json_func",
		`{"doc":{"status":"ok"}}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if result["status"] != "ok" {
		t.Fatalf("result = %#v, want status=ok", result)
	}
}

// Issue #196: POST /rpc procedure with null for a JSON parameter returns null.
func TestPostRPCProcedureEchoesNullJSON(t *testing.T) {
	response, body := apitest.PostJSON(
		t,
		serve(t, "myrest_fixture").URL()+"/rpc/echo_json",
		`{"doc":null}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if back, ok := result["back"]; !ok || back != nil {
		t.Fatalf("back = %#v, want null; body = %s", back, body)
	}
}
