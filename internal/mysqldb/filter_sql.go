package mysqldb

import (
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

func filterSQL(table schemacache.Table, filter readquery.Filter) (string, []any, error) {
	column, err := columnExpr(table, filter.Column, filter.Path)
	if err != nil {
		return "", nil, err
	}
	if filter.Op == readquery.OpILike {
		if err := requireCaseInsensitiveCollation(table, filter.Column); err != nil {
			return "", nil, err
		}
	}
	switch filter.Op {
	case readquery.OpEq, readquery.OpNeq, readquery.OpGt, readquery.OpGte, readquery.OpLt, readquery.OpLte:
		return comparisonSQL(column, filter)
	case readquery.OpLike, readquery.OpILike:
		return likeSQL(column, filter)
	case readquery.OpIn:
		return inSQL(column, filter)
	case readquery.OpIs:
		sql, err := isSQL(column, filter.Value, filter.Negated)
		return sql, nil, err
	case readquery.OpIsDistinct:
		return distinctSQL(column, filter)
	default:
		return "", nil, fmt.Errorf("filter operator %q is not supported", filter.Op)
	}
}

func requireCaseInsensitiveCollation(table schemacache.Table, name string) error {
	column, ok := tableColumn(table, name)
	if !ok {
		return unknownColumn(name)
	}
	if !isMySQLCaseInsensitiveCollation(column.Collation) {
		return readquery.UnsupportedFeature{
			Message: "ilike needs a MySQL Unicode case-insensitive (*_ci) column collation",
		}
	}
	return nil
}

func isMySQLCaseInsensitiveCollation(collation string) bool {
	lower := strings.ToLower(collation)
	return strings.HasSuffix(lower, "_ci")
}

func comparisonSQL(column string, filter readquery.Filter) (string, []any, error) {
	sql := column + " " + comparisonOp(filter.Op) + " ?"
	if filter.Negated {
		sql = "NOT (" + sql + ")"
	}
	return sql, []any{filter.Value}, nil
}

func likeSQL(column string, filter readquery.Filter) (string, []any, error) {
	pattern := strings.ReplaceAll(filter.Value, "*", "%")
	sql := column + " LIKE ?"
	if filter.Negated {
		sql = column + " NOT LIKE ?"
	}
	return sql, []any{pattern}, nil
}

func inSQL(column string, filter readquery.Filter) (string, []any, error) {
	if len(filter.Values) == 0 {
		if filter.Negated {
			return "1=1", nil, nil
		}
		return "0=1", nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(filter.Values)), ",")
	sql := column + " IN (" + placeholders + ")"
	if filter.Negated {
		sql = column + " NOT IN (" + placeholders + ")"
	}
	args := make([]any, len(filter.Values))
	for i, value := range filter.Values {
		args[i] = value
	}
	return sql, args, nil
}

func distinctSQL(column string, filter readquery.Filter) (string, []any, error) {
	sql := column + " IS DISTINCT FROM ?"
	if filter.Negated {
		sql = column + " IS NOT DISTINCT FROM ?"
	}
	return sql, []any{filter.Value}, nil
}

func comparisonOp(op readquery.Operator) string {
	switch op {
	case readquery.OpEq:
		return "="
	case readquery.OpNeq:
		return "<>"
	case readquery.OpGt:
		return ">"
	case readquery.OpGte:
		return ">="
	case readquery.OpLt:
		return "<"
	case readquery.OpLte:
		return "<="
	default:
		return "="
	}
}

func isSQL(column, value string, negated bool) (string, error) {
	kind := strings.ToLower(value)
	positive, ok := isPredicates[kind]
	if !ok {
		return "", fmt.Errorf("is filter value %q is not supported", value)
	}
	if negated {
		return column + " " + positive.negated, nil
	}
	return column + " " + positive.plain, nil
}

type isPredicate struct {
	plain   string
	negated string
}

var isPredicates = map[string]isPredicate{
	"null":     {plain: "IS NULL", negated: "IS NOT NULL"},
	"not_null": {plain: "IS NOT NULL", negated: "IS NULL"},
	"notnull":  {plain: "IS NOT NULL", negated: "IS NULL"},
	"true":     {plain: "IS TRUE", negated: "IS NOT TRUE"},
	"false":    {plain: "IS FALSE", negated: "IS NOT FALSE"},
	"unknown":  {plain: "IS UNKNOWN", negated: "IS NOT UNKNOWN"},
}
