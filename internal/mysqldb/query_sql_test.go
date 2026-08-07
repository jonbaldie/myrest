package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestBuildSelectAppliesFilterOrderAndPage(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	limit := uint64(1)
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id"}, {Name: "name"}},
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpEq, Value: "alpha"}},
		Order:   []readquery.Order{{Column: "id", Desc: true}},
		Limit:   &limit,
		Offset:  0,
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `id`, `name` FROM `shop`.`items` WHERE `name` = ? ORDER BY `id` DESC LIMIT 1"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "alpha" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildSelectUsesLargeLimitWhenOnlyOffsetIsSet(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}},
	}
	parts, err := buildSelect(table, readquery.Query{Offset: 1})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `id` FROM `shop`.`items` LIMIT 18446744073709551615 OFFSET 1"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}

func TestBuildCountUsesTheSameFilters(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildCount(table, readquery.Query{
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpLike, Value: "a*"}},
	})
	if err != nil {
		t.Fatalf("buildCount: %v", err)
	}
	want := "SELECT COUNT(*) FROM `shop`.`items` WHERE `name` LIKE ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "a%" {
		t.Fatalf("args = %#v", parts.args)
	}
}
