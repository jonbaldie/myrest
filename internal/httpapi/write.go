package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Writer inserts, updates, and deletes rows as one database role.
type Writer interface {
	Insert(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		rows []map[string]any,
	) (int, error)
	Update(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		patch map[string]any,
		query readquery.Query,
	) (int64, error)
	Delete(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		query readquery.Query,
	) (int64, error)
}

const (
	// codeBadBody is the parity-target code for a bad JSON write body.
	codeBadBody = "PGRST102"
)

// insertTable answers POST /<table>: one JSON object or a JSON array of objects.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) insertTable(writer http.ResponseWriter, request *http.Request) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "INSERT")
	if !ok {
		return
	}

	rows, ok := readInsertRows(writer, request)
	if !ok {
		return
	}

	_, err := s.writer.Insert(request.Context(), role, table, rows)
	if err != nil {
		s.log.Printf("myrest: insert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	writeMinimal(writer, http.StatusCreated)
}

// patchTable answers PATCH /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) patchTable(writer http.ResponseWriter, request *http.Request) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "UPDATE")
	if !ok {
		return
	}

	query, err := parseMutateQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return
	}
	if refuseUnbounded(writer, request, query) {
		return
	}

	patch, ok := readPatchObject(writer, request)
	if !ok {
		return
	}
	_, err = s.writer.Update(request.Context(), role, table, patch, query)
	if err != nil {
		s.log.Printf("myrest: update %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	writeMinimal(writer, http.StatusNoContent)
}

// deleteTable answers DELETE /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) deleteTable(writer http.ResponseWriter, request *http.Request) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "DELETE")
	if !ok {
		return
	}

	query, err := parseMutateQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return
	}
	if refuseUnbounded(writer, request, query) {
		return
	}

	_, err = s.writer.Delete(request.Context(), role, table, query)
	if err != nil {
		s.log.Printf("myrest: delete %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	writeMinimal(writer, http.StatusNoContent)
}

// lookupWriteTable finds the table for a write under Content-Profile. It also
// checks that a Writer is configured and that the role holds the privilege.
func (s *Service) lookupWriteTable(
	writer http.ResponseWriter,
	request *http.Request,
	privilege string,
) (schemacache.Role, schemacache.TableID, schemacache.Table, bool) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	if s.writer == nil {
		writeNoHandler(writer, request)
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	database, ok := s.requestDatabase(writer, request, headerContentProfile)
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	asked := schemacache.TableID{
		Database: database,
		Name:     request.PathValue("table"),
	}
	table, isResource := s.cache.TableWithPrivilege(role, asked, privilege)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	return role, asked, table, true
}

func refuseUnbounded(writer http.ResponseWriter, request *http.Request, query readquery.Query) bool {
	if !unboundedWrite(query) || preferHolds(request, "all-rows") {
		return false
	}
	writeFailure(
		writer,
		http.StatusBadRequest,
		codeParseFailure,
		"PATCH/DELETE needs a filter or Prefer: all-rows",
	)
	return true
}

func unboundedWrite(query readquery.Query) bool {
	return len(query.Filters) == 0 && len(query.Groups) == 0
}

func parseMutateQuery(request *http.Request) (readquery.Query, error) {
	return readquery.Parse(request.URL.Query(), nil)
}

func readInsertRows(writer http.ResponseWriter, request *http.Request) ([]map[string]any, bool) {
	body, ok := readJSONBody(writer, request)
	if !ok {
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}

	trimmed := bytes.TrimSpace(body)
	if trimmed[0] == '[' {
		var rows []map[string]any
		if err := json.Unmarshal(body, &rows); err != nil {
			writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
			return nil, false
		}
		if len(rows) == 0 {
			writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty JSON array")
			return nil, false
		}
		return rows, true
	}

	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
		return nil, false
	}
	if row == nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	return []map[string]any{row}, true
}

func readPatchObject(writer http.ResponseWriter, request *http.Request) (map[string]any, bool) {
	body, ok := readJSONBody(writer, request)
	if !ok {
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
		return nil, false
	}
	if patch == nil || len(patch) == 0 {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	return patch, true
}

func readJSONBody(writer http.ResponseWriter, request *http.Request) ([]byte, bool) {
	defer func() { _ = request.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not read the request body")
		return nil, false
	}
	return body, true
}

func writeMinimal(writer http.ResponseWriter, status int) {
	writer.WriteHeader(status)
}

func (s *Service) writeWriteFailure(writer http.ResponseWriter, err error) {
	var missing readquery.ColumnNotFound
	if errors.As(err, &missing) {
		writeFailure(writer, http.StatusBadRequest, codeNoColumn, missing.Error())
		return
	}
	var gap readquery.UnsupportedFeature
	if errors.As(err, &gap) {
		writeUnsupportedFeature(writer, gap.Message)
		return
	}
	writeDatabaseFailure(writer, err)
}
