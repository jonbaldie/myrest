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

func TestBuildCountGroupedAggregate(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "orders"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "item_id"}},
	}
	parts, err := buildCount(table, readquery.Query{
		Columns: []readquery.Column{
			{Agg: readquery.AggCount},
			{Name: "item_id"},
		},
		Filters: []readquery.Filter{{Column: "item_id", Op: readquery.OpEq, Value: "1"}},
	})
	if err != nil {
		t.Fatalf("buildCount: %v", err)
	}
	want := "SELECT COUNT(*) FROM (SELECT 1 FROM `shop`.`orders` WHERE `item_id` = ? GROUP BY `item_id`) AS `_myrest_count`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "1" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildCountPureAggregate(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildCount(table, readquery.Query{
		Columns: []readquery.Column{{Agg: readquery.AggCount}},
	})
	if err != nil {
		t.Fatalf("buildCount: %v", err)
	}
	want := "SELECT COUNT(*) FROM (SELECT COUNT(*) FROM `shop`.`items`) AS `_myrest_count`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}

func TestBuildCountPureAggregateWithFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildCount(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id", Agg: readquery.AggSum}},
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpEq, Value: "alpha"}},
	})
	if err != nil {
		t.Fatalf("buildCount: %v", err)
	}
	want := "SELECT COUNT(*) FROM (SELECT COUNT(*) FROM `shop`.`items` WHERE `name` = ?) AS `_myrest_count`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "alpha" {
		t.Fatalf("args = %#v", parts.args)
	}
}
