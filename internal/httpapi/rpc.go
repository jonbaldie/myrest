package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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
	args, ok := readNamedJSONArgs(writer, request)
	if !ok {
		return
	}
	s.runRoutine(writer, request, args, false)
}

// getRoutine answers GET /rpc/<name>: named query-string arguments, only when
// the routine is read-safe under MySQL SQL_DATA_ACCESS.
func (s *Service) getRoutine(writer http.ResponseWriter, request *http.Request) {
	s.runRoutine(writer, request, namedQueryArgs(request), true)
}

func (s *Service) runRoutine(
	writer http.ResponseWriter,
	request *http.Request,
	args map[string]any,
	getOnly bool,
) {
	role, ok := s.requestRole(writer, request)
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
	if getOnly && !routine.ReadSafe() {
		writeFailure(
			writer,
			http.StatusBadRequest,
			codePostgresOnlyFeature,
			"Only a read-safe routine can be called with GET",
		)
		return
	}
	if _, missing := missingRequiredArgument(routine, args); missing {
		// The parity target treats a signature mismatch as a missing routine.
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

// missingRequiredArgument finds an IN or INOUT argument the body does not name.
func missingRequiredArgument(routine schemacache.RoutineFact, args map[string]any) (string, bool) {
	for _, param := range routine.Parameters {
		if param.Ordinal == 0 || param.Name == "" {
			continue
		}
		if strings.EqualFold(param.Mode, "OUT") {
			continue
		}
		if _, held := args[param.Name]; !held {
			return param.Name, true
		}
	}
	return "", false
}

// namedQueryArgs maps each query key to its first value as a named argument.
func namedQueryArgs(request *http.Request) map[string]any {
	args := map[string]any{}
	for name, values := range request.URL.Query() {
		if len(values) == 0 {
			args[name] = ""
			continue
		}
		args[name] = values[0]
	}
	return args
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
