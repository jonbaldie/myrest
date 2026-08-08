package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// codeNoRoutine is the parity-target code for a routine the schema cache does
// not hold for the active database role.
const codeNoRoutine = "PGRST202"

// messageRowSetFeaturesRequired is the stable refusal when read features land
// on a scalar or non-tabular RPC result (rpc-006).
const messageRowSetFeaturesRequired = "Filter, order, pagination, and embed need a row-set RPC result"

// CallOptions carries Prefer-driven RPC behaviour into the database layer.
type CallOptions struct {
	// PreferTx is Prefer: tx=commit|rollback when the client sent it.
	PreferTx string
}

// Caller runs a routine as one database role with named JSON arguments.
type Caller interface {
	Call(
		ctx context.Context,
		role schemacache.Role,
		routine schemacache.RoutineFact,
		args map[string]any,
		options CallOptions,
	) (any, error)
}

// callRoutine answers POST /rpc/<name>: named JSON body arguments, optional
// read features on the query string for row-set results.
func (s *Service) callRoutine(writer http.ResponseWriter, request *http.Request) {
	args, ok := readNamedJSONArgs(writer, request)
	if !ok {
		return
	}
	role, asked, routine, ok := s.lookupRoutine(writer, request)
	if !ok {
		return
	}
	query, err := parseReadQuery(request, s.settings.DB.MaxRows)
	if err != nil {
		writeQueryFailure(writer, err)
		return
	}
	s.invokeRoutine(writer, request, role, asked, routine, args, query)
}

// getRoutine answers GET /rpc/<name>: named query-string arguments for the
// routine parameters, and the remaining query keys as read features when the
// routine is read-safe under MySQL SQL_DATA_ACCESS.
func (s *Service) getRoutine(writer http.ResponseWriter, request *http.Request) {
	role, asked, routine, ok := s.lookupRoutine(writer, request)
	if !ok {
		return
	}
	if !routine.ReadSafe() {
		writeFailure(
			writer,
			http.StatusBadRequest,
			codePostgresOnlyFeature,
			"Only a read-safe routine can be called with GET",
		)
		return
	}
	args, readValues := splitRPCQuery(routine, request.URL.Query())
	query, err := readquery.Parse(readValues, request.Header.Values("Prefer"))
	if err != nil {
		writeQueryFailure(writer, err)
		return
	}
	if s.settings.DB.MaxRows.Capped {
		rows := uint64(s.settings.DB.MaxRows.Rows)
		query.MaxRows = &rows
	}
	s.invokeRoutine(writer, request, role, asked, routine, args, query)
}

func (s *Service) lookupRoutine(
	writer http.ResponseWriter,
	request *http.Request,
) (schemacache.Role, schemacache.RoutineID, schemacache.RoutineFact, bool) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return "", schemacache.RoutineID{}, schemacache.RoutineFact{}, false
	}
	header := headerContentProfile
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		header = headerAcceptProfile
	}
	database, ok := s.requestDatabase(writer, request, header)
	if !ok {
		return "", schemacache.RoutineID{}, schemacache.RoutineFact{}, false
	}
	asked := schemacache.RoutineID{
		Database: database,
		Name:     request.PathValue("name"),
	}
	routine, isResource := s.cache.Routine(role, asked)
	if !isResource {
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return "", schemacache.RoutineID{}, schemacache.RoutineFact{}, false
	}
	return role, asked, routine, true
}

