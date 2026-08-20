package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Claimed response media types for row data. OpenAPI stays on GET /.
const (
	mediaJSON       = "application/json"
	mediaArrayJSON  = "application/vnd.pgrst.array+json"
	mediaObjectJSON = "application/vnd.pgrst.object+json"
	mediaCSV        = "text/csv"
	mediaCSVCharset = "text/csv; charset=utf-8"

	// codeUnsupportedMedia is Accept negotiation with no claimed media type.
	codeUnsupportedMedia = "PGRST107"
	// codeSingularObject is Accept singular when the result is not one row.
	codeSingularObject = "PGRST116"
)

// representation is the claimed response shape for row data.
type representation struct {
	kind        representationKind
	contentType string
}

type representationKind int

const (
	representationJSONArray representationKind = iota
	representationJSONObject
	representationCSV
)

// requestRepresentation negotiates Accept for row data or writes the refusal.
func requestRepresentation(writer http.ResponseWriter, request *http.Request) (representation, bool) {
	repr, err := negotiateRepresentation(request.Header.Values("Accept"))
	if err != nil {
		writeUnsupportedMedia(writer, err.(*unsupportedMediaError))
		return representation{}, false
	}
	return repr, true
}

// negotiateRepresentation picks a claimed Accept media type for row data.
// An empty Accept, application/json, application/vnd.pgrst.array+json, and
// */* claim the JSON array. Object and CSV are claimed. Every other type
// refuses with PGRST107.
func negotiateRepresentation(acceptHeaders []string) (representation, error) {
	preferences := acceptMediaPreferences(acceptHeaders)
	if len(preferences) == 0 {
		return representation{
			kind:        representationJSONArray,
			contentType: mediaJSON,
		}, nil
	}

	var selected representation
	quality := -1.0
	for _, preference := range preferences {
		if preference.quality == 0 {
			continue
		}
		if chosen, ok := claimRepresentation(preference.mediaType); ok && preference.quality > quality {
			selected = chosen
			quality = preference.quality
		}
	}
	if quality >= 0 {
		return selected, nil
	}
	return representation{}, &unsupportedMediaError{offered: acceptMediaTypes(acceptHeaders)}
}

type mediaPreference struct {
	mediaType string
	quality   float64
}

func acceptMediaPreferences(headers []string) []mediaPreference {
	var preferences []mediaPreference
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			mediaType, quality, ok := parseMediaPreference(part)
			if !ok {
				continue
			}
			preferences = append(preferences, mediaPreference{
				mediaType: mediaType,
				quality:   quality,
			})
		}
	}
	return preferences
}

func parseMediaPreference(part string) (mediaType string, quality float64, ok bool) {
	parts := strings.Split(part, ";")
	mediaType = strings.ToLower(strings.TrimSpace(parts[0]))
	if mediaType == "" {
		return "", 0, false
	}
	quality = 1
	for _, parameter := range parts[1:] {
		name, value, hasValue := strings.Cut(strings.TrimSpace(parameter), "=")
		if !hasValue || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(parsed) || parsed < 0 || parsed > 1 {
			return mediaType, 0, true
		}
		quality = parsed
	}
	return mediaType, quality, true
}

func acceptMediaTypes(headers []string) []string {
	var offered []string
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
			mediaType = strings.ToLower(mediaType)
			if mediaType == "" {
				continue
			}
			offered = append(offered, mediaType)
		}
	}
	return offered
}

func claimRepresentation(mediaType string) (representation, bool) {
	switch mediaType {
	case mediaJSON, "*/*":
		return representation{kind: representationJSONArray, contentType: mediaJSON}, true
	case mediaArrayJSON, "application/vnd.pgrst.array":
		return representation{kind: representationJSONArray, contentType: mediaArrayJSON}, true
	case mediaObjectJSON, "application/vnd.pgrst.object":
		return representation{
			kind:        representationJSONObject,
			contentType: mediaObjectJSON,
		}, true
	case mediaCSV:
		return representation{kind: representationCSV, contentType: mediaCSVCharset}, true
	default:
		return representation{}, false
	}
}

type unsupportedMediaError struct {
	offered []string
}

func (e *unsupportedMediaError) Error() string {
	return "None of these media types are available: " + strings.Join(e.offered, ", ")
}

func writeUnsupportedMedia(writer http.ResponseWriter, err *unsupportedMediaError) {
	writeFailure(writer, http.StatusUnsupportedMediaType, codeUnsupportedMedia, err.Error())
}

func writeSingularObjectFailure(writer http.ResponseWriter, rowCount int) {
	writeFailureExtra(
		writer,
		http.StatusNotAcceptable,
		codeSingularObject,
		"Cannot coerce the result to a single JSON object",
		fmt.Sprintf("The result contains %d rows", rowCount),
		nil,
	)
}

func writeRows(
	writer http.ResponseWriter,
	status int,
	repr representation,
	bodyRows []rows.Row,
	csvHeader []string,
) {
	switch repr.kind {
	case representationJSONObject:
		if len(bodyRows) != 1 {
			writeSingularObjectFailure(writer, len(bodyRows))
			return
		}
		writeJSONWithType(writer, status, repr.contentType, bodyRows[0])
	case representationCSV:
		writeCSV(writer, status, csvHeader, bodyRows)
	default:
		writeJSONWithType(writer, status, repr.contentType, bodyRows)
	}
}

func writeJSONWithType(writer http.ResponseWriter, status int, contentType string, body any) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeCSV(writer http.ResponseWriter, status int, header []string, bodyRows []rows.Row) {
	writer.Header().Set("Content-Type", mediaCSVCharset)
	writer.WriteHeader(status)
	if len(header) == 0 && len(bodyRows) > 0 {
		header = bodyRows[0].Columns
	}
	if len(header) == 0 {
		return
	}
	csvWriter := csv.NewWriter(writer)
	_ = csvWriter.Write(header)
	for _, row := range bodyRows {
		record := make([]string, len(header))
		for i := range header {
			record[i] = csvCell(row.Values, i)
		}
		_ = csvWriter.Write(record)
	}
	csvWriter.Flush()
}

func csvHeaderNames(query readquery.Query, bodyRows []rows.Row) []string {
	if len(bodyRows) > 0 {
		return bodyRows[0].Columns
	}
	if query.SelectAll || len(query.Columns) == 0 {
		return nil
	}
	names := make([]string, len(query.Columns))
	for i, column := range query.Columns {
		names[i] = column.ResultName()
	}
	return names
}

func csvCell(values []any, index int) string {
	if index >= len(values) || values[index] == nil {
		return ""
	}
	return csvValue(values[index])
}

func csvValue(value any) string {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return csvEncoded(value)
	}
}

func csvEncoded(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	if len(encoded) >= 2 && encoded[0] == '"' {
		var unquoted string
		if json.Unmarshal(encoded, &unquoted) == nil {
			return unquoted
		}
	}
	return string(encoded)
}
