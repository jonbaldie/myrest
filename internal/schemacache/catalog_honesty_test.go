package schemacache_test

import (
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// The schema cache holds the catalog facts the parent spec names: views,
// columns with comments, keys, foreign keys, routines, and the grant data the
// exposure rule needs. Later tickets consume views, foreign keys, and routines
// over HTTP; this ticket only fills the cache.
func TestCacheHoldsTheCatalogFactsTheSpecNames(t *testing.T) {
	t.Parallel()

	items := table("shop", "items")
	orders := table("shop", "orders")
	itemsView := table("shop", "items_view")
	countRoutine := schemacache.RoutineID{Database: "shop", Name: "item_count"}

	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders},
		Views:  []schemacache.TableID{itemsView},
		RelationComments: []schemacache.CommentFact{
			{Relation: items, Comment: "stock rows"},
			{Relation: itemsView, Comment: "readable items"},
		},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id", DataType: "bigint", Nullable: false, Comment: "pk"},
			{Table: items, Name: "name", DataType: "varchar", Nullable: false},
			{Table: items, Name: "name_len", DataType: "int", Nullable: true, Generated: true},
			{Table: orders, Name: "id", DataType: "bigint", Nullable: false},
			{Table: orders, Name: "item_id", DataType: "bigint", Nullable: false},
			{Table: itemsView, Name: "id", DataType: "bigint", Nullable: false},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{
				Name:              "orders_item",
				Table:             orders,
				Columns:           []string{"item_id"},
				ReferencedTable:   items,
				ReferencedColumns: []string{"id"},
				UpdateRule:        "NO ACTION",
				DeleteRule:        "NO ACTION",
			},
		},
		Routines: []schemacache.RoutineFact{
			{
				ID:            countRoutine,
				Kind:          "FUNCTION",
				Comment:       "how many items",
				ReturnType:    "bigint",
				SQLDataAccess: "READS SQL DATA",
				Parameters: []schemacache.ParameterFact{
					{Name: "", Mode: "", Ordinal: 0, DataType: "bigint"},
				},
			},
		},
		Selects: []schemacache.SelectFact{{Role: anonRole, Table: items}},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: anonRole, Table: items, Privilege: "SELECT"},
			{Role: anonRole, Table: items, Privilege: "INSERT"},
			{Role: "writer", Table: orders, Privilege: "UPDATE"},
		},
		RoutinePrivileges: []schemacache.RoutinePrivilegeFact{
			{Role: anonRole, Routine: countRoutine, Privilege: "EXECUTE"},
		},
		Roles: []schemacache.RoleFact{{Holder: anonRole, Granted: "writer"}},
	})

	if got := cache.Views(); !reflect.DeepEqual(got, []schemacache.TableID{itemsView}) {
		t.Fatalf("views = %#v, want items_view", got)
	}
	if got := cache.Comment(items); got != "stock rows" {
		t.Fatalf("comment of items = %q, want stock rows", got)
	}
	if got := cache.Comment(itemsView); got != "readable items" {
		t.Fatalf("comment of items_view = %q, want readable items", got)
	}

	columns := cache.ColumnsOf(items)
	if want := []schemacache.Column{
		{Name: "id", DataType: "bigint", Nullable: false, Comment: "pk"},
		{Name: "name", DataType: "varchar", Nullable: false},
		{Name: "name_len", DataType: "int", Nullable: true, Generated: true},
	}; !reflect.DeepEqual(columns, want) {
		t.Fatalf("columns of items = %#v, want %#v", columns, want)
	}

	// A generated column is still an ordinary column of the resource select.
	resource, ok := cache.Resource(anonRole, items)
	if !ok {
		t.Fatal("items was not a resource")
	}
	if want := []schemacache.Column{
		{Name: "id", DataType: "bigint", Nullable: false, Comment: "pk"},
		{Name: "name", DataType: "varchar", Nullable: false},
		{Name: "name_len", DataType: "int", Nullable: true, Generated: true},
	}; !reflect.DeepEqual(resource.Columns, want) {
		t.Fatalf("resource columns = %#v, want ordinary columns including the generated one", resource.Columns)
	}

	if got := cache.KeysOf(items); !reflect.DeepEqual(got, []schemacache.KeyFact{
		{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
	}) {
		t.Fatalf("keys of items = %#v", got)
	}
	if got := cache.ForeignKeys(); !reflect.DeepEqual(got, []schemacache.ForeignKeyFact{
		{
			Name:              "orders_item",
			Table:             orders,
			Columns:           []string{"item_id"},
			ReferencedTable:   items,
			ReferencedColumns: []string{"id"},
			UpdateRule:        "NO ACTION",
			DeleteRule:        "NO ACTION",
		},
	}) {
		t.Fatalf("foreign keys = %#v", got)
	}
	if got := cache.Routines(); !reflect.DeepEqual(got, []schemacache.RoutineFact{
		{
			ID:            countRoutine,
			Kind:          "FUNCTION",
			Comment:       "how many items",
			ReturnType:    "bigint",
			SQLDataAccess: "READS SQL DATA",
			Parameters: []schemacache.ParameterFact{
				{Name: "", Mode: "", Ordinal: 0, DataType: "bigint"},
			},
		},
	}) {
		t.Fatalf("routines = %#v", got)
	}
	if !cache.HasTablePrivilege(anonRole, items, "INSERT") {
		t.Fatal("cache lost the INSERT grant the exposure rule needs")
	}
	if !cache.HasTablePrivilege(anonRole, orders, "UPDATE") {
		t.Fatal("cache lost the UPDATE grant reached through a role grant")
	}
	if !cache.HasRoutinePrivilege(anonRole, countRoutine, "EXECUTE") {
		t.Fatal("cache lost the EXECUTE grant the exposure rule needs")
	}

	// Views stay in the cache, but this ticket does not expose them as HTTP
	// resources. The views ticket consumes them.
	if _, ok := cache.Resource(anonRole, itemsView); ok {
		t.Fatal("a view became an HTTP resource before the views ticket")
	}
}