func (s *Service) invokeRoutine(
	writer http.ResponseWriter,
	request *http.Request,
	role schemacache.Role,
	asked schemacache.RoutineID,
	routine schemacache.RoutineFact,
	args map[string]any,
	query readquery.Query,
) {
	if _, missing := missingRequiredArgument(routine, args); missing {
		// The parity target treats a signature mismatch as a missing routine.
		writeFailure(writer, http.StatusNotFound, codeNoRoutine, noRoutineMessage(asked))
		return
	}

	// RPC only honours Prefer: tx= from the write Prefer parser; other write
	// Prefer tokens are accepted for strict handling but not applied on /rpc.
	prefer, ok := s.readWritePrefer(writer, request)
	if !ok {
		return
	}
	preferTx := prefer.Tx
	result, err := s.caller.Call(
		request.Context(),
		role,
		routine,
		args,
		CallOptions{PreferTx: preferTx},
	)
	if err != nil {
		s.log.Printf("myrest: rpc %s.%s as %s: %v", asked.Database, asked.Name, role, err)
		writeDatabaseFailure(writer, err)
		return
	}

	setTxPreferenceApplied(writer, preferTx, s.settings.DB.TxEnd)
	set, tabular := rowSetResult(result)
	if readquery.HasRowSetFeatures(query) && !tabular {
		writeUnsupportedFeature(writer, messageRowSetFeaturesRequired)
		return
	}
	if !tabular {
		writeJSON(writer, http.StatusOK, result)
		return
	}

	read, err := s.shapeRPCRowSet(request.Context(), role, asked.Database, set, query)
	if err != nil {
		s.writeReadFailure(writer, schemacache.TableID{Database: asked.Database, Name: asked.Name}, role, err)
		return
	}
	writeRead(writer, request.Method == http.MethodHead, query, read)
}

func (s *Service) shapeRPCRowSet(
	ctx context.Context,
	role schemacache.Role,
	database string,
	set []rows.Row,
	query readquery.Query,
) (readquery.Result, error) {
	shaped, err := readquery.Shape(set, query)
	if err != nil {
		return readquery.Result{}, err
	}
	if len(query.Embeds) > 0 {
		origin, err := s.rowSetOrigin(role, database, shaped.Rows, query.Embeds)
		if err != nil {
			return readquery.Result{}, err
		}
		plan, err := s.planEmbeds(role, origin.ID, query.Embeds)
		if err != nil {
			return readquery.Result{}, err
		}
		nested, err := s.nestEmbeds(ctx, role, origin, shaped.Rows, plan)
		if err != nil {
			return readquery.Result{}, err
		}
		shaped.Rows = nested
	}
	projected, err := readquery.Project(shaped.Rows, query)
	if err != nil {
		return readquery.Result{}, err
	}
	shaped.Rows = projected
	return shaped, nil
}

// rowSetOrigin finds the one table resource that can own the embed graph for
// this row set: same database, SELECT for the role, every result column on the
// table, and a successful embed plan whose join columns the rows hold.
func (s *Service) rowSetOrigin(
	role schemacache.Role,
	database string,
	set []rows.Row,
	embeds []readquery.Embed,
) (schemacache.Table, error) {
	columns := rowSetColumns(set)
	matches, firstMissing := s.collectRowSetOrigins(role, database, columns, set, embeds)
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return noRowSetOrigin(database, embeds, firstMissing)
	default:
		return tightestColumnCover(matches, len(columns)), nil
	}
}

func (s *Service) collectRowSetOrigins(
	role schemacache.Role,
	database string,
	columns []string,
	set []rows.Row,
	embeds []readquery.Embed,
) ([]schemacache.Table, error) {
	var matches []schemacache.Table
	var firstMissing error
	for _, id := range schemacache.TableIDs(s.cache) {
		table, ok := s.candidateRowSetOrigin(role, database, id, columns)
		if !ok {
			continue
		}
		plan, err := s.planEmbeds(role, table.ID, embeds)
		if err != nil {
			if firstMissing == nil {
				firstMissing = err
			}
			continue
		}
		if rowsHoldOriginKeys(set, plan) {
			matches = append(matches, table)
		}
	}
	return matches, firstMissing
}

func (s *Service) candidateRowSetOrigin(
	role schemacache.Role,
	database string,
	id schemacache.TableID,
	columns []string,
) (schemacache.Table, bool) {
	if id.Database != database {
		return schemacache.Table{}, false
	}
	table, ok := s.cache.Resource(role, id)
	if !ok || !tableHasColumns(table, columns) {
		return schemacache.Table{}, false
	}
	return table, true
}

func noRowSetOrigin(database string, embeds []readquery.Embed, firstMissing error) (schemacache.Table, error) {
	if firstMissing != nil {
		return schemacache.Table{}, firstMissing
	}
	target := ""
	if len(embeds) > 0 {
		target = embeds[0].Resource
	}
	return schemacache.Table{}, schemacache.RelationshipMissing{
		Origin: schemacache.TableID{Database: database, Name: "rpc"},
		Target: target,
	}
}

