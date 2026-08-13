package httpapi

import (
	"net/http"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// readResource identifies a requested read Resource under the selected
// database role. The HTTP read module validates its query before this module
// admits the Resource, as the established refusal order requires.
type readResource struct {
	role  schemacache.Role
	asked schemacache.TableID
}

// selectReadResource selects the database role and database of a requested
// read Resource. It keeps request role and Accept-Profile work together.
func (s *Service) selectReadResource(
	writer http.ResponseWriter,
	request *http.Request,
	profileHeader string,
) (readResource, bool) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return readResource{}, false
	}
	database, ok := s.requestDatabase(writer, request, profileHeader)
	if !ok {
		return readResource{}, false
	}
	asked := schemacache.TableID{
		Database: database,
		Name:     request.PathValue("table"),
	}
	return readResource{role: role, asked: asked}, true
}

// admitReadResource admits a readable table Resource. A refused Resource
// always gets the established table-not-found response.
func (s *Service) admitReadResource(
	writer http.ResponseWriter,
	requested readResource,
) (schemacache.Table, bool) {
	table, ok := s.cache.Resource(requested.role, requested.asked)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(requested.asked))
		return schemacache.Table{}, false
	}
	return table, true
}
