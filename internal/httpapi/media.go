package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
		var media *unsupportedMediaError
		if errors.As(err, &media) {
			writeUnsupportedMedia(writer, *media)
			return representation{}, false
		}
		writeFailure(writer, http.StatusUnsupportedMediaType, codeUnsupportedMedia, err.Error())
		return representation{}, false
	}
	return repr, true
}

// negotiateRepresentation picks a claimed Accept media type for row data.
// An empty Accept, application/json, application/vnd.pgrst.array+json, and
// */* claim the JSON array. Object and CSV are claimed. Every other type
// refuses with PGRST107.
func negotiateRepresentation(acceptHeaders []string) (representation, error) {
	offered := acceptMediaTypes(acceptHeaders)
	if len(offered) == 0 {
		return representation{
			kind:        representationJSONArray,
			contentType: mediaJSON,
		}, nil
	}
	for _, offeredType := range offered {
		if chosen, ok := claimRepresentation(offeredType); ok {
			return chosen, nil
		}
	}
	return representation{}, &unsupportedMediaError{offered: offered}
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
	case mediaJSON, mediaArrayJSON, "application/vnd.pgrst.array", "*/*":
		return representation{kind: representationJSONArray, contentType: mediaJSON}, true
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

func writeUnsupportedMedia(writer http.ResponseWriter, err unsupportedMediaError) {
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
) {
	switch repr.kind {
	case representationJSONObject:
		if len(bodyRows) != 1 {
			writeSingularObjectFailure(writer, len(bodyRows))
			return
		}
		writeJSONWithType(writer, status, repr.contentType, bodyRows[0])
	case representationCSV:
		writeCSV(writer, status, bodyRows)
	default:
		writeJSONWithType(writer, status, repr.contentType, bodyRows)
	}
}

func writeJSONWithType(writer http.ResponseWriter, status int, contentType string, body any) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeCSV(writer http.ResponseWriter, status int, bodyRows []rows.Row) {
	writer.Header().Set("Content-Type", mediaCSVCharset)
	writer.WriteHeader(status)
	if len(bodyRows) == 0 {
		return
	}
	csvWriter := csv.NewWriter(writer)
	_ = csvWriter.Write(bodyRows[0].Columns)
	for _, row := range bodyRows {
		record := make([]string, len(row.Columns))
		for i := range row.Columns {
			record[i] = csvCell(row.Values, i)
		}
		_ = csvWriter.Write(record)
	}
	csvWriter.Flush()
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
