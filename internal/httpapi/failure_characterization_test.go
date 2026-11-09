package httpapi

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/rpcexec"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

func TestFailureWriterCharacterization(t *testing.T) {
	missing := readquery.ColumnNotFound{Name: "missing"}
	invalid := rows.InvalidJSON{}
	tests := []struct {
		name   string
		writer string
		err    error
		status int
		body   string
		log    string
	}{
		{"rpc singular before non-tabular and row-set", "rpc", errors.Join(rpcexec.RowSetFeaturesRefusal{}, rpcexec.NonTabularRepresentationRefusal{}, representation.SingularObjectRefusal{RowCount: 2}), 406, `{"code":"PGRST116","message":"Cannot coerce the result to a single JSON object","details":"The result contains 2 rows","hint":null}` + "\n", ""},
		{"rpc non-tabular before row-set", "rpc", errors.Join(rpcexec.RowSetFeaturesRefusal{}, rpcexec.NonTabularRepresentationRefusal{}), 415, `{"code":"PGRST107","message":"None of these media types are available: application/json","details":null,"hint":null}` + "\n", ""},
		{"rpc row-set", "rpc", rpcexec.RowSetFeaturesRefusal{}, 400, `{"code":"MYREST001","message":"Filter, order, pagination, and embed need a row-set RPC result","details":null,"hint":null}` + "\n", ""},
		{"write max affected before column", "write", errors.Join(missing, writequery.MaxAffectedExceeded{Affected: 5, Max: 2}), 400, `{"code":"PGRST124","message":"Query result exceeds max-affected preference constraint","details":"The query affects 5 rows","hint":null}` + "\n", ""},
		{"read column before invalid JSON", "read", errors.Join(invalid, missing), 400, `{"code":"PGRST204","message":"column not found: missing","details":null,"hint":null}` + "\n", ""},
		{"read invalid JSON", "read", invalid, 400, `{"code":"MYREST002","message":"JSON column holds invalid JSON","details":null,"hint":null}` + "\n", ""},
		{"read unknown database error", "read", errors.New("secret database error"), 500, `{"code":"MYREST002","message":"The database did not answer the request","details":null,"hint":null}` + "\n", "myrest: read shop.items as reader: secret database error\n"},
		{"database translation", "database", &mysql.MySQLError{Number: 1062, Message: "secret database error"}, 409, `{"code":"MYREST002","message":"The database did not answer the request","details":null,"hint":null}` + "\n", ""},
		{"database invalid JSON", "database", invalid, 400, `{"code":"MYREST002","message":"JSON column holds invalid JSON","details":null,"hint":null}` + "\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			s := &Service{log: log.New(&logs, "", 0)}
			w := httptest.NewRecorder()
			w.Header().Set("Content-Profile", "shop")
			w.Header().Set("X-Keep", "value")
			switch tt.writer {
			case "rpc":
				r := httptest.NewRequest(http.MethodPost, "/rpc/test", nil)
				r.Header.Set("Accept", "application/json")
				writeRPCCallFailure(w, r, schemacache.RoutineID{}, tt.err)
			case "write":
				s.writeWriteFailure(w, tt.err)
			case "read":
				s.writeReadFailure(w, schemacache.TableID{Database: "shop", Name: "items"}, schemacache.Role("reader"), tt.err)
			case "database":
				writeDatabaseFailure(w, tt.err)
			}
			if w.Code != tt.status || w.Body.String() != tt.body {
				t.Errorf("response = %d %s, want %d %s", w.Code, w.Body.String(), tt.status, tt.body)
			}
			if got := logs.String(); got != tt.log {
				t.Errorf("log = %q, want %q", got, tt.log)
			}
			if got, exists := w.Header()[http.CanonicalHeaderKey("Content-Profile")]; exists {
				t.Errorf("Content-Profile = %q, want absent", got)
			}
			if got := w.Header().Get("X-Keep"); got != "value" {
				t.Errorf("X-Keep = %q, want value", got)
			}
			if got := w.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
		})
	}
}
