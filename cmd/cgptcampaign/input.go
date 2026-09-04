package main

import (
	"math/rand/v2"
	"net/url"
	"strconv"
	"strings"
)

type kind string

const (
	kindHTTP  kind = "http"
	kindParse kind = "parse"
	kindJWT   kind = "jwt"
)

type input struct {
	Kind     kind
	Method   string
	Path     string
	RawQuery string
	Headers  map[string][]string
	Body     string
	Token    string
	Select   string
	Order    string
	Filter   string
	Group    string
}

func (in input) size() int {
	n := len(in.Method) + len(in.Path) + len(in.RawQuery) + len(in.Body) + len(in.Token)
	n += len(in.Select) + len(in.Order) + len(in.Filter) + len(in.Group)
	for name, values := range in.Headers {
		n += len(name)
		for _, value := range values {
			n += len(value)
		}
	}
	return n
}

func (in input) clone() input {
	out := in
	if in.Headers != nil {
		out.Headers = make(map[string][]string, len(in.Headers))
		for name, values := range in.Headers {
			out.Headers[name] = append([]string(nil), values...)
		}
	}
	return out
}

var (
	methods = []string{
		httpGET, httpHEAD, httpPOST, httpPATCH, httpPUT, httpDELETE, httpOPTIONS, "TRACE", "CONNECT",
	}
	paths = []string{
		"/items", "/orders", "/secrets", "/missing", "/", "/rpc/add_them",
		"/rpc/list_items", "/rpc/write_marker", "/rpc/nope", "/items/extra",
		"/rpc/", "/ITEMS", "/shop.items",
	}
	selects = []string{
		"", "*", "id", "id,name", "name", "orders(id)", "orders(id,name)",
		"count()", "id.sum()", "meta->>k", "id::int", "computed()",
		"...orders(id)", "orders(id,name).limit(1)", "id,orders(*)",
		`id,"a,b"`, strings.Repeat("id,", 40) + "name",
	}
	orders = []string{"", "id.asc", "id.desc", "name.asc.nullsfirst", "meta->>k.asc", "id.desc,name.asc"}
	ops    = []string{"eq", "neq", "gt", "gte", "lt", "lte", "like", "ilike", "in", "is", "isdistinct", "fts", "cs", "match"}
	filterValues = []string{
		"1", "alpha", "x' OR 1=1 --", "semi;colon", "*", "null",
		"true", `"a""b"`, "A-", strings.Repeat("z", 200),
	}
	bodies = []string{
		"", "{}", `{"name":"x"}`, `{"id":1,"name":"alpha"}`, `[{"name":"x"}]`,
		"[]", "null", "{", `"x"`, `{"id":1}`, `{"id":"1","name":"a"}`,
	}
	accepts = []string{
		"", "application/json", "text/csv", "application/vnd.pgrst.object+json",
		"text/html", "application/json;q=NaN", "*/*",
		"text/html;q=1, application/json;q=0.1", "application/json;q=Infinity",
	}
	prefers = []string{
		"", "count=exact", "count=planned", "all-rows", "return=representation",
		"handling=strict", "timezone=UTC", "row-security", "jwt-claims",
		"resolution=merge-duplicates", "resolution=ignore-duplicates",
		"resolution=nope", "tx=rollback", "max-affected=0", "missing=default",
	}
)

const (
	httpGET     = "GET"
	httpHEAD    = "HEAD"
	httpPOST    = "POST"
	httpPATCH   = "PATCH"
	httpPUT     = "PUT"
	httpDELETE  = "DELETE"
	httpOPTIONS = "OPTIONS"
)

func seedCorpus() []input {
	return []input{
		{Kind: kindHTTP, Method: httpGET, Path: "/items"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", RawQuery: "name=eq.alpha"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", RawQuery: "name=fts.alpha"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", RawQuery: "or=(id.eq.1))"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", Headers: map[string][]string{"Authorization": {"Bearer "}}},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", Headers: map[string][]string{"Authorization": {"Bearer"}}},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", Headers: map[string][]string{"Accept": {"text/html"}}},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", Headers: map[string][]string{"Accept-Profile": {"other"}}},
		{Kind: kindHTTP, Method: httpPATCH, Path: "/items", Body: `{"name":"x"}`, Headers: map[string][]string{"Content-Type": {"application/json"}}},
		{Kind: kindHTTP, Method: httpGET, Path: "/missing"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", RawQuery: "offset=18446744073709551615"},
		{Kind: kindHTTP, Method: httpGET, Path: "/items", RawQuery: "select=orders(id)"},
		{Kind: kindJWT, Token: ""},
		{Kind: kindJWT, Token: "a.b"},
		{Kind: kindParse, Select: "id", Filter: "1", Group: "id.eq.1)"},
	}
}

