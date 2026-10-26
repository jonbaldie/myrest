package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jonbaldie/myrest/internal/httpapi/httperror"
)

// The error codes of the PostgREST envelope and the myrest gap family. The
// codes of the shared error translation are in httperror.
const (
	// codeNoTable is the code of the parity target for a table the schema
	// cache does not hold.
	codeNoTable = "PGRST205"
	// codeAggregatesDisabled is the parity-target code when aggregates are off.
	codeAggregatesDisabled = "PGRST123"
	// codeSpreadAggregate is the parity-target code for aggregates inside a
	// to-many or many-to-many spread embed.
	codeSpreadAggregate = "PGRST127"
	// codeNoHandler says that the current myrest surface has no handler for
	// the request path and method.
	codeNoHandler = "MYREST003"
)

const (
	msgAggregatesDisabled  = "Use of aggregate functions is not allowed"
	msgSpreadAggregate     = "Feature not implemented"
	detailsSpreadAggregate = "Aggregates are not implemented for one-to-many or many-to-many spreads."
	msgSpreadNotSupported  = "PostgREST spread embeds are not available with MySQL"
)

// writeFailure answers with the error envelope. Only myrest writes the
// message: what the database says goes to the log of the operator.
func writeFailure(writer http.ResponseWriter, status int, code, message string) {
	writeFailureExtra(writer, status, code, message, nil, nil)
}

// writeFailureExtra answers with details and hint set.
func writeFailureExtra(writer http.ResponseWriter, status int, code, message string, details, hint any) {
	writeEnvelope(writer, status, httperror.Envelope{Code: code, Message: message, Details: details, Hint: hint})
}

// writeEnvelope answers with one error envelope. An error envelope names no
// profile, as in the parity target, so it drops Content-Profile.
func writeEnvelope(writer http.ResponseWriter, status int, envelope httperror.Envelope) {
	writer.Header().Del(headerContentProfile)
	writeJSON(writer, status, envelope)
}

// writeErrorFailure answers a request-independent domain or database error
// through the shared translation, without disclosing database account data.
func writeErrorFailure(writer http.ResponseWriter, err error) {
	writeTranslated(writer, httperror.From(err))
}

func writeTranslated(writer http.ResponseWriter, failure httperror.Failure) {
	writeEnvelope(writer, failure.Status, failure.Envelope)
}

// writeUnsupportedFeature answers a documented PostgreSQL semantic gap.
func writeUnsupportedFeature(writer http.ResponseWriter, message string) {
	writeFailure(writer, http.StatusBadRequest, httperror.CodePostgresOnlyFeature, message)
}

// writeNoHandler answers a path or method outside the current service surface.
func writeNoHandler(writer http.ResponseWriter, _ *http.Request) {
	writeFailure(
		writer,
		http.StatusNotFound,
		codeNoHandler,
		"The requested path and method are not available",
	)
}

// writeJSON answers with an application/json body (errors, scalars, OpenAPI
// helpers). Row-data Accept negotiation uses writeRows instead.
func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
