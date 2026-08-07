package acceptance_test

import (
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
