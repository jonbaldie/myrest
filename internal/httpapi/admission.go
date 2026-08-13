package httpapi

import (
	"net/http"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// requestedResource identifies a requested Resource under the selected
// database role.
type requestedResource struct {
	role     schemacache.Role
	database string
	name     string
}

// selectResource selects a requested Resource for a known database role. The
// caller chooses the profile header and route name for its HTTP method.
func (s *Service) selectResource(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	profileHeader string,
	name string,
) (requestedResource, bool) {
	database, ok := s.requestDatabase(writer, request, profileHeader)
	if !ok {
		return requestedResource{}, false
	}
	return requestedResource{role: role, database: database, name: name}, true
}

func (requested requestedResource) table() schemacache.TableID {
	return schemacache.TableID{Database: requested.database, Name: requested.name}
}

func (requested requestedResource) routine() schemacache.RoutineID {
	return schemacache.RoutineID{Database: requested.database, Name: requested.name}
}

// admitReadResource admits a readable table Resource. A refused Resource
// always gets the established table-not-found response.
func (s *Service) admitReadResource(
	writer http.ResponseWriter,
	requested requestedResource,
) (schemacache.Table, bool) {
	asked := requested.table()
	table, ok := s.cache.Resource(requested.role, asked)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return schemacache.Table{}, false
	}
	return table, true
}

// admitWriteResource admits a writable table Resource with the method grant.
// A denied grant and a non-updatable view keep their established refusals.
func (s *Service) admitWriteResource(
	writer http.ResponseWriter,
	requested requestedResource,
	privilege string,
) (schemacache.Table, bool) {
	asked := requested.table()
	table, ok := s.cache.TableWithPrivilege(requested.role, asked, privilege)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return schemacache.Table{}, false
	}
	if !s.cache.IsWritable(asked) {
		writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, "The view is not updatable")
		return schemacache.Table{}, false
	}
	return table, true
}

// admitRoutineResource admits a routine Resource with EXECUTE. A refused
// Resource always gets the established routine-not-found response.
func (s *Service) admitRoutineResource(
	writer http.ResponseWriter,
	requested requestedResource,
) (schemacache.RoutineFact, bool) {
	asked := requested.routine()
	routine, ok := s.cache.Routine(requested.role, asked)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return schemacache.RoutineFact{}, false
	}
	return routine, true
}
