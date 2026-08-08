package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestBuildInsertSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildInsert(table, []string{"name"}, []map[string]any{
		{"name": "gamma"},
		{"name": "delta"},
	})
	if err != nil {
		t.Fatalf("buildInsert: %v", err)
	}
	want := "INSERT INTO `shop`.`items` (`name`) VALUES (?), (?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != "gamma" || parts.args[1] != "delta" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildUpdateSQLWithFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpdate(
		table,
		map[string]any{"name": "alpha2"},
		readquery.Query{Filters: []readquery.Filter{{
			Column: "name", Op: readquery.OpEq, Value: "alpha",
		}}},
	)
	if err != nil {
		t.Fatalf("buildUpdate: %v", err)
	}
	want := "UPDATE `shop`.`items` SET `name` = ? WHERE `name` = ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != "alpha2" || parts.args[1] != "alpha" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildDeleteSQLWithFilter(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildDelete(table, readquery.Query{Filters: []readquery.Filter{{
		Column: "name", Op: readquery.OpEq, Value: "beta",
	}}})
	if err != nil {
		t.Fatalf("buildDelete: %v", err)
	}
	want := "DELETE FROM `shop`.`items` WHERE `name` = ?"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 1 || parts.args[0] != "beta" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildUpsertMergeSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpsert(
		table,
		map[string]any{"id": 1, "name": "alpha2"},
		[]string{"id"},
		httpapi.UpsertMergeDuplicates,
	)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	want := "INSERT INTO `shop`.`items` (`id`, `name`) VALUES (?,?) AS `new` ON DUPLICATE KEY UPDATE `name` = `new`.`name`"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
	if len(parts.args) != 2 || parts.args[0] != 1 || parts.args[1] != "alpha2" {
		t.Fatalf("args = %#v", parts.args)
	}
}

func TestBuildUpsertIgnoreSQL(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID: schemacache.TableID{Database: "shop", Name: "items"},
		Columns: []schemacache.Column{
			{Name: "id"},
			{Name: "name"},
		},
	}
	parts, err := buildUpsert(
		table,
		map[string]any{"id": 99, "name": "fresh"},
		[]string{"id"},
		httpapi.UpsertIgnoreDuplicates,
	)
	if err != nil {
		t.Fatalf("buildUpsert: %v", err)
	}
	want := "INSERT IGNORE INTO `shop`.`items` (`id`, `name`) VALUES (?,?)"
	if parts.statement != want {
		t.Fatalf("statement = %q, want %q", parts.statement, want)
	}
}
