package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
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
		options writequery.Options,
	) (writequery.Result, error)
	Update(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		patch map[string]any,
		query readquery.Query,
		options writequery.Options,
	) (writequery.Result, error)
	Delete(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		query readquery.Query,
		options writequery.Options,
	) (writequery.Result, error)
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
	prefer, ok := s.readWritePrefer(writer, request)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, prefer, false)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, prefer, primaryKey, writeKindInsert)
	if !ok {
		return
	}

	bodyRows, ok := readInsertRows(writer, request)
	if !ok {
		return
	}

	result, err := s.writer.Insert(request.Context(), role, table, bodyRows, options)
	if err != nil {
		s.log.Printf("myrest: insert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: prefer, Method: http.MethodPost, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan,
	})
}

// patchTable answers PATCH /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) patchTable(writer http.ResponseWriter, request *http.Request) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "UPDATE")
	if !ok {
		return
	}
	prefer, ok := s.readWritePrefer(writer, request)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, prefer, true)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, prefer, primaryKey, writeKindPatch)
	if !ok {
		return
	}

	patch, ok := readPatchObject(writer, request)
	if !ok {
		return
	}
	result, err := s.writer.Update(request.Context(), role, table, patch, query, options)
	if err != nil {
		s.log.Printf("myrest: update %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: prefer, Method: http.MethodPatch, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan,
	})
}

// deleteTable answers DELETE /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) deleteTable(writer http.ResponseWriter, request *http.Request) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, "DELETE")
	if !ok {
		return
	}
	prefer, ok := s.readWritePrefer(writer, request)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, prefer, true)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, prefer, primaryKey, writeKindDelete)
	if !ok {
		return
	}

	result, err := s.writer.Delete(request.Context(), role, table, query, options)
	if err != nil {
		s.log.Printf("myrest: delete %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: prefer, Method: http.MethodDelete, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan,
	})
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

func (s *Service) readWritePrefer(writer http.ResponseWriter, request *http.Request) (writePrefer, bool) {
	prefer, err := parseWritePrefer(request.Header.Values("Prefer"))
	if err != nil {
		var invalid invalidPreferError
		if errors.As(err, &invalid) {
			writeInvalidPrefer(writer, invalid)
			return writePrefer{}, false
		}
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, err.Error())
		return writePrefer{}, false
	}
	return prefer, true
}

// writeKind selects which honesty rules apply for return=representation.
type writeKind int

const (
	writeKindInsert writeKind = iota
	writeKindPatch
	writeKindDelete
)

// buildWriteOptions checks representation honesty and builds database options.
func (s *Service) buildWriteOptions(
	writer http.ResponseWriter,
	role schemacache.Role,
	asked schemacache.TableID,
	prefer writePrefer,
	primaryKey []string,
	kind writeKind,
) (writequery.Options, bool) {
	options := writequery.Options{
		PrimaryKey:     primaryKey,
		MissingDefault: prefer.MissingDefault,
	}
	if prefer.Strict && prefer.MaxAffected != nil {
		options.MaxAffected = prefer.MaxAffected
	}

	switch prefer.Return {
	case returnHeadersOnly:
		options.ReturnKeys = true
	case returnRepresentation:
		if !s.canReturnRepresentation(kind, primaryKey) {
			writeUnsupportedFeature(writer, representationLimitMessage(kind))
			return writequery.Options{}, false
		}
		if !s.cache.HasTablePrivilege(role, asked, "SELECT") {
			writeUnsupportedFeature(
				writer,
				"Prefer return=representation needs SELECT to return affected rows honestly",
			)
			return writequery.Options{}, false
		}
		options.ReturnRepresentation = true
		options.ReturnKeys = true
	}
	return options, true
}

func (s *Service) canReturnRepresentation(kind writeKind, primaryKey []string) bool {
	switch kind {
	case writeKindInsert, writeKindPatch:
		return len(primaryKey) > 0
	case writeKindDelete:
		return true
	default:
		return false
	}
}

