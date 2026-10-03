package httpapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for aggregate reads.

func aggregatesOn() config.Settings {
	resolved := settings()
	resolved.DB.AggregatesEnabled = true
	return resolved
}

// read-011: with aggregates off, an aggregate select refuses stably.
func TestAggregateSelectRefusesWhenDisabled(t *testing.T) {
	t.Parallel()

	response, body := get(t, serve(t, &reader{}, settings()), "/items?select=count()")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST123")
	if failure.Message != "Use of aggregate functions is not allowed" {
		t.Fatalf("message = %q", failure.Message)
	}
}

// read-011 also covers aggregates inside embeds while the gate is off.
func TestAggregateInsideEmbedRefusesWhenDisabled(t *testing.T) {
	t.Parallel()

	response, body := get(
		t,
		serveEmbed(t, &reader{}),
		"/items?select=name,orders(count())",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST123")
}

// read-010: with aggregates enabled, an aggregate select reaches the reader.
func TestAggregateSelectPassesQueryWhenEnabled(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"count"}, Values: []any{int64(2)}},
	}}
	response, body := get(t, serve(t, source, aggregatesOn()), "/items?select=count()")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if want := `[{"count":2}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.query.Columns) != 1 || source.query.Columns[0].Agg != readquery.AggCount {
		t.Fatalf("query columns = %#v", source.query.Columns)
	}
}

// read-010 auto group: non-aggregate columns stay on the reader query.
func TestAggregateSelectKeepsGroupColumns(t *testing.T) {
	t.Parallel()

	source := &reader{read: []rows.Row{
		{Columns: []string{"sum", "name"}, Values: []any{int64(1), "alpha"}},
	}}
	response, body := get(
		t,
		serve(t, source, aggregatesOn()),
		"/items?select=id.sum(),name",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if len(source.query.Columns) != 2 {
		t.Fatalf("columns = %#v", source.query.Columns)
	}
	if source.query.Columns[0].Agg != readquery.AggSum || source.query.Columns[1].Name != "name" {
		t.Fatalf("columns = %#v", source.query.Columns)
	}
	if want := `[{"sum":1,"name":"alpha"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s", body)
	}
}

// read-012: allowed aggregate + embed combination reaches the reader chain.
func TestAggregateInsideEmbedWhenEnabled(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
		{rows: []rows.Row{{Columns: []string{"count", "item_id"}, Values: []any{int64(2), int64(1)}}}},
	}}
	resolved := aggregatesOn()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    embedCache(),
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/items?select=name,orders(count())&id=eq.1")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"name":"alpha","orders":[{"count":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func manyToManyCache() *schemacache.Cache {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	tags := schemacache.TableID{Database: "shop", Name: "tags"}
	itemTags := schemacache.TableID{Database: "shop", Name: "item_tags"}
	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, tags, itemTags},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: tags, Name: "id"},
			{Table: tags, Name: "name"},
			{Table: itemTags, Name: "item_id"},
			{Table: itemTags, Name: "tag_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: tags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: itemTags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"item_id", "tag_id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "item_tags_item", Table: itemTags, Columns: []string{"item_id"}, ReferencedTable: items, ReferencedColumns: []string{"id"}},
			{Name: "item_tags_tag", Table: itemTags, Columns: []string{"tag_id"}, ReferencedTable: tags, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: tags},
			{Role: "myrest_anon", Table: itemTags},
		},
	})
}

func serveManyToMany(t *testing.T, source httpapi.Reader) *httpapi.Service {
	t.Helper()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: aggregatesOn(),
		Cache:    manyToManyCache(),
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })
	return service
}

