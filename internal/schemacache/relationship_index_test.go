package schemacache

import (
	"fmt"
	"reflect"
	"testing"
)

func TestRelationshipIndexExcludesUnrelatedCatalog(t *testing.T) {
	parent := TableID{Database: "shop", Name: "parent"}
	target := TableID{Database: "shop", Name: "target"}
	join := TableID{Database: "shop", Name: "parent_target"}
	catalog := Catalog{
		Tables: []TableID{parent, target, join},
		Keys: []KeyFact{
			{Table: parent, Kind: "PRIMARY", Columns: []string{"id", "tenant"}},
			{Table: target, Kind: "PRIMARY", Columns: []string{"id", "tenant"}},
			{Table: join, Kind: "PRIMARY", Columns: []string{"parent_id", "target_id", "tenant"}},
		},
		ForeignKeys: []ForeignKeyFact{
			{Name: "parent_fk", Table: join, Columns: []string{"parent_id", "tenant"}, ReferencedTable: parent, ReferencedColumns: []string{"id", "tenant"}},
			{Name: "target_fk", Table: join, Columns: []string{"target_id", "tenant"}, ReferencedTable: target, ReferencedColumns: []string{"id", "tenant"}},
		},
		Selects: []SelectFact{{Role: "anon", Table: target}},
	}
	for i := 0; i < 1000; i++ {
		a := TableID{Database: "other", Name: fmt.Sprintf("a%d", i)}
		b := TableID{Database: "other", Name: fmt.Sprintf("b%d", i)}
		catalog.Tables = append(catalog.Tables, a, b)
		catalog.ForeignKeys = append(catalog.ForeignKeys,
			ForeignKeyFact{Table: a, ReferencedTable: b},
			ForeignKeyFact{Table: b, ReferencedTable: parent},
		)
	}
	snapshot := ReadSnapshot(Build(catalog))
	if got := len(snapshot.relationshipIndex.direct[tablePair{parent, target}]); got != 0 {
		t.Fatalf("direct candidates = %d, want 0", got)
	}
	if got := len(snapshot.relationshipIndex.incoming[parent]); got != 1001 {
		t.Fatalf("parent candidates = %d, want 1001", got)
	}
	if got := len(snapshot.relationshipIndex.incoming[target]); got != 1 {
		t.Fatalf("target candidates = %d, want 1", got)
	}
	rel, err := ResolveEmbedIn(snapshot, "anon", parent, "target", "")
	if err != nil {
		t.Fatal(err)
	}
	want := Relationship{
		Cardinality: ManyToMany, Name: "parent_target", Origin: parent, Target: target,
		OriginColumns: []string{"id", "tenant"}, TargetColumns: []string{"id", "tenant"},
		JoinTable: join, JoinOriginColumns: []string{"parent_id", "tenant"},
		JoinTargetColumns: []string{"target_id", "tenant"},
	}
	if !reflect.DeepEqual(rel, want) {
		t.Fatalf("relationship = %#v, want %#v", rel, want)
	}
}
