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
	database, ok := s.requestDatabase(writer, request, headerAcceptProfile)
	if !ok {
		return
	}
	asked := schemacache.TableID{
		Database: database,
		Name:     request.PathValue("table"),
	}
	methods := tableAllowMethods(s.cache, role, asked)
	if len(methods) == 0 {
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
	database, ok := s.requestDatabase(writer, request, headerAcceptProfile)
	if !ok {
		return
	}
	asked := schemacache.RoutineID{
		Database: database,
		Name:     request.PathValue("name"),
	}
	routine, isResource := s.cache.Routine(role, asked)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return
	}
	writeAllow(writer, routineAllowMethods(routine))
}

// tableAllowMethods builds the Allow list for a table from grants. OPTIONS is
// always present when the role holds any usable privilege. PUT upsert is not
// on the served surface yet, so it is never advertised.
func tableAllowMethods(cache *schemacache.Cache, role schemacache.Role, id schemacache.TableID) []string {
	methods := []string{http.MethodOptions}
	usable := false
	if cache.HasTablePrivilege(role, id, "SELECT") {
		methods = append(methods, http.MethodGet, http.MethodHead)
		usable = true
	}
	if cache.HasTablePrivilege(role, id, "INSERT") {
		methods = append(methods, http.MethodPost)
		usable = true
	}
	if cache.HasTablePrivilege(role, id, "UPDATE") {
		methods = append(methods, http.MethodPatch)
		usable = true
	}
	if cache.HasTablePrivilege(role, id, "DELETE") {
		methods = append(methods, http.MethodDelete)
		usable = true
	}
	if !usable {
		return nil
	}
	return methods
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
