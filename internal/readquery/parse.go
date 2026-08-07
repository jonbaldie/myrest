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
	query.ExactCount = preferHoldsCountExact(prefer)
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

func preferHoldsCountExact(prefer []string) bool {
	for _, header := range prefer {
		for _, part := range strings.Split(header, ",") {
			name, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if found && strings.EqualFold(name, "count") && strings.EqualFold(value, "exact") {
				return true
			}
		}
	}
	return false
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
		return Column{}, ParseFailure{Message: "PostgREST domain and cast features are not available with MySQL"}
	}
	if strings.HasSuffix(part, "()") {
		return Column{}, ParseFailure{Message: "PostgREST row computed-field features are not available with MySQL"}
	}
	if strings.Contains(part, "->") {
		return Column{}, ParseFailure{Message: "JSON path select is not available in this ordinary-read ticket"}
	}
	alias, name, found := strings.Cut(part, ":")
	if found {
		return Column{Name: name, Alias: alias}, nil
	}
	return Column{Name: part}, nil
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
	name, direction, found := strings.Cut(part, ".")
	order := Order{Column: name}
	if !found {
		return order, nil
	}
	switch strings.ToLower(direction) {
	case "asc":
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
