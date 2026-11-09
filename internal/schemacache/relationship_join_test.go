package schemacache_test

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestResolveEmbedDoesNotTreatEndpointsAsJoinTables(t *testing.T) {
	a := schemacache.TableID{Database: "shop", Name: "a"}
	b := schemacache.TableID{Database: "shop", Name: "b"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{a, b},
		Keys: []schemacache.KeyFact{
			{Table: a, Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: b, Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "aa", Table: a, Columns: []string{"id"}, ReferencedTable: a, ReferencedColumns: []string{"id"}},
			{Name: "ab", Table: a, Columns: []string{"id"}, ReferencedTable: b, ReferencedColumns: []string{"id"}},
			{Name: "ba", Table: b, Columns: []string{"id"}, ReferencedTable: a, ReferencedColumns: []string{"id"}},
			{Name: "bb", Table: b, Columns: []string{"id"}, ReferencedTable: b, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{{Role: "anon", Table: b}},
	})
	_, err := cache.ResolveEmbed("anon", a, "b", "")
	var ambiguous schemacache.RelationshipAmbiguous
	if !errors.As(err, &ambiguous) || len(ambiguous.Options) != 2 {
		t.Fatalf("relationships = %#v, error = %v; want two direct paths", ambiguous.Options, err)
	}
}