func representationLimitMessage(kind writeKind) string {
	switch kind {
	case writeKindInsert:
		return "Prefer return=representation needs a primary key to return inserted rows honestly"
	case writeKindPatch:
		return "Prefer return=representation needs a primary key to return updated rows honestly"
	default:
		return "Prefer return=representation cannot return affected rows honestly"
	}
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
	primaryKey := schemacache.PrimaryKeyOf(cache.KeysOf(asked))
	if len(primaryKey) == 0 {
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
	if !s.cache.IsWritable(asked) {
		writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, "The view is not updatable")
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	return role, asked, table, true
}

func refuseUnbounded(writer http.ResponseWriter, prefer writePrefer, query readquery.Query) bool {
	if !unboundedWrite(query) || prefer.AllRows {
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

// writeOutcome holds the Prefer, method, and representation pieces of one
// ordinary write response.
type writeOutcome struct {
	Prefer     writePrefer
	Method     string
	TableName  string
	PrimaryKey []string
	Result     writequery.Result
	Query      readquery.Query
	Plan       []plannedEmbed
}

// parseWriteQuery reads the mutate query, optional unbounded-write gate, and
// embed plan for Prefer return=representation.
func (s *Service) parseWriteQuery(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	origin schemacache.TableID,
	prefer writePrefer,
	boundRequired bool,
) (readquery.Query, []plannedEmbed, bool) {
	query, err := parseMutateQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if boundRequired && refuseUnbounded(writer, prefer, query) {
		return readquery.Query{}, nil, false
	}
	plan, ok := s.planWriteEmbeds(writer, role, origin, prefer, query)
	if !ok {
		return readquery.Query{}, nil, false
	}
	return query, plan, true
}

// planWriteEmbeds resolves nested select relationships before a write when
// Prefer return=representation asks for an embed. A missing relationship
// refuses here so myrest never invents one and never writes on a bad select.
func (s *Service) planWriteEmbeds(
	writer http.ResponseWriter,
	role schemacache.Role,
	origin schemacache.TableID,
	prefer writePrefer,
	query readquery.Query,
) ([]plannedEmbed, bool) {
	if prefer.Return != returnRepresentation || len(query.Embeds) == 0 {
		return nil, true
	}
	plan, err := s.planEmbeds(role, origin, query.Embeds)
	if err != nil {
		if writeEmbedPlanFailure(writer, err) {
			return nil, false
		}
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, err.Error())
		return nil, false
	}
	return plan, true
}

func (s *Service) shapeWriteRepresentation(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	set []rows.Row,
	query readquery.Query,
	plan []plannedEmbed,
) ([]rows.Row, error) {
	if set == nil {
		set = []rows.Row{}
	}
	if len(plan) > 0 {
		nested, err := s.nestEmbeds(ctx, role, table, set, plan)
		if err != nil {
			return nil, err
		}
		set = nested
	}
	return readquery.Project(set, query)
}

func (s *Service) writeWriteResponse(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	table schemacache.Table,
	outcome writeOutcome,
) {
	setPreferenceApplied(writer, outcome.Prefer)

	switch outcome.Prefer.Return {
	case returnRepresentation:
		status := http.StatusOK
		if outcome.Method == http.MethodPost {
			status = http.StatusCreated
			if location := locationHeader(outcome.TableName, outcome.PrimaryKey, outcome.Result.Keys); location != "" {
				writer.Header().Set("Location", location)
			}
		}
		shaped, err := s.shapeWriteRepresentation(
			request.Context(), role, table, outcome.Result.Rows, outcome.Query, outcome.Plan,
		)
		if err != nil {
			s.writeReadFailure(writer, table.ID, role, err)
			return
		}
		writeJSON(writer, status, shaped)
	case returnHeadersOnly:
		if location := locationHeader(outcome.TableName, outcome.PrimaryKey, outcome.Result.Keys); location != "" {
			writer.Header().Set("Location", location)
		}
		if outcome.Method == http.MethodPost {
			writer.WriteHeader(http.StatusCreated)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		if outcome.Method == http.MethodPost {
			writer.WriteHeader(http.StatusCreated)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}
}

func locationHeader(tableName string, primaryKey []string, keys []map[string]any) string {
	if len(primaryKey) == 0 || len(keys) != 1 {
		return ""
	}
	key := keys[0]
	parts := make([]string, 0, len(primaryKey))
	for _, column := range primaryKey {
		value, held := key[column]
		if !held || value == nil {
			return ""
		}
		parts = append(parts, column+"=eq."+locationValue(value))
	}
	return "/" + tableName + "?" + strings.Join(parts, "&")
}

func locationValue(value any) string {
	switch typed := value.(type) {
	case string:
		return url.QueryEscape(typed)
	case []byte:
		return url.QueryEscape(string(typed))
	default:
		return url.QueryEscape(fmt.Sprint(typed))
	}
}

func (s *Service) writeWriteFailure(writer http.ResponseWriter, err error) {
	var maxErr writequery.MaxAffectedExceeded
	if errors.As(err, &maxErr) {
		writeMaxAffected(writer, maxAffectedError{Affected: maxErr.Affected, Max: maxErr.Max})
		return
	}
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
