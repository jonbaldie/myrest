package schemacache_test

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestResolveEmbedUsesHeldSnapshotAfterReplace(t *testing.T) {
	t.Parallel()
	parent := schemacache.TableID{Database: "shop", Name: "parent"}
	child := schemacache.TableID{Database: "shop", Name: "child"}
	catalog := schemacache.Catalog{
		Tables:      []schemacache.TableID{parent, child},
		Selects:     []schemacache.SelectFact{{Role: "anon", Table: parent}, {Role: "anon", Table: child}},
		ForeignKeys: []schemacache.ForeignKeyFact{{Name: "old", Table: child, Columns: []string{"parent_id"}, ReferencedTable: parent, ReferencedColumns: []string{"id"}}},
	}
	cache := schemacache.Build(catalog)
	old := schemacache.ReadSnapshot(cache)
	catalog.ForeignKeys = []schemacache.ForeignKeyFact{{Name: "new", Table: child, Columns: []string{"parent_id"}, ReferencedTable: parent, ReferencedColumns: []string{"id"}}}
	cache.Replace(catalog)
	for _, tc := range []struct {
		snapshot schemacache.Snapshot
		want     string
	}{{old, "old"}, {schemacache.ReadSnapshot(cache), "new"}} {
		rel, err := schemacache.ResolveEmbedIn(tc.snapshot, "anon", child, "parent", "")
		if err != nil || rel.Name != tc.want {
			t.Fatalf("relationship = %#v, %v; want %s", rel, err, tc.want)
		}
		_, err = schemacache.ResolveEmbedIn(tc.snapshot, "other", child, "parent", "")
		var missing schemacache.RelationshipMissing
		if !errors.As(err, &missing) {
			t.Fatalf("unprivileged relationship error = %v", err)
		}
	}
}
