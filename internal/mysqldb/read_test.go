package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestSelectStatementReadsEveryColumnOfTheResource(t *testing.T) {
	t.Parallel()

	table := schemacache.Table{
		ID:      schemacache.TableID{Database: "shop", Name: "items"},
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