// read-012: a many-to-many aggregate query keeps each parent's target scope.
func TestAggregateInsideManyToManyEmbedUsesParentScope(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{
			{Columns: []string{"id"}, Values: []any{int64(1)}},
			{Columns: []string{"id"}, Values: []any{int64(2)}},
		}},
		{rows: []rows.Row{
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}},
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(2)}},
			{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(2), int64(1)}},
		}},
		{rows: []rows.Row{{Columns: []string{"count", "_myrest_aggregate_presence"}, Values: []any{int64(2), int64(2)}}}},
		{rows: []rows.Row{{Columns: []string{"count", "_myrest_aggregate_presence"}, Values: []any{int64(1), int64(1)}}}},
	}}
	service := serveManyToMany(t, source)

	response, body := get(t, service, "/items?select=id,tags(count())")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"tags":[{"count":2}]},{"id":2,"tags":[{"count":1}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) != 4 {
		t.Fatalf("reader calls = %d, want 4", len(source.seen))
	}
	for i, query := range source.seen[2:] {
		if len(query.Filters) != 1 || query.Filters[0].Column != "id" || query.Filters[0].Op != readquery.OpIn || len(query.Filters[0].Values) == 0 {
			t.Fatalf("aggregate query filters = %#v", query.Filters)
		}
		if i == 0 && (len(query.Filters[0].Values) != 2 || query.Filters[0].Values[0] != "1" || query.Filters[0].Values[1] != "2") {
			t.Fatalf("item 1 target keys = %#v", query.Filters[0].Values)
		}
		if i == 1 && (len(query.Filters[0].Values) != 1 || query.Filters[0].Values[0] != "1") {
			t.Fatalf("item 2 target keys = %#v", query.Filters[0].Values)
		}
		foundPresence := false
		for _, column := range query.Columns {
			if column.Name == "id" && column.Agg == "" {
				t.Fatalf("aggregate query selected target id: %#v", query.Columns)
			}
			if column.Alias == "_myrest_aggregate_presence" && column.Agg == readquery.AggCount {
				foundPresence = true
			}
		}
		if !foundPresence {
			t.Fatalf("aggregate query has no presence count: %#v", query.Columns)
		}
	}
}

func TestAggregateInsideManyToManyReturnsEmptyForNoMatches(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"count", "_myrest_aggregate_presence"}, Values: []any{int64(0), int64(0)}}}},
	}}
	service := serveManyToMany(t, source)

	response, body := get(t, service, "/items?select=id,tags(count())&tags.name=eq.missing")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	if want := `[{"id":1,"tags":[]}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestAggregateInsideManyToManyPropagatesReadFailure(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(1)}}}},
		{err: errors.New("read failed")},
	}}
	response, body := get(t, serveManyToMany(t, source), "/items?select=id,tags(count())")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want %d", response.StatusCode, body, http.StatusInternalServerError)
	}
}

// read-013: aggregates inside a to-many spread refuse with the parity-target code.
func TestAggregateInToManySpreadRefuses(t *testing.T) {
	t.Parallel()

	resolved := aggregatesOn()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    embedCache(),
		Reader:   &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/items?select=id,...orders(count())")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST127")
	if failure.Message != "Feature not implemented" {
		t.Fatalf("message = %q", failure.Message)
	}
	if failure.Details != "Aggregates are not implemented for one-to-many or many-to-many spreads." {
		t.Fatalf("details = %#v", failure.Details)
	}
}

// Grouping by an embedded resource injects the join column into the reader query.
func TestAggregateGroupedByEmbedInjectsJoinColumn(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"count", "item_id"}, Values: []any{int64(2), int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
	}}
	resolved := aggregatesOn()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    embedCache(),
		Reader:   source,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	response, body := get(t, service, "/orders?select=count(),items(name)")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	// projectEmbedRow keeps asked columns only; items(name) drops id.
	want := `[{"count":2,"items":{"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
	if len(source.seen) < 1 {
		t.Fatal("reader was not called")
	}
	// First read is the parent aggregate query and must hold the join column.
	parent := source.seen[0]
	foundJoin := false
	for _, column := range parent.Columns {
		if column.Name == "item_id" && column.Agg == "" {
			foundJoin = true
		}
	}
	if !foundJoin {
		t.Fatalf("parent columns = %#v, want injected item_id", parent.Columns)
	}
}
