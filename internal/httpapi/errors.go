package httpapi

import (
	"encoding/json"
	"net/http"
)

// The error codes the anonymous read needs. The full code contract comes with
// the error envelope slice.
const (
	// codeNoTable is the code of the parity target for a table the schema
	// cache does not hold.
	codeNoTable = "PGRST205"
	// codeNoAnonymousRole says that the request carries no usable JWT and
	// that no anonymous database role is configured.
	codeNoAnonymousRole = "PGRST301"
	// codeDatabaseFailure says that the database refused the read.
	codeDatabaseFailure = "PGRST000"
)

// failure is the error envelope of the parity target. Every field is written,
// and details and hint stay null until a ticket gives them a value.
type failure struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Details *string `json:"details"`
	Hint    *string `json:"hint"`
}

// writeFailure answers with the error envelope. Only myrest writes the
// message: what the database says goes to the log of the operator.
func writeFailure(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, failure{Code: code, Message: message})
}

// writeJSON answers with a JSON body. The parity target needs no content
// negotiation for row data, so JSON is the only representation here.
func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
