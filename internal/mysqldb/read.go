package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// readTable reads every column of every row of the table. The read is narrow
// on purpose: no filter, no order, and no page. Later tickets add those.
func readTable(ctx context.Context, conn *sql.Conn, table schemacache.Table) ([]rows.Row, error) {
	if len(table.Columns) == 0 {
		return nil, fmt.Errorf("the table %s.%s holds no column", table.Schema, table.Name)
	}

	names := columnNames(table)
	result, err := conn.QueryContext(ctx, selectStatement(table, names))
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	read := []rows.Row{}
	for result.Next() {
		values, err := scanValues(result, len(names))
		if err != nil {
			return nil, err
		}
		read = append(read, rows.Row{Columns: names, Values: values})
	}
	return read, result.Err()
}

// selectStatement builds the read of one table. The names come from the schema
// cache, never from the request, and MySQL quoting keeps them one identifier.
func selectStatement(table schemacache.Table, names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = quoteIdentifier(name)
	}
	return fmt.Sprintf(
		"SELECT %s FROM %s.%s",
		strings.Join(quoted, ", "),
		quoteIdentifier(table.Schema),
		quoteIdentifier(table.Name),
	)
}

func columnNames(table schemacache.Table) []string {
	names := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		names[i] = column.Name
	}
	return names
}

// scanValues reads one row as JSON-ready values.
func scanValues(result *sql.Rows, count int) ([]any, error) {
	scanned := make([]any, count)
	targets := make([]any, count)
	for i := range scanned {
		targets[i] = &scanned[i]
	}
	if err := result.Scan(targets...); err != nil {
		return nil, err
	}
	for i, value := range scanned {
		scanned[i] = jsonValue(value)
	}
	return scanned, nil
}

// jsonValue turns a MySQL text value into a JSON string. The driver gives text
// and blob columns as bytes, which JSON would otherwise write as base64.
func jsonValue(value any) any {
	if text, isBytes := value.([]byte); isBytes {
		return string(text)
	}
	return value
}

// quoteIdentifier writes a MySQL identifier in back quotes.
func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
