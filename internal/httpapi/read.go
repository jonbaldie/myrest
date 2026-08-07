package httpapi

import (
	"context"
	"net/http"
	"strings"

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
	if hasPostgRESTFullTextSearchOperator(request) {
		writeUnsupportedFeature(
			writer,
			"PostgREST full-text search operators are not available with MySQL",
		)
		return
	}
	if hasPostgRESTCastOrDomainSyntax(request) {
		writeUnsupportedFeature(
			writer,
			"PostgREST domain and cast features are not available with MySQL",
		)
		return
	}
	if hasPostgRESTComputedFieldSyntax(request) {
		writeUnsupportedFeature(
			writer,
			"PostgREST row computed-field features are not available with MySQL",
		)
		return
	}

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
		writeDatabaseFailure(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, read)
}

// noTableMessage names the object the client asked for, the way the parity
// target does: with the database the request reads.
func noTableMessage(asked schemacache.TableID) string {
	return "Could not find the table '" + asked.Database + "." + asked.Name + "' in the schema cache"
}

// hasPostgRESTFullTextSearchOperator finds PostgREST full-text search
// operators. myrest refuses them because a MySQL full-text query has different
// semantics.
func hasPostgRESTFullTextSearchOperator(request *http.Request) bool {
	for _, values := range request.URL.Query() {
		for _, value := range values {
			operator, _, found := strings.Cut(strings.TrimPrefix(value, "not."), ".")
			if found && isPostgRESTFullTextSearchOperator(operator) {
				return true
			}
		}
	}
	return false
}

func isPostgRESTFullTextSearchOperator(operator string) bool {
	switch operator {
	case "fts", "plfts", "phfts", "wfts":
		return true
	default:
		return false
	}
}

// hasPostgRESTCastOrDomainSyntax finds a PostgREST cast or domain mark in the
// query string. MySQL has no matching catalog feature, so myrest refuses it.
func hasPostgRESTCastOrDomainSyntax(request *http.Request) bool {
	for key, values := range request.URL.Query() {
		if strings.Contains(key, "::") {
			return true
		}
		for _, value := range values {
			if strings.Contains(value, "::") {
				return true
			}
		}
	}
	return false
}

// hasPostgRESTComputedFieldSyntax finds a PostgREST row computed-field call in
// select. MySQL has no row-type functions of that kind, so myrest refuses it.
func hasPostgRESTComputedFieldSyntax(request *http.Request) bool {
	for _, value := range request.URL.Query()["select"] {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if strings.HasSuffix(part, "()") {
				return true
			}
		}
	}
	return false
}
