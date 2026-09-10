package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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
	// codeInvalidRange is the parity-target code for an unsatisfiable Range.
	codeInvalidRange = "PGRST103"
)

// msgInvalidRange is the parity-target message for an unsatisfiable Range.
const msgInvalidRange = "Requested range not satisfiable"

// readTable answers GET and HEAD /<table>: it finds the resource of the active
// database role in the schema cache, and reads under the ordinary-read query.
// Accept-Profile selects the database; with no header the table comes from
// the default database.
func (s *Service) readTable(writer http.ResponseWriter, request *http.Request) {
	role, ok := s.requestRole(writer, request)
	if !ok {
		return
	}
	requested, ok := s.selectResource(
		writer, request, role, headerAcceptProfile, request.PathValue("table"),
	)
	if !ok {
		return
	}

	query, err := parseReadQuery(request, s.settings.DB.MaxRows)
	if err != nil {
		writeQueryFailure(writer, err)
		return
	}
	if readquery.HasAggregates(query) && !s.settings.DB.AggregatesEnabled {
		writeFailure(writer, http.StatusBadRequest, codeAggregatesDisabled, msgAggregatesDisabled)
		return
	}
	table, ok := s.admitReadResource(writer, requested)
	if !ok {
		return
	}

	repr, ok := requestRepresentation(writer, request)
	if !ok {
		return
	}

	read, err := s.readWithEmbeds(request.Context(), requested.role, table, query)
	if err != nil {
		s.writeReadFailure(writer, requested.table(), requested.role, err)
		return
	}
	writeRead(writer, request.Method == http.MethodHead, query, read, repr)
}

func (s *Service) readWithEmbeds(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	plan, err := s.planEmbeds(role, table.ID, query.Embeds)
	if err != nil {
		return readquery.Result{}, err
	}
	query, injected := withJoinColumns(table, query, plan)
	read, err := s.reader.Read(ctx, role, table, query)
	if err != nil {
		return readquery.Result{}, err
	}
	nested, err := s.nestEmbeds(ctx, role, table, read.Rows, plan)
	if err != nil {
		return readquery.Result{}, err
	}
	read.Rows = dropInjectedColumns(nested, injected)
	return read, nil
}

func (s *Service) writeReadFailure(
	writer http.ResponseWriter,
	asked schemacache.TableID,
	role schemacache.Role,
	err error,
) {
	if writeEmbedPlanFailure(writer, err) {
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
	if writeInvalidJSON(writer, err) {
		return
	}
	s.log.Printf("myrest: read %s.%s as %s: %v", asked.Database, asked.Name, role, err)
	writeDatabaseFailure(writer, err)
}

func requestQuery(request *http.Request) (url.Values, error) {
	return url.ParseQuery(request.URL.RawQuery)
}

func parseReadQuery(request *http.Request, maxRows config.RowLimit) (readquery.Query, error) {
	values, err := requestQuery(request)
	if err != nil {
		return readquery.Query{}, err
	}
	query, err := readquery.Parse(values, request.Header.Values("Prefer"))
	if err != nil {
		return readquery.Query{}, err
	}
	if maxRows.Capped {
		rows := uint64(maxRows.Rows)
		query.MaxRows = &rows
	}
	if err := applyRequestRange(request, &query); err != nil {
		return readquery.Query{}, err
	}
	return query, nil
}

// applyRequestRange bounds the query with the Range request header. The
// header bounds GET reads only: other methods ignore it (RFC 9110), so a
// HEAD read keeps the whole window. A header outside the range grammar of
// the parity target is ignored.
func applyRequestRange(request *http.Request, query *readquery.Query) error {
	if request.Method != http.MethodGet {
		return nil
	}
	first, last, ok := parseRangeHeader(request.Header.Get(headerRange))
	if !ok {
		return nil
	}
	return readquery.ApplyRange(query, first, last)
}

// headerRange is the Range request header of the parity target.
const headerRange = "Range"

// parseRangeHeader reads first-last with both ends inclusive and the last
// end optional. It answers false when the value does not match the range
// grammar of the parity target, so the caller can ignore it.
func parseRangeHeader(raw string) (uint64, *uint64, bool) {
	firstText, lastText, found := strings.Cut(raw, "-")
	if !found || firstText == "" || !allDigits(firstText) || !allDigits(lastText) {
		return 0, nil, false
	}
	first, err := strconv.ParseUint(firstText, 10, 64)
	if err != nil {
		return 0, nil, false
	}
	if lastText == "" {
		return first, nil, true
	}
	last, err := strconv.ParseUint(lastText, 10, 64)
	if err != nil {
		return 0, nil, false
	}
	return first, &last, true
}

func allDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func writeQueryFailure(writer http.ResponseWriter, err error) {
	var parse readquery.ParseFailure
	if errors.As(err, &parse) {
		if parse.Gap {
			writeUnsupportedFeature(writer, parse.Message)
			return
		}
		writeFailure(writer, http.StatusBadRequest, codeParseFailure, parse.Message)
		return
	}
	var unsatisfiable readquery.RangeFailure
	if errors.As(err, &unsatisfiable) {
		details := "Limit should be greater than or equal to zero."
		if unsatisfiable.LowerGTUpper {
			details = "The lower boundary must be lower than or equal to the upper boundary in the Range header."
		}
		writeFailureExtra(
			writer,
			http.StatusRequestedRangeNotSatisfiable,
			codeInvalidRange,
			msgInvalidRange,
			details,
			nil,
		)
		return
	}
	writeFailure(writer, http.StatusBadRequest, codeParseFailure, err.Error())
}

func writeRead(
	writer http.ResponseWriter,
	head bool,
	query readquery.Query,
	read readquery.Result,
	repr representation,
) {
	writer.Header().Set("Range-Unit", "items")
	writer.Header().Set("Content-Range", contentRange(query, read))
	status := readStatus(query, read)
	if repr.kind == representationJSONObject && len(read.Rows) != 1 {
		writeSingularObjectFailure(writer, len(read.Rows))
		return
	}
	if head {
		writer.Header().Set("Content-Type", repr.contentType)
		writer.WriteHeader(status)
		return
	}
	writeRows(writer, status, repr, read.Rows, csvHeaderNames(query, read.Rows))
}

func rowRange(query readquery.Query, rowCount int) (start, end uint64) {
	start = query.Offset
	end = start + uint64(rowCount) - 1
	return start, end
}

func contentRange(query readquery.Query, read readquery.Result) string {
	if len(read.Rows) == 0 {
		if read.Total != nil {
			return "*/" + strconv.FormatInt(*read.Total, 10)
		}
		return "*/*"
	}
	start, end := rowRange(query, len(read.Rows))
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
	start, end := rowRange(query, len(read.Rows))
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
