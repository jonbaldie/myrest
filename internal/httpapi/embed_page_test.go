package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

var errReadFailed = errors.New("read failed")

func listenAggregates(t *testing.T, source httpapi.Reader) (*httpapi.Service, error) {
	t.Helper()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: aggregatesOn(),
		Cache:    embedCache(),
		Reader:   source,
	})
	if err != nil {
		return nil, err
	}
	go func() { _ = service.Serve() }()
	return service, nil
}

// pagingDBReader stands in for the database at the Reader seam: it holds
// every matching child row, applies the key condition, the embed filter,
// the embed order, and the limit/offset of the query it receives the way
// SQL does, and records how many rows it returned to the service. A small
// embed page must therefore return only the page, not every matching row.
type pagingDBReader struct {
	parents  []rows.Row
	orders   []rows.Row
	seen     []readquery.Query
	returned int
}

func (r *pagingDBReader) Read(
	_ context.Context,
	_ schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	r.seen = append(r.seen, query)
	if table.ID.Name == "items" {
		return readquery.Result{Rows: r.parents}, nil
	}
	matched := r.matchingOrders(query)
	// Apply the SQL page, if the service asked for one. An unpaged batch
	// read returns every matching row, which the test counts.
	if query.Offset > 0 {
		if query.Offset >= uint64(len(matched)) {
			matched = nil
		} else {
			matched = matched[query.Offset:]
		}
	}
	if limit := query.EffectiveLimit(); limit != nil && uint64(len(matched)) > *limit {
		matched = matched[:*limit]
	}
	r.returned += len(matched)
	return readquery.Result{Rows: matched}, nil
}

// matchingOrders applies the key IN filter, the id>1 embed filter, and
// the id order of the queries this reader is used with.
func (r *pagingDBReader) matchingOrders(query readquery.Query) []rows.Row {
	var keys map[int64]bool
	for _, filter := range query.Filters {
		if filter.Column == "item_id" && filter.Op == readquery.OpIn {
			keys = map[int64]bool{}
			for _, raw := range filter.Values {
				n, _ := strconv.ParseInt(raw, 10, 64)
				keys[n] = true
			}
		}
	}
	var out []rows.Row
	for _, order := range r.orders {
		itemID, _ := order.Values[1].(int64)
		if keys != nil && !keys[itemID] {
			continue
		}
		id, _ := order.Values[0].(int64)
		if id <= 1 {
			continue
		}
		out = append(out, order)
	}
	desc := len(query.Order) > 0 && query.Order[0].Desc
	sort.SliceStable(out, func(i, j int) bool {
		left, _ := out[i].Values[0].(int64)
		right, _ := out[j].Values[0].(int64)
		if desc {
			return left > right
		}
		return left < right
	})
	return out
}

func pagingOrders() []rows.Row {
	var out []rows.Row
	for id := int64(1); id <= 8; id++ {
		itemID := int64(1)
		if id >= 6 && id <= 7 {
			itemID = 2
		}
		if id == 8 {
			itemID = 3
		}
		out = append(out, rows.Row{
			Columns: []string{"id", "item_id"},
			Values:  []any{id, itemID},
		})
	}
	// Order 8 is the only match of parent 3, so an offset of 1 pages it
	// to an empty page. Orders 6 and 7 belong to parent 2.
	return out
}

func pagingParents() []rows.Row {
	return []rows.Row{
		{Columns: []string{"id"}, Values: []any{int64(1)}},
		{Columns: []string{"id"}, Values: []any{int64(2)}},
		{Columns: []string{"id"}, Values: []any{int64(3)}},
	}
}

