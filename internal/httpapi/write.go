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

	"github.com/jonbaldie/myrest/internal/httpapi/httperror"
	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writeexec"
	"github.com/jonbaldie/myrest/internal/writequery"
)

type Writer = writeexec.Executor

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
	role, asked, admission, ok := s.lookupWriteTable(writer, request, preferences, "INSERT")
	if !ok {
		return
	}
	snapshot, table := admission.snapshot, admission.table
	written, ok := s.readWritePrefer(writer, preferences, writeKindInsert)
	if !ok {
		return
	}
	query, plan, ok := s.parseInsertQuery(writer, request, snapshot, role, asked, written)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(schemacache.KeysFrom(snapshot, asked))
	ok = s.checkWriteRepresentation(writer, snapshot, role, asked, preferences, primaryKey, writeexec.Insert)
	if !ok {
		return
	}

	if _, ok := parseUpsertResolution(writer, preferences); !ok {
		return
	}

	bodyRows, repr, ok := readInsertRowsAndRepr(writer, request, written)
	if !ok {
		return
	}
	outcome, err := s.writer.Execute(request.Context(), writeexec.Intent{Action: writeexec.Insert, Role: role, Target: table, Payload: bodyRows, Filter: query, Preferences: preferences, PrimaryKey: primaryKey, CanSelect: schemacache.HasTablePrivilegeFrom(snapshot, role, asked, "SELECT"), Representation: repr})
	result := writequery.Result{Affected: outcome.Affected, Rows: outcome.Rows, Keys: outcome.Keys}
	written.applied = outcome.Applied
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
	if !written.MissingDefault && !matchingInsertKeys(rows) {
		writeFailure(writer, http.StatusBadRequest, codeBadBody, "All object keys must match")
		return nil, representation.Spec{}, false
	}
	repr, ok := writeRepresentationPrecheck(writer, request, written)
	if !ok {
		return nil, representation.Spec{}, false
	}
	return rows, repr, true
}

// matchingInsertKeys compares key presence, not values: an explicit null is
// still a supplied key. Prefer missing=default permits different key sets.
func matchingInsertKeys(rows []map[string]any) bool {
	first := rows[0]
	for _, row := range rows[1:] {
		if len(row) != len(first) {
			return false
		}
		for key := range first {
			if _, ok := row[key]; !ok {
				return false
			}
		}
	}
	return true
}

