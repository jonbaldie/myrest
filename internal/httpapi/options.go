package httpapi

import (
	"net/http"
	"strings"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// optionsTable answers OPTIONS /{table}: Allow lists only methods the active
// database role can use on that resource, from the schema cache grants.
func (s *Service) optionsTable(writer http.ResponseWriter, request *http.Request) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return
	}
	requested, ok := s.selectResource(
		writer, request, role, headerAcceptProfile, request.PathValue("table"),
	)
	if !ok {
		return
	}
	methods := s.tableAllowMethods(requested)
	if len(methods) == 0 {
		asked := requested.table()
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return
	}
	writeAllow(writer, methods)
}

// optionsRoutine answers OPTIONS /rpc/{name}: Allow lists only methods the
// active database role can use on that routine resource.
func (s *Service) optionsRoutine(writer http.ResponseWriter, request *http.Request) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return
	}
	requested, ok := s.selectResource(
		writer, request, role, headerAcceptProfile, request.PathValue("name"),
	)
	if !ok {
		return
	}
	routine, ok := s.admitRoutineResource(writer, requested)
	if !ok {
		return
	}
	writeAllow(writer, routineAllowMethods(routine))
}

// routineAllowMethods builds the Allow list for a routine. EXECUTE is already
// required by the caller. GET and HEAD join only when the routine is read-safe.
func routineAllowMethods(routine schemacache.RoutineFact) []string {
	methods := []string{http.MethodOptions, http.MethodPost}
	if routine.ReadSafe() {
		methods = append(methods, http.MethodGet, http.MethodHead)
	}
	return methods
}

func writeAllow(writer http.ResponseWriter, methods []string) {
	writer.Header().Set("Allow", strings.Join(methods, ","))
	writer.WriteHeader(http.StatusOK)
}
