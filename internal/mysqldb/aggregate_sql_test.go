package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestBuildSelectBareCount(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Agg: readquery.AggCount}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT COUNT(*) AS `count` FROM `shop`.`items`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.columns) != 1 || parts.columns[0] != "count" {
		t.Fatalf("columns = %#v", parts.columns)
	}
}

func TestBuildSelectAggregatesGroupByNonAggregateColumns(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{
			{Name: "id", Alias: "total", Agg: readquery.AggSum},
			{Name: "id", Agg: readquery.AggAvg},
			{Name: "id", Agg: readquery.AggMin},
			{Name: "id", Agg: readquery.AggMax},
			{Name: "name"},
		},
		Order: []readquery.Order{{Column: "name"}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT SUM(`id`) AS `total`, AVG(`id`) AS `avg`, MIN(`id`) AS `min`, MAX(`id`) AS `max`, `name` " +
		"FROM `shop`.`items` GROUP BY `name` ORDER BY `name` ASC"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}

func TestBuildSelectColumnCount(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "name", Agg: readquery.AggCount}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT COUNT(`name`) AS `count` FROM `shop`.`items`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}
