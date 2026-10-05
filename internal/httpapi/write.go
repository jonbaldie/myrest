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
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
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
		options writequery.Options,
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
func (s *Service) insertTable(writer http.ResponseWriter, request *http.Request, preferences prefer.Preferences) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, preferences, "INSERT")
	if !ok {
		return
	}
	written, ok := s.readWritePrefer(writer, preferences, writeKindInsert)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, written, writeBoundOptional)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, written, primaryKey, writeKindInsert)
	if !ok {
		return
	}

	if !applyInsertResolution(writer, request, &written, &options) {
		return
	}

	bodyRows, repr, ok := readInsertRowsAndRepr(writer, request, written)
	if !ok {
		return
	}
	options.Validate = validateRepresentation(query, repr)

	result, err := s.writer.Insert(request.Context(), role, table, bodyRows, options)
	if err != nil {
		s.log.Printf("myrest: insert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: written, Method: http.MethodPost, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan, Repr: repr,
	})
}

// validateRepresentation builds the in-unit validation of one representation
// write: a singular-object Accept needs exactly one row, and the client
// select list must project. The unit runs it before commit, so a refusal
// (406 PGRST116 for the row count) rolls the write back (issue #175).
func validateRepresentation(
	query readquery.Query,
	repr representation.Spec,
) func(writequery.Result) error {
	if repr.Kind != representation.KindSingularObject && (query.SelectAll || len(query.Columns) == 0) {
		return nil
	}
	return func(result writequery.Result) error {
		if err := representation.ValidateCardinality(repr, len(result.Rows)); err != nil {
			return err
		}
		_, err := readquery.Project(result.Rows, query)
		return err
	}
}

// writeRepresentationPrecheck negotiates Accept for a representation response
// before the write unit runs, so a refused Accept header commits no data. It
// answers only when the response would claim a representation.
func writeRepresentationPrecheck(
	writer http.ResponseWriter,
	request *http.Request,
	written writePrefer,
) (representation.Spec, bool) {
	if written.Return != returnRepresentation {
		return representation.Spec{}, true
	}
	return requestRepresentation(writer, request)
}

// readInsertRowsAndRepr reads the JSON insert rows and, when the response
// claims a representation, the negotiated Accept media type.
func readInsertRowsAndRepr(
	writer http.ResponseWriter,
	request *http.Request,
	written writePrefer,
) ([]map[string]any, representation.Spec, bool) {
	rows, ok := readInsertRows(writer, request)
	if !ok {
		return nil, representation.Spec{}, false
	}
	repr, ok := writeRepresentationPrecheck(writer, request, written)
	if !ok {
		return nil, representation.Spec{}, false
	}
	return rows, repr, true
}

// applyInsertResolution sets the POST duplicate-key mode from Prefer
// resolution. With no resolution a POST stays a plain INSERT.
func applyInsertResolution(
	writer http.ResponseWriter,
	request *http.Request,
	written *writePrefer,
	options *writequery.Options,
) bool {
	if written.Resolution == "" && !written.BadResolution {
		return true
	}
	resolution, ok := parseUpsertResolution(writer, written.Preferences)
	if !ok {
		return false
	}
	if resolution == UpsertIgnoreDuplicates && options.ReturnRepresentation {
		writeUnsupportedFeature(
			writer,
			"Prefer return=representation cannot tell inserted rows from ignored rows honestly",
		)
		return false
	}
	options.OnDuplicate = writequery.DuplicateMerges
	if resolution == UpsertIgnoreDuplicates {
		options.OnDuplicate = writequery.DuplicateIgnored
	}
	written.applied = append(written.applied, appliedResolutionTokens(written.Preferences)...)
	return true
}

func appliedResolutionTokens(preferences prefer.Preferences) []string {
	if preferences.Resolution == "" {
		return nil
	}
	return []string{"resolution=" + preferences.Resolution}
}

// patchTable answers PATCH /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) patchTable(writer http.ResponseWriter, request *http.Request, preferences prefer.Preferences) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, preferences, "UPDATE")
	if !ok {
		return
	}
	written, ok := s.readWritePrefer(writer, preferences, writeKindPatch)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, written, writeBoundRequired)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, written, primaryKey, writeKindPatch)
	if !ok {
		return
	}

	patch, ok := readPatchObject(writer, request)
	if !ok {
		return
	}
	repr, ok := writeRepresentationPrecheck(writer, request, written)
	if !ok {
		return
	}
	options.Validate = validateRepresentation(query, repr)
	result, err := s.writer.Update(request.Context(), role, table, patch, query, options)
	if err != nil {
		s.log.Printf("myrest: update %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: written, Method: http.MethodPatch, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan, Repr: repr,
	})
}

