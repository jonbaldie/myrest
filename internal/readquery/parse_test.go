package readquery_test

import (
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseSelectColumnsAndAlias(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"select": []string{"id,fullName:name"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []readquery.Column{{Name: "id"}, {Name: "name", Alias: "fullName"}}
	if len(query.Columns) != len(want) {
		t.Fatalf("columns = %#v, want %#v", query.Columns, want)
	}
	for i := range want {
		if query.Columns[i] != want[i] {
			t.Fatalf("columns[%d] = %#v, want %#v", i, query.Columns[i], want[i])
		}
	}
}

func TestParseEqFilterAndOrderLimitOffset(t *testing.T) {
	t.Parallel()

	values := url.Values{
		"name":   []string{"eq.alpha"},
		"order":  []string{"id.desc"},
		"limit":  []string{"1"},
		"offset": []string{"0"},
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 {
		t.Fatalf("filters = %#v", query.Filters)
	}
	got := query.Filters[0]
	if got.Column != "name" || got.Op != readquery.OpEq || got.Value != "alpha" || got.Negated {
		t.Fatalf("filter = %#v", got)
	}
	if len(query.Order) != 1 || query.Order[0] != (readquery.Order{Column: "id", Desc: true}) {
		t.Fatalf("order = %#v", query.Order)
	}
	if query.Limit == nil || *query.Limit != 1 || query.Offset != 0 {
		t.Fatalf("page = limit %#v offset %d", query.Limit, query.Offset)
	}
}

func TestParsePreferCountExact(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{}, []string{"count=exact"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !query.ExactCount {
		t.Fatal("ExactCount is false")
	}
}

func TestParseRejectsUnknownOperator(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{"name": []string{"ilike.alpha"}}, nil)
	if err == nil {
		t.Fatal("Parse accepted ilike")
	}
}

func TestEffectiveLimitTakesTheLowerCap(t *testing.T) {
	t.Parallel()

	client := uint64(50)
	maxRows := uint64(10)
	query := readquery.Query{Limit: &client, MaxRows: &maxRows}
	got := query.EffectiveLimit()
	if got == nil || *got != 10 {
		t.Fatalf("EffectiveLimit = %#v, want 10", got)
	}
}

func TestFullMatchOperatorsAreListed(t *testing.T) {
	t.Parallel()

	if len(readquery.FullMatchOperators) != 10 {
		t.Fatalf("FullMatchOperators = %#v", readquery.FullMatchOperators)
	}
}
