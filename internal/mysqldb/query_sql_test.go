package mysqldb

import (
	"errors"
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

func TestBuildSelectAppliesJSONPathProjectionAndFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "meta", DataType: "json"},
		},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{
			{Name: "id"},
			{Name: "meta", Path: &readquery.JSONPath{
				Steps:  []readquery.PathStep{{Key: "blood_type"}},
				AsText: true,
			}},
		},
		Filters: []readquery.Filter{{
			Column: "meta",
			Path: &readquery.JSONPath{
				Steps:  []readquery.PathStep{{Key: "blood_type"}},
				AsText: true,
			},
			Op:    readquery.OpEq,
			Value: "A-",
		}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `id`, `meta`->>'$.blood_type' FROM `shop`.`items` WHERE `meta`->>'$.blood_type' = ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if parts.columns[1] != "blood_type" {
		t.Fatalf("output columns = %#v", parts.columns)
	}
}

func TestBuildSelectRefusesJSONPathOnNonJSONColumn(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "name", DataType: "varchar"}},
	}
	_, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{
			Name: "name",
			Path: &readquery.JSONPath{Steps: []readquery.PathStep{{Key: "x"}}, AsText: true},
		}},
	})
	var gap readquery.UnsupportedFeature
	if err == nil || !errors.As(err, &gap) {
		t.Fatalf("err = %v, want UnsupportedFeature", err)
	}
}

func TestILikeRefusesNonCaseInsensitiveCollation(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "name", DataType: "varchar", Collation: "utf8mb4_bin"},
		},
	}
	_, err := buildSelect(table, readquery.Query{
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpILike, Value: "ALPHA"}},
	})
	var gap readquery.UnsupportedFeature
	if err == nil || !errors.As(err, &gap) {
		t.Fatalf("err = %v, want UnsupportedFeature for non-ci collation", err)
	}
}

func TestILikeAllowsCaseInsensitiveCollation(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "name", DataType: "varchar", Collation: "utf8mb4_0900_ai_ci"},
		},
	}
	parts, err := buildSelect(table, readquery.Query{
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpILike, Value: "ALPHA"}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `name` FROM `shop`.`items` WHERE `name` LIKE ?"
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