// deleteTable answers DELETE /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) deleteTable(writer http.ResponseWriter, request *http.Request, preferences prefer.Preferences) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, preferences, "DELETE")
	if !ok {
		return
	}
	written, ok := s.readWritePrefer(writer, preferences, writeKindDelete)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, role, asked, written, writeBoundRequired)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(s.cache.KeysOf(asked))
	options, ok := s.buildWriteOptions(writer, role, asked, written, primaryKey, writeKindDelete)
	if !ok {
		return
	}

	repr, ok := writeRepresentationPrecheck(writer, request, written)
	if !ok {
		return
	}
	options.Validate = validateRepresentation(query, repr)
	result, err := s.writer.Delete(request.Context(), role, table, query, options)
	if err != nil {
		s.log.Printf("myrest: delete %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	s.writeWriteResponse(writer, request, role, table, writeOutcome{
		Prefer: written, Method: http.MethodDelete, TableName: asked.Name,
		PrimaryKey: primaryKey, Result: result, Query: query, Plan: plan, Repr: repr,
	})
}

// putTable answers PUT /<table>?pk=eq.value: one-row upsert by primary key.
// Prefer resolution selects merge-duplicates (default) or ignore-duplicates.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) putTable(writer http.ResponseWriter, request *http.Request, preferences prefer.Preferences) {
	written, ok := s.readWritePrefer(writer, preferences, writeKindPut)
	if !ok {
		return
	}
	resolution, ok := parseUpsertResolution(writer, preferences)
	if !ok {
		return
	}
	role, asked, table, ok := s.lookupPutTable(writer, request, preferences, resolution)
	if !ok {
		return
	}
	row, primaryKey, ok := readPutRow(writer, request, s.cache, asked)
	if !ok {
		return
	}
	options, ok := s.buildWriteOptions(writer, role, asked, written, primaryKey, writeKindPut)
	if !ok {
		return
	}

	inserted, err := s.writer.Upsert(
		request.Context(),
		role,
		table,
		row,
		primaryKey,
		resolution,
		options,
	)
	if err != nil {
		s.log.Printf("myrest: upsert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	written.applied = append(written.applied, appliedResolutionTokens(written.Preferences)...)
	setPreferenceApplied(writer, written)
	if inserted {
		writeMinimal(writer, http.StatusCreated)
		return
	}
	writeMinimal(writer, http.StatusNoContent)
}

// readWritePrefer refuses invalid preferences under handling=strict and
// builds the write view of the parsed preferences.
func (s *Service) readWritePrefer(
	writer http.ResponseWriter,
	preferences prefer.Preferences,
	kind writeKind,
) (writePrefer, bool) {
	if refuseInvalidPrefer(writer, preferences, kind.surface()) {
		return writePrefer{}, false
	}
	return newWritePrefer(preferences, s.settings.DB.TxEnd, kind), true
}

// writeKind selects which honesty rules apply for return=representation and
// which write preferences apply.
type writeKind int

const (
	writeKindInsert writeKind = iota
	writeKindPatch
	writeKindDelete
	writeKindPut
	// writeKindRPC marks the /rpc surface, which applies the write
	// preference tx= only.
	writeKindRPC
)

// surface is the Prefer surface of the write kind.
func (kind writeKind) surface() prefer.Surface {
	if kind == writeKindRPC {
		return prefer.SurfaceRPC
	}
	return prefer.SurfaceWrite
}

// honoursMaxAffected reports whether the write kind enforces Prefer
// max-affected. Updates, deletes, and upserts refuse with PGRST124 when they
// exceed the limit; inserts write normally.
func honoursMaxAffected(kind writeKind) bool {
	switch kind {
	case writeKindPatch, writeKindDelete, writeKindPut:
		return true
	default:
		return false
	}
}

// buildWriteOptions checks representation honesty and builds database options.
func (s *Service) buildWriteOptions(
	writer http.ResponseWriter,
	role schemacache.Role,
	asked schemacache.TableID,
	written writePrefer,
	primaryKey []string,
	kind writeKind,
) (writequery.Options, bool) {
	options := writequery.Options{
		PrimaryKey:     primaryKey,
		MissingDefault: written.MissingDefault,
		PreferTx:       written.Tx,
	}
	if written.Strict && written.MaxAffected != nil && honoursMaxAffected(kind) {
		options.MaxAffected = written.MaxAffected
	}

	switch written.Return {
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
	case writeKindPut:
		return false
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
	preferences prefer.Preferences,
	resolution UpsertResolution,
) (schemacache.Role, schemacache.TableID, schemacache.Table, bool) {
	role, asked, table, ok := s.lookupWriteTable(writer, request, preferences, "INSERT")
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

func parseUpsertResolution(writer http.ResponseWriter, preferences prefer.Preferences) (UpsertResolution, bool) {
	if preferences.BadResolution {
		writeFailure(
			writer,
			http.StatusBadRequest,
			codeParseFailure,
			"Prefer resolution must be merge-duplicates or ignore-duplicates",
		)
		return 0, false
	}
	if preferences.Resolution == prefer.ResolutionIgnoreDuplicates {
		return UpsertIgnoreDuplicates, true
	}
	return UpsertMergeDuplicates, true
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
		if !primaryKeyValueMatches(got, want) {
			writePutPrimaryKeyFailure(writer)
			return false
		}
	}
	return true
}

func primaryKeyValueMatches(got any, want string) bool {
	if fmt.Sprint(got) == want {
		return true
	}
	switch value := got.(type) {
	case float64:
		if strconv.FormatFloat(value, 'f', -1, 64) == want {
			return true
		}
		if wantNum, err := strconv.ParseFloat(want, 64); err == nil {
			return value == wantNum
		}
	case int64:
		if strconv.FormatInt(value, 10) == want {
			return true
		}
	}
	return false
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
	if err := decodeJSONNumber(body, &row); err != nil {
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
	preferences prefer.Preferences,
	privilege string,
) (schemacache.Role, schemacache.TableID, schemacache.Table, bool) {
	role, ok := s.requestRole(writer, request, preferences)
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	if s.writer == nil {
		writeNoHandler(writer, request)
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	requested, ok := s.selectResource(
		writer, request, role, headerContentProfile, request.PathValue("table"),
	)
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	table, ok := s.admitWriteResource(writer, requested, privilege)
	if !ok {
		return "", schemacache.TableID{}, schemacache.Table{}, false
	}
	return requested.role, requested.table(), table, true
}

func refuseUnbounded(writer http.ResponseWriter, written writePrefer, query readquery.Query) bool {
	if !unboundedWrite(query) || written.AllRows {
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
	values, err := requestQuery(request)
	if err != nil {
		return readquery.Query{}, err
	}
	return readquery.Parse(values, readquery.CountNone)
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
	var rows []map[string]any
	if trimmed[0] == '[' {
		if err := decodeJSONNumber(body, &rows); err != nil {
			writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
			return nil, false
		}
		if len(rows) == 0 {
			writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty JSON array")
			return nil, false
		}
	} else {
		var row map[string]any
		if err := decodeJSONNumber(body, &row); err != nil {
			writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
			return nil, false
		}
		rows = []map[string]any{row}
	}
	if !hasInsertColumns(rows) {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	return rows, true
}

func hasInsertColumns(rows []map[string]any) bool {
	for _, row := range rows {
		if len(row) > 0 {
			return true
		}
	}
	return false
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
	if err := decodeJSONNumber(body, &patch); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Could not parse the JSON body")
		return nil, false
	}
	if patch == nil || len(patch) == 0 {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "Empty body")
		return nil, false
	}
	return patch, true
}

func decodeJSONNumber(data []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing data")
	}
	return nil
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
	// Repr is the Accept negotiation the write checked before the write unit
	// ran; the response reuses it instead of negotiating again.
	Repr representation.Spec
}

// writeBound says whether PATCH/DELETE must have a filter or Prefer: all-rows.
type writeBound bool

const (
	writeBoundOptional writeBound = false
	writeBoundRequired writeBound = true
)

// parseWriteQuery reads the mutate query, optional unbounded-write gate, and
// embed plan for Prefer return=representation.
func (s *Service) parseWriteQuery(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	origin schemacache.TableID,
	written writePrefer,
	bound writeBound,
) (readquery.Query, []plannedEmbed, bool) {
	query, err := parseMutateQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if bound == writeBoundRequired && refuseUnbounded(writer, written, query) {
		return readquery.Query{}, nil, false
	}
	plan, ok := s.planWriteEmbeds(writer, role, origin, written, query)
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
	written writePrefer,
	query readquery.Query,
) ([]plannedEmbed, bool) {
	if written.Return != returnRepresentation || len(query.Embeds) == 0 {
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
		s.writeRepresentationResponse(writer, request, role, table, outcome)
	case returnHeadersOnly:
		writeEmptyWriteResponse(writer, outcome, true)
	default:
		writeEmptyWriteResponse(writer, outcome, false)
	}
}

func (s *Service) writeRepresentationResponse(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	table schemacache.Table,
	outcome writeOutcome,
) {
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
	writeRows(writer, status, outcome.Repr, shaped, csvHeaderNames(outcome.Query, shaped))
}

func writeEmptyWriteResponse(writer http.ResponseWriter, outcome writeOutcome, headersOnly bool) {
	if headersOnly {
		if location := locationHeader(outcome.TableName, outcome.PrimaryKey, outcome.Result.Keys); location != "" {
			writer.Header().Set("Location", location)
		}
	}
	if outcome.Method == http.MethodPost {
		writer.WriteHeader(http.StatusCreated)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
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
	case float64:
		return url.QueryEscape(strconv.FormatFloat(typed, 'f', -1, 64))
	case json.Number:
		return url.QueryEscape(typed.String())
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
	var refusal representation.SingularObjectRefusal
	if errors.As(err, &refusal) {
		writeSingularObjectFailure(writer, refusal.RowCount)
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