// patchTable answers PATCH /<table> with the ordinary-read filter surface.
// Content-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) patchTable(writer http.ResponseWriter, request *http.Request, preferences prefer.Preferences) {
	role, asked, admission, ok := s.lookupWriteTable(writer, request, preferences, "UPDATE")
	if !ok {
		return
	}
	snapshot, table := admission.snapshot, admission.table
	written, ok := s.readWritePrefer(writer, preferences, writeKindPatch)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, snapshot, role, asked, written, writeBoundRequired)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(schemacache.KeysFrom(snapshot, asked))
	ok = s.checkWriteRepresentation(writer, snapshot, role, asked, preferences, primaryKey, writeexec.Update)
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
	outcome, err := s.writer.Execute(request.Context(), writeexec.Intent{Action: writeexec.Update, Role: role, Target: table, Payload: []map[string]any{patch}, Filter: query, Preferences: preferences, PrimaryKey: primaryKey, CanSelect: schemacache.HasTablePrivilegeFrom(snapshot, role, asked, "SELECT"), Representation: repr})
	result := writequery.Result{Affected: outcome.Affected, Rows: outcome.Rows, Keys: outcome.Keys}
	written.applied = outcome.Applied
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
	role, asked, admission, ok := s.lookupWriteTable(writer, request, preferences, "DELETE")
	if !ok {
		return
	}
	snapshot, table := admission.snapshot, admission.table
	written, ok := s.readWritePrefer(writer, preferences, writeKindDelete)
	if !ok {
		return
	}
	query, plan, ok := s.parseWriteQuery(writer, request, snapshot, role, asked, written, writeBoundRequired)
	if !ok {
		return
	}

	primaryKey := schemacache.PrimaryKeyOf(schemacache.KeysFrom(snapshot, asked))
	ok = s.checkWriteRepresentation(writer, snapshot, role, asked, preferences, primaryKey, writeexec.Delete)
	if !ok {
		return
	}

	repr, ok := writeRepresentationPrecheck(writer, request, written)
	if !ok {
		return
	}
	outcome, err := s.writer.Execute(request.Context(), writeexec.Intent{Action: writeexec.Delete, Role: role, Target: table, Filter: query, Preferences: preferences, PrimaryKey: primaryKey, CanSelect: schemacache.HasTablePrivilegeFrom(snapshot, role, asked, "SELECT"), Representation: repr})
	result := writequery.Result{Affected: outcome.Affected, Rows: outcome.Rows, Keys: outcome.Keys}
	written.applied = outcome.Applied
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
	role, asked, admission, ok := s.lookupPutTable(writer, request, preferences, resolution)
	if !ok {
		return
	}
	snapshot, table := admission.snapshot, admission.table
	row, primaryKey, ok := s.readPutRow(writer, request, snapshot, asked)
	if !ok {
		return
	}
	ok = s.checkWriteRepresentation(writer, snapshot, role, asked, preferences, primaryKey, writeexec.Upsert)
	if !ok {
		return
	}

	outcome, err := s.writer.Execute(request.Context(), writeexec.Intent{Action: writeexec.Upsert, Role: role, Target: table, Payload: []map[string]any{row}, Preferences: preferences, PrimaryKey: primaryKey})
	if err != nil {
		s.log.Printf("myrest: upsert %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		s.writeWriteFailure(writer, err)
		return
	}
	written.applied = outcome.Applied
	setPreferenceApplied(writer, written)
	if outcome.Inserted {
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

func (s *Service) writeWriteFailure(writer http.ResponseWriter, err error) {
	var maxErr writequery.MaxAffectedExceeded
	if errors.As(err, &maxErr) {
		writeEnvelope(writer, httperror.MaxAffected(maxErr))
		return
	}
	var refusal representation.SingularObjectRefusal
	if errors.As(err, &refusal) {
		writeSingularObjectFailure(writer, refusal.RowCount)
		return
	}
	writeTranslatedError(writer, err)
}

func (s *Service) checkWriteRepresentation(writer http.ResponseWriter, snapshot schemacache.Snapshot, role schemacache.Role, asked schemacache.TableID, preferences prefer.Preferences, primaryKey []string, action writeexec.Action) bool {
	err := writeexec.CheckRepresentation(writeexec.Intent{Action: action, PrimaryKey: primaryKey, Preferences: preferences, CanSelect: schemacache.HasTablePrivilegeFrom(snapshot, role, asked, "SELECT")})
	if err != nil {
		s.writeWriteFailure(writer, err)
		return false
	}
	return true
}

// lookupPutTable finds the table for PUT. INSERT is always required.
// merge-duplicates also needs UPDATE so a missing grant stays a privilege
// filter, not a silent write.
func (s *Service) lookupPutTable(
	writer http.ResponseWriter,
	request *http.Request,
	preferences prefer.Preferences,
	resolution writequery.OnDuplicate,
) (schemacache.Role, schemacache.TableID, admissionResult, bool) {
	role, asked, admission, ok := s.lookupWriteTable(writer, request, preferences, "INSERT")
	if !ok {
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	if resolution == writequery.DuplicateMerges &&
		!schemacache.HasTablePrivilegeFrom(admission.snapshot, role, asked, "UPDATE") {
		writeFailure(writer, http.StatusNotFound, codeNoTable, noTableMessage(asked))
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	return role, asked, admission, true
}

// readPutRow validates the primary-key filters and the single JSON object body.
func (s *Service) readPutRow(
	writer http.ResponseWriter,
	request *http.Request,
	snapshot schemacache.Snapshot,
	asked schemacache.TableID,
) (map[string]any, []string, bool) {
	primaryKey := schemacache.PrimaryKeyOf(schemacache.KeysFrom(snapshot, asked))
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
	if s.refuseDisabledAggregates(writer, query) {
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

func parseUpsertResolution(writer http.ResponseWriter, preferences prefer.Preferences) (writequery.OnDuplicate, bool) {
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
		return writequery.DuplicateIgnored, true
	}
	return writequery.DuplicateMerges, true
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
// The admission it returns carries the snapshot the whole write answers from.
func (s *Service) lookupWriteTable(
	writer http.ResponseWriter,
	request *http.Request,
	preferences prefer.Preferences,
	privilege string,
) (schemacache.Role, schemacache.TableID, admissionResult, bool) {
	role, ok := s.requestRole(writer, request, preferences)
	if !ok {
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	if s.writer == nil {
		writeNoHandler(writer, request)
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	requested, ok := s.selectResource(writer, request, role, request.PathValue("table"))
	if !ok {
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	admission, ok := s.admitWriteResource(writer, requested, privilege)
	if !ok {
		return "", schemacache.TableID{}, admissionResult{}, false
	}
	return requested.role, requested.table(), admission, true
}

func refuseUnbounded(writer http.ResponseWriter, written writePrefer, query readquery.Query) bool {
	if !writeexec.UnboundedFilter(query) || written.AllRows {
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

// parseInsertQuery reads the mutate query for POST. The on_conflict query
// option is recognized before generic filter parsing and refused with MYREST001,
// because MySQL cannot target specific conflict constraints. Issue #153.
func (s *Service) parseInsertQuery(
	writer http.ResponseWriter,
	request *http.Request,
	snapshot schemacache.Snapshot,
	role schemacache.Role,
	origin schemacache.TableID,
	written writePrefer,
) (readquery.Query, []plannedEmbed, bool) {
	values, err := requestQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if refuseWriteColumns(writer, values) {
		return readquery.Query{}, nil, false
	}
	if values.Has("on_conflict") {
		writeUnsupportedFeature(writer, "PostgREST on_conflict is not available with MySQL")
		return readquery.Query{}, nil, false
	}
	query, err := readquery.Parse(values, readquery.CountNone)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if s.refuseDisabledAggregates(writer, query) {
		return readquery.Query{}, nil, false
	}
	plan, ok := planWriteEmbeds(writer, snapshot, role, origin, written, query)
	if !ok {
		return readquery.Query{}, nil, false
	}
	return query, plan, true
}

// refuseWriteColumns prevents writes of body keys that columns would exclude.
// Column selection is not supported; never silently discard it. Issue #265.
func refuseWriteColumns(writer http.ResponseWriter, values url.Values) bool {
	if !values.Has("columns") {
		return false
	}
	writeUnsupportedFeature(writer, "PostgREST columns on POST and PATCH is not supported")
	return true
}

// boundedWriteOptions are the top-level query options that would bound or
// sort a PATCH or DELETE. myrest does not apply them to the MySQL write, so it
// refuses them with MYREST001 instead of changing every matching row. Issue #210.
var boundedWriteOptions = []string{"limit", "offset", "order"}

func refuseMutationOptions(writer http.ResponseWriter, method string, values url.Values) bool {
	if method == http.MethodPatch && refuseWriteColumns(writer, values) {
		return true
	}
	for _, option := range boundedWriteOptions {
		if values.Has(option) {
			writeUnsupportedFeature(
				writer,
				"PostgREST limit, offset, and order on PATCH and DELETE are not available with MySQL",
			)
			return true
		}
	}
	return false
}

// parseWriteQuery reads the mutate query, optional unbounded-write gate, and
// embed plan for Prefer return=representation. Top-level limit, offset, and
// order refuse before generic query parsing.
func (s *Service) parseWriteQuery(
	writer http.ResponseWriter,
	request *http.Request,
	snapshot schemacache.Snapshot,
	role schemacache.Role,
	origin schemacache.TableID,
	written writePrefer,
	bound writeBound,
) (readquery.Query, []plannedEmbed, bool) {
	values, err := requestQuery(request)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if refuseMutationOptions(writer, request.Method, values) {
		return readquery.Query{}, nil, false
	}
	query, err := readquery.Parse(values, readquery.CountNone)
	if err != nil {
		writeQueryFailure(writer, err)
		return readquery.Query{}, nil, false
	}
	if s.refuseDisabledAggregates(writer, query) {
		return readquery.Query{}, nil, false
	}
	if bound == writeBoundRequired && refuseUnbounded(writer, written, query) {
		return readquery.Query{}, nil, false
	}
	plan, ok := planWriteEmbeds(writer, snapshot, role, origin, written, query)
	if !ok {
		return readquery.Query{}, nil, false
	}
	return query, plan, true
}

// planWriteEmbeds resolves nested select relationships before a write when
// Prefer return=representation asks for an embed. A missing relationship
// refuses here so myrest never invents one and never writes on a bad select.
func planWriteEmbeds(
	writer http.ResponseWriter,
	snapshot schemacache.Snapshot,
	role schemacache.Role,
	origin schemacache.TableID,
	written writePrefer,
	query readquery.Query,
) ([]plannedEmbed, bool) {
	if written.Return != returnRepresentation || len(query.Embeds) == 0 {
		return nil, true
	}
	plan, err := planEmbeds(snapshot, role, origin, query.Embeds)
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
	s.setContentProfile(writer, request, table.ID.Database)
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
