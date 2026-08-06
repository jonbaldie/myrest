package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestSelectStatementReadsEveryColumnOfTheResource(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		Schema:  "shop",
		Name:    "items",
		Columns: []schemacache.Column{{Name: "id"}, {Name: "name"}},
	}

	statement := selectStatement(table, columnNames(table))
	want := "SELECT `id`, `name` FROM `shop`.`items`"
	if statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
}

func TestQuoteIdentifierKeepsABackQuoteInsideTheName(t *testing.T) {
	t.Parallel()

	if quoted := quoteIdentifier("od`d"); quoted != "`od``d`" {
		t.Fatalf("quoted = %q, want %q", quoted, "`od``d`")
	}
}

func TestJSONValueReadsTextAsAString(t *testing.T) {
	t.Parallel()

	if value := jsonValue([]byte("alpha")); value != "alpha" {
		t.Fatalf("value = %#v, want the string alpha", value)
	}
}

func TestJSONValueKeepsOtherValuesAsTheyAre(t *testing.T) {
	t.Parallel()

	if value := jsonValue(int64(7)); value != int64(7) {
		t.Fatalf("value = %#v, want 7", value)
	}
	if value := jsonValue(nil); value != nil {
		t.Fatalf("value = %#v, want nil", value)
	}
}

func TestRoleOfGranteeReadsBothGranteeShapes(t *testing.T) {
	t.Parallel()

	for grantee, want := range map[string]string{
		"'myrest_anon'@'%'": "myrest_anon",
		"myrest_anon@%":     "myrest_anon",
		"myrest_anon":       "myrest_anon",
		"'unfinished":       "",
	} {
		t.Run(grantee, func(t *testing.T) {
			t.Parallel()

			if role := roleOfGrantee(grantee); role != want {
				t.Fatalf("role of %q = %q, want %q", grantee, role, want)
			}
		})
	}
}

func TestPlaceholdersMatchTheNumberOfDatabases(t *testing.T) {
	t.Parallel()

	for count, want := range map[int]string{0: "", 1: "?", 3: "?, ?, ?"} {
		if list := placeholders(count); list != want {
			t.Fatalf("placeholders(%d) = %q, want %q", count, list, want)
		}
	}
}

func TestArgumentsPassEveryDatabase(t *testing.T) {
	t.Parallel()

	passed := arguments([]string{"shop", "warehouse"})
	if len(passed) != 2 || passed[0] != "shop" || passed[1] != "warehouse" {
		t.Fatalf("arguments = %#v, want shop and warehouse", passed)
	}
}
