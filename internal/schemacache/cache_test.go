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

func routine(database, name string) schemacache.RoutineID {
	return schemacache.RoutineID{Database: database, Name: name}
}

// A routine the active database role can EXECUTE is a routine resource.
func TestRoutineWithExecuteIsAResource(t *testing.T) {
	t.Parallel()

	count := routine("shop", "item_count")
	cache := schemacache.Build(schemacache.Catalog{
		Routines: []schemacache.RoutineFact{{
			ID:         count,
			Kind:       "FUNCTION",
			ReturnType: "bigint",
			Parameters: []schemacache.ParameterFact{
				{Ordinal: 0, DataType: "bigint"},
			},
		}},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: anonRole, Routine: count, Privilege: "EXECUTE"},
		},
	})

	found, ok := cache.Routine(anonRole, count)
	if !ok {
		t.Fatal("shop.item_count is not a routine resource for myrest_anon")
	}
	if found.ID != count || found.Kind != "FUNCTION" {
		t.Fatalf("routine = %#v, want FUNCTION shop.item_count", found)
	}
}

// A routine without EXECUTE is not a routine resource.
func TestRoutineWithoutExecuteIsNotAResource(t *testing.T) {
	t.Parallel()

	secret := routine("shop", "secret_count")
	cache := schemacache.Build(schemacache.Catalog{
		Routines: []schemacache.RoutineFact{{
			ID:   secret,
			Kind: "FUNCTION",
		}},
	})

	if _, ok := cache.Routine(anonRole, secret); ok {
		t.Fatal("a routine without EXECUTE became a resource")
	}
}

// A view with SELECT is a resource under the same exposure rule as a table.
func TestViewWithSelectIsAResource(t *testing.T) {
	t.Parallel()

	itemsView := table("shop", "items_view")
	cache := schemacache.Build(schemacache.Catalog{
		Views: []schemacache.TableID{itemsView},
		Columns: []schemacache.ColumnFact{
			{Table: itemsView, Name: "id"},
			{Table: itemsView, Name: "name"},
		},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: itemsView}},
	})

	found, ok := cache.Resource(anonRole, itemsView)
	if !ok {
		t.Fatal("a view with SELECT was not a resource")
	}
	if want := []schemacache.Column{{Name: "id"}, {Name: "name"}}; !reflect.DeepEqual(found.Columns, want) {
		t.Fatalf("columns = %#v, want %#v", found.Columns, want)
	}
}

// A view without SELECT is not a resource.
func TestViewWithoutSelectIsNotAResource(t *testing.T) {
	t.Parallel()

	locked := table("shop", "locked_view")
	cache := schemacache.Build(schemacache.Catalog{
		Views:   []schemacache.TableID{locked},
		Columns: []schemacache.ColumnFact{{Table: locked, Name: "id"}},
	})

	if _, ok := cache.Resource(anonRole, locked); ok {
		t.Fatal("a view without SELECT became a resource")
	}
}

// A base table is writable. An updatable view is writable. A view MySQL marks
// as not updatable is not writable.
func TestViewWritabilityFollowsTheCatalogFlag(t *testing.T) {
	t.Parallel()

	items := table("shop", "items")
	writable := table("shop", "items_view")
	readonly := table("shop", "items_stats")
	cache := schemacache.Build(schemacache.Catalog{
		Tables:          []schemacache.TableID{items},
		Views:           []schemacache.TableID{writable, readonly},
		UpdatableViews:  []schemacache.TableID{writable},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: writable, Name: "id"},
			{Table: readonly, Name: "total"},
		},
	})

	if !cache.IsWritable(items) {
		t.Fatal("a base table must be writable")
	}
	if !cache.IsWritable(writable) {
		t.Fatal("an updatable view must be writable")
	}
	if cache.IsWritable(readonly) {
		t.Fatal("a non-updatable view must not be writable")
	}
	if cache.IsWritable(table("shop", "ghost")) {
		t.Fatal("an unknown name must not be writable")
	}
}

// Replace puts the new catalog into the cache, so a table that arrives after
// the first build becomes a resource without a new Cache value.
func TestReplaceShowsANewSelectGrant(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(catalog())
	if _, ok := cache.Resource(anonRole, table("shop", "secrets")); ok {
		t.Fatal("secrets was a resource before the grant arrived")
	}

	updated := catalog()
	updated.Selects = append(updated.Selects, schemacache.SelectFact{
		Role:  anonRole,
		Table: table("shop", "secrets"),
	})
	cache.Replace(updated)

	found, ok := cache.Resource(anonRole, table("shop", "secrets"))
	if !ok {
		t.Fatal("Replace did not make secrets a resource")
	}
	if want := []schemacache.Column{{Name: "payload"}}; !reflect.DeepEqual(found.Columns, want) {
		t.Fatalf("columns = %#v, want %#v", found.Columns, want)
	}
}
