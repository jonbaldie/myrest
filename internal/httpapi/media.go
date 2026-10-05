package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
)

const (
	// codeUnsupportedMedia is Accept negotiation with no claimed media type.
	codeUnsupportedMedia = "PGRST107"
	// codeSingularObject is Accept singular when the result is not one row.
	codeSingularObject = "PGRST116"
)

// requestRepresentation negotiates Accept for row data or writes the refusal.
func requestRepresentation(writer http.ResponseWriter, request *http.Request) (representation.Spec, bool) {
	spec, err := representation.Negotiate(request.Header.Values("Accept"))
	var refusal representation.UnsupportedMedia
	if errors.As(err, &refusal) {
		writeUnsupportedMedia(writer, refusal)
		return representation.Spec{}, false
	}
	return spec, true
}

func writeUnsupportedMedia(writer http.ResponseWriter, refusal representation.UnsupportedMedia) {
	writeFailure(writer, http.StatusUnsupportedMediaType, codeUnsupportedMedia, refusal.Error())
}

// refuseRequestMedia answers PGRST107 for every Accept media type of the
// request, also the claimed ones, when the result cannot take that shape.
func refuseRequestMedia(writer http.ResponseWriter, request *http.Request) {
	writeUnsupportedMedia(writer, representation.Refuse(request.Header.Values("Accept")))
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

// writeRows answers row data in the negotiated representation. A singular
// object that does not hold one row answers PGRST116 instead.
func writeRows(
	writer http.ResponseWriter,
	status int,
	spec representation.Spec,
	bodyRows []rows.Row,
	csvHeader []string,
) {
	if err := representation.ValidateCardinality(spec, len(bodyRows)); err != nil {
		writeSingularObjectFailure(writer, len(bodyRows))
		return
	}
	writer.Header().Set("Content-Type", spec.ContentType)
	writer.WriteHeader(status)
	_ = representation.Format(writer, spec, bodyRows, csvHeader)
}

// csvHeaderNames is the CSV header of a result: the row columns, or the
// projected names of an explicit select list when no row came back.
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