// A multi-parent embed pages each parent separately before the service
// receives the child rows: the reader returns only the three paged rows,
// not the seven matching rows.
func TestPagedOneToManyEmbedBoundsChildReadsPerParent(t *testing.T) {
	t.Parallel()

	source := &pagingDBReader{parents: pagingParents(), orders: pagingOrders()}
	response, body := get(
		t,
		serveEmbed(t, source),
		"/items?select=id,orders(id)&order=id.asc&orders.order=id.desc&orders.id=gt.1&orders.limit=2&orders.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[{"id":4},{"id":3}]},{"id":2,"orders":[{"id":6}]},{"id":3,"orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	// Parent read plus one paged child read per distinct parent.
	if len(source.seen) != 4 {
		t.Fatalf("reader calls = %d, want 4 (parent plus one per parent)", len(source.seen))
	}
	for i, query := range source.seen[1:] {
		if query.Limit == nil || *query.Limit != 2 || query.Offset != 1 {
			t.Fatalf("child query %d page = limit %#v offset %d, want limit 2 offset 1", i, query.Limit, query.Offset)
		}
		if len(query.Order) != 1 || !query.Order[0].Desc {
			t.Fatalf("child query %d order = %#v, want id desc", i, query.Order)
		}
	}
	if source.returned != 3 {
		t.Fatalf("child rows returned to service = %d, want 3 paged rows, not every matching row", source.returned)
	}
}

func TestPagedOneToManyEmbedLimitZeroReturnsEmptyPages(t *testing.T) {
	t.Parallel()

	source := &pagingDBReader{parents: pagingParents(), orders: pagingOrders()}
	response, body := get(
		t,
		serveEmbed(t, source),
		"/items?select=id,orders(id)&order=id.asc&orders.order=id.desc&orders.limit=0",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[]},{"id":2,"orders":[]},{"id":3,"orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 4 {
		t.Fatalf("reader calls = %d, want 4", len(source.seen))
	}
	for i, query := range source.seen[1:] {
		if query.Limit == nil || *query.Limit != 0 {
			t.Fatalf("child query %d limit = %#v, want 0", i, query.Limit)
		}
	}
	if source.returned != 0 {
		t.Fatalf("child rows returned to service = %d, want 0 for limit=0", source.returned)
	}
}

// Without a requested page the embed keeps the single batch read.
func TestUnpagedOneToManyEmbedKeepsBatchRead(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
			{Columns: []string{"id"}, Values: []any{int64(2)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}},
			{Columns: []string{"id", "item_id"}, Values: []any{int64(2), int64(2)}},
		}},
	}}
	response, body := get(t, serveEmbed(t, source), "/items?select=id,orders(id)&order=id.asc&orders.order=id.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[{"id":1}]},{"id":2,"orders":[{"id":2}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 2 {
		t.Fatalf("reader calls = %d, want 2 for an unpaged batch read", len(source.seen))
	}
	if source.seen[1].Limit != nil || source.seen[1].Offset != 0 {
		t.Fatalf("unpaged child query page = %#v offset %d, want no page", source.seen[1].Limit, source.seen[1].Offset)
	}
}

func TestPagedCompositeOneToManyEmbedBoundsChildReadsPerParent(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"name", "tenant_id", "id"}, Values: []any{"alpha", int64(7), int64(3)}},
			{Columns: []string{"name", "tenant_id", "id"}, Values: []any{"beta", int64(7), int64(4)}},
		}},
		// Paged rows the database returns for parent (7, 3): orders 11
		// then 10 in descending order, offset 1 limit 1 keeps 10.
		{rows: []rows.Row{
			{Columns: []string{"id", "tenant_id", "item_id"}, Values: []any{int64(10), int64(7), int64(3)}},
		}},
		// Parent (7, 4) has one match, so offset 1 pages it to empty.
		{rows: []rows.Row{}},
	}}
	response, body := get(
		t,
		serveComposite(t, source),
		"/items?select=name,orders(id)&order=name.asc&orders.order=id.desc&orders.limit=1&orders.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"name":"alpha","orders":[{"id":10}]},{"name":"beta","orders":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 3 {
		t.Fatalf("reader calls = %d, want 3 (parent plus one per parent)", len(source.seen))
	}
	for i, query := range source.seen[1:] {
		if query.Limit == nil || *query.Limit != 1 || query.Offset != 1 {
			t.Fatalf("child query %d page = limit %#v offset %d, want limit 1 offset 1", i, query.Limit, query.Offset)
		}
		group := compositeGroup(t, query)
		if len(group.Groups) != 1 {
			t.Fatalf("child query %d key tuples = %#v, want one parent tuple", i, group.Groups)
		}
		first := group.Groups[0].Filters
		if len(first) != 2 || first[0].Column != "tenant_id" || first[1].Column != "item_id" {
			t.Fatalf("child query %d tuple filters = %#v, want tenant_id and item_id", i, first)
		}
	}
}

