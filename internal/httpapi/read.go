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
// target does: with the database the request reads.
func (s *Service) noTableMessage(name string) string {
	return "Could not find the table '" +
		s.settings.DefaultDatabase() + "." + name +
		"' in the schema cache"
}
