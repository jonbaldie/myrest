package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
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
