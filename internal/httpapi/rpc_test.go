package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// caller answers a routine call with a fixed body, or with a failure, and
// keeps what the service asked of it.
type caller struct {
	body      any
	failure   error
	stoppable bool
	role      schemacache.Role
	routine   schemacache.RoutineFact
	args      map[string]any
}

func (c *caller) Call(
	ctx context.Context,
	role schemacache.Role,
	routine schemacache.RoutineFact,
	args map[string]any,
) (any, error) {
	c.stoppable = ctx != nil && ctx.Done() != nil
	c.role, c.routine, c.args = role, routine, args
	if c.failure != nil {
		return nil, c.failure
	}
	return c.body, nil
}

func rpcCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	addThem := schemacache.RoutineID{Database: "shop", Name: "add_them"}
	ping := schemacache.RoutineID{Database: "shop", Name: "ping"}
	secret := schemacache.RoutineID{Database: "shop", Name: "secret_count"}
	writeMarker := schemacache.RoutineID{Database: "shop", Name: "write_marker"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: items, Name: "name"},
		},
		Selects: []schemacache.SelectFact{{Role: "myrest_anon", Table: items}},
		Routines: []schemacache.RoutineFact{
			{
				ID:            addThem,
				Kind:          "FUNCTION",
				ReturnType:    "bigint",
				SQLDataAccess: "NO SQL",
				Parameters: []schemacache.ParameterFact{
					{Ordinal: 0, DataType: "bigint"},
					{Name: "a", Mode: "IN", Ordinal: 1, DataType: "bigint"},
					{Name: "b", Mode: "IN", Ordinal: 2, DataType: "bigint"},
				},
			},
			{
				ID:            ping,
				Kind:          "PROCEDURE",
				SQLDataAccess: "CONTAINS SQL",
			},
			{
				ID:            secret,
				Kind:          "FUNCTION",
				SQLDataAccess: "NO SQL",
			},
			{
				ID:            writeMarker,
				Kind:          "PROCEDURE",
				SQLDataAccess: "MODIFIES SQL DATA",
			},
		},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: "myrest_anon", Routine: addThem, Privilege: "EXECUTE"},
			{Role: "myrest_anon", Routine: ping, Privilege: "EXECUTE"},
			{Role: "myrest_anon", Routine: writeMarker, Privilege: "EXECUTE"},
		},
	})
}

func serveRPC(t *testing.T, source httpapi.Caller) *httpapi.Service {
	t.Helper()

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    rpcCache(),
		Reader:   &reader{},
		Caller:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})
	return service
}

// rpc-001: POST /rpc/<function> with named JSON arguments succeeds.
func TestPostRPCFunctionWithNamedJSONArgsSucceeds(t *testing.T) {
	t.Parallel()

	source := &caller{body: int64(3)}
	response, body := apitest.PostJSON(
		t,
		serveRPC(t, source).URL()+"/rpc/add_them",
		`{"a":1,"b":2}`,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if string(body) != "3\n" {
		t.Fatalf("body = %q, want 3", body)
	}
	if source.role != "myrest_anon" {
		t.Errorf("called as role %q, want myrest_anon", source.role)
	}
	if source.routine.ID.Name != "add_them" {
		t.Errorf("routine = %v, want add_them", source.routine.ID)
	}
	if source.args["a"] != float64(1) || source.args["b"] != float64(2) {
		t.Errorf("args = %#v, want a=1 b=2", source.args)
	}
	if !source.stoppable {
		t.Error("the call carries no context a request can stop")
	}
}

// rpc-002: POST /rpc/<procedure> succeeds with the stable OUT/INOUT object.
func TestPostRPCProcedureAnswersWithTheStableObject(t *testing.T) {
	t.Parallel()

	source := &caller{body: map[string]any{}}
	response, body := apitest.PostJSON(t, serveRPC(t, source).URL()+"/rpc/ping", `{}`)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if string(body) != "{}\n" {
		t.Fatalf("body = %q, want {}", body)
	}
	if source.routine.Kind != "PROCEDURE" {
		t.Fatalf("kind = %q, want PROCEDURE", source.routine.Kind)
	}
}

// A routine without EXECUTE is not a routine resource.
func TestRoutineWithoutExecuteIsNotUsableAsAResource(t *testing.T) {
	t.Parallel()

	response, body := apitest.PostJSON(
		t,
		serveRPC(t, &caller{}).URL()+"/rpc/secret_count",
		`{}`,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
	if want := "Could not find the function shop.secret_count in the schema cache"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// Missing named arguments are a signature mismatch, the way the parity target
// treats them: not a found routine.
func TestPostRPCWithAMissingArgumentIsNotAFoundRoutine(t *testing.T) {
	t.Parallel()

	response, body := apitest.PostJSON(
		t,
		serveRPC(t, &caller{body: int64(0)}).URL()+"/rpc/add_them",
		`{"a":1}`,
	)

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST202")
}

// rpc-003: GET /rpc/<read-safe routine> with named query-string arguments
// succeeds.
func TestGetRPCReadSafeRoutineWithNamedQueryArgsSucceeds(t *testing.T) {
	t.Parallel()

	source := &caller{body: int64(3)}
	response, body := apitest.Get(
		t,
		serveRPC(t, source).URL()+"/rpc/add_them?a=1&b=2",
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if string(body) != "3\n" {
		t.Fatalf("body = %q, want 3", body)
	}
	if source.routine.ID.Name != "add_them" {
		t.Errorf("routine = %v, want add_them", source.routine.ID)
	}
	if source.args["a"] != "1" || source.args["b"] != "2" {
		t.Errorf("args = %#v, want a=1 b=2 as query-string values", source.args)
	}
}

// rpc-004: GET /rpc/<routine that is not read-safe> refuses stably.
func TestGetRPCNonReadSafeRoutineRefusesStably(t *testing.T) {
	t.Parallel()

	source := &caller{body: int64(1)}
	response, body := apitest.Get(
		t,
		serveRPC(t, source).URL()+"/rpc/write_marker",
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if want := "Only a read-safe routine can be called with GET"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
	if source.routine.ID.Name != "" {
		t.Fatalf("caller ran for %v, want no call", source.routine.ID)
	}
}
