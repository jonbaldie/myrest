package httpapi

import (
	"context"
	"net/http"

	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Reader reads the rows of a resource as one database role.
type Reader interface {
	Read(ctx context.Context, role string, table schemacache.Table) ([]rows.Row, error)
}

// readTable answers GET /<table>: it finds the resource of the active database
// role in the schema cache, and reads all of its rows.
func (s *Service) readTable(writer http.ResponseWriter, request *http.Request) {
	role := s.settings.DB.AnonRole
	if role == "" {
		writeFailure(
			writer, http.StatusUnauthorized, codeNoAnonymousRole,
			"Anonymous requests need db-anon-role",
		)
		return
	}

	name := request.PathValue("table")
	table, isResource := s.cache.Resource(role, name)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoTable, s.noTableMessage(name))
		return
	}

	read, err := s.reader.Read(request.Context(), role, table)
	if err != nil {
		writeFailure(writer, http.StatusInternalServerError, codeDatabaseFailure, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, read)
}

// noTableMessage names the object the client asked for, the way the parity
// target does. The name carries the first configured database, because a
// request without a profile reads the first database of db-schemas.
func (s *Service) noTableMessage(name string) string {
	schema := ""
	if len(s.settings.DB.Schemas) > 0 {
		schema = s.settings.DB.Schemas[0] + "."
	}
	return "Could not find the table '" + schema + name + "' in the schema cache"
}
