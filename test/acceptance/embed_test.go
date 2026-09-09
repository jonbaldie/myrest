package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// embed-001 / smoke-005: GET with a nested select over a declared FK succeeds.
func TestEmbedManyToOneOverDeclaredForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/orders?select=id,items(id,name)&id=eq.1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"items":{"id":1,"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: one-to-many embed nests an array.
func TestEmbedOneToManyOverDeclaredForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,name,orders(id)&id=eq.1&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"name":"alpha","orders":[{"id":1},{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: a standalone * part keeps every parent column with the embed.
func TestEmbedStarPartKeepsParentColumns(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=*,orders(id)&id=eq.1&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"name":"alpha","name_len":5,"orders":[{"id":1},{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: an embed-only select keeps the embed and no parent columns.
func TestEmbedOnlySelectKeepsEmbedOnly(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=orders(id)&id=eq.1&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"orders":[{"id":1},{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: a nested * select expands every column at both levels.
func TestEmbedNestedStarSelects(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(*,items(*))&id=eq.1&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":1,"item_id":1,"items":{"id":1,"name":"alpha","name_len":5}},{"id":2,"item_id":1,"items":{"id":1,"name":"alpha","name_len":5}}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-002: nested filter, order, and limit succeed.
func TestEmbedNestedFilterOrderAndLimit(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id)&id=eq.1&orders.order=id.desc&orders.limit=1&orders.id=gt.1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// Many-to-many embed through a declared join table succeeds.
func TestEmbedManyToManyThroughJoinTable(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,tags(id,name)&id=eq.1&tags.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"tags":[{"id":1,"name":"hot"},{"id":2,"name":"cold"}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// Disambiguation selects one relationship when more than one applies.
func TestEmbedDisambiguationSelectsOneRelationship(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/deliveries?select=id,addresses(label)")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusMultipleChoices, "PGRST201")
	if !strings.Contains(failure.Message, "more than one relationship") {
		t.Fatalf("message = %q", failure.Message)
	}

	response, body = get(
		t,
		service,
		"/deliveries?select=id,from_addr:addresses!deliveries_from(label)",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"from_addr":{"label":"from-here"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: nested embed over declared FKs succeeds.
func TestNestedEmbedOverDeclaredForeignKeys(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id,items(name))&id=eq.2&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":2,"orders":[{"id":3,"items":{"name":"beta"}}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-003: view-chain embed with no declared FK refuses like any missing path.
func TestEmbedThroughViewWithoutForeignKeyRefuses(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,items_view(id)",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST200")
}

// embed-004: a computed relationship embed refuses stably.
func TestComputedRelationshipEmbedRefuses(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,item_count(*)",
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "computed relationship") {
		t.Fatalf("message = %q, want a computed relationship refusal", failure.Message)
	}
}

func TestEmbedNegatedLogicalGroups(t *testing.T) {
	service := serve(t, "myrest_fixture")

	t.Run("not.or", func(t *testing.T) {
		response, body := get(
			t,
			service,
			"/items?select=id,orders(id)&id=eq.1&orders.not.or=(id.eq.1)&orders.order=id.asc",
		)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		want := `[{"id":1,"orders":[{"id":2}]}]`
		if string(body) != want+"\n" {
			t.Fatalf("body = %s, want %s", body, want)
		}
	})

	t.Run("not.and", func(t *testing.T) {
		response, body := get(
			t,
			service,
			"/items?select=id,orders(id)&id=eq.1&orders.not.and=(id.gte.1,id.lte.2)&orders.order=id.asc",
		)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		want := `[{"id":1,"orders":[]}]`
		if string(body) != want+"\n" {
			t.Fatalf("body = %s, want %s", body, want)
		}
	})

	t.Run("aliased not.or", func(t *testing.T) {
		response, body := get(
			t,
			service,
			"/items?select=id,my_orders:orders(id)&id=eq.1&my_orders.not.or=(id.eq.1)&my_orders.order=id.asc",
		)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		want := `[{"id":1,"my_orders":[{"id":2}]}]`
		if string(body) != want+"\n" {
			t.Fatalf("body = %s, want %s", body, want)
		}
	})
}

// embed-001 / issue 126: a many-to-one embed over a composite foreign key
// nests the parent row and matches on every key column.
func TestEmbedManyToOneOverCompositeForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/stock_moves?select=qty,stock_lines(label)&order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"qty":5,"stock_lines":{"label":"one-aa"}},` +
		`{"qty":6,"stock_lines":{"label":"one-aa"}},` +
		`{"qty":7,"stock_lines":{"label":"one-bb"}},` +
		`{"qty":8,"stock_lines":{"label":"two-aa"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001 / issue 126: a one-to-many embed over a composite foreign key
// nests only the children that match every key column.
func TestEmbedOneToManyOverCompositeForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/stock_lines?select=label,stock_moves(qty)&order=tenant_id.asc,sku.asc&stock_moves.order=qty.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"label":"one-aa","stock_moves":[{"qty":5},{"qty":6}]},` +
		`{"label":"one-bb","stock_moves":[{"qty":7}]},` +
		`{"label":"two-aa","stock_moves":[{"qty":8}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-002 / issue 126: a hint naming the composite constraint picks it.
func TestEmbedCompositeForeignKeyHintByConstraintName(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/stock_moves?select=qty,stock_lines!stock_moves_line(label)&qty=eq.7",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"qty":7,"stock_lines":{"label":"one-bb"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}
