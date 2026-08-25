package mysqldb

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// FuzzReadFilterValueStaysAnArgument checks the SQL boundary after a valid
// PostgREST-shaped filter has passed the parser.
func FuzzReadFilterValueStaysAnArgument(f *testing.F) {
	f.Add("x' OR 1=1 --")
	f.Add("semi;colon")
	f.Add("plain")
	f.Fuzz(func(t *testing.T, value string) {
		if value == "" || strings.Contains(value, "*") {
			t.Skip()
		}

		query, err := readquery.Parse(url.Values{"name": {"eq." + value}}, nil)
		if err != nil {
			t.Skip()
		}
		table := schemacache.Table{
			ID:      schemacache.TableID{Database: "shop", Name: "items"},
			Columns: []schemacache.Column{{Name: "name"}},
		}
		parts, err := buildSelect(table, query)
		if err != nil {
			t.Fatalf("buildSelect: %v", err)
		}
		control, err := readquery.Parse(url.Values{"name": {"eq.control"}}, nil)
		if err != nil {
			t.Fatalf("parse control query: %v", err)
		}
		controlParts, err := buildSelect(table, control)
		if err != nil {
			t.Fatalf("build control query: %v", err)
		}
		if parts.statement != controlParts.statement {
			t.Fatalf("filter value changed SQL text: %q", parts.statement)
		}
		if len(parts.args) != 1 || parts.args[0] != value {
			t.Fatalf("args = %#v, want filter value %q", parts.args, value)
		}
	})
}
