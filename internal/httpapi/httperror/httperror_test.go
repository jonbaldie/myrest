package httperror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jonbaldie/myrest/internal/httpapi/httperror"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/writequery"
)

func mysqlError(number uint16, state string) *mysql.MySQLError {
	failure := &mysql.MySQLError{Number: number, Message: "secret account data"}
	copy(failure.SQLState[:], state)
	return failure
}

func TestMySQLStatusFollowsThePublishedTable(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"1044 database access denied", mysqlError(1044, "HY000"), http.StatusForbidden},
		{"1045 account access denied", mysqlError(1045, "HY000"), http.StatusForbidden},
		{"1142 table command denied", mysqlError(1142, "HY000"), http.StatusForbidden},
		{"1143 column command denied", mysqlError(1143, "HY000"), http.StatusForbidden},
		{"1227 privilege needed", mysqlError(1227, "HY000"), http.StatusForbidden},
		{"1370 routine command denied", mysqlError(1370, "HY000"), http.StatusForbidden},
		{"1062 duplicate key", mysqlError(1062, "HY000"), http.StatusConflict},
		{"1451 parent row in use", mysqlError(1451, "HY000"), http.StatusConflict},
		{"1452 no parent row", mysqlError(1452, "HY000"), http.StatusConflict},
		{"1213 deadlock", mysqlError(1213, "HY000"), http.StatusConflict},
		{"SQLSTATE 08 connection", mysqlError(2013, "08S01"), http.StatusServiceUnavailable},
		{"SQLSTATE 22 data", mysqlError(1366, "22007"), http.StatusBadRequest},
		{"SQLSTATE 42 syntax or access", mysqlError(1064, "42000"), http.StatusBadRequest},
		{"SQLSTATE 23 integrity", mysqlError(1048, "23000"), http.StatusConflict},
		{"SQLSTATE 40 rollback", mysqlError(1205, "40001"), http.StatusConflict},
		{"SQLSTATE 28 authorization", mysqlError(1698, "28000"), http.StatusForbidden},
		{"other SQLSTATE", mysqlError(1105, "HY000"), http.StatusInternalServerError},
		{"wrapped MySQL error", fmt.Errorf("read: %w", mysqlError(1062, "23000")), http.StatusConflict},
		{"not a MySQL error", errors.New("connection reset"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := httperror.MySQLStatus(tc.err); got != tc.status {
				t.Fatalf("MySQLStatus = %d, want %d", got, tc.status)
			}
		})
	}
}

func TestFromMapsEachDomainError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		status   int
		envelope httperror.Envelope
		database bool
	}{
		{
			name:     "missing column",
			err:      fmt.Errorf("plan: %w", readquery.ColumnNotFound{Name: "nope"}),
			status:   http.StatusBadRequest,
			envelope: httperror.Envelope{Code: "PGRST204", Message: readquery.ColumnNotFound{Name: "nope"}.Error()},
		},
		{
			name:     "PostgreSQL-only feature",
			err:      readquery.UnsupportedFeature{Message: "not with MySQL"},
			status:   http.StatusBadRequest,
			envelope: httperror.Envelope{Code: "MYREST001", Message: "not with MySQL"},
		},
		{
			name:     "invalid JSON",
			err:      rows.InvalidJSON{},
			status:   http.StatusBadRequest,
			envelope: httperror.Envelope{Code: "MYREST002", Message: rows.InvalidJSON{}.Error()},
		},
		{
			name:     "MySQL error",
			err:      mysqlError(1062, "23000"),
			status:   http.StatusConflict,
			envelope: httperror.Envelope{Code: "MYREST002", Message: "The database did not answer the request"},
			database: true,
		},
		{
			name:     "unknown error",
			err:      errors.New("connection reset"),
			status:   http.StatusInternalServerError,
			envelope: httperror.Envelope{Code: "MYREST002", Message: "The database did not answer the request"},
			database: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httperror.From(tc.err)
			if got.Status != tc.status {
				t.Fatalf("Status = %d, want %d", got.Status, tc.status)
			}
			if got.Envelope != tc.envelope {
				t.Fatalf("Envelope = %#v, want %#v", got.Envelope, tc.envelope)
			}
			if got.Database != tc.database {
				t.Fatalf("Database = %t, want %t", got.Database, tc.database)
			}
		})
	}
}

func TestTranslationPrecedence(t *testing.T) {
	missing := readquery.ColumnNotFound{Name: "missing"}
	gap := readquery.UnsupportedFeature{Message: "unsupported"}
	invalid := rows.InvalidJSON{}
	cases := []struct {
		name    string
		got     httperror.Failure
		status  int
		code    string
		message string
	}{
		{"column before unsupported", httperror.From(errors.Join(gap, missing)), 400, "PGRST204", "column not found: missing"},
		{"unsupported before invalid JSON", httperror.From(errors.Join(invalid, gap)), 400, "MYREST001", "unsupported"},
		{"database ignores column", httperror.Database(errors.Join(missing, invalid)), 400, "MYREST002", "JSON column holds invalid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Status != tc.status || tc.got.Envelope != (httperror.Envelope{Code: tc.code, Message: tc.message}) || tc.got.Database {
				t.Fatalf("failure = %#v", tc.got)
			}
		})
	}
}

func TestExtractedRefusals(t *testing.T) {
	cases := []struct {
		name string
		got  httperror.Failure
		want httperror.Failure
	}{
		{"singular", httperror.SingularObject(3), httperror.Failure{Status: 406, Envelope: httperror.Envelope{Code: "PGRST116", Message: "Cannot coerce the result to a single JSON object", Details: "The result contains 3 rows"}}},
		{"max affected", httperror.MaxAffected(writequery.MaxAffectedExceeded{Affected: 5, Max: 2}), httperror.Failure{Status: 400, Envelope: httperror.Envelope{Code: "PGRST124", Message: "Query result exceeds max-affected preference constraint", Details: "The query affects 5 rows"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("failure = %#v, want %#v", tc.got, tc.want)
			}
		})
	}
}

func TestEnvelopeWithoutDetailsOrHintWritesNulls(t *testing.T) {
	body, err := json.Marshal(httperror.Envelope{Code: "PGRST100", Message: "bad"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"code":"PGRST100","message":"bad","details":null,"hint":null}`
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}
