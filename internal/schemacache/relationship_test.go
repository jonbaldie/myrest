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

func TestResolveEmbedSelfReferentialForeignKey(t *testing.T) {
	t.Parallel()

	// One declared self-referential foreign key:
	// employees.manager_id -> employees.id.
	employees := schemacache.TableID{Database: "shop", Name: "employees"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{employees},
		Columns: []schemacache.ColumnFact{
			{Table: employees, Name: "id"},
			{Table: employees, Name: "manager_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: employees, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "employees_manager", Table: employees, Columns: []string{"manager_id"},
			ReferencedTable: employees, ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: employees},
		},
	})

	// A single self-FK yields two distinct directions, but a bare
	// employees-to-employees embed still cannot pick one.
	_, err := cache.ResolveEmbed("anon", employees, "employees", "")
	var ambiguous schemacache.RelationshipAmbiguous
	if !errors.As(err, &ambiguous) || len(ambiguous.Options) != 2 {
		t.Fatalf("unhinted self embed = %v", err)
	}
	directions := map[schemacache.Cardinality]bool{}
	for _, option := range ambiguous.Options {
		directions[option.Cardinality] = true
	}
	if !directions[schemacache.ManyToOne] || !directions[schemacache.OneToMany] {
		t.Fatalf("unhinted self embed options = %#v", ambiguous.Options)
	}

	// A constraint-name hint selects the declared many-to-one direction.
	parent, err := cache.ResolveEmbed("anon", employees, "employees", "employees_manager")
	if err != nil {
		t.Fatalf("constraint-name hint: %v", err)
	}
	if parent.Cardinality != schemacache.ManyToOne {
		t.Fatalf("constraint-name hint = %#v, want many-to-one", parent)
	}

	// A key-column hint selects the one-to-many direction the column belongs to.
	children, err := cache.ResolveEmbed("anon", employees, "employees", "manager_id")
	if err != nil {
		t.Fatalf("key-column hint: %v", err)
	}
	if children.Cardinality != schemacache.OneToMany ||
		children.Name != "employees_manager" ||
		len(children.OriginColumns) != 1 || children.OriginColumns[0] != "id" ||
		len(children.TargetColumns) != 1 || children.TargetColumns[0] != "manager_id" {
		t.Fatalf("key-column hint = %#v, want one-to-many", children)
	}
}

func TestResolveEmbedSelfReferentialForeignKeyColumnHint(t *testing.T) {
	t.Parallel()

	// Two distinct self-referential foreign keys on one table.
	employees := schemacache.TableID{Database: "shop", Name: "employees"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{employees},
		Columns: []schemacache.ColumnFact{
			{Table: employees, Name: "id"},
			{Table: employees, Name: "manager_id"},
			{Table: employees, Name: "mentor_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: employees, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "employees_manager", Table: employees, Columns: []string{"manager_id"}, ReferencedTable: employees, ReferencedColumns: []string{"id"}},
			{Name: "employees_mentor", Table: employees, Columns: []string{"mentor_id"}, ReferencedTable: employees, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: employees},
		},
	})

	// Two genuinely distinct self-FKs stay ambiguous without a hint.
	_, err := cache.ResolveEmbed("anon", employees, "employees", "")
	var ambiguous schemacache.RelationshipAmbiguous
	if !errors.As(err, &ambiguous) || len(ambiguous.Options) != 4 {
		t.Fatalf("unhinted two-self-FK embed = %v", err)
	}

	// A constraint-name hint picks the declared direction of that FK alone.
	byName, err := cache.ResolveEmbed("anon", employees, "employees", "employees_mentor")
	if err != nil {
		t.Fatalf("constraint-name hint: %v", err)
	}
	if byName.Cardinality != schemacache.ManyToOne {
		t.Fatalf("constraint-name hint = %#v, want many-to-one", byName)
	}

	// A key-column hint picks the one-to-many direction of that FK alone.
	byColumn, err := cache.ResolveEmbed("anon", employees, "employees", "mentor_id")
	if err != nil {
		t.Fatalf("key-column hint: %v", err)
	}
	if byColumn.Cardinality != schemacache.OneToMany {
		t.Fatalf("key-column hint = %#v, want one-to-many", byColumn)
	}

	// A bad hint finds no relationship.
	_, err = cache.ResolveEmbed("anon", employees, "employees", "no_such_hint")
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("bad hint = %v", err)
	}
}

