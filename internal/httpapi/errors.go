package httpapi

import (
	"encoding/json"
	"net/http"
)

// The error codes this ticket needs. The ticket "Error envelope, PGRST codes,
// myrest gap codes, and SQLSTATE map" locks the full code contract.
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
// and details and hint are null when myrest has nothing to add.
type failure struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Details *string `json:"details"`
	Hint    *string `json:"hint"`
}

// writeFailure answers with the error envelope.
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
