package readquery_test

import (
	"net/url"
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

// A parsed quoted value filters a row set by its literal, not by the quote
// characters. Issue #145.
func TestShapeFiltersARowSetWithAQuotedValue(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"name": {`eq."alpha,beta"`}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	set := []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha,beta"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "alpha"}},
	}
	result, err := readquery.Shape(set, query)
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Values[1] != "alpha,beta" {
		t.Fatalf("rows = %#v, want the alpha,beta row", result.Rows)
	}
}

// A double-quoted "null" isdistinct value compares against the literal string,
// while an unquoted null keeps its SQL NULL meaning. Issue #160.
func TestShapeIsDistinctQuotedNullIsALiteral(t *testing.T) {
	t.Parallel()

	set := []rows.Row{
		{Columns: []string{"id", "name"}, Values: []any{int64(1), "null"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(2), "red"}},
		{Columns: []string{"id", "name"}, Values: []any{int64(3), nil}},
	}

	cases := []struct {
		raw string
		ids []int64
	}{
		{raw: `isdistinct."null"`, ids: []int64{2, 3}},
		{raw: `not.isdistinct."null"`, ids: []int64{1}},
		{raw: `isdistinct.null`, ids: []int64{1, 2}},
		{raw: `not.isdistinct.null`, ids: []int64{3}},
	}
	for _, c := range cases {
		query, err := readquery.Parse(url.Values{"name": {c.raw}}, nil)
		if err != nil {
			t.Fatalf("Parse %s: %v", c.raw, err)
		}
		result, err := readquery.Shape(set, query)
		if err != nil {
			t.Fatalf("Shape %s: %v", c.raw, err)
		}
		var ids []int64
		for _, row := range result.Rows {
			ids = append(ids, row.Values[0].(int64))
		}
		if len(ids) != len(c.ids) {
			t.Fatalf("%s: ids = %v, want %v", c.raw, ids, c.ids)
		}
		for i, id := range c.ids {
			if ids[i] != id {
				t.Fatalf("%s: ids = %v, want %v", c.raw, ids, c.ids)
			}
		}
	}
}