func TestResolveEmbedHintByColumnAndJoinTableWithoutPK(t *testing.T) {
	t.Parallel()

	left := schemacache.TableID{Database: "shop", Name: "left_t"}
	right := schemacache.TableID{Database: "shop", Name: "right_t"}
	bridge := schemacache.TableID{Database: "shop", Name: "bridge"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{left, right, bridge},
		Columns: []schemacache.ColumnFact{
			{Table: left, Name: "id"},
			{Table: right, Name: "id"},
			{Table: bridge, Name: "left_id"},
			{Table: bridge, Name: "right_id"},
			{Table: bridge, Name: "extra"},
		},
		Keys: []schemacache.KeyFact{
			{Table: left, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: right, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			// Bridge PK does not cover both FKs, so it is not a join table.
			{Table: bridge, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"extra"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "bridge_left", Table: bridge, Columns: []string{"left_id"}, ReferencedTable: left, ReferencedColumns: []string{"id"}},
			{Name: "bridge_right", Table: bridge, Columns: []string{"right_id"}, ReferencedTable: right, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: left},
			{Role: "anon", Table: right},
			{Role: "anon", Table: bridge},
		},
	})

	_, err := cache.ResolveEmbed("anon", left, "right_t", "")
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("bridge without PK cover = %v", err)
	}

	addresses := schemacache.TableID{Database: "shop", Name: "addresses"}
	deliveries := schemacache.TableID{Database: "shop", Name: "deliveries"}
	withHint := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{addresses, deliveries},
		Columns: []schemacache.ColumnFact{
			{Table: addresses, Name: "id"},
			{Table: deliveries, Name: "from_address_id"},
			{Table: deliveries, Name: "to_address_id"},
		},
		Keys: []schemacache.KeyFact{
			{Table: addresses, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
			{Table: deliveries, Name: "PRIMARY", Kind: "PRIMARY", Columns: []string{"id"}},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{
			{Name: "deliveries_from", Table: deliveries, Columns: []string{"from_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
			{Name: "deliveries_to", Table: deliveries, Columns: []string{"to_address_id"}, ReferencedTable: addresses, ReferencedColumns: []string{"id"}},
		},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: addresses},
			{Role: "anon", Table: deliveries},
		},
	})
	chosen, err := withHint.ResolveEmbed("anon", deliveries, "addresses", "from_address_id")
	if err != nil || chosen.Name != "deliveries_from" {
		t.Fatalf("column hint = %#v err %v", chosen, err)
	}
	_, err = withHint.ResolveEmbed("anon", deliveries, "addresses", "no_such_hint")
	if !errors.As(err, &missing) {
		t.Fatalf("bad hint = %v", err)
	}
}

func TestResolveEmbedWithoutSelectOnTarget(t *testing.T) {
	t.Parallel()

	items := schemacache.TableID{Database: "shop", Name: "items"}
	orders := schemacache.TableID{Database: "shop", Name: "orders"}
	cache := schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{items, orders},
		Columns: []schemacache.ColumnFact{
			{Table: items, Name: "id"},
			{Table: orders, Name: "item_id"},
		},
		ForeignKeys: []schemacache.ForeignKeyFact{{
			Name: "orders_item", Table: orders, Columns: []string{"item_id"},
			ReferencedTable: items, ReferencedColumns: []string{"id"},
		}},
		Selects: []schemacache.SelectFact{
			{Role: "anon", Table: items},
			// orders has no SELECT for anon
		},
	})
	_, err := cache.ResolveEmbed("anon", items, "orders", "")
	var missing schemacache.RelationshipMissing
	if !errors.As(err, &missing) {
		t.Fatalf("no select = %v", err)
	}
}
