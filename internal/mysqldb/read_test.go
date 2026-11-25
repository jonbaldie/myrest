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

// Issue #287: BIGINT UNSIGNED bytes above max int64 must stay a JSON number.
func TestJSONValueReadsBigintUnsignedBytesAsJSONNumber(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte("18446744073709551615"), "UNSIGNED BIGINT")
	if want := `{"meta":18446744073709551615}`; encoded != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestJSONValueReadsBigintNamesAsJSONNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		value    string
		typeName string
		want     string
	}{
		{"driver bigint", "42", "BIGINT", `{"meta":42}`},
		{"driver unsigned bigint", "42", "UNSIGNED BIGINT", `{"meta":42}`},
		{"catalog bigint", "42", "bigint", `{"meta":42}`},
		{"catalog bigint unsigned", "18446744073709551615", "bigint unsigned", `{"meta":18446744073709551615}`},
		{"catalog bigint unsigned width", "18446744073709551615", "bigint(20) unsigned", `{"meta":18446744073709551615}`},
		{"signed bigint min", "-9223372036854775808", "BIGINT", `{"meta":-9223372036854775808}`},
		{"above int64", "9223372036854775808", "UNSIGNED BIGINT", `{"meta":9223372036854775808}`},
		{"at int64 max", "9223372036854775807", "UNSIGNED BIGINT", `{"meta":9223372036854775807}`},
		{"longlong unsigned", "18446744073709551615", "longlong unsigned", `{"meta":18446744073709551615}`},
		{"unsigned longlong", "18446744073709551615", "UNSIGNED LONGLONG", `{"meta":18446744073709551615}`},
		{"bigint zerofill", "18446744073709551615", "BIGINT UNSIGNED ZEROFILL", `{"meta":18446744073709551615}`},
		{"float", "1.5", "FLOAT", `{"meta":1.5}`},
		{"double", "1.5", "DOUBLE", `{"meta":1.5}`},
		{"numeric", "1.5", "NUMERIC", `{"meta":1.5}`},
		{"newdecimal", "1.5", "NEWDECIMAL", `{"meta":1.5}`},
		{"real", "1.5", "REAL", `{"meta":1.5}`},
	}
	for _, tc := range cases {
		if got := marshaledJSONValue(t, []byte(tc.value), tc.typeName); got != tc.want {
			t.Fatalf("%s: row = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestJSONValueKeepsBigintTextString(t *testing.T) {
	t.Parallel()

	encoded := marshaledJSONValue(t, []byte("alpha"), "TEXT")
	if want := `{"meta":"alpha"}`; encoded != want {
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
