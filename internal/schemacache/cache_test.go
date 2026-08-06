package schemacache_test

import (
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// catalog holds two tables of one configured database. The anonymous role can
// select from items only.
func catalog() schemacache.Catalog {
	return schemacache.Catalog{
		Tables: []schemacache.TableFact{
			{Schema: "shop", Name: "items"},
			{Schema: "shop", Name: "secrets"},
		},
		Columns: []schemacache.ColumnFact{
			{Schema: "shop", Table: "items", Name: "id"},
			{Schema: "shop", Table: "items", Name: "name"},
			{Schema: "shop", Table: "secrets", Name: "payload"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Schema: "shop", Table: "items"},
		},
	}
}

// cache-001: a table the active database role can select from is a resource.
func TestTableWithSelectIsAResource(t *testing.T) {
	t.Parallel()

	table, ok := schemacache.Build(catalog()).Resource("myrest_anon", "items")
	if !ok {
		t.Fatal("items is not a resource for myrest_anon")
	}
	if table.Schema != "shop" || table.Name != "items" {
		t.Fatalf("table = %s.%s, want shop.items", table.Schema, table.Name)
	}
	if want := []schemacache.Column{{Name: "id"}, {Name: "name"}}; !reflect.DeepEqual(table.Columns, want) {
		t.Fatalf("columns = %v, want %v", table.Columns, want)
	}
}

// cache-002: a table the active database role cannot select from is not one.
func TestTableWithoutSelectIsNotAResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource("myrest_anon", "secrets"); ok {
		t.Fatal("secrets is a resource for myrest_anon, which has no SELECT on it")
	}
}

func TestTableOutsideTheCatalogIsNotAResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource("myrest_anon", "outside_items"); ok {
		t.Fatal("a table the catalog does not hold is a resource")
	}
}

func TestAnotherRoleGetsNoResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource("other_role", "items"); ok {
		t.Fatal("items is a resource for a role with no grant on it")
	}
}

// A table name in two configured databases resolves to the one the role can
// select from.
func TestNameInTwoDatabasesResolvesToTheGrantedTable(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableFact{
			{Schema: "warehouse", Name: "items"},
			{Schema: "shop", Name: "items"},
		},
		Columns: []schemacache.ColumnFact{
			{Schema: "shop", Table: "items", Name: "id"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Schema: "shop", Table: "items"},
		},
	})

	table, ok := cache.Resource("myrest_anon", "items")
	if !ok {
		t.Fatal("items is not a resource for myrest_anon")
	}
	if table.Schema != "shop" {
		t.Fatalf("schema = %q, want shop", table.Schema)
	}
}

// Catalog facts about tables the cache does not hold cannot make a resource.
func TestGrantOnAnUnknownTableMakesNoResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Columns: []schemacache.ColumnFact{{Schema: "shop", Table: "ghost", Name: "id"}},
		Selects: []schemacache.SelectFact{{Role: "myrest_anon", Schema: "shop", Table: "ghost"}},
	})

	if _, ok := cache.Resource("myrest_anon", "ghost"); ok {
		t.Fatal("a grant alone made a resource out of a table the catalog does not hold")
	}
}

// The same table reported twice keeps one set of columns.
func TestRepeatedTableFactKeepsOneTable(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableFact{
			{Schema: "shop", Name: "items"},
			{Schema: "shop", Name: "items"},
		},
		Columns: []schemacache.ColumnFact{{Schema: "shop", Table: "items", Name: "id"}},
		Selects: []schemacache.SelectFact{{Role: "myrest_anon", Schema: "shop", Table: "items"}},
	})

	table, ok := cache.Resource("myrest_anon", "items")
	if !ok {
		t.Fatal("items is not a resource for myrest_anon")
	}
	if len(table.Columns) != 1 {
		t.Fatalf("columns = %v, want one column", table.Columns)
	}
}
