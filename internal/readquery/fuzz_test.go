package readquery

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzParseDoesNotPanic(f *testing.F) {
	f.Add("id,orders(id)", "id.desc", "alpha", "id.eq.1")
	f.Add("meta->>name", "meta->>name.asc", "A-", "or(id.eq.1)")
	f.Fuzz(func(t *testing.T, selectText, orderText, filterValue, groupValue string) {
		values := url.Values{}
		values.Set("select", selectText)
		values.Set("order", orderText)
		values.Set("name", "eq."+filterValue)
		values.Set("or", "("+groupValue+")")
		_, _ = Parse(values, nil)
	})
}

func FuzzParseRejectsExtraLogicalClosingParen(f *testing.F) {
	f.Add("1")
	f.Fuzz(func(t *testing.T, value string) {
		if strings.ContainsAny(value, "()") {
			t.Skip()
		}
		_, err := Parse(url.Values{"or": {"(id.eq." + value + "))"}}, nil)
		if err == nil {
			t.Fatalf("accepted logical filter with an extra closing parenthesis: %q", value)
		}
	})
}

func FuzzParseRejectsExtraInClosingParen(f *testing.F) {
	f.Add("1")
	f.Fuzz(func(t *testing.T, value string) {
		if strings.ContainsAny(value, "()") {
			t.Skip()
		}
		_, err := Parse(url.Values{"id": {"in.(" + value + "))"}}, nil)
		if err == nil {
			t.Fatalf("accepted an in filter with an extra closing parenthesis: %q", value)
		}
	})
}

func FuzzParseInListPreservesEscapedQuotes(f *testing.F) {
	f.Add(`a"b`)
	f.Fuzz(func(t *testing.T, value string) {
		if strings.ContainsAny(value, "(),") {
			t.Skip()
		}
		escaped := strings.ReplaceAll(value, `"`, `""`)
		query, err := Parse(url.Values{"id": {`in.("` + escaped + `")`}}, nil)
		if err != nil {
			t.Skip()
		}
		if len(query.Filters) != 1 || len(query.Filters[0].Values) != 1 || query.Filters[0].Values[0] != value {
			t.Fatalf("filter = %#v, want value %q", query.Filters, value)
		}
	})
}
