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

	found, ok := schemacache.Build(catalog()).Resource(anonRole, table("shop", "items"))
	if !ok {
		t.Fatal("shop.items is not a resource for myrest_anon")
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

	if _, ok := schemacache.Build(catalog()).Resource(anonRole, table("shop", "secrets")); ok {
		t.Fatal("shop.secrets is a resource for myrest_anon, which has no SELECT on it")
	}
}

func TestTableOutsideTheCatalogIsNotAResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource(anonRole, table("shop", "ghost")); ok {
		t.Fatal("a table the catalog does not hold is a resource")
	}
}

func TestAnotherRoleGetsNoResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(catalog()).Resource("other_role", table("shop", "items")); ok {
		t.Fatal("shop.items is a resource for a role with no grant on it")
	}
}

// One table name in two configured databases answers from the database the
// request names, and from no other one.
func TestOneNameInTwoDatabasesAnswersFromTheDatabaseAsked(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{table("warehouse", "items"), table("shop", "items")},
		Columns: []schemacache.ColumnFact{
			{Table: table("warehouse", "items"), Name: "sku"},
			{Table: table("shop", "items"), Name: "id"},
		},
		Selects: []schemacache.SelectFact{
			{Role: anonRole, Table: table("warehouse", "items")},
			{Role: anonRole, Table: table("shop", "items")},
		},
	})

	found, ok := cache.Resource(anonRole, table("warehouse", "items"))
	if !ok {
		t.Fatal("warehouse.items is not a resource for myrest_anon")
	}
	if found.ID.Database != "warehouse" {
		t.Fatalf("database = %q, want warehouse", found.ID.Database)
	}
	if want := []schemacache.Column{{Name: "sku"}}; !reflect.DeepEqual(found.Columns, want) {
		t.Fatalf("columns = %v, want the columns of warehouse.items", found.Columns)
	}
}

// A grant on a table of another database opens nothing in this one.
func TestGrantInAnotherDatabaseOpensNoTableHere(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("warehouse", "items"), table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("warehouse", "items")}},
	})

	if _, ok := cache.Resource(anonRole, table("shop", "items")); ok {
		t.Fatal("shop.items is a resource through the grant on warehouse.items")
	}
}

// MySQL reads with the privileges of the roles granted to the active role, so
// the cache must hold what those roles can select from.
func TestGrantThroughAnotherRoleMakesAResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: "reader", Table: table("shop", "items")}},
		Roles:   []schemacache.RoleFact{{Holder: anonRole, Granted: "reader"}},
	})

	if _, ok := cache.Resource(anonRole, table("shop", "items")); !ok {
		t.Fatal("shop.items is not a resource through the role granted to myrest_anon")
	}
}

// The grants reach as deep as the role grants go.
func TestGrantThroughAChainOfRolesMakesAResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: "deep_reader", Table: table("shop", "items")}},
		Roles: []schemacache.RoleFact{
			{Holder: anonRole, Granted: "reader"},
			{Holder: "reader", Granted: "deep_reader"},
		},
	})

	if _, ok := cache.Resource(anonRole, table("shop", "items")); !ok {
		t.Fatal("shop.items is not a resource through the chain of roles")
	}
}

// A role grant carries the privileges one way only: the granted role does not
// read with the privileges of its holder.
func TestGrantDoesNotReachTheHolderOfARole(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("shop", "items")}},
		Roles:   []schemacache.RoleFact{{Holder: anonRole, Granted: "reader"}},
	})

	if _, ok := cache.Resource("reader", table("shop", "items")); ok {
		t.Fatal("the granted role reads with the privileges of its holder")
	}
}

// MySQL takes a role that is granted back to its holder, so the walk over the
// role grants must end.
func TestGrantLoopBetweenRolesStillAnswers(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: "reader", Table: table("shop", "items")}},
		Roles: []schemacache.RoleFact{
			{Holder: anonRole, Granted: "reader"},
			{Holder: "reader", Granted: anonRole},
		},
	})

	if _, ok := cache.Resource(anonRole, table("shop", "items")); !ok {
		t.Fatal("shop.items is not a resource through the role loop")
	}
}

// MySQL names a role name@host, so both shapes of one role name answer.
func TestRoleNameWithAHostFindsTheSameResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Tables:  []schemacache.TableID{table("shop", "items")},
		Selects: []schemacache.SelectFact{{Role: "myrest_anon@%", Table: table("shop", "items")}},
	})

	for _, role := range []schemacache.Role{"myrest_anon", "myrest_anon@%", "myrest_anon@localhost"} {
		if _, ok := cache.Resource(role, table("shop", "items")); !ok {
			t.Errorf("shop.items is not a resource for the role %q", role)
		}
	}
}

// Catalog facts about tables the cache does not hold cannot make a resource.
func TestGrantOnAnUnknownTableMakesNoResource(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(schemacache.Catalog{
		Columns: []schemacache.ColumnFact{{Table: table("shop", "ghost"), Name: "id"}},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: table("shop", "ghost")}},
	})

	if _, ok := cache.Resource(anonRole, table("shop", "ghost")); ok {
		t.Fatal("a grant alone made a resource out of a table the catalog does not hold")
	}
}

// An empty catalog holds no resource.
func TestEmptyCatalogHoldsNoResource(t *testing.T) {
	t.Parallel()

	if _, ok := schemacache.Build(schemacache.Catalog{}).Resource(anonRole, table("shop", "items")); ok {
		t.Fatal("an empty catalog gave a resource")
	}
}
