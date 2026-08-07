package readquery_test

import (
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseNestedSelectEmbed(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select": []string{"id,items(id,name)"},
	}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Columns) != 1 || query.Columns[0].Name != "id" {
		t.Fatalf("columns = %#v", query.Columns)
	}
	if len(query.Embeds) != 1 {
		t.Fatalf("embeds = %#v", query.Embeds)
	}
	embed := query.Embeds[0]
	if embed.Resource != "items" || embed.Alias != "" || embed.Hint != "" {
		t.Fatalf("embed head = %#v", embed)
	}
	if len(embed.Columns) != 2 || embed.Columns[0].Name != "id" || embed.Columns[1].Name != "name" {
		t.Fatalf("embed columns = %#v", embed.Columns)
	}
}

func TestParseEmbedHintAliasAndNestedFilterOrderLimit(t *testing.T) {
	t.Parallel()

	values := url.Values{
		"select":         []string{"id,billing:addresses!deliveries_from(label)"},
		"billing.order":  []string{"label.asc"},
		"billing.limit":  []string{"1"},
		"billing.offset": []string{"0"},
		"billing.label":  []string{"eq.from-here"},
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Embeds) != 1 {
		t.Fatalf("embeds = %#v", query.Embeds)
	}
	embed := query.Embeds[0]
	if embed.Resource != "addresses" || embed.Alias != "billing" || embed.Hint != "deliveries_from" {
		t.Fatalf("embed = %#v", embed)
	}
	if len(embed.Columns) != 1 || embed.Columns[0].Name != "label" {
		t.Fatalf("embed columns = %#v", embed.Columns)
	}
	if len(embed.Order) != 1 || embed.Order[0].Column != "label" || embed.Order[0].Desc {
		t.Fatalf("embed order = %#v", embed.Order)
	}
	if embed.Limit == nil || *embed.Limit != 1 || embed.Offset != 0 {
		t.Fatalf("embed page = %#v offset %d", embed.Limit, embed.Offset)
	}
	if len(embed.Filters) != 1 || embed.Filters[0].Column != "label" || embed.Filters[0].Value != "from-here" {
		t.Fatalf("embed filters = %#v", embed.Filters)
	}
	if len(query.Filters) != 0 {
		t.Fatalf("top-level filters = %#v, want none", query.Filters)
	}
}
