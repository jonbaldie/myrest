package acceptance_test

import (
	"net/http"
	"testing"
)

// A paged one-to-many embed pages each parent separately over MySQL 8:
// a global page would give the first parent its rows and the second
// parent none.
func TestPagedOneToManyEmbedPagesEachParentOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(
		t,
		service,
		"/items?select=id,orders(id)&order=id.asc&orders.order=id.desc&orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":2}]},{"id":2,"orders":[{"id":3}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}

	// With a filter, an order, and an offset, parent 2 pages to empty.
	response, body = get(
		t,
		service,
		"/items?select=id,orders(id)&order=id.asc&orders.order=id.desc&orders.id=gt.1&orders.limit=1&orders.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want = `[{"id":1,"orders":[]},{"id":2,"orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedOneToManyEmbedOffsetPagesEachParentOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id)&order=id.asc&orders.order=id.desc&orders.limit=1&orders.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	// Parent 1 has orders 2 then 1 in descending order; parent 2 has
	// only order 3, so the offset pages it to empty.
	want := `[{"id":1,"orders":[{"id":1}]},{"id":2,"orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedOneToManyEmbedLimitZeroOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id)&order=id.asc&orders.limit=0",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[]},{"id":2,"orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedManyToManyEmbedPagesEachParentOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(
		t,
		service,
		"/items?select=id,tags(id,name)&order=id.asc&tags.order=id.desc&tags.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"tags":[{"id":2,"name":"cold"}]},{"id":2,"tags":[{"id":1,"name":"hot"}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}

	response, body = get(
		t,
		service,
		"/items?select=id,tags(id)&order=id.asc&tags.order=id.desc&tags.limit=1&tags.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want = `[{"id":1,"tags":[{"id":1}]},{"id":2,"tags":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedCompositeEmbedPagesEachParentOverMySQL(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/stock_lines?select=label,stock_moves(qty)&order=label.asc&stock_moves.order=qty.desc&stock_moves.limit=1&stock_moves.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	// one-aa has moves 6 then 5 descending; the other lines have one
	// move each, so the offset pages them to empty.
	want := `[{"label":"one-aa","stock_moves":[{"qty":5}]},{"label":"one-bb","stock_moves":[]},{"label":"two-aa","stock_moves":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedNestedEmbedPagesChildrenBeforeNestingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(
		t,
		service,
		"/items?select=id,orders(id,items(name))&order=id.asc&orders.order=id.desc&orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":2,"items":{"name":"alpha"}}]},{"id":2,"orders":[{"id":3,"items":{"name":"beta"}}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}

	response, body = get(
		t,
		service,
		"/orders?select=id,items(id,orders(id))&order=id.asc&items.orders.order=id.desc&items.orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want = `[{"id":1,"items":{"id":1,"orders":[{"id":2}]}},{"id":2,"items":{"id":1,"orders":[{"id":2}]}},{"id":3,"items":{"id":2,"orders":[{"id":3}]}}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}
