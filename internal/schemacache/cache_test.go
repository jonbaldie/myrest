package schemacache_test

import (
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

const anonRole schemacache.Role = "myrest_anon"

func table(database, name string) schemacache.TableID {
	return schemacache.TableID{Database: database, Name: name}
}

// catalog holds two tables of one configured database. The anonymous role can
// select from items only.
func catalog() schemacache.Catalog {
	return schemacache.Catalog{
		Tables: []schemacache.TableID{table("shop", "items"), table("shop", "secrets")},
		Columns: []schemacache.ColumnFact{
			{Table: table("shop", "items"), Name: "id"},
			{Table: table("shop", "items"), Name: "name"},
			{Table: table("shop", "secrets"), Name: "payload"},
		},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("shop", "items")}},
	}
}

// cache-001: a table the active database role can select from is a resource.
func TestTableWithSelectIsAResource(t *testing.T) {
	t.Parallel()

	found, ok := schemacache.Build(catalog()).Resource(anonRole, "items")
	if !ok {
		t.Fatal("items is not a resource for myrest_anon")
	}
	if found.ID != table("shop", "items") {
		t.Fatalf("table = %v, want shop.items", found.ID)
	}
	if want := []schemacache.Column{{Name: "id"}, {Name: "name"}}; !reflect.DeepEqual(found.Columns, want) {
		t.Fatalf("columns = %v, want %v", found.Columns, want)
	}
}

// cache-002: a table the active database role cannot select from is not one.
func TestTableWithoutSelectIsNotAResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource(anonRole, "secrets"); ok {
		t.Fatal("secrets is a resource for myrest_anon, which has no SELECT on it")
	}
}

func TestTableOutsideTheCatalogIsNotAResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource(anonRole, "outside_items"); ok {
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
		Tables:  []schemacache.TableID{table("warehouse", "items"), table("shop", "items")},
		Columns: []schemacache.ColumnFact{{Table: table("shop", "items"), Name: "id"}},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("shop", "items")}},
	})

	found, ok := cache.Resource(anonRole, "items")
	if !ok {
		t.Fatal("items is not a resource for myrest_anon")
	}
	if found.ID.Database != "shop" {
		t.Fatalf("database = %q, want shop", found.ID.Database)
	}
}

// Catalog facts about tables the cache does not hold cannot make a resource.
func TestGrantOnAnUnknownTableMakesNoResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Columns: []schemacache.ColumnFact{{Table: table("shop", "ghost"), Name: "id"}},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("shop", "ghost")}},
	})

	if _, ok := cache.Resource(anonRole, "ghost"); ok {
		t.Fatal("a grant alone made a resource out of a table the catalog does not hold")
	}
}

// An empty catalog holds no resource.
func TestEmptyCatalogHoldsNoResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(schemacache.Catalog{}).Resource(anonRole, "items"); ok {
		t.Fatal("an empty catalog gave a resource")
	}
}
