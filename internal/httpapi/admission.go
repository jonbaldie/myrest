package httpapi

import (
	"net/http"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// requestedResource identifies a requested Resource under the selected
// database role.
type requestedResource struct {
	role  schemacache.Role
	asked schemacache.TableID
}

// selectResource selects the database of a requested Resource for a known
// database role. The caller chooses the profile header for its HTTP method.
func (s *Service) selectResource(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	profileHeader string,
) (requestedResource, bool) {
	database, ok := s.requestDatabase(writer, request, profileHeader)
	if !ok {
		return requestedResource{}, false
	}
	asked := schemacache.TableID{
		Database: database,
		Name:     request.PathValue("table"),
	}
	return requestedResource{role: role, asked: asked}, true
}

// admitReadResource admits a readable table Resource. A refused Resource
// always gets the established table-not-found response.
func (s *Service) admitReadResource(
	writer http.ResponseWriter,
	requested requestedResource,
) (schemacache.Table, bool) {
	table, ok := s.cache.Resource(requested.role, requested.asked)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(requested.asked))
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
	table, ok := s.cache.TableWithPrivilege(requested.role, requested.asked, privilege)
	if !ok {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(requested.asked))
		return schemacache.Table{}, false
	}
	if !s.cache.IsWritable(requested.asked) {
		writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, "The view is not updatable")
		return schemacache.Table{}, false
	}
	return table, true
}
