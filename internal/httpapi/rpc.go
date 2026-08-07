package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// codeNoRoutine is the parity-target code for a routine the schema cache does
// not hold for the active database role.
const codeNoRoutine = "PGRST202"

// Caller runs a routine as one database role with named JSON arguments.
type Caller interface {
	Call(
		ctx context.Context,
		role schemacache.Role,
		routine schemacache.RoutineFact,
		args map[string]any,
	) (any, error)
}

// callRoutine answers POST /rpc/<name>: it finds the routine resource of the
// active database role and runs it with named JSON arguments. Functions follow
// the PostgREST scalar body. Procedures return the stable OUT/INOUT object.
func (s *Service) callRoutine(writer http.ResponseWriter, request *http.Request) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return
	}

	args, ok := readNamedJSONArgs(writer, request)
	if !ok {
		return
	}

	asked := schemacache.RoutineID{
		Database: s.settings.DefaultDatabase(),
		Name:     request.PathValue("name"),
	}
	routine, isResource := s.cache.Routine(role, asked)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return
	}

	result, err := s.caller.Call(request.Context(), role, routine, args)
	if err != nil {
		s.log.Printf("myrest: rpc %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		writeDatabaseFailure(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// readNamedJSONArgs reads the PostgREST named-argument object. An empty body
// means no arguments.
func readNamedJSONArgs(writer http.ResponseWriter, request *http.Request) (map[string]any, bool) {
	defer func() { _ = request.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, "Could not read the request body")
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, true
	}

	var args map[string]any
	if err := json.Unmarshal(body, &args); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, "Could not parse the JSON body")
		return nil, false
	}
	if args == nil {
		return map[string]any{}, true
	}
	return args, true
}

// noRoutineMessage names the routine the client asked for, the way the parity
// target does for a missing function.
func noRoutineMessage(asked schemacache.RoutineID) string {
	return "Could not find the function " + asked.Database + "." + asked.Name + " in the schema cache"
}
