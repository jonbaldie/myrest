package readquery_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseNestedSelectEmbed(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select": []string{"id,items(id,name)"},
	}, readquery.CountNone)
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
	query, err := readquery.Parse(values, readquery.CountNone)
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

func TestParseEmbedNegatedLogicalGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		selectQ string
		key     string
		raw     string
		or      bool
		filters int
	}{
		{name: "not.or", selectQ: "id,orders(id)", key: "orders.not.or", raw: "(id.eq.1)", or: true, filters: 1},
		{name: "not.and", selectQ: "id,orders(id)", key: "orders.not.and", raw: "(id.gte.1,id.lte.2)", or: false, filters: 2},
		{name: "aliased not.or", selectQ: "id,my_orders:orders(id)", key: "my_orders.not.or", raw: "(id.eq.1)", or: true, filters: 1},
		{name: "aliased not.and", selectQ: "id,my_orders:orders(id)", key: "my_orders.not.and", raw: "(id.eq.1)", or: false, filters: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			values := url.Values{
				"select": []string{tc.selectQ},
				tc.key:   []string{tc.raw},
			}
			query, err := readquery.Parse(values, readquery.CountNone)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(query.Embeds) != 1 {
				t.Fatalf("embeds = %#v, want one embed", query.Embeds)
			}
			embed := query.Embeds[0]
			if len(embed.Groups) != 1 {
				t.Fatalf("embed.Groups = %#v, want one group", embed.Groups)
			}
			group := embed.Groups[0]
			if !group.Negated || group.Or != tc.or || len(group.Filters) != tc.filters {
				t.Fatalf("group = %#v, want negated group (or=%v, filters=%d)", group, tc.or, tc.filters)
			}
		})
	}
}

func TestParseEmbedMultipleLogicalGroups(t *testing.T) {
	t.Parallel()

	values := url.Values{
		"select":         []string{"id,orders(id)"},
		"orders.not.or":  []string{"(id.eq.1)"},
		"orders.not.and": []string{"(id.eq.2)"},
	}
	query, err := readquery.Parse(values, readquery.CountNone)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Embeds) != 1 {
		t.Fatalf("embeds = %#v, want one embed", query.Embeds)
	}
	embed := query.Embeds[0]
	if len(embed.Groups) != 2 {
		t.Fatalf("embed.Groups = %#v, want 2 groups", embed.Groups)
	}
	if embed.Groups[0].Negated != true || embed.Groups[0].Or != false {
		t.Fatalf("first group = %#v, want negated and", embed.Groups[0])
	}
	if embed.Groups[1].Negated != true || embed.Groups[1].Or != true {
		t.Fatalf("second group = %#v, want negated or", embed.Groups[1])
	}
}

func TestParseEmbedRefusesEmptyLogicalGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		raw  string
	}{
		{name: "empty not.or", key: "orders.not.or", raw: "()"},
		{name: "empty not.and", key: "orders.not.and", raw: "()"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			values := url.Values{
				"select": []string{"id,orders(id)"},
				tc.key:   []string{tc.raw},
			}
			_, err := readquery.Parse(values, readquery.CountNone)
			var failure readquery.ParseFailure
			if err == nil || !errors.As(err, &failure) || failure.Gap {
				t.Fatalf("query %s=%s err = %v, want non-gap ParseFailure", tc.key, tc.raw, err)
			}
		})
	}
}

func TestParseChainedEmbedParamsTargetNestedEmbed(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select":              []string{"id,orders(id,items(id))"},
		"orders.items.id":     []string{"eq.1"},
		"orders.items.order":  []string{"id.desc"},
		"orders.items.limit":  []string{"1"},
		"orders.items.offset": []string{"2"},
		"orders.items.and":    []string{"(id.gte.1)"},
		"orders.items.not.or": []string{"(id.eq.3)"},
	}, readquery.CountNone)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	orders := query.Embeds[0]
	if len(orders.Filters) != 0 || len(orders.Order) != 0 || orders.Limit != nil || orders.Offset != 0 || len(orders.Groups) != 0 {
		t.Fatalf("parent embed got child params: %#v", orders)
	}
	items := orders.Embeds[0]
	if len(items.Filters) != 1 || items.Filters[0].Column != "id" || items.Filters[0].Op != readquery.OpEq {
		t.Fatalf("child filters = %#v", items.Filters)
	}
	if len(items.Order) != 1 || items.Order[0].Column != "id" || !items.Order[0].Desc {
		t.Fatalf("child order = %#v", items.Order)
	}
	if items.Limit == nil || *items.Limit != 1 || items.Offset != 2 {
		t.Fatalf("child page = %v, %d", items.Limit, items.Offset)
	}
	if len(items.Groups) != 2 {
		t.Fatalf("child groups = %#v", items.Groups)
	}
}

func TestParseChainedEmbedParamPicksNamedBranch(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select":             []string{"id,orders(id,items(id)),shipments(id,items(id))"},
		"shipments.items.id": []string{"eq.1"},
	}, readquery.CountNone)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(query.Embeds[0].Embeds[0].Filters); got != 0 {
		t.Fatalf("orders.items filters = %d, want 0", got)
	}
	if got := len(query.Embeds[1].Embeds[0].Filters); got != 1 {
		t.Fatalf("shipments.items filters = %d, want 1", got)
	}
}

func TestParseChainedEmbedParamMatchesAliasOrResource(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"orders.line.id", "orders.items.id"} {
		query, err := readquery.Parse(url.Values{
			"select": []string{"id,orders(id,line:items(id))"},
			key:      []string{"eq.1"},
		}, readquery.CountNone)
		if err != nil {
			t.Fatalf("%s: Parse: %v", key, err)
		}
		if got := len(query.Embeds[0].Embeds[0].Filters); got != 1 {
			t.Fatalf("%s: nested filters = %d, want 1", key, got)
		}
	}
}

func TestParseShortEmbedParamPrefersTopLevelThenNested(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select":   []string{"id,orders(id,items(id)),items(id)"},
		"items.id": []string{"eq.1"},
	}, readquery.CountNone)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if nested, top := len(query.Embeds[0].Embeds[0].Filters), len(query.Embeds[1].Filters); nested != 0 || top != 1 {
		t.Fatalf("nested filters = %d, top filters = %d, want 0 and 1", nested, top)
	}

	query, err = readquery.Parse(url.Values{
		"select":   []string{"id,tags(id),orders(id,items(id))"},
		"items.id": []string{"eq.1"},
	}, readquery.CountNone)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(query.Embeds[1].Embeds[0].Filters); got != 1 {
		t.Fatalf("short form nested filters = %d, want 1", got)
	}
}

func TestParseChainedEmbedParamRefusals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key     string
		message string
	}{
		{key: "orders.", message: "embed parameter 'orders.' is incomplete"},
		{key: "orders.items.", message: "embed parameter 'orders.items.' is incomplete"},
		{key: "nope.id", message: "no embed named 'nope' for parameter 'nope.id'"},
		{key: "orders.nope.id", message: "no embed named 'nope' for parameter 'orders.nope.id'"},
		{key: "orders.items.nope.id", message: "no embed named 'nope' for parameter 'orders.items.nope.id'"},
	}
	for _, tc := range cases {
		_, err := readquery.Parse(url.Values{
			"select": []string{"id,orders(id,items(id))"},
			tc.key:   []string{"eq.1"},
		}, readquery.CountNone)
		var failure readquery.ParseFailure
		if !errors.As(err, &failure) || failure.Message != tc.message {
			t.Fatalf("%s: err = %v, want %q", tc.key, err, tc.message)
		}
	}
}
