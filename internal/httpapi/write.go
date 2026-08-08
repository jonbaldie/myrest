package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// UpsertResolution is one Prefer resolution value of the parity target.
type UpsertResolution int

const (
	// UpsertMergeDuplicates maps to Prefer: resolution=merge-duplicates.
	UpsertMergeDuplicates UpsertResolution = iota
	// UpsertIgnoreDuplicates maps to Prefer: resolution=ignore-duplicates.
	UpsertIgnoreDuplicates
)

// Writer inserts, updates, deletes, and upserts rows as one database role.
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
	// Upsert writes one row by primary key. inserted is true when MySQL created
	// the row and false when it updated or ignored an existing row.
	Upsert(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		row map[string]any,
		primaryKey []string,
		resolution UpsertResolution,
	) (inserted bool, err error)
}

const (
	// codeBadBody is the parity-target code for a bad JSON write body.
	codeBadBody = "PGRST102"
	// codePutPrimaryKey is the parity-target code for a PUT that is not a
	// single-row primary-key upsert.
	codePutPrimaryKey = "PGRST105"
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

// putTable answers PUT /<table>?pk=eq.value: one-row upsert by primary key.
// Prefer resolution selects merge-duplicates (default) or ignore-duplicates.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) putTable(writer http.ResponseWriter, request *http.Request) {
	resolution, ok := parseUpsertResolution(writer, request)
	if !ok {
		return
	}
	role, asked, table, ok := s.lookupPutTable(writer, request, resolution)
	if !ok {
		return
	}
	row, primaryKey, ok := readPutRow(writer, request, s.cache, asked)
	if !ok {
		return
	}

	inserted, err := s.writer.Upsert(
		request.Context(), role, table, row, primaryKey, resolution,
	)
	if err != nil {
		s.log.Printf("myrest: upsert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	if inserted {
		writeMinimal(writer, http.StatusCreated)
		return
	}
	writeMinimal(writer, http.StatusNoContent)
}

// lookupPutTable finds the table for PUT. INSERT is always required.
// merge-duplicates also needs UPDATE so a missing grant stays a privilege
// filter, not a silent write.
func (s *Service) lookupPutTable(
	writer http.ResponseWriter,
	request *http.Request,
	resolution UpsertResolution,
) (schemacache.Role, schemacache.TableID, schemacache.Table, bool) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "INSERT")
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	if resolution == UpsertMergeDuplicates &&
		!s.cache.HasTablePrivilege(role, asked, "UPDATE") {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	return role, asked, table, true
}

// readPutRow validates the primary-key filters and the single JSON object body.
func readPutRow(
	writer http.ResponseWriter,
	request *http.Request,
	cache *schemacache.Cache,
	asked schemacache.TableID,
) (map[string]any, []string, bool) {
	primaryKey, ok := primaryKeyColumns(cache, asked)
	if !ok {
		writeFailure(
			writer,
			http.StatusBadRequest,
			codePutPrimaryKey,
			"PUT needs a primary key on the table",
		)
		return nil, nil, false
	}
	query, err := parseMutateQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return nil, nil, false
	}
	pkValues, ok := putPrimaryKeyValues(writer, query, primaryKey)
	if !ok {
		return nil, nil, false
	}
	row, ok := readPutObject(writer, request)
	if !ok {
		return nil, nil, false
	}
	if !rowMatchesPrimaryKey(writer, row, pkValues) {
		return nil, nil, false
	}
	return row, primaryKey, true
}

func parseUpsertResolution(writer http.ResponseWriter, request *http.Request) (UpsertResolution, bool) {
	value, held := preferValue(request, "resolution")
	if !held || value == "" {
		return UpsertMergeDuplicates, true
	}
	switch strings.ToLower(value) {
	case "merge-duplicates":
		return UpsertMergeDuplicates, true
	case "ignore-duplicates":
		return UpsertIgnoreDuplicates, true
	default:
		writeFailure(
			writer,
			http.StatusBadRequest,
			codeParseFailure,
			"Prefer resolution must be merge-duplicates or ignore-duplicates",
		)
		return 0, false
	}
}

func preferValue(request *http.Request, name string) (string, bool) {
	for _, header := range request.Header.Values("Prefer") {
		for _, part := range strings.Split(header, ",") {
			key, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if !found {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(key), name) {
				return strings.TrimSpace(value), true
			}
		}
	}
	return "", false
}

func primaryKeyColumns(cache *schemacache.Cache, id schemacache.TableID) ([]string, bool) {
	for _, key := range cache.KeysOf(id) {
		if strings.EqualFold(key.Kind, "PRIMARY") && len(key.Columns) > 0 {
			return append([]string(nil), key.Columns...), true
		}
	}
	return nil, false
}

func putPrimaryKeyValues(
	writer http.ResponseWriter,
	query readquery.Query,
	primaryKey []string,
) (map[string]string, bool) {
	if len(query.Groups) != 0 || len(query.Filters) != len(primaryKey) {
		writePutPrimaryKeyFailure(writer)
		return nil, false
	}
	values := make(map[string]string, len(primaryKey))
	for _, filter := range query.Filters {
		if filter.Op != readquery.OpEq || filter.Negated || filter.Path != nil {
			writePutPrimaryKeyFailure(writer)
			return nil, false
		}
		values[filter.Column] = fmt.Sprint(filter.Value)
	}
	for _, column := range primaryKey {
		if _, held := values[column]; !held {
			writePutPrimaryKeyFailure(writer)
			return nil, false
		}
	}
	return values, true
}

func rowMatchesPrimaryKey(
	writer http.ResponseWriter,
	row map[string]any,
	pkValues map[string]string,
) bool {
	for column, want := range pkValues {
		got, held := row[column]
		if !held {
			writePutPrimaryKeyFailure(writer)
			return false
		}
		if fmt.Sprint(got) != want {
			writePutPrimaryKeyFailure(writer)
			return false
		}
	}
	return true
}

func writePutPrimaryKeyFailure(writer http.ResponseWriter) {
	writeFailure(
		writer,
		http.StatusBadRequest,
		codePutPrimaryKey,
		"PUT filters must name all and only primary key columns with eq",
	)
}

func readPutObject(writer http.ResponseWriter, request *http.Request) (map[string]any, bool) {
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
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "PUT body must be one JSON object")
		return nil, false
	}
	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
		return nil, false
	}
	if row == nil || len(row) == 0 {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	return row, true
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
