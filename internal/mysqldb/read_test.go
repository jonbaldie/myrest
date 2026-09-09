package mysqldb

import (
	"encoding/json"
	"testing"

	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestSelectStatementReadsEveryColumnOfTheResource(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}

	statement := selectStatement(table, columnNames(table))
	want := "SELECT `id`, `name` FROM `shop`.`items`"
	if statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
}

func TestQuoteIdentifierKeepsABackQuoteInsideTheName(t *testing.T) {
	t.Parallel()

	if quoted := quoteIdentifier("od`d"); quoted != "`od``d`" {
		t.Fatalf("quoted = %q, want %q", quoted, "`od``d`")
	}
}

func TestJSONValueReadsTextAsAString(t *testing.T) {
	t.Parallel()

	value, err := jsonValue([]byte("alpha"), "")
	if err != nil {
		t.Fatalf("jsonValue: %v", err)
	}
	if value != "alpha" {
		t.Fatalf("value = %#v, want the string alpha", value)
	}
}

func TestJSONValueKeepsOtherValuesAsTheyAre(t *testing.T) {
	t.Parallel()

	value, err := jsonValue(int64(7), "")
	if err != nil {
		t.Fatalf("jsonValue: %v", err)
	}
	if value != int64(7) {
		t.Fatalf("value = %#v, want 7", value)
	}
	value, err = jsonValue(nil, "")
	if err != nil {
		t.Fatalf("jsonValue: %v", err)
	}
	if value != nil {
		t.Fatalf("value = %#v, want nil", value)
	}
}

func TestJSONValueReadsJSONTypeAsJSONValue(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte(`{"blood_type":"A-"}`), "JSON")
	if want := `{"meta":{"blood_type":"A-"}}`; encoded != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestJSONValueReadsJSONArrayAsJSONValue(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte(`[{"number":"917-929-5745"}]`), "JSON")
	if want := `{"meta":[{"number":"917-929-5745"}]}`; encoded != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestJSONValueKeepsJSONTextAsAString(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte(`{"blood_type":"A-"}`), "TEXT")
	if want := `{"meta":"{\"blood_type\":\"A-\"}"}`; encoded != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestJSONValueReadsNumericBytesAsJSONNumber(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte("1.50"), "DECIMAL")
	if want := `{"meta":1.50}`; encoded != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestJSONValueRefusesMalformedJSON(t *testing.T) {
	t.Parallel()

	_, err := jsonValue([]byte("{"), "JSON")
	if err != (rows.InvalidJSON{}) {
		t.Fatalf("err = %v, want InvalidJSON", err)
	}
}

func marshaledJSONValue(t *testing.T, value any, typeName string) string {
	t.Helper()

	converted, err := jsonValue(value, typeName)
	if err != nil {
		t.Fatalf("jsonValue: %v", err)
	}
	encoded, err := json.Marshal(rows.Row{Columns: []string{"meta"}, Values: []any{converted}})
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	return string(encoded)
}
