package schemacache_test

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestResolveEmbedManyToOneAndOneToMany(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: orders, Name: "id"},
			{Table: orders, Name: "item_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: orders, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_item", Table: orders, Columns: []string{"item_id"},
			ReferencedTable: items, ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: items},
			{Role: "anon", Table: orders},
		},
	})

	m2o, err := cache.ResolveEmbed("anon", orders, "items", "")
	if err != nil {
		t.Fatalf("orders→items: %v", err)
	}
	if m2o.Cardinality != schemacache.ManyToOne || m2o.Name != "orders_item" {
		t.Fatalf("many-to-one = %#v", m2o)
	}

	o2m, err := cache.ResolveEmbed("anon", items, "orders", "")
	if err != nil {
		t.Fatalf("items→orders: %v", err)
	}
	if o2m.Cardinality != schemacache.OneToMany || o2m.Name != "orders_item" {
		t.Fatalf("one-to-many = %#v", o2m)
	}
}

func TestResolveEmbedManyToManyAndDisambiguation(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	tags := schemacache.TableID{Database: "shop", Name: "tags"}
	itemTags := schemacache.TableID{Database: "shop", Name: "item_tags"}
	addresses := schemacache.TableID{Database: "shop", Name: "addresses"}
	deliveries := schemacache.TableID{Database: "shop", Name: "deliveries"}

	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, tags, itemTags, addresses, deliveries},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: tags, Name: "id"},
			{Table: itemTags, Name: "item_id"},
			{Table: itemTags, Name: "tag_id"},
			{Table: addresses, Name: "id"},
			{Table: deliveries, Name: "id"},
			{Table: deliveries, Name: "from_address_id"},
			{Table: deliveries, Name: "to_address_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: items, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: tags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: itemTags, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"item_id", "tag_id"}},
			{Table: addresses, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: deliveries, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "item_tags_item", Table: itemTags, Columns: []string{"item_id"}, ReferencedTable: items, ReferencedColumns: []string{"id"}},
			{Name: "item_tags_tag", Table: itemTags, Columns: []string{"tag_id"}, ReferencedTable: tags, ReferencedColumns: []string{"id"}},
			{Name: "deliveries_from", Table: deliveries, Columns: []string{"from_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
			{Name: "deliveries_to", Table: deliveries, Columns: []string{"to_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: items},
			{Role: "anon", Table: tags},
			{Role: "anon", Table: itemTags},
			{Role: "anon", Table: addresses},
			{Role: "anon", Table: deliveries},
		},
	})

	m2m, err := cache.ResolveEmbed("anon", items, "tags", "")
	if err != nil {
		t.Fatalf("items→tags: %v", err)
	}
	if m2m.Cardinality != schemacache.ManyToMany || m2m.JoinTable != itemTags {
		t.Fatalf("many-to-many = %#v", m2m)
	}

	_, err = cache.ResolveEmbed("anon", deliveries, "addresses", "")
	var ambiguous schemacache.RelationshipAmbiguous
	if !errors.As(err, &ambiguous) || len(ambiguous.Options) != 2 {
		t.Fatalf("ambiguous = %v", err)
	}
	chosen, err := cache.ResolveEmbed("anon", deliveries, "addresses", "deliveries_from")
	if err != nil || chosen.Name != "deliveries_from" {
		t.Fatalf("hinted = %#v err %v", chosen, err)
	}
}

func TestResolveEmbedMissingAndComputed(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	profiles := schemacache.TableID{Database: "shop", Name: "profiles"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, profiles},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: profiles, Name: "id"},
		},
		Routines: []schemacache.RoutineFact{{
			ID: schemacache.RoutineID{Database: "shop", Name: "item_count"}, Kind: "FUNCTION",
		}},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: items},
			{Role: "anon", Table: profiles},
		},
	})

	_, err := cache.ResolveEmbed("anon", items, "profiles", "")
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("missing = %v", err)
	}
	_, err = cache.ResolveEmbed("anon", items, "item_count", "")
	var computed schemacache.ComputedRelationship
	if !errors.As(err, &computed) {
		t.Fatalf("computed = %v", err)
	}
}
