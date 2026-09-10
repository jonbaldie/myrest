package readquery_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseSelectColumnsAndAlias(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"select": []string{"id,fullName:name"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []readquery.Column{{Name: "id"}, {Name: "name", Alias: "fullName"}}
	if len(query.Columns) != len(want) {
		t.Fatalf("columns = %#v, want %#v", query.Columns, want)
	}
	for i := range want {
		if query.Columns[i] != want[i] {
			t.Fatalf("columns[%d] = %#v, want %#v", i, query.Columns[i], want[i])
		}
	}
}

func TestParseEqFilterAndOrderLimitOffset(t *testing.T) {
	t.Parallel()

	values := url.Values{
		"name":   []string{"eq.alpha"},
		"order":  []string{"id.desc"},
		"limit":  []string{"1"},
		"offset": []string{"0"},
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 {
		t.Fatalf("filters = %#v", query.Filters)
	}
	got := query.Filters[0]
	if got.Column != "name" || got.Op != readquery.OpEq || got.Value != "alpha" || got.Negated {
		t.Fatalf("filter = %#v", got)
	}
	if len(query.Order) != 1 || query.Order[0] != (readquery.Order{Column: "id", Desc: true}) {
		t.Fatalf("order = %#v", query.Order)
	}
	if query.Limit == nil || *query.Limit != 1 || query.Offset != 0 {
		t.Fatalf("page = limit %#v offset %d", query.Limit, query.Offset)
	}
}

func TestParsePreferCountExact(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{}, []string{"count=exact"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !query.ExactCount {
		t.Fatal("ExactCount is false")
	}
}

func TestParseRejectsUnknownOperator(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{"name": []string{"bogus.alpha"}}, nil)
	if err == nil {
		t.Fatal("Parse accepted an unknown operator")
	}
}

func TestParseRejectsExtraLogicalClosingParenthesis(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{"or": []string{"(id.eq.1))"}}, nil)
	if err == nil {
		t.Fatal("Parse accepted an extra logical closing parenthesis")
	}
}

func TestParseRejectsExtraInClosingParenthesis(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{"id": []string{"in.(1))"}}, nil)
	if err == nil {
		t.Fatal("Parse accepted an extra in-filter closing parenthesis")
	}
}

func TestParseInListUnescapesQuotedDoubleQuote(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"id": []string{`in.("a""b")`}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 || len(query.Filters[0].Values) != 1 || query.Filters[0].Values[0] != `a"b` {
		t.Fatalf("filter = %#v", query.Filters)
	}
}

func TestParseInListPreservesInvalidUTF8Bytes(t *testing.T) {
	t.Parallel()

	value := string([]byte{0xea})
	query, err := readquery.Parse(url.Values{"id": []string{"in.(\"" + value + "\")"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 || len(query.Filters[0].Values) != 1 || query.Filters[0].Values[0] != value {
		t.Fatalf("filter = %#v, want byte %x", query.Filters, []byte(value))
	}
}

func TestParseAcceptsILikeAsPartialMatch(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"name": []string{"ilike.ALPHA"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 || query.Filters[0].Op != readquery.OpILike || query.Filters[0].Value != "ALPHA" {
		t.Fatalf("filters = %#v", query.Filters)
	}
}

func TestParseRefusesPreferCountPlanned(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{}, []string{"count=planned"})
	var failure readquery.ParseFailure
	if err == nil || !errors.As(err, &failure) || !failure.Gap {
		t.Fatalf("err = %v, want a gap ParseFailure", err)
	}
}

func TestEffectiveLimitTakesTheLowerCap(t *testing.T) {
	t.Parallel()

	client := uint64(50)
	maxRows := uint64(10)
	query := readquery.Query{Limit: &client, MaxRows: &maxRows}
	got := query.EffectiveLimit()
	if got == nil || *got != 10 {
		t.Fatalf("EffectiveLimit = %#v, want 10", got)
	}
}

func TestFullMatchOperatorsAreListed(t *testing.T) {
	t.Parallel()

	if len(readquery.FullMatchOperators) != 10 {
		t.Fatalf("FullMatchOperators = %#v", readquery.FullMatchOperators)
	}
}

func TestParseJSONPathRefusals(t *testing.T) {
	t.Parallel()
	cases := []string{`meta#>>{blood_type}`, `meta->"blood type"`, `meta->*`, `meta->>phones->0`}
	for _, selectPart := range cases {
		_, err := readquery.Parse(url.Values{"select": []string{selectPart}}, nil)
		var failure readquery.ParseFailure
		if err == nil || !errors.As(err, &failure) || !failure.Gap {
			t.Fatalf("select %q err = %v, want gap", selectPart, err)
		}
	}
}

func TestParseJSONPathSelectAndFilter(t *testing.T) {
	t.Parallel()
	values := url.Values{
		"select":            []string{"id,meta->>blood_type"},
		"meta->>blood_type": []string{"eq.A-"},
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Columns) != 2 || query.Columns[1].Path == nil || !query.Columns[1].Path.AsText {
		t.Fatalf("columns = %#v", query.Columns)
	}
	if len(query.Filters) != 1 || query.Filters[0].Path == nil || query.Filters[0].Value != "A-" {
		t.Fatalf("filters = %#v", query.Filters)
	}
}

func TestParseChainedJSONPathSelectFilterAndOrder(t *testing.T) {
	t.Parallel()
	values := url.Values{
		"select":                   []string{"id,meta->phones->0->>number"},
		"meta->phones->0->>number": []string{"eq.917-929-5745"},
		"order":                    []string{"meta->phones->0->>number.desc"},
	}
	query, err := readquery.Parse(values, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Columns) != 2 || query.Columns[1].Path == nil || !query.Columns[1].Path.AsText {
		t.Fatalf("columns = %#v", query.Columns)
	}
	if len(query.Columns[1].Path.Steps) != 3 {
		t.Fatalf("steps = %#v, want 3 steps", query.Columns[1].Path.Steps)
	}
	if len(query.Filters) != 1 || query.Filters[0].Path == nil || query.Filters[0].Value != "917-929-5745" {
		t.Fatalf("filters = %#v", query.Filters)
	}
	if len(query.Order) != 1 || query.Order[0].Path == nil || !query.Order[0].Desc {
		t.Fatalf("order = %#v", query.Order)
	}
}

func TestParseTopLevelNegatedLogicalGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		raw     string
		or      bool
		filters int
	}{
		{name: "and", key: "not.and", raw: "(id.gte.1,id.lte.2)", or: false, filters: 2},
		{name: "or", key: "not.or", raw: "(id.eq.1)", or: true, filters: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query, err := readquery.Parse(url.Values{test.key: []string{test.raw}}, nil)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(query.Groups) != 1 {
				t.Fatalf("groups = %#v, want one group", query.Groups)
			}
			group := query.Groups[0]
			if !group.Negated || group.Or != test.or || len(group.Filters) != test.filters {
				t.Fatalf("group = %#v, want one negated group", group)
			}
		})
	}
}

func TestParseRefusesEmptyLogicalGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		raw  string
	}{
		{name: "empty or", key: "or", raw: "()"},
		{name: "empty and", key: "and", raw: "()"},
		{name: "empty not.or", key: "not.or", raw: "()"},
		{name: "empty not.and", key: "not.and", raw: "()"},
		{name: "nested empty group or(and())", key: "or", raw: "(and())"},
		{name: "nested empty group and(not.or())", key: "and", raw: "(not.or())"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := readquery.Parse(url.Values{test.key: []string{test.raw}}, nil)
			var failure readquery.ParseFailure
			if err == nil || !errors.As(err, &failure) || failure.Gap {
				t.Fatalf("query %s=%s err = %v, want non-gap ParseFailure", test.key, test.raw, err)
			}
		})
	}
}

func TestParseStarPartWithEmbedRecordsSelectAll(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"select": []string{"*,orders(id)"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !query.SelectAll {
		t.Fatalf("SelectAll = false, want true for a standalone * part")
	}
	if len(query.Columns) != 0 {
		t.Fatalf("columns = %#v, want none", query.Columns)
	}
	if len(query.Embeds) != 1 {
		t.Fatalf("embeds = %#v, want one", query.Embeds)
	}
	if query.Embeds[0].Resource != "orders" {
		t.Fatalf("embed resource = %q, want orders", query.Embeds[0].Resource)
	}
}

func TestParseStarPartWithColumnKeepsExplicitSelect(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"select": []string{"*,name"}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if query.SelectAll {
		t.Fatalf("SelectAll = true, want false when an explicit column part coexists with *")
	}
	if len(query.Columns) != 1 || query.Columns[0].Name != "name" {
		t.Fatalf("columns = %#v, want name", query.Columns)
	}
}

// An is filter takes one documented value; anything else is a parse failure
// at the parse layer, before any SQL is built. Issue #130.
func TestParseRejectsUnknownIsValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		raw  string
	}{
		{name: "unknown value", key: "id", raw: "is.bogus"},
		{name: "unknown negated value", key: "id", raw: "not.is.bogus"},
		{name: "uppercase value", key: "id", raw: "is.NULL"},
		{name: "unknown value in a group", key: "or", raw: "(id.is.bogus)"},
	}
	for _, test := range cases {
		_, err := readquery.Parse(url.Values{test.key: []string{test.raw}}, nil)
		var failure readquery.ParseFailure
		if err == nil || !errors.As(err, &failure) || failure.Gap {
			t.Fatalf("query %s=%s err = %v, want a non-gap ParseFailure", test.key, test.raw, err)
		}
	}
}

func TestParseAcceptsDocumentedIsValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"null", "not_null", "true", "false", "unknown"} {
		query, err := readquery.Parse(url.Values{"id": {"is." + value}}, nil)
		if err != nil {
			t.Fatalf("Parse is.%s: %v", value, err)
		}
		if len(query.Filters) != 1 || query.Filters[0].Op != readquery.OpIs || query.Filters[0].Value != value {
			t.Fatalf("is.%s: filters = %#v", value, query.Filters)
		}
	}
}

