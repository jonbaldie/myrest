package readquery_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseEmbedJoinTypeRefuses(t *testing.T) {
	t.Parallel()
	for _, selectValue := range []string{
		"orders!inner(id)",
		"orders!left(id)",
		"orders!orders_item!inner(id)",
		"orders!left!orders_item(id)",
		"purchases:orders!inner(id)",
		"orders(id,items!left(name))",
		"...orders!inner(id)",
	} {
		t.Run(selectValue, func(t *testing.T) {
			query, err := readquery.Parse(url.Values{"select": {selectValue}}, readquery.CountNone)
			var failure readquery.ParseFailure
			if !errors.As(err, &failure) || !failure.Gap || !strings.Contains(failure.Message, "join types") {
				t.Fatalf("Parse = %#v, %v; want join type gap", query, err)
			}
		})
	}
}

func TestParseEmbedHintWithJoinTypeSubstring(t *testing.T) {
	t.Parallel()
	for _, hint := range []string{"orders_inner", "left_address", "inner_id"} {
		query, err := readquery.Parse(url.Values{"select": {"orders!" + hint + "(id)"}}, readquery.CountNone)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(query.Embeds) != 1 || query.Embeds[0].Hint != hint {
			t.Fatalf("embeds = %#v, want hint %q", query.Embeds, hint)
		}
	}
}