func generate(rng *rand.Rand) input {
	switch rng.IntN(10) {
	case 0, 1:
		return generateParse(rng)
	case 2:
		return generateJWT(rng)
	default:
		return generateHTTP(rng)
	}
}

func generateHTTP(rng *rand.Rand) input {
	in := input{
		Kind:    kindHTTP,
		Method:  pick(rng, methods),
		Path:    pick(rng, paths),
		Headers: map[string][]string{},
	}
	query := url.Values{}
	if rng.IntN(3) != 0 {
		query.Set("select", pick(rng, selects))
	}
	if rng.IntN(2) == 0 {
		query.Set("order", pick(rng, orders))
	}
	if rng.IntN(3) != 0 {
		op := pick(rng, ops)
		value := pick(rng, filterValues)
		if op == "in" {
			query.Set("id", "in.("+value+")")
		} else {
			query.Set(pick(rng, []string{"id", "name", "meta", "missing"}), op+"."+value)
		}
	}
	if rng.IntN(3) == 0 {
		query.Set("or", "("+pick(rng, []string{"id.eq.1", "id.eq.1)", "name.eq.alpha", "and(id.eq.1,name.eq.x)"})+")")
	}
	if rng.IntN(3) == 0 {
		query.Set("limit", pick(rng, []string{"0", "1", "10", "999999"}))
	}
	if rng.IntN(4) == 0 {
		query.Set("offset", pick(rng, []string{"0", "1", "5", "18446744073709551615"}))
	}
	in.RawQuery = query.Encode()
	if accept := pick(rng, accepts); accept != "" {
		in.Headers["Accept"] = []string{accept}
	}
	if prefer := pick(rng, prefers); prefer != "" {
		in.Headers["Prefer"] = []string{prefer}
	}
	switch rng.IntN(6) {
	case 0:
		in.Headers["Authorization"] = []string{"Bearer " + signedRoleToken("myrest_user", nil)}
	case 1:
		in.Headers["Authorization"] = []string{"Bearer " + pick(rng, []string{"", "x", "a.b", "a.b.c", "not-a-jwt"})}
	case 2:
		in.Headers["Authorization"] = []string{"Basic dXNlcjpwYXNz"}
	case 3:
		in.Headers["Authorization"] = []string{"Bearer " + signedRoleToken("myrest_user", map[string]any{"exp": 1})}
	}
	if rng.IntN(5) == 0 {
		in.Headers["Accept-Profile"] = []string{pick(rng, []string{"shop", "other", "mysql"})}
	}
	if rng.IntN(5) == 0 {
		in.Headers["Content-Profile"] = []string{pick(rng, []string{"shop", "other"})}
	}
	if rng.IntN(4) == 0 {
		in.Headers["Origin"] = []string{pick(rng, []string{"http://example.com", "null", "http://evil.test"})}
	}
	if in.Method == httpPOST || in.Method == httpPATCH || in.Method == httpPUT {
		in.Body = pick(rng, bodies)
		in.Headers["Content-Type"] = []string{"application/json"}
	}
	return in
}

func generateParse(rng *rand.Rand) input {
	return input{
		Kind:   kindParse,
		Select: pick(rng, selects),
		Order:  pick(rng, orders),
		Filter: pick(rng, filterValues),
		Group:  pick(rng, []string{"id.eq.1", "id.eq.1)", "and(id.eq.1,name.eq.x)", "not.or(id.eq.1)", ""}),
	}
}

func generateJWT(rng *rand.Rand) input {
	switch rng.IntN(6) {
	case 0:
		return input{Kind: kindJWT, Token: signedRoleToken("myrest_user", nil)}
	case 1:
		return input{Kind: kindJWT, Token: signedRoleToken("myrest_anon", map[string]any{"aud": "nope"})}
	case 2:
		return input{Kind: kindJWT, Token: signedRoleToken("myrest_user", map[string]any{"exp": 1})}
	case 3:
		return input{Kind: kindJWT, Token: ""}
	case 4:
		return input{Kind: kindJWT, Token: "a.b"}
	default:
		return input{Kind: kindJWT, Token: pick(rng, filterValues)}
	}
}

