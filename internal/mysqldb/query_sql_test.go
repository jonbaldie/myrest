package mysqldb

import (
	"errors"
	"strings"
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

func TestBuildSelectAppliesIsDistinctFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id"}, {Name: "name"}},
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpIsDistinct, Value: "alpha"}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `id`, `name` FROM `shop`.`items` WHERE NOT (`name` <=> ?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "alpha" {
		t.Fatalf("args = %#v", parts.args)
	}

	negatedParts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id"}, {Name: "name"}},
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpIsDistinct, Value: "alpha", Negated: true}},
	})
	if err != nil {
		t.Fatalf("buildSelect negated: %v", err)
	}
	wantNegated := "SELECT `id`, `name` FROM `shop`.`items` WHERE `name` <=> ?"
	if negatedParts.statement != wantNegated {
		t.Fatalf("statement = %q, want %q", negatedParts.statement, wantNegated)
	}

	nullParts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id"}, {Name: "name"}},
		Filters: []readquery.Filter{{Column: "name", Op: readquery.OpIsDistinct, Value: "null"}},
	})
	if err != nil {
		t.Fatalf("buildSelect null: %v", err)
	}
	if nullParts.statement != want {
		t.Fatalf("statement = %q, want %q", nullParts.statement, want)
	}
	if len(nullParts.args) != 1 || nullParts.args[0] != nil {
		t.Fatalf("null arg = %#v, want nil", nullParts.args)
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

func TestBuildSelectAppliesChainedJSONPath(t *testing.T) {
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
				Steps: []readquery.PathStep{
					{Key: "phones"},
					{IsIndex: true, Index: 0},
					{Key: "number"},
				},
				AsText: true,
			}},
		},
		Filters: []readquery.Filter{{
			Column: "meta",
			Path: &readquery.JSONPath{
				Steps: []readquery.PathStep{
					{Key: "phones"},
					{IsIndex: true, Index: 0},
					{Key: "number"},
				},
				AsText: true,
			},
			Op:    readquery.OpEq,
			Value: "917-929-5745",
		}},
		Order: []readquery.Order{{
			Column: "meta",
			Path: &readquery.JSONPath{
				Steps: []readquery.PathStep{
					{Key: "phones"},
					{IsIndex: true, Index: 0},
					{Key: "number"},
				},
				AsText: true,
			},
		}},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	want := "SELECT `id`, `meta`->>'$.phones[0].number' FROM `shop`.`items` WHERE `meta`->>'$.phones[0].number' = ? ORDER BY `meta`->>'$.phones[0].number' ASC"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if parts.columns[1] != "number" {
		t.Fatalf("output columns = %#v", parts.columns)
	}

	partsAsJSON, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{
			{Name: "meta", Path: &readquery.JSONPath{
				Steps: []readquery.PathStep{
					{Key: "phones"},
					{IsIndex: true, Index: 0},
				},
				AsText: false,
			}},
		},
	})
	if err != nil {
		t.Fatalf("buildSelect as JSON: %v", err)
	}
	wantAsJSON := "SELECT `meta`->'$.phones[0]' FROM `shop`.`items`"
	if partsAsJSON.statement != wantAsJSON {
		t.Fatalf("statement = %q, want %q", partsAsJSON.statement, wantAsJSON)
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

func TestBuildSelectEmptyGroupDoesNotEmitAlwaysTrue(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}
	parts, err := buildSelect(table, readquery.Query{
		Columns: []readquery.Column{{Name: "id"}},
		Groups: []readquery.Group{
			{},
			{Groups: []readquery.Group{{}}},
		},
	})
	if err != nil {
		t.Fatalf("buildSelect: %v", err)
	}
	if strings.Contains(parts.statement, "1=1") {
		t.Fatalf("statement must not contain 1=1 for empty groups, got: %q", parts.statement)
	}
	want := "SELECT `id` FROM `shop`.`items`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}
