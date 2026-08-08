package readquery_test

import (
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseBareCountAggregate(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"select": []string{"count()"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Columns) != 1 {
		t.Fatalf("columns = %#v", query.Columns)
	}
	got := query.Columns[0]
	if got.Name != "" || got.Agg != readquery.AggCount || got.Alias != "" {
		t.Fatalf("column = %#v", got)
	}
	if !readquery.HasAggregates(query) {
		t.Fatal("HasAggregates is false")
	}
}

func TestParseColumnAggregatesAndGroupColumn(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select": []string{"total:id.sum(),id.avg(),id.min(),id.max(),name"},
	}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Columns) != 5 {
		t.Fatalf("columns = %#v", query.Columns)
	}
	if query.Columns[0] != (readquery.Column{Name: "id", Alias: "total", Agg: readquery.AggSum}) {
		t.Fatalf("sum = %#v", query.Columns[0])
	}
	if query.Columns[1].Agg != readquery.AggAvg || query.Columns[2].Agg != readquery.AggMin {
		t.Fatalf("avg/min = %#v %#v", query.Columns[1], query.Columns[2])
	}
	if query.Columns[3].Agg != readquery.AggMax || query.Columns[4] != (readquery.Column{Name: "name"}) {
		t.Fatalf("max/group = %#v %#v", query.Columns[3], query.Columns[4])
	}
}

func TestParseAggregateInsideEmbedAndSpreadMark(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select": []string{"name,orders(count()),...tags(id.count())"},
	}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Embeds) != 2 {
		t.Fatalf("embeds = %#v", query.Embeds)
	}
	if query.Embeds[0].Resource != "orders" || query.Embeds[0].Spread {
		t.Fatalf("orders embed = %#v", query.Embeds[0])
	}
	if len(query.Embeds[0].Columns) != 1 || query.Embeds[0].Columns[0].Agg != readquery.AggCount {
		t.Fatalf("orders columns = %#v", query.Embeds[0].Columns)
	}
	if !query.Embeds[1].Spread || query.Embeds[1].Resource != "tags" {
		t.Fatalf("spread embed = %#v", query.Embeds[1])
	}
	if !readquery.HasAggregates(query) {
		t.Fatal("HasAggregates is false")
	}
}

func TestColumnResultNamePrefersAliasThenAggregate(t *testing.T) {
	t.Parallel()

	if name := (readquery.Column{Name: "id", Alias: "total", Agg: readquery.AggSum}).ResultName(); name != "total" {
		t.Fatalf("alias name = %q", name)
	}
	if name := (readquery.Column{Name: "id", Agg: readquery.AggSum}).ResultName(); name != "sum" {
		t.Fatalf("agg name = %q", name)
	}
	if name := (readquery.Column{Agg: readquery.AggCount}).ResultName(); name != "count" {
		t.Fatalf("count name = %q", name)
	}
}

func TestFullMatchAggregatesListsClaimedForms(t *testing.T) {
	t.Parallel()

	want := []readquery.Aggregate{
		readquery.AggAvg, readquery.AggCount, readquery.AggMax, readquery.AggMin, readquery.AggSum,
	}
	if len(readquery.FullMatchAggregates) != len(want) {
		t.Fatalf("FullMatchAggregates = %#v", readquery.FullMatchAggregates)
	}
	for i := range want {
		if readquery.FullMatchAggregates[i] != want[i] {
			t.Fatalf("FullMatchAggregates[%d] = %s", i, readquery.FullMatchAggregates[i])
		}
	}
}
