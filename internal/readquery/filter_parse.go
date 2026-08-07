package readquery

import (
	"fmt"
	"strings"
)

func parseFilter(column, raw string) (Filter, error) {
	name, path, err := parseField(column)
	if err != nil {
		return Filter{}, err
	}
	negated := false
	body := raw
	if strings.HasPrefix(body, "not.") {
		negated = true
		body = strings.TrimPrefix(body, "not.")
	}
	opText, value, found := strings.Cut(body, ".")
	if !found {
		return Filter{}, ParseFailure{Message: fmt.Sprintf("filter for '%s' needs an operator", column)}
	}
	op, err := classifyOperator(opText)
	if err != nil {
		return Filter{}, err
	}
	filter := Filter{Column: name, Path: path, Op: op, Negated: negated}
	if op == OpIn {
		values, err := parseInList(value)
		if err != nil {
			return Filter{}, err
		}
		filter.Values = values
		return filter, nil
	}
	filter.Value = value
	return filter, nil
}

func classifyOperator(opText string) (Operator, error) {
	op := Operator(opText)
	if isListedOperator(op, FullMatchOperators) || isListedOperator(op, PartialMatchOperators) {
		return op, nil
	}
	if isPostgRESTFullTextSearchOperator(opText) {
		return "", ParseFailure{
			Message: "PostgREST full-text search operators are not available with MySQL",
			Gap:     true,
		}
	}
	if isPostgresArrayOrRangeOperator(opText) {
		return "", ParseFailure{
			Message: "PostgREST array and range operators are not available with MySQL",
			Gap:     true,
		}
	}
	if isPostgresRegexTextOperator(opText) {
		return "", ParseFailure{
			Message: "PostgREST match and imatch regex operators are not available with MySQL",
			Gap:     true,
		}
	}
	return "", ParseFailure{
		Message: fmt.Sprintf("filter operator '%s' is not a supported ordinary-read operator", opText),
	}
}

func isListedOperator(op Operator, listed []Operator) bool {
	for _, known := range listed {
		if op == known {
			return true
		}
	}
	return false
}

func isPostgRESTFullTextSearchOperator(operator string) bool {
	switch operator {
	case "fts", "plfts", "phfts", "wfts":
		return true
	default:
		return false
	}
}

func isPostgresArrayOrRangeOperator(operator string) bool {
	switch operator {
	case "cs", "cd", "ov", "sl", "sr", "nxr", "nxl", "adj":
		return true
	default:
		return false
	}
}

func isPostgresRegexTextOperator(operator string) bool {
	switch operator {
	case "match", "imatch":
		return true
	default:
		return false
	}
}

func parseInList(raw string) ([]string, error) {
	if !strings.HasPrefix(raw, "(") || !strings.HasSuffix(raw, ")") {
		return nil, ParseFailure{Message: "in filter needs a list in parentheses"}
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(raw, "("), ")")
	if inner == "" {
		return []string{}, nil
	}
	return splitCSV(inner), nil
}

func splitCSV(raw string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	for _, ch := range raw {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ',' && !inQuotes:
			parts = append(parts, unquote(current.String()))
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	parts = append(parts, unquote(current.String()))
	return parts
}

func unquote(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return strings.ReplaceAll(raw[1:len(raw)-1], `""`, `"`)
	}
	return raw
}

func parseGroup(raw string, or bool) (Group, error) {
	negated := false
	body := raw
	if strings.HasPrefix(body, "not.") {
		negated = true
		body = strings.TrimPrefix(body, "not.")
	}
	if !strings.HasPrefix(body, "(") || !strings.HasSuffix(body, ")") {
		return Group{}, ParseFailure{Message: "logical filter needs a list in parentheses"}
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(body, "("), ")")
	group := Group{Or: or, Negated: negated}
	for _, part := range splitTopLevel(inner) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if err := appendGroupPart(&group, part); err != nil {
			return Group{}, err
		}
	}
	return group, nil
}

func appendGroupPart(group *Group, part string) error {
	if isNestedGroup(part) {
		nested, err := parseNestedGroup(part)
		if err != nil {
			return err
		}
		group.Groups = append(group.Groups, nested)
		return nil
	}
	column, rest, found := strings.Cut(part, ".")
	if !found {
		return ParseFailure{Message: fmt.Sprintf("filter '%s' needs an operator", part)}
	}
	filter, err := parseFilter(column, rest)
	if err != nil {
		return err
	}
	group.Filters = append(group.Filters, filter)
	return nil
}

func isNestedGroup(part string) bool {
	return strings.HasPrefix(part, "or(") ||
		strings.HasPrefix(part, "and(") ||
		strings.HasPrefix(part, "not.or(") ||
		strings.HasPrefix(part, "not.and(")
}

func parseNestedGroup(part string) (Group, error) {
	negated := false
	body := part
	if strings.HasPrefix(body, "not.") {
		negated = true
		body = strings.TrimPrefix(body, "not.")
	}
	or := strings.HasPrefix(body, "or")
	body = strings.TrimPrefix(strings.TrimPrefix(body, "or"), "and")
	group, err := parseGroup(body, or)
	if err != nil {
		return Group{}, err
	}
	group.Negated = negated
	return group, nil
}

func splitTopLevel(raw string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inQuotes := false
	for _, ch := range raw {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			current.WriteRune(ch)
		case inQuotes:
			current.WriteRune(ch)
		case ch == '(':
			depth++
			current.WriteRune(ch)
		case ch == ')':
			depth--
			current.WriteRune(ch)
		case ch == ',' && depth == 0:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
