package mysqldb

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// sqlParts holds one parameterized SELECT or COUNT statement.
type sqlParts struct {
	statement string
	args      []any
	columns   []string // output column names in result order
}

func buildSelect(table schemacache.Table, query readquery.Query) (sqlParts, error) {
	columns, err := resolveColumns(table, query.Columns)
	if err != nil {
		return sqlParts{}, err
	}
	where, args, err := buildWhere(table, query)
	if err != nil {
		return sqlParts{}, err
	}
	order, err := buildOrder(table, query.Order)
	if err != nil {
		return sqlParts{}, err
	}
	return sqlParts{
		statement: selectSQL(table, columns, where, order, query),
		args:      args,
		columns:   outputNames(columns),
	}, nil
}

func selectSQL(
	table schemacache.Table,
	columns []resolvedColumn,
	where, order string,
	query readquery.Query,
) string {
	statement := fmt.Sprintf(
		"SELECT %s FROM %s.%s",
		selectList(columns),
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
	)
	if where != "" {
		statement += " WHERE " + where
	}
	if order != "" {
		statement += " ORDER BY " + order
	}
	return statement + pageSQL(query)
}

func selectList(columns []resolvedColumn) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = column.Expr
		if column.Alias != "" {
			quoted[i] += " AS " + quoteIdentifier(column.Alias)
		}
	}
	return strings.Join(quoted, ", ")
}

func outputNames(columns []resolvedColumn) []string {
	names := make([]string, len(columns))
	for i, column := range columns {
		if column.Alias != "" {
			names[i] = column.Alias
			continue
		}
		names[i] = column.Output
	}
	return names
}

func pageSQL(query readquery.Query) string {
	limit := query.EffectiveLimit()
	if limit == nil && query.Offset == 0 {
		return ""
	}
	// MySQL needs LIMIT with OFFSET. When the client gives only offset, use
	// the largest unsigned 64-bit limit so the page still starts at offset.
	if limit == nil {
		return " LIMIT 18446744073709551615 OFFSET " + strconv.FormatUint(query.Offset, 10)
	}
	suffix := " LIMIT " + strconv.FormatUint(*limit, 10)
	if query.Offset > 0 {
		suffix += " OFFSET " + strconv.FormatUint(query.Offset, 10)
	}
	return suffix
}

func buildCount(table schemacache.Table, query readquery.Query) (sqlParts, error) {
	where, args, err := buildWhere(table, query)
	if err != nil {
		return sqlParts{}, err
	}
	statement := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.%s",
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
	)
	if where != "" {
		statement += " WHERE " + where
	}
	return sqlParts{statement: statement, args: args}, nil
}

type resolvedColumn struct {
	Expr   string
	Output string
	Alias  string
}

func resolveColumns(table schemacache.Table, asked []readquery.Column) ([]resolvedColumn, error) {
	if len(asked) == 0 {
		out := make([]resolvedColumn, len(table.Columns))
		for i, column := range table.Columns {
			out[i] = resolvedColumn{
				Expr:   quoteIdentifier(column.Name),
				Output: column.Name,
			}
		}
		return out, nil
	}
	out := make([]resolvedColumn, 0, len(asked))
	for _, column := range asked {
		expr, err := columnExpr(table, column.Name, column.Path)
		if err != nil {
			return nil, err
		}
		output := column.Name
		if column.Path != nil {
			output = defaultJSONOutputName(column.Path)
		}
		out = append(out, resolvedColumn{Expr: expr, Output: output, Alias: column.Alias})
	}
	return out, nil
}

func defaultJSONOutputName(path *readquery.JSONPath) string {
	last := path.Steps[len(path.Steps)-1]
	if last.IsIndex {
		return strconv.Itoa(last.Index)
	}
	return last.Key
}

func columnExpr(table schemacache.Table, name string, path *readquery.JSONPath) (string, error) {
	column, ok := tableColumn(table, name)
	if !ok {
		return "", unknownColumn(name)
	}
	if path == nil {
		return quoteIdentifier(name), nil
	}
	if !isJSONDataType(column.DataType) {
		return "", readquery.UnsupportedFeature{
			Message: "JSON path on a non-JSON column is not available with MySQL",
		}
	}
	return jsonPathSQL(name, path), nil
}

func tableColumn(table schemacache.Table, name string) (schemacache.Column, bool) {
	for _, column := range table.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return schemacache.Column{}, false
}

func tableHasColumn(table schemacache.Table, name string) bool {
	_, ok := tableColumn(table, name)
	return ok
}

func isJSONDataType(dataType string) bool {
	return strings.EqualFold(dataType, "json")
}

func jsonPathSQL(column string, path *readquery.JSONPath) string {
	expr := quoteIdentifier(column)
	for i, step := range path.Steps {
		op := "->"
		if path.AsText && i == len(path.Steps)-1 {
			op = "->>"
		}
		expr += op + "'" + mysqlJSONPathLeg(step) + "'"
	}
	return expr
}

func mysqlJSONPathLeg(step readquery.PathStep) string {
	if step.IsIndex {
		return "$[" + strconv.Itoa(step.Index) + "]"
	}
	return "$." + step.Key
}

func unknownColumn(name string) error {
	return readquery.ColumnNotFound{Name: name}
}

func buildOrder(table schemacache.Table, orders []readquery.Order) (string, error) {
	if len(orders) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(orders))
	for _, order := range orders {
		expr, err := columnExpr(table, order.Column, order.Path)
		if err != nil {
			return "", err
		}
		part := expr + " ASC"
		if order.Desc {
			part = expr + " DESC"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", "), nil
}

func buildWhere(table schemacache.Table, query readquery.Query) (string, []any, error) {
	var parts []string
	var args []any
	for _, filter := range query.Filters {
		sql, filterArgs, err := filterSQL(table, filter)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, sql)
		args = append(args, filterArgs...)
	}
	for _, group := range query.Groups {
		sql, groupArgs, err := groupSQL(table, group)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, sql)
		args = append(args, groupArgs...)
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return strings.Join(parts, " AND "), args, nil
}

func groupSQL(table schemacache.Table, group readquery.Group) (string, []any, error) {
	var parts []string
	var args []any
	for _, filter := range group.Filters {
		sql, filterArgs, err := filterSQL(table, filter)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, sql)
		args = append(args, filterArgs...)
	}
	for _, nested := range group.Groups {
		sql, nestedArgs, err := groupSQL(table, nested)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, sql)
		args = append(args, nestedArgs...)
	}
	if len(parts) == 0 {
		return "(1=1)", nil, nil
	}
	joiner := " AND "
	if group.Or {
		joiner = " OR "
	}
	sql := "(" + strings.Join(parts, joiner) + ")"
	if group.Negated {
		sql = "NOT " + sql
	}
	return sql, args, nil
}