func tightestColumnCover(matches []schemacache.Table, columnCount int) schemacache.Table {
	best := matches[0]
	bestExtra := len(best.Columns) - columnCount
	for _, candidate := range matches[1:] {
		extra := len(candidate.Columns) - columnCount
		if extra < bestExtra {
			best = candidate
			bestExtra = extra
		}
	}
	return best
}

func rowSetColumns(set []rows.Row) []string {
	if len(set) == 0 {
		return nil
	}
	return append([]string(nil), set[0].Columns...)
}

func tableHasColumns(table schemacache.Table, names []string) bool {
	have := map[string]bool{}
	for _, column := range table.Columns {
		have[column.Name] = true
	}
	for _, name := range names {
		if !have[name] {
			return false
		}
	}
	return true
}

func rowsHoldOriginKeys(set []rows.Row, plan []plannedEmbed) bool {
	if len(set) == 0 {
		return true
	}
	have := map[string]bool{}
	for _, column := range set[0].Columns {
		have[column] = true
	}
	for _, embed := range plan {
		for _, column := range embed.relationship.OriginColumns {
			if !have[column] {
				return false
			}
		}
	}
	return true
}

// rowSetResult reports whether the caller answer is a tabular row set.
func rowSetResult(result any) ([]rows.Row, bool) {
	set, ok := result.([]rows.Row)
	if !ok {
		return nil, false
	}
	return set, true
}

// splitRPCQuery takes IN/INOUT parameter names as arguments and leaves the
// remaining query keys for ordinary-read parsing.
func splitRPCQuery(routine schemacache.RoutineFact, values url.Values) (map[string]any, url.Values) {
	args := map[string]any{}
	readValues := url.Values{}
	for key, list := range values {
		readValues[key] = append([]string(nil), list...)
	}
	for _, param := range routine.Parameters {
		if param.Ordinal == 0 || param.Name == "" {
			continue
		}
		if strings.EqualFold(param.Mode, "OUT") {
			continue
		}
		if list, held := readValues[param.Name]; held && len(list) > 0 {
			args[param.Name] = list[0]
			delete(readValues, param.Name)
		}
	}
	return args, readValues
}

// missingRequiredArgument finds an IN or INOUT argument the body does not name.
func missingRequiredArgument(routine schemacache.RoutineFact, args map[string]any) (string, bool) {
	for _, param := range routine.Parameters {
		if param.Ordinal == 0 || param.Name == "" {
			continue
		}
		if strings.EqualFold(param.Mode, "OUT") {
			continue
		}
		if _, held := args[param.Name]; !held {
			return param.Name, true
		}
	}
	return "", false
}

// readNamedJSONArgs reads the PostgREST named-argument object. An empty body
// means no arguments. Unusual whole-body argument modes are not supported and
// refuse with MYREST001 (see docs/rpc-body-modes.md).
func readNamedJSONArgs(writer http.ResponseWriter, request *http.Request) (map[string]any, bool) {
	defer func() { _ = request.Body.Close() }()

	if message, refused := refusedWholeBodyMode(request.Header.Get("Content-Type")); refused {
		writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, message)
		return nil, false
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, "Could not read the request body")
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, true
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, "Could not parse the JSON body")
		return nil, false
	}
	args, isObject := decoded.(map[string]any)
	if !isObject {
		writeFailure(
			writer,
			http.StatusBadRequest,
			codePostgresOnlyFeature,
			"A single unnamed JSON RPC argument is not supported",
		)
		return nil, false
	}
	return args, true
}

// refusedWholeBodyMode reports a stable refusal for PostgREST single-unnamed
// bytea, text, and xml body modes. JSON whole-body is detected after parse.
func refusedWholeBodyMode(contentType string) (string, bool) {
	mediaType, _, _ := strings.Cut(contentType, ";")
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "application/octet-stream":
		return "A single unnamed bytea RPC argument is not supported", true
	case "text/plain":
		return "A single unnamed text RPC argument is not supported", true
	case "text/xml", "application/xml":
		return "A single unnamed xml RPC argument is not supported", true
	default:
		return "", false
	}
}

// noRoutineMessage names the routine the client asked for, the way the parity
// target does for a missing function.
func noRoutineMessage(asked schemacache.RoutineID) string {
	return "Could not find the function " + asked.Database + "." + asked.Name + " in the schema cache"
}
