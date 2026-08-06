package rows_test

import (
	"encoding/json"
	"testing"

	"github.com/jonbaldie/myrest/internal/rows"
)

// repr-001: a row is a JSON object that keeps the column order of the resource.
func TestRowKeepsColumnOrder(t *testing.T) {
	t.Parallel()

	row := rows.Row{
		Columns: []string{"name", "id"},
		Values:  []any{"alpha", 1},
	}

	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	if want := `{"name":"alpha","id":1}`; string(encoded) != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestRowWritesAMissingValueAsNull(t *testing.T) {
	t.Parallel()

	row := rows.Row{Columns: []string{"id", "name"}, Values: []any{1}}

	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	if want := `{"id":1,"name":null}`; string(encoded) != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestEmptyRowIsAnEmptyObject(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(rows.Row{})
	if err != nil {
		t.Fatalf("marshal row: %v", err)
	}
	if want := `{}`; string(encoded) != want {
		t.Fatalf("row = %s, want %s", encoded, want)
	}
}

func TestRowRefusesAValueJSONCannotHold(t *testing.T) {
	t.Parallel()

	row := rows.Row{Columns: []string{"broken"}, Values: []any{make(chan int)}}

	if _, err := json.Marshal(row); err == nil {
		t.Fatal("marshal accepted a value JSON cannot hold")
	}
}
