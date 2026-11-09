// Package httperror owns the error envelope of the parity target and the
// translation of request-independent domain and database errors to an HTTP
// status and an envelope. Refusals that need the request (an asked table or
// routine name, the Accept header, auth challenges) stay with their handlers.
package httperror

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/writequery"
)

// The error codes that the translation gives.
const (
	// CodeNoColumn is the parity-target code for a missing column.
	CodeNoColumn = "PGRST204"
	// CodeSingularObject is Accept singular when the result is not one row.
	CodeSingularObject = "PGRST116"
	// CodeMaxAffected is Prefer max-affected under handling=strict.
	CodeMaxAffected = "PGRST124"
	// CodePostgresOnlyFeature says that the request needs PostgREST behavior
	// that myrest cannot provide over MySQL.
	CodePostgresOnlyFeature = "MYREST001"
	// CodeMySQLDatabaseFailure says that MySQL returned an error for which
	// myrest cannot claim PostgreSQL semantics.
	CodeMySQLDatabaseFailure = "MYREST002"
)

const msgDatabaseFailure = "The database did not answer the request"

// Envelope is the error envelope of the parity target. Every field is
// written, and details and hint stay null until a ticket gives them a value.
type Envelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
	Hint    any    `json:"hint"`
}

// Failure is the status and the envelope of one error answer.
type Failure struct {
	Status   int
	Envelope Envelope
	// Database is true when the error is a database or unknown error. Only
	// myrest writes its message: what the database says goes to the log of
	// the operator.
	Database bool
}

// From translates a request-independent domain or database error. Any error
// it does not know is a database failure with the status of MySQLStatus.
func From(err error) Failure {
	var missing readquery.ColumnNotFound
	if errors.As(err, &missing) {
		return failure(http.StatusBadRequest, CodeNoColumn, missing.Error(), nil)
	}
	var gap readquery.UnsupportedFeature
	if errors.As(err, &gap) {
		return failure(http.StatusBadRequest, CodePostgresOnlyFeature, gap.Message, nil)
	}
	var refusal representation.SingularObjectRefusal
	if errors.As(err, &refusal) {
		return SingularObject(refusal.RowCount)
	}
	var maxErr writequery.MaxAffectedExceeded
	if errors.As(err, &maxErr) {
		details := fmt.Sprintf("The query affects %d rows", maxErr.Affected)
		return failure(http.StatusBadRequest, CodeMaxAffected, maxErr.Error(), details)
	}
	var invalid rows.InvalidJSON
	if errors.As(err, &invalid) {
		return failure(http.StatusBadRequest, CodeMySQLDatabaseFailure, invalid.Error(), nil)
	}
	database := failure(MySQLStatus(err), CodeMySQLDatabaseFailure, msgDatabaseFailure, nil)
	database.Database = true
	return database
}

// SingularObject is the refusal of Accept singular for a result that does not
// hold one row.
func SingularObject(rowCount int) Failure {
	return failure(
		http.StatusNotAcceptable,
		CodeSingularObject,
		"Cannot coerce the result to a single JSON object",
		fmt.Sprintf("The result contains %d rows", rowCount),
	)
}

func failure(status int, code, message string, details any) Failure {
	return Failure{
		Status:   status,
		Envelope: Envelope{Code: code, Message: message, Details: details},
	}
}

// MySQLStatus maps the supported MySQL errors to their HTTP status. Any error
// outside the published table gives the documented internal-error fallback.
func MySQLStatus(err error) int {
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
	case 1044, 1045, 1142, 1143, 1227, 1370:
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