// A double-quoted scalar filter value decodes to its literal, the same way
// in.(...) values do. Issue #145.
func TestParseQuotedScalarFilterValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		raw   string
		value string
	}{
		{name: "quoted value decodes", raw: `eq."alpha"`, value: "alpha"},
		{name: "quoted value keeps the unquoted form", raw: "eq.alpha", value: "alpha"},
		{name: "quoted comma is value data", raw: `eq."a,b"`, value: "a,b"},
		{name: "quoted reserved characters are value data", raw: `eq."a.b(c)"`, value: "a.b(c)"},
		{name: "doubled quote is an escaped quote", raw: `eq."say ""hi"""`, value: `say "hi"`},
		{name: "empty quoted value", raw: `eq.""`, value: ""},
		{name: "neq", raw: `neq."a,b"`, value: "a,b"},
		{name: "gt", raw: `gt."a,b"`, value: "a,b"},
		{name: "gte", raw: `gte."a,b"`, value: "a,b"},
		{name: "lt", raw: `lt."a,b"`, value: "a,b"},
		{name: "lte", raw: `lte."a,b"`, value: "a,b"},
		{name: "like", raw: `like."a*b"`, value: "a*b"},
		{name: "ilike", raw: `ilike."a,b"`, value: "a,b"},
		{name: "isdistinct", raw: `isdistinct."a,b"`, value: "a,b"},
		{name: "negated", raw: `not.eq."a,b"`, value: "a,b"},
		{name: "json path filter", raw: `eq."a,b"`, value: "a,b"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			key := "name"
			if test.name == "json path filter" {
				key = "meta->>tag"
			}
			query, err := readquery.Parse(url.Values{key: []string{test.raw}}, nil)
			if err != nil {
				t.Fatalf("Parse %s: %v", test.raw, err)
			}
			if len(query.Filters) != 1 {
				t.Fatalf("filters = %#v", query.Filters)
			}
			if got := query.Filters[0].Value; got != test.value {
				t.Fatalf("value = %q, want %q", got, test.value)
			}
		})
	}
}

func TestParseQuotedScalarValueInLogicalGroup(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"or": []string{`(name.eq."a,b",name.eq.beta)`}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Groups) != 1 || len(query.Groups[0].Filters) != 2 {
		t.Fatalf("groups = %#v", query.Groups)
	}
	if got := query.Groups[0].Filters[0].Value; got != "a,b" {
		t.Fatalf("first filter value = %q, want a,b", got)
	}
}

func TestParseQuotedScalarValueOnEmbedFilter(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{
		"select":    []string{"*,orders(id)"},
		"orders.id": []string{`eq."a,b"`},
	}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Embeds) != 1 || len(query.Embeds[0].Filters) != 1 {
		t.Fatalf("embeds = %#v", query.Embeds)
	}
	if got := query.Embeds[0].Filters[0].Value; got != "a,b" {
		t.Fatalf("embed filter value = %q, want a,b", got)
	}
}

// A quoted value inside an in list keeps the existing list-splitting rules.
func TestParseInListKeepsQuotedElementRules(t *testing.T) {
	t.Parallel()

	query, err := readquery.Parse(url.Values{"name": []string{`in.("a,b",alpha)`}}, nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(query.Filters) != 1 || len(query.Filters[0].Values) != 2 {
		t.Fatalf("filters = %#v", query.Filters)
	}
	if query.Filters[0].Values[0] != "a,b" || query.Filters[0].Values[1] != "alpha" {
		t.Fatalf("values = %#v", query.Filters[0].Values)
	}
}

// A malformed quoted scalar value is a parse failure, not a literal value
// with quote characters. Issue #145.
func TestParseRejectsMalformedQuotedScalarValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{name: "unterminated quote", raw: `eq."a,b`},
		{name: "trailing text after closing quote", raw: `eq."a"b`},
		{name: "lone quote", raw: `eq."`},
		{name: "unterminated in a group", raw: `(name.eq."a,b)`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			key := "name"
			if strings.HasPrefix(test.raw, "(") {
				key = "or"
			}
			_, err := readquery.Parse(url.Values{key: []string{test.raw}}, nil)
			var failure readquery.ParseFailure
			if err == nil || !errors.As(err, &failure) || failure.Gap {
				t.Fatalf("query name=%s err = %v, want a non-gap ParseFailure", test.raw, err)
			}
		})
	}
}

// The is operator keeps its documented value set; a quoted value does not
// become an is value.
func TestParseIsOperatorKeepsLiteralValidation(t *testing.T) {
	t.Parallel()

	_, err := readquery.Parse(url.Values{"id": []string{`is."null"`}}, nil)
	var failure readquery.ParseFailure
	if err == nil || !errors.As(err, &failure) || failure.Gap {
		t.Fatalf("err = %v, want a non-gap ParseFailure", err)
	}
}
