package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

// Seam under test: one request against a schema-cache reload. A reload that
// lands while a write runs must not split the request across two schemas
// (issue #230).

// reloadingWriter commits an insert and, inside the same call, reloads the
// schema cache, the way a SIGUSR1 reload can land during a write.
type reloadingWriter struct {
	writer
	cache   *schemacache.Cache
	reload  schemacache.Catalog
	commits int
}

func (w *reloadingWriter) Insert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	rows []map[string]any,
	options writequery.Options,
) (writequery.Result, error) {
	result, err := w.writer.Insert(ctx, role, table, rows, options)
	if err != nil {
		return result, err
	}
	w.commits++
	w.cache.Replace(w.reload)
	return result, nil
}

// manyToManyWriteCatalog grants INSERT on items, and SELECT on items, tags,
// and the item_tags join table. withJoin false leaves item_tags out.
func manyToManyWriteCatalog(withJoin bool) schemacache.Catalog {
	items := schemacache.TableID{Database: "shop", Name: "items"}
	tags := schemacache.TableID{Database: "shop", Name: "tags"}
	itemTags := schemacache.TableID{Database: "shop", Name: "item_tags"}
	catalog := schemacache.Catalog{
		Tables: []schemacache.TableID{items, tags},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: tags, Name: "id"},
			{Table: tags, Name: "name"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: tags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: items},
			{Role: "myrest_anon", Table: tags},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: items, Privilege: "INSERT"},
		},
	}
	if !withJoin {
		return catalog
	}
	catalog.Tables = append(catalog.Tables, itemTags)
	catalog.Columns = append(catalog.Columns,
		schemacache.ColumnFact{Table: itemTags, Name: "item_id"},
		schemacache.ColumnFact{Table: itemTags, Name: "tag_id"},
	)
	catalog.Keys = append(catalog.Keys, schemacache.KeyFact{
		Table: itemTags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"item_id", "tag_id"},
	})
	catalog.ForeignKeys = []schemacache.ForeignKeyFact{
		{Name: "item_tags_item", Table: itemTags, Columns: []string{"item_id"}, ReferencedTable: items, ReferencedColumns: []string{"id"}},
		{Name: "item_tags_tag", Table: itemTags, Columns: []string{"tag_id"}, ReferencedTable: tags, ReferencedColumns: []string{"id"}},
	}
	catalog.Selects = append(catalog.Selects, schemacache.SelectFact{Role: "myrest_anon", Table: itemTags})
	return catalog
}

// A reload during a representation write never refuses after the commit: the
// request answers from the schema that admitted it.
func TestReloadDuringWriteKeepsAdmittedSnapshot(t *testing.T) {
	t.Parallel()

	cache := schemacache.Build(manyToManyWriteCatalog(true))
	sink := &reloadingWriter{
		writer: writer{
			resultRows: []rows.Row{{Columns: []string{"id"}, Values: []any{int64(1)}}},
			resultKeys: []map[string]any{{"id": int64(1)}},
		},
		cache:  cache,
		reload: manyToManyWriteCatalog(false),
	}
	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"item_id", "tag_id"}, Values: []any{int64(1), int64(7)}}}},
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(7), "hot"}}}},
	}}
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: settings(),
		Cache:    cache,
		Reader:   source,
		Writer:   sink,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items?select=id,tags(name)",
		strings.NewReader(`{"id":1}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if sink.commits != 1 {
		t.Fatalf("commits = %d, want 1", sink.commits)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d after a commit, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if want := `[{"id":1,"tags":[{"name":"hot"}]}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}

	// The reload still holds for the next request (ADR 0003).
	if _, ok := cache.Resource("myrest_anon", schemacache.TableID{Database: "shop", Name: "item_tags"}); ok {
		t.Fatal("reload did not replace the cache")
	}
}
