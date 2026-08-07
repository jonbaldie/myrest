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
		quoted[i] = quoteIdentifier(column.Name)
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
		names[i] = column.Name
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
	Name  string
	Alias string
}

func resolveColumns(table schemacache.Table, asked []readquery.Column) ([]resolvedColumn, error) {
	if len(asked) == 0 {
		out := make([]resolvedColumn, len(table.Columns))
		for i, column := range table.Columns {
			out[i] = resolvedColumn{Name: column.Name}
		}
		return out, nil
	}
	out := make([]resolvedColumn, 0, len(asked))
	for _, column := range asked {
		if !tableHasColumn(table, column.Name) {
			return nil, unknownColumn(column.Name)
		}
		out = append(out, resolvedColumn{Name: column.Name, Alias: column.Alias})
	}
	return out, nil
}

func tableHasColumn(table schemacache.Table, name string) bool {
	for _, column := range table.Columns {
		if column.Name == name {
			return true
		}
	}
	return false
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
		if !tableHasColumn(table, order.Column) {
			return "", unknownColumn(order.Column)
		}
		part := quoteIdentifier(order.Column) + " ASC"
		if order.Desc {
			part = quoteIdentifier(order.Column) + " DESC"
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
