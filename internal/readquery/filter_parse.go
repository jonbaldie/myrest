package readquery

import (
	"fmt"
	"slices"
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
	if err := parseFilterValue(&filter, value); err != nil {
		return Filter{}, err
	}
	return filter, nil
}

// parseFilterValue fills the filter's value or values. The in operator
// parses a list; is keeps its documented value set as written; every other
// operator decodes a double-quoted value to its literal. Issue #145.
func parseFilterValue(filter *Filter, value string) error {
	switch filter.Op {
	case OpIn:
		values, err := parseInList(value)
		if err != nil {
			return err
		}
		filter.Values = values
	case OpIs:
		if !isIsValue(value) {
			return ParseFailure{
				Message: "is operator value must be null, not_null, true, false, or unknown",
			}
		}
		filter.Value = value
	default:
		literal, err := dequoteScalarValue(value)
		if err != nil {
			return err
		}
		filter.Value = literal
	}
	return nil
}

// dequoteScalarValue decodes a double-quoted scalar filter value to its
// literal, the same way the in-list parser decodes a quoted element: a
// doubled quote inside the value is an escaped quote. An unquoted value
// passes through as written. A quoted value must close and hold nothing
// else, so a client never compares against quote characters by accident.
// Issue #145.
func dequoteScalarValue(value string) (string, error) {
	if !strings.HasPrefix(value, `"`) {
		return value, nil
	}
	var literal strings.Builder
	body := value[1:]
	for {
		end := strings.IndexByte(body, '"')
		if end < 0 {
			return "", ParseFailure{Message: "quoted filter value needs a closing double quote"}
		}
		if end == len(body)-1 {
			literal.WriteString(body[:end])
			return literal.String(), nil
		}
		if body[end+1] == '"' {
			literal.WriteString(body[:end] + `"`)
			body = body[end+2:]
			continue
		}
		return "", ParseFailure{Message: "quoted filter value has text after the closing double quote"}
	}
}

// isValues lists the values an is filter takes, as ordinary-read.md claims them.
var isValues = []string{"null", "not_null", "true", "false", "unknown"}

// isIsValue matches the value as written: is values, like operators, are
// case-sensitive.
func isIsValue(value string) bool {
	return slices.Contains(isValues, value)
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
	return splitCSV(inner)
}

func splitCSV(raw string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inQuotes := false
	depth := 0
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			current.WriteByte(ch)
		case ch == ',' && !inQuotes:
			parts = append(parts, unquote(current.String()))
			current.Reset()
		case ch == '(' && !inQuotes:
			depth++
			current.WriteByte(ch)
		case ch == ')' && !inQuotes:
			if depth == 0 {
				return nil, ParseFailure{Message: "in filter has unbalanced parentheses"}
			}
			depth--
			current.WriteByte(ch)
		default:
			current.WriteByte(ch)
		}
	}
	if depth != 0 || inQuotes {
		return nil, ParseFailure{Message: "in filter has unbalanced parentheses or quotes"}
	}
	parts = append(parts, unquote(current.String()))
	return parts, nil
}

func unquote(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return strings.ReplaceAll(raw[1:len(raw)-1], `""`, `"`)
	}
	return raw
}

func stripGroupNegation(raw string) (string, bool) {
	if strings.HasPrefix(raw, "not.") {
		return strings.TrimPrefix(raw, "not."), true
	}
	return raw, false
}

func requireNonEmptyGroup(group Group) error {
	if len(group.Filters) == 0 && len(group.Groups) == 0 {
		return ParseFailure{Message: "logical filter needs at least one condition"}
	}
	return nil
}

func parseGroup(raw string, or bool) (Group, error) {
	body, negated := stripGroupNegation(raw)
	if !strings.HasPrefix(body, "(") || !strings.HasSuffix(body, ")") {
		return Group{}, ParseFailure{Message: "logical filter needs a list in parentheses"}
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(body, "("), ")")
	group := Group{Or: or, Negated: negated}
	parts, err := splitTopLevel(inner)
	if err != nil {
		return Group{}, err
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if err := appendGroupPart(&group, part); err != nil {
			return Group{}, err
		}
	}
	if err := requireNonEmptyGroup(group); err != nil {
		return Group{}, err
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

func splitTopLevel(raw string) ([]string, error) {
	var parts []string
	var current strings.Builder
	depth := 0
	inQuotes := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			current.WriteByte(ch)
		case inQuotes:
			current.WriteByte(ch)
		case ch == '(':
			depth++
			current.WriteByte(ch)
		case ch == ')':
			if depth == 0 {
				return nil, ParseFailure{Message: "logical filter has unbalanced parentheses"}
			}
			depth--
			current.WriteByte(ch)
		case ch == ',' && depth == 0:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if depth != 0 || inQuotes {
		return nil, ParseFailure{Message: "logical filter has unbalanced parentheses or quotes"}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts, nil
}
