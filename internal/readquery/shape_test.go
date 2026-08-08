package readquery_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
)

func sampleRows() []rows.Row {
	return []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(3), "gamma"}},
	}
}

func TestShapeFiltersOrdersAndPagesARowSet(t *testing.T) {
	t.Parallel()

	limit := uint64(1)
	result, err := readquery.Shape(sampleRows(), readquery.Query{
		Filters: []readquery.Filter{{Column: "id", Op: readquery.OpGt, Value: "1"}},
		Order:   []readquery.Order{{Column: "name", Desc: true}},
		Limit:   &limit,
	})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %#v, want one row", result.Rows)
	}
	if result.Rows[0].Values[1] != "gamma" {
		t.Fatalf("row = %#v, want gamma first after desc order and limit", result.Rows[0])
	}
}

func TestHasRowSetFeatures(t *testing.T) {
	t.Parallel()

	if readquery.HasRowSetFeatures(readquery.Query{SelectAll: true}) {
		t.Fatal("empty read query must not demand a row set")
	}
	limit := uint64(1)
	if !readquery.HasRowSetFeatures(readquery.Query{Limit: &limit}) {
		t.Fatal("limit must demand a row set")
	}
	if !readquery.HasRowSetFeatures(readquery.Query{
		Embeds: []readquery.Embed{{Resource: "orders"}},
	}) {
		t.Fatal("embed must demand a row set")
	}
}
