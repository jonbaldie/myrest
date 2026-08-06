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
// and details and hint are null when myrest has nothing to add.
type failure struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Details *string `json:"details"`
	Hint    *string `json:"hint"`
}

// writeFailure answers with the error envelope. The message is what myrest
// says; details carry what the database said, and are null when the failure
// comes from myrest alone.
func writeFailure(writer http.ResponseWriter, status int, code, message string, details error) {
	body := failure{Code: code, Message: message}
	if details != nil {
		said := details.Error()
		body.Details = &said
	}
	writeJSON(writer, status, body)
}

// writeJSON answers with a JSON body. The parity target needs no content
// negotiation for row data, so JSON is the only representation here.
func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
