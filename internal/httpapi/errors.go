package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// The error codes of the PostgREST envelope and the myrest gap family.
const (
	// codeNoTable is the code of the parity target for a table the schema
	// cache does not hold.
	codeNoTable = "PGRST205"
	// codeMySQLDatabaseFailure says that MySQL returned an error for which
	// myrest cannot claim PostgreSQL semantics.
	codeMySQLDatabaseFailure = "MYREST002"
	// codeNoHandler says that the current myrest surface has no handler for
	// the request path and method.
	codeNoHandler = "MYREST003"
	// codePostgresOnlyFeature says that the request needs PostgREST behavior
	// that myrest cannot provide over MySQL.
	codePostgresOnlyFeature = "MYREST001"
)

// failure is the error envelope of the parity target. Every field is written,
// and details and hint stay null until a ticket gives them a value.
type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
	Hint    any    `json:"hint"`
}

// writeFailure answers with the error envelope. Only myrest writes the
// message: what the database says goes to the log of the operator.
func writeFailure(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, failure{Code: code, Message: message})
}

// writeFailureExtra answers with details and hint set.
func writeFailureExtra(writer http.ResponseWriter, status int, code, message string, details, hint any) {
	writeJSON(writer, status, failure{Code: code, Message: message, Details: details, Hint: hint})
}

// writeDatabaseFailure answers a database error without disclosing database
// account data. The status comes from the published MySQL SQLSTATE table.
func writeDatabaseFailure(writer http.ResponseWriter, err error) {
	writeFailure(
		writer,
		mysqlErrorStatus(err),
		codeMySQLDatabaseFailure,
		"The database did not answer the request",
	)
}

// writeUnsupportedFeature answers a documented PostgreSQL semantic gap.
func writeUnsupportedFeature(writer http.ResponseWriter, message string) {
	writeFailure(writer, http.StatusBadRequest, codePostgresOnlyFeature, message)
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

// mysqlErrorStatus maps the supported MySQL errors to their HTTP status. Any
// error outside the published table gives the documented internal-error
// fallback.
func mysqlErrorStatus(err error) int {
	var mysqlError *mysql.MySQLError
	if !errors.As(err, &mysqlError) {
		return http.StatusInternalServerError
	}

	return mysqlStatus(mysqlError.Number, string(mysqlError.SQLState[:]))
}

// mysqlStatus is the status table in code. It has no side effects so each
// database error maps to one status regardless of request state.
func mysqlStatus(number uint16, state string) int {
	switch number {
	case 1044, 1045, 1142, 1227:
		return http.StatusForbidden
	case 1062, 1451, 1452, 1213:
		return http.StatusConflict
	}

	switch {
	case strings.HasPrefix(state, "08"):
		return http.StatusServiceUnavailable
	case strings.HasPrefix(state, "22"), strings.HasPrefix(state, "42"):
		return http.StatusBadRequest
	case strings.HasPrefix(state, "23"), strings.HasPrefix(state, "40"):
		return http.StatusConflict
	case strings.HasPrefix(state, "28"):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// writeJSON answers with a JSON body. The parity target needs no content
// negotiation for row data, so JSON is the only representation here.
func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
