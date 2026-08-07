package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Reader reads a resource as one database role under an ordinary-read query.
type Reader interface {
	Read(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		query readquery.Query,
	) (readquery.Result, error)
}

const (
	// codeParseFailure is the parity-target code for a bad query string.
	codeParseFailure = "PGRST100"
	// codeNoColumn is the parity-target code for a missing column.
	codeNoColumn = "PGRST204"
)

// readTable answers GET and HEAD /<table>: it finds the resource of the active
// database role in the schema cache, and reads under the ordinary-read query.
// A request names no database, so the table comes from the default database;
// the profile header of the parity target comes with the content negotiation
// ticket.
func (s *Service) readTable(writer http.ResponseWriter, request *http.Request) {
	if hasPostgRESTFullTextSearchOperator(request) {
		writeUnsupportedFeature(
			writer,
			"PostgREST full-text search operators are not available with MySQL",
		)
		return
	}

	role, ok := s.requestRole(writer, request)
	if !ok {
		return
	}

	query, err := parseReadQuery(request, s.settings.DB.MaxRows)
	if err != nil {
		writeQueryFailure(writer, err)
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

	read, err := s.reader.Read(request.Context(), role, table, query)
	if err != nil {
		if isUnknownColumn(err) {
			writeFailure(writer, http.StatusBadRequest, codeNoColumn, err.Error())
			return
		}
		// The words of the database name the accounts of the deployment,
		// so the operator reads them and the client does not.
		s.log.Printf("myrest: read %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		writeDatabaseFailure(writer, err)
		return
	}

	writeRead(writer, request.Method == http.MethodHead, query, read)
}

func parseReadQuery(request *http.Request, maxRows config.RowLimit) (readquery.Query, error) {
	query, err := readquery.Parse(request.URL.Query(), request.Header.Values("Prefer"))
	if err != nil {
		return readquery.Query{}, err
	}
	if maxRows.Capped {
		rows := uint64(maxRows.Rows)
		query.MaxRows = &rows
	}
	return query, nil
}

func writeQueryFailure(writer http.ResponseWriter, err error) {
	var parse readquery.ParseFailure
	if errors.As(err, &parse) {
		message := parse.Message
		if strings.Contains(message, "not available with MySQL") {
			writeUnsupportedFeature(writer, message)
			return
		}
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, message)
		return
	}
	writeFailure(writer, http.StatusBadRequest, codeParseFailure, err.Error())
}

func isUnknownColumn(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "column not found:")
}

func writeRead(writer http.ResponseWriter, head bool, query readquery.Query, read readquery.Result) {
	writer.Header().Set("Range-Unit", "items")
	writer.Header().Set("Content-Range", contentRange(query, read))
	status := readStatus(query, read)
	if head {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		return
	}
	writeJSON(writer, status, read.Rows)
}

func contentRange(query readquery.Query, read readquery.Result) string {
	if len(read.Rows) == 0 {
		if read.Total != nil {
			return "*/" + strconv.FormatInt(*read.Total, 10)
		}
		return "*/*"
	}
	start := query.Offset
	end := start + uint64(len(read.Rows)) - 1
	rangePart := strconv.FormatUint(start, 10) + "-" + strconv.FormatUint(end, 10)
	if read.Total != nil {
		return rangePart + "/" + strconv.FormatInt(*read.Total, 10)
	}
	return rangePart + "/*"
}

func readStatus(query readquery.Query, read readquery.Result) int {
	if read.Total == nil {
		return http.StatusOK
	}
	if len(read.Rows) == 0 {
		if *read.Total == 0 {
			return http.StatusOK
		}
		return http.StatusPartialContent
	}
	start := query.Offset
	end := start + uint64(len(read.Rows)) - 1
	if start == 0 && end+1 == uint64(*read.Total) {
		return http.StatusOK
	}
	return http.StatusPartialContent
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
