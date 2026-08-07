package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// The schema cache that myrest builds from MySQL holds views, keys, foreign
// keys, routines, comments, and the grant data the exposure rule needs. A
// generated column is an ordinary column of a resource.
func TestSchemaCacheHoldsTheLiveCatalog(t *testing.T) {
	pool, _, service := serveWithPool(t, "myrest_fixture")

	catalog, err := pool.Catalog(t.Context(), []string{"myrest_fixture"})
	if err != nil {
		t.Fatalf("read the catalog: %v", err)
	}
	cache := schemacache.Build(catalog)

	items := schemacache.TableID{Database: "myrest_fixture", Name: "items"}
	orders := schemacache.TableID{Database: "myrest_fixture", Name: "orders"}
	itemsView := schemacache.TableID{Database: "myrest_fixture", Name: "items_view"}
	countRoutine := schemacache.RoutineID{Database: "myrest_fixture", Name: "item_count"}

	if got := cache.Views(); len(got) != 1 || got[0] != itemsView {
		t.Fatalf("views = %#v, want items_view", got)
	}
	if got := cache.Comment(items); got != "stock rows" {
		t.Fatalf("comment of items = %q, want stock rows", got)
	}
	if got := cache.KeysOf(items); len(got) != 1 || got[0].Kind != "PRIMARY" {
		t.Fatalf("keys of items = %#v, want one PRIMARY key", got)
	}
	foreignKeys := cache.ForeignKeys()
	if len(foreignKeys) != 1 || foreignKeys[0].Name != "orders_item" || foreignKeys[0].Table != orders {
		t.Fatalf("foreign keys = %#v, want orders_item", foreignKeys)
	}
	routines := cache.Routines()
	foundCount := false
	for _, routine := range routines {
		if routine.ID == countRoutine && routine.Kind == "FUNCTION" {
			foundCount = true
		}
	}
	if !foundCount {
		t.Fatalf("routines = %#v, want item_count FUNCTION among them", routines)
	}
	if !cache.HasTablePrivilege(anonRole, items, "INSERT") {
		t.Fatal("cache lost the INSERT grant")
	}
	if !cache.HasRoutinePrivilege(anonRole, countRoutine, "EXECUTE") {
		t.Fatal("cache lost the EXECUTE grant")
	}
	if _, ok := cache.Resource(anonRole, itemsView); ok {
		t.Fatal("a view became an HTTP resource before the views ticket")
	}

	columns := cache.ColumnsOf(items)
	generated := false
	for _, column := range columns {
		if column.Name == "name_len" && column.Generated {
			generated = true
		}
	}
	if !generated {
		t.Fatalf("columns of items = %#v, want generated name_len", columns)
	}

	// A generated column can be selected as an ordinary column.
	response, body := get(t, service, "/items")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"alpha","name_len":5},{"id":2,"name":"beta","name_len":4}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}