func TestPagedManyToManyEmbedBoundsChildReadsPerParent(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
			{Columns: []string{"id"}, Values: []any{int64(2)}},
		}},
		// Links of parent 1 only.
		{rows: []rows.Row{
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}},
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(2)}},
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(3)}},
		}},
		// Tags of parent 1, paged by the database: id desc is 3,2,1 and
		// offset 1 limit 1 keeps tag 2.
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(2), "cold"}},
		}},
		// Links of parent 2 only.
		{rows: []rows.Row{
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(2), int64(2)}},
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(2), int64(3)}},
		}},
		// Tags of parent 2, paged: id desc is 3,2 and the page keeps 2.
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(2), "cold"}},
		}},
	}}
	response, body := get(
		t,
		serveManyToMany(t, source),
		"/items?select=id,tags(name)&order=id.asc&tags.order=id.desc&tags.limit=1&tags.offset=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"tags":[{"name":"cold"}]},{"id":2,"tags":[{"name":"cold"}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 5 {
		t.Fatalf("reader calls = %d, want 5 (parent, links and tags per parent)", len(source.seen))
	}
	for i, index := range []int{2, 4} {
		query := source.seen[index]
		if query.Limit == nil || *query.Limit != 1 || query.Offset != 1 {
			t.Fatalf("tag query %d page = limit %#v offset %d, want limit 1 offset 1", i, query.Limit, query.Offset)
		}
		if len(query.Order) != 1 || !query.Order[0].Desc {
			t.Fatalf("tag query %d order = %#v, want id desc", i, query.Order)
		}
	}
}

func TestPagedManyToManyEmbedLimitZeroReturnsEmptyPages(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}},
		}},
		{rows: []rows.Row{}},
	}}
	response, body := get(
		t,
		serveManyToMany(t, source),
		"/items?select=id,tags(name)&tags.limit=0",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"tags":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 3 {
		t.Fatalf("reader calls = %d, want 3", len(source.seen))
	}
	if source.seen[2].Limit == nil || *source.seen[2].Limit != 0 {
		t.Fatalf("tag query limit = %#v, want 0", source.seen[2].Limit)
	}
}

func TestPagedNestedEmbedPagesChildrenBeforeNesting(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
			{Columns: []string{"id"}, Values: []any{int64(2)}},
		}},
		// Orders of parent 1, paged to order 4 only.
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(4), int64(1)}},
		}},
		// The nested items read for the paged orders of parent 1 only.
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
		}},
		// Orders of parent 2, paged to order 7 only.
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(7), int64(2)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
		}},
	}}
	response, body := get(
		t,
		serveEmbed(t, source),
		"/items?select=id,orders(id,items(name))&order=id.asc&orders.order=id.desc&orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"orders":[{"id":4,"items":{"name":"alpha"}}]},{"id":2,"orders":[{"id":7,"items":{"name":"beta"}}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 5 {
		t.Fatalf("reader calls = %d, want 5 (parent, orders and nested items per parent)", len(source.seen))
	}
	for i, index := range []int{1, 3} {
		query := source.seen[index]
		if query.Limit == nil || *query.Limit != 1 {
			t.Fatalf("orders query %d limit = %#v, want 1", i, query.Limit)
		}
	}
	// The nested items read must hold only the paged order's key.
	nested := source.seen[2]
	if len(nested.Filters) != 1 || len(nested.Filters[0].Values) != 1 || nested.Filters[0].Values[0] != "1" {
		t.Fatalf("nested items filters = %#v, want only the paged order key 1", nested.Filters)
	}
}