func mutate(rng *rand.Rand, in input) input {
	out := in.clone()
	switch out.Kind {
	case kindParse:
		switch rng.IntN(4) {
		case 0:
			out.Select = tweakString(rng, out.Select)
		case 1:
			out.Order = tweakString(rng, out.Order)
		case 2:
			out.Filter = tweakString(rng, out.Filter)
		default:
			out.Group = tweakString(rng, out.Group)
		}
	case kindJWT:
		out.Token = tweakString(rng, out.Token)
	default:
		mutateHTTP(rng, &out)
	}
	if rng.IntN(20) == 0 {
		out.Kind = pick(rng, []kind{kindHTTP, kindParse, kindJWT})
	}
	return out
}

func mutateHTTP(rng *rand.Rand, in *input) {
	switch rng.IntN(8) {
	case 0:
		in.Method = pick(rng, methods)
	case 1:
		in.Path = pick(rng, paths)
	case 2:
		in.RawQuery = tweakQuery(rng, in.RawQuery)
	case 3:
		if in.Headers == nil {
			in.Headers = map[string][]string{}
		}
		in.Headers["Prefer"] = []string{pick(rng, prefers)}
	case 4:
		if in.Headers == nil {
			in.Headers = map[string][]string{}
		}
		in.Headers["Accept"] = []string{pick(rng, accepts)}
	case 5:
		in.Body = tweakString(rng, in.Body)
	case 6:
		if in.Headers == nil {
			in.Headers = map[string][]string{}
		}
		in.Headers["Authorization"] = []string{"Bearer " + tweakString(rng, signedRoleToken("myrest_user", nil))}
	default:
		if rng.IntN(2) == 0 && len(in.Headers) > 0 {
			for name := range in.Headers {
				delete(in.Headers, name)
				break
			}
		} else {
			in.Path = tweakString(rng, in.Path)
		}
	}
}

func tweakQuery(rng *rand.Rand, raw string) string {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return pick(rng, []string{"select=id", "id=eq.1", "or=(id.eq.1))", "limit=1"})
	}
	switch rng.IntN(5) {
	case 0:
		query.Set("select", pick(rng, selects))
	case 1:
		query.Set("id", pick(rng, ops)+"."+pick(rng, filterValues))
	case 2:
		query.Set("or", "("+tweakString(rng, query.Get("or"))+")")
	case 3:
		query.Del("id")
	default:
		query.Set("offset", strconv.FormatUint(rng.Uint64(), 10))
	}
	return query.Encode()
}

func tweakString(rng *rand.Rand, value string) string {
	if value == "" {
		return pick(rng, filterValues)
	}
	switch rng.IntN(6) {
	case 0:
		return value + pick(rng, []string{")", "(", ",", ".", "*", "'", `"`})
	case 1:
		if len(value) > 1 {
			return value[:rng.IntN(len(value))]
		}
		return value
	case 2:
		return strings.ToUpper(value)
	case 3:
		return value + value
	case 4:
		runes := []rune(value)
		i := rng.IntN(len(runes))
		runes[i] = rune(32 + rng.IntN(95))
		return string(runes)
	default:
		return pick(rng, filterValues)
	}
}

func shrink(in input) []input {
	var out []input
	add := func(next input) {
		if next.size() < in.size() {
			out = append(out, next)
		}
	}
	next := in.clone()
	next.Body = ""
	add(next)
	next = in.clone()
	next.Headers = nil
	add(next)
	next = in.clone()
	next.RawQuery = ""
	add(next)
	next = in.clone()
	next.Token = shrinkText(in.Token)
	add(next)
	next = in.clone()
	next.Select = shrinkText(in.Select)
	add(next)
	next = in.clone()
	next.Filter = shrinkText(in.Filter)
	add(next)
	next = in.clone()
	next.Group = shrinkText(in.Group)
	add(next)
	if in.Kind == kindHTTP && in.Path != "/items" {
		next = in.clone()
		next.Path = "/items"
		add(next)
	}
	if strings.Contains(in.RawQuery, "or") {
		next = in.clone()
		values, err := url.ParseQuery(in.RawQuery)
		if err == nil {
			values.Del("or")
			next.RawQuery = values.Encode()
			add(next)
		}
	}
	return out
}

func shrinkText(value string) string {
	if len(value) <= 1 {
		return ""
	}
	return value[:len(value)/2]
}

func pick[T any](rng *rand.Rand, items []T) T {
	return items[rng.IntN(len(items))]
}
