package representation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
)

// Seam under test: Format, the body of a row data response.

var (
	jsonArray = representation.Spec{Kind: representation.KindJSONArray}
	singular  = representation.Spec{Kind: representation.KindSingularObject}
	csvSpec   = representation.Spec{Kind: representation.KindCSV}
)

func format(t *testing.T, spec representation.Spec, bodyRows []rows.Row, header []string) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := representation.Format(&buffer, spec, bodyRows, header); err != nil {
		t.Fatalf("Format: %v", err)
	}
	return buffer.String()
}

func TestFormatJSONArrayKeepsColumnOrder(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{
		{Columns: []string{"name", "id"}, Values: []any{"alpha", int64(1)}},
		{Columns: []string{"name", "id"}, Values: []any{nil, int64(2)}},
	}
	got := format(t, jsonArray, bodyRows, nil)
	if want := `[{"name":"alpha","id":1},{"name":null,"id":2}]` + "\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if got := format(t, jsonArray, []rows.Row{}, nil); got != "[]\n" {
		t.Fatalf("empty body = %q", got)
	}
}

func TestFormatSingularObjectWritesTheOneRow(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{Columns: []string{"id"}, Values: []any{int64(7)}}}
	if got, want := format(t, singular, bodyRows, nil), `{"id":7}`+"\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestFormatSingularObjectRefusesOtherRowCounts(t *testing.T) {
	t.Parallel()

	two := []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
	}
	var buffer bytes.Buffer
	err := representation.Format(&buffer, singular, two, nil)
	var refusal representation.SingularObjectRefusal
	if !errors.As(err, &refusal) || refusal.RowCount != 2 {
		t.Fatalf("err = %T %v, want SingularObjectRefusal{2}", err, err)
	}
	if buffer.Len() != 0 {
		t.Fatalf("body = %q, want no body", buffer.String())
	}
}

func TestFormatCSVQuotesFieldsAndWritesNullAsEmpty(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{
		{
			Columns: []string{"id", "name", "note", "data", "raw", "num", "flag"},
			Values: []any{
				int64(1), "a,b", "say \"hi\"\nbye", map[string]any{"k": "v"},
				[]byte("bytes"), json.Number("2.50"), true,
			},
		},
		{
			Columns: []string{"id", "name", "note", "data", "raw", "num", "flag"},
			Values:  []any{int64(2), nil, "plain", []any{1, "x"}, nil, 3.5, false},
		},
	}
	got := format(t, csvSpec, bodyRows, []string{"id", "name", "note", "data", "raw", "num", "flag"})
	want := "id,name,note,data,raw,num,flag\n" +
		"1,\"a,b\",\"say \"\"hi\"\"\nbye\",\"{\"\"k\"\":\"\"v\"\"}\",bytes,2.50,true\n" +
		"2,,plain,\"[1,\"\"x\"\"]\",,3.5,false\n"
	if got != want {
		t.Fatalf("body =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatCSVHeaderFallsBackToRowColumns(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}
	if got, want := format(t, csvSpec, bodyRows, nil), "id,name\n1,alpha\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestFormatCSVEmptyResultKeepsHeaderOrWritesNothing(t *testing.T) {
	t.Parallel()

	if got, want := format(t, csvSpec, nil, []string{"id", "name"}), "id,name\n"; got != want {
		t.Fatalf("header body = %q, want %q", got, want)
	}
	if got := format(t, csvSpec, nil, nil); got != "" {
		t.Fatalf("no header body = %q, want empty", got)
	}
}

func TestFormatCSVMissingValuesAreEmpty(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}
	if got, want := format(t, csvSpec, bodyRows, []string{"id", "extra"}), "id,extra\n1,\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestFormatCSVUnencodableValueIsEmpty(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{Columns: []string{"id", "bad"}, Values: []any{int64(1), make(chan int)}}}
	if got, want := format(t, csvSpec, bodyRows, nil), "id,bad\n1,\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

// label is a value the JSON encoder writes as a JSON string.
type label string

func TestFormatCSVWritesJSONStringValuesWithoutQuotes(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{
		Columns: []string{"tag", "at"},
		Values:  []any{label("a<b"), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}}
	if got, want := format(t, csvSpec, bodyRows, nil), "tag,at\na<b,2026-01-02T03:04:05Z\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFormatReportsWriterFailure(t *testing.T) {
	t.Parallel()

	bodyRows := []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}
	for _, spec := range []representation.Spec{jsonArray, singular, csvSpec} {
		if err := representation.Format(failingWriter{}, spec, bodyRows, nil); err == nil {
			t.Fatalf("kind %v: err = nil, want the writer failure", spec.Kind)
		}
	}
}
