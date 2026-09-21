package readexec

import (
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// withJoinColumns adds the parent columns an embed plan needs, and returns the
// names that were injected so the response can drop them again.
func withJoinColumns(table schemacache.Table, query readquery.Query, plan []plannedEmbed) (readquery.Query, []string) {
	if len(plan) == 0 || (len(query.Columns) == 0 && query.SelectAll) {
		return query, nil
	}
	needed := originColumnsNeeded(plan)
	have := selectedNames(query.Columns)
	var injected []string
	for _, column := range table.Columns {
		if !needed[column.Name] || have[column.Name] {
			continue
		}
		query.Columns = append(query.Columns, readquery.Column{Name: column.Name})
		injected = append(injected, column.Name)
		have[column.Name] = true
	}
	return query, injected
}

func originColumnsNeeded(plan []plannedEmbed) map[string]bool {
	needed := map[string]bool{}
	for _, embed := range plan {
		for _, column := range embed.relationship.OriginColumns {
			needed[column] = true
		}
	}
	return needed
}

func selectedNames(columns []readquery.Column) map[string]bool {
	have := map[string]bool{}
	for _, column := range columns {
		if column.Agg != "" || column.Name == "" {
			continue
		}
		have[column.Name] = true
	}
	return have
}

// ensureColumns adds named columns of table to the select list when the
// client did not ask for every column.
func ensureColumns(table schemacache.Table, query readquery.Query, names []string) readquery.Query {
	if len(query.Columns) == 0 && query.SelectAll {
		return query
	}
	have := selectedNames(query.Columns)
	for _, name := range names {
		if have[name] || !hasColumn(table, name) {
			continue
		}
		query.Columns = append(query.Columns, readquery.Column{Name: name})
		have[name] = true
	}
	return query
}

func hasColumn(table schemacache.Table, name string) bool {
	for _, column := range table.Columns {
		if column.Name == name {
			return true
		}
	}
	return false
}

func dropColumns(read []rows.Row, names []string) []rows.Row {
	if len(names) == 0 {
		return read
	}
	drop := map[string]bool{}
	for _, name := range names {
		drop[name] = true
	}
	cleaned := make([]rows.Row, len(read))
	for i, row := range read {
		cleaned[i] = keepColumns(row, func(column string) bool { return !drop[column] })
	}
	return cleaned
}

func exceptNames(names, keep []string) []string {
	blocked := map[string]bool{}
	for _, name := range keep {
		blocked[name] = true
	}
	var out []string
	for _, name := range names {
		if !blocked[name] {
			out = append(out, name)
		}
	}
	return out
}

// projectEmbedRow keeps the columns the client asked for, plus nested embeds.
func projectEmbedRow(row rows.Row, ask readquery.Embed) rows.Row {
	if len(ask.Columns) == 0 {
		return row
	}
	keep := map[string]bool{}
	for _, column := range ask.Columns {
		keep[column.ResultName()] = true
	}
	for _, nested := range ask.Embeds {
		keep[nested.Key()] = true
	}
	return keepColumns(row, func(column string) bool { return keep[column] })
}

// keepColumns copies the columns of row that keep accepts, in row order.
func keepColumns(row rows.Row, keep func(string) bool) rows.Row {
	var columns []string
	var values []any
	for i, column := range row.Columns {
		if !keep(column) {
			continue
		}
		columns = append(columns, column)
		if i < len(row.Values) {
			values = append(values, row.Values[i])
		} else {
			values = append(values, nil)
		}
	}
	return rows.Row{Columns: columns, Values: values}
}

func columnList(names []string) []readquery.Column {
	columns := make([]readquery.Column, 0, len(names))
	for _, name := range names {
		columns = append(columns, readquery.Column{Name: name})
	}
	return columns
}

func appendColumn(row rows.Row, name string, value any) rows.Row {
	columns := append(append([]string{}, row.Columns...), name)
	values := append(append([]any{}, row.Values...), value)
	return rows.Row{Columns: columns, Values: values}
}
