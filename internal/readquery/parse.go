package readquery

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// reservedQueryKeys are not column filters.
var reservedQueryKeys = map[string]bool{
	"select":  true,
	"order":   true,
	"limit":   true,
	"offset":  true,
	"and":     true,
	"or":      true,
	"columns": true,
}

// ParseFailure is a client query that myrest cannot run.
type ParseFailure struct {
	Message string
	// Gap is true when the failure is a documented MySQL semantic gap
	// (MYREST001), not a plain query parse failure (PGRST100).
	Gap bool
}

func (e ParseFailure) Error() string { return e.Message }

// Parse reads a PostgREST-shaped ordinary-read query from the URL values and
// Prefer header tokens.
func Parse(values url.Values, prefer []string) (Query, error) {
	var query Query
	if err := parseSelect(values.Get("select"), &query); err != nil {
		return Query{}, err
	}
	if err := parseOrder(values.Get("order"), &query); err != nil {
		return Query{}, err
	}
	if err := parseLimitOffset(values, &query); err != nil {
		return Query{}, err
	}
	if err := parsePreferCount(prefer, &query); err != nil {
		return Query{}, err
	}
	if err := parseColumnFilters(values, &query); err != nil {
		return Query{}, err
	}
	if err := parseLogicalGroups(values, &query); err != nil {
		return Query{}, err
	}
	return query, nil
}

func parseColumnFilters(values url.Values, query *Query) error {
	for _, key := range sortedKeys(values) {
		if reservedQueryKeys[key] {
			continue
		}
		for _, raw := range values[key] {
			filter, err := parseFilter(key, raw)
			if err != nil {
				return err
			}
			query.Filters = append(query.Filters, filter)
		}
	}
	return nil
}

func parseLogicalGroups(values url.Values, query *Query) error {
	for _, raw := range values["or"] {
		group, err := parseGroup(raw, true)
		if err != nil {
			return err
		}
		query.Groups = append(query.Groups, group)
	}
	for _, raw := range values["and"] {
		group, err := parseGroup(raw, false)
		if err != nil {
			return err
		}
		query.Groups = append(query.Groups, group)
	}
	return nil
}

func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parsePreferCount(prefer []string, query *Query) error {
	for _, header := range prefer {
		for _, part := range strings.Split(header, ",") {
			name, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if !found || !strings.EqualFold(name, "count") {
				continue
			}
			switch strings.ToLower(value) {
			case "exact":
				query.ExactCount = true
			case "planned", "estimated":
				return ParseFailure{
					Message: "Prefer count=planned and count=estimated are not available with MySQL",
					Gap:     true,
				}
			}
		}
	}
	return nil
}

func parseSelect(raw string, query *Query) error {
	if raw == "" || raw == "*" {
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "*" {
			continue
		}
		column, err := parseSelectPart(part)
		if err != nil {
			return err
		}
		query.Columns = append(query.Columns, column)
	}
	return nil
}

func parseSelectPart(part string) (Column, error) {
	if strings.Contains(part, "::") {
		return Column{}, ParseFailure{
			Message: "PostgREST domain and cast features are not available with MySQL",
			Gap:     true,
		}
	}
	if strings.HasSuffix(part, "()") {
		return Column{}, ParseFailure{
			Message: "PostgREST row computed-field features are not available with MySQL",
			Gap:     true,
		}
	}
	alias := ""
	field := part
	if name, rest, found := strings.Cut(part, ":"); found {
		alias = name
		field = rest
	}
	columnName, path, err := parseField(field)
	if err != nil {
		return Column{}, err
	}
	return Column{Name: columnName, Alias: alias, Path: path}, nil
}

func parseOrder(raw string, query *Query) error {
	if raw == "" {
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		order, err := parseOrderPart(part)
		if err != nil {
			return err
		}
		query.Order = append(query.Order, order)
	}
	return nil
}

func parseOrderPart(part string) (Order, error) {
	field, direction, err := splitOrderDirection(part)
	if err != nil {
		return Order{}, err
	}
	name, path, err := parseField(field)
	if err != nil {
		return Order{}, err
	}
	order := Order{Column: name, Path: path}
	switch direction {
	case "", "asc":
		return order, nil
	case "desc":
		order.Desc = true
		return order, nil
	case "asc.nullsfirst", "asc.nullslast", "desc.nullsfirst", "desc.nullslast":
		return Order{}, ParseFailure{Message: "nulls order options are not available in this ordinary-read ticket"}
	default:
		return Order{}, ParseFailure{Message: fmt.Sprintf("unknown order direction '%s'", direction)}
	}
}

func splitOrderDirection(part string) (field, direction string, err error) {
	lower := strings.ToLower(part)
	for _, suffix := range []string{
		".asc.nullsfirst", ".asc.nullslast", ".desc.nullsfirst", ".desc.nullslast",
		".asc", ".desc",
	} {
		if strings.HasSuffix(lower, suffix) {
			return part[:len(part)-len(suffix)], strings.TrimPrefix(suffix, "."), nil
		}
	}
	if strings.Contains(part, "->") {
		return part, "", nil
	}
	name, direction, found := strings.Cut(part, ".")
	if !found {
		return part, "", nil
	}
	return name, direction, nil
}

func parseLimitOffset(values url.Values, query *Query) error {
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return ParseFailure{Message: "limit must be a non-negative integer"}
		}
		query.Limit = &limit
	}
	if raw := values.Get("offset"); raw != "" {
		offset, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return ParseFailure{Message: "offset must be a non-negative integer"}
		}
		query.Offset = offset
	}
	return nil
}
