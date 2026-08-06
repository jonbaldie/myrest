package httpapi

import (
	"context"
	"net/http"

	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Reader reads the rows of a resource as one database role.
type Reader interface {
	Read(ctx context.Context, role schemacache.Role, table schemacache.Table) ([]rows.Row, error)
}

// readTable answers GET /<table>: it finds the resource of the active database
// role in the schema cache, and reads all of its rows. A request names no
// database, so the table comes from the default database; the profile header
// of the parity target comes with the content negotiation ticket.
func (s *Service) readTable(writer http.ResponseWriter, request *http.Request) {
	role := schemacache.Role(s.settings.DB.AnonRole)
	if role == "" {
		writeFailure(
			writer, http.StatusUnauthorized, codeNoAnonymousRole,
			"Anonymous requests need db-anon-role",
		)
		return
	}

	asked := schemacache.TableID{
		Database: s.settings.DefaultDatabase(),
		Name:     request.PathValue("table"),
	}
	table, isResource := s.cache.Resource(role, asked)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return
	}

	read, err := s.reader.Read(request.Context(), role, table)
	if err != nil {
		// The words of the database name the accounts of the deployment,
		// so the operator reads them and the client does not.
		s.log.Printf("myrest: read %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		writeFailure(
			writer, http.StatusInternalServerError, codeDatabaseFailure,
			"The database did not answer the read",
		)
		return
	}
	writeJSON(writer, http.StatusOK, read)
}

// noTableMessage names the object the client asked for, the way the parity
// target does: with the database the request reads.
func noTableMessage(asked schemacache.TableID) string {
	return "Could not find the table '" + asked.Database + "." + asked.Name + "' in the schema cache"
}