func TestPagedDeeplyNestedEmbedPagesEachLevelPerParent(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}},
			{Columns: []string{"id", "item_id"}, Values: []any{int64(2), int64(2)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}},
			{Columns: []string{"id", "name"}, Values: []any{int64(2), "beta"}},
		}},
		// Orders of item 1, paged to order 5 only.
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(5), int64(1)}},
		}},
		// Orders of item 2, paged to order 6 only.
		{rows: []rows.Row{
			{Columns: []string{"id", "item_id"}, Values: []any{int64(6), int64(2)}},
		}},
	}}
	response, body := get(
		t,
		serveEmbed(t, source),
		"/orders?select=id,items(id,orders(id))&order=id.asc&items.orders.order=id.desc&items.orders.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"id":1,"orders":[{"id":5}]}},{"id":2,"items":{"id":2,"orders":[{"id":6}]}}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 4 {
		t.Fatalf("reader calls = %d, want 4 (parents, items, orders per item)", len(source.seen))
	}
	for i, query := range source.seen[2:] {
		if query.Limit == nil || *query.Limit != 1 {
			t.Fatalf("nested orders query %d limit = %#v, want 1", i, query.Limit)
		}
		if len(query.Filters) != 1 || len(query.Filters[0].Values) != 1 {
			t.Fatalf("nested orders query %d filters = %#v, want one parent key", i, query.Filters)
		}
	}
}

func TestPagedAggregateOneToManyEmbedBoundsRead(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
		{rows: []rows.Row{{Columns: []string{"count", "item_id"}, Values: []any{int64(2), int64(1)}}}},
	}}
	service, err := listenAggregates(t, source)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	response, body := get(t, service, "/items?select=name,orders(count())&id=eq.1&orders.limit=1")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"name":"alpha","orders":[{"count":2}]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 2 {
		t.Fatalf("reader calls = %d, want 2", len(source.seen))
	}
	if source.seen[1].Limit == nil || *source.seen[1].Limit != 1 {
		t.Fatalf("aggregate child query limit = %#v, want 1", source.seen[1].Limit)
	}
}

func TestPagedAggregateManyToManyEmbedLimitZero(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}}}},
		{rows: []rows.Row{}},
	}}
	response, body := get(
		t,
		serveManyToMany(t, source),
		"/items?select=id,tags(count())&tags.limit=0",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"tags":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 3 {
		t.Fatalf("reader calls = %d, want 3", len(source.seen))
	}
	if source.seen[2].Limit == nil || *source.seen[2].Limit != 0 {
		t.Fatalf("aggregate tag query limit = %#v, want 0", source.seen[2].Limit)
	}
}

func TestPagedOneToManyEmbedPropagatesReadFailure(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{err: errReadFailed},
	}}
	response, body := get(t, serveEmbed(t, source), "/items?select=id,orders(id)&orders.limit=1")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", response.StatusCode, body)
	}
}

func TestPagedManyToManyEmbedPropagatesTargetFailure(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}}}},
		{err: errReadFailed},
	}}
	response, body := get(t, serveManyToMany(t, source), "/items?select=id,tags(name)&tags.limit=1")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", response.StatusCode, body)
	}
}

func TestPagedManyToManyEmbedParentWithoutLinksIsEmpty(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
			{Columns: []string{"id"}, Values: []any{int64(2)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"id", "name"}, Values: []any{int64(1), "hot"}},
		}},
		// Parent 2 has no links, so no tag read runs for it.
		{rows: []rows.Row{}},
	}}
	response, body := get(
		t,
		serveManyToMany(t, source),
		"/items?select=id,tags(name)&order=id.asc&tags.limit=1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"tags":[{"name":"hot"}]},{"id":2,"tags":[]}]` + "\n"
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestPagedManyToManyEmbedPropagatesLinkFailure(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{err: errReadFailed},
	}}
	response, body := get(t, serveManyToMany(t, source), "/items?select=id,tags(name)&tags.limit=1")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500", response.StatusCode, body)
	}
}
