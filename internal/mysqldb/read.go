package mysqldb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// readTable reads the resource under the ordinary-read query.
func readTable(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	if len(table.Columns) == 0 {
		return readquery.Result{}, fmt.Errorf("the table %s.%s holds no column", table.ID.Database, table.ID.Name)
	}

	total, err := exactTotal(ctx, conn, table, query)
	if err != nil {
		return readquery.Result{}, err
	}
	read, err := selectRows(ctx, conn, table, query)
	if err != nil {
		return readquery.Result{}, err
	}
	return readquery.Result{Rows: read, Total: total}, nil
}

func exactTotal(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	query readquery.Query,
) (*int64, error) {
	if !query.ExactCount {
		return nil, nil
	}
	parts, err := buildCount(table, query)
	if err != nil {
		return nil, err
	}
	var counted int64
	if err := conn.QueryRowContext(ctx, parts.statement, parts.args...).Scan(&counted); err != nil {
		return nil, err
	}
	return &counted, nil
}

func selectRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	query readquery.Query,
) ([]rows.Row, error) {
	parts, err := buildSelect(table, query)
	if err != nil {
		return nil, err
	}
	result, err := conn.QueryContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	read := []rows.Row{}
	for result.Next() {
		values, err := scanValues(result, len(parts.columns))
		if err != nil {
			return nil, err
		}
		read = append(read, rows.Row{Columns: parts.columns, Values: values})
	}
	return read, result.Err()
}

// selectStatement builds the unbounded read of one table. Kept for unit tests
// of identifier quoting.
func selectStatement(table schemacache.Table, names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = quoteIdentifier(name)
	}
	return fmt.Sprintf(
		"SELECT %s FROM %s.%s",
		strings.Join(quoted, ", "),
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
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
	types, err := result.ColumnTypes()
	if err != nil {
		return nil, err
	}
	for i, value := range scanned {
		var columnType *sql.ColumnType
		if i < len(types) {
			columnType = types[i]
		}
		scanned[i] = jsonValue(value, columnType)
	}
	return scanned, nil
}

// jsonValue turns a MySQL driver value into a JSON-ready value. Text and blob
// columns arrive as bytes (JSON would otherwise write base64). Decimal and
// floating types from aggregates also arrive as bytes and become JSON numbers.
func jsonValue(value any, columnType *sql.ColumnType) any {
	text, isBytes := value.([]byte)
	if !isBytes {
		return value
	}
	if columnType != nil && isNumericDBType(columnType.DatabaseTypeName()) {
		return numericJSON(text)
	}
	return string(text)
}

func isNumericDBType(name string) bool {
	switch {
	case strings.EqualFold(name, "DECIMAL"),
		strings.EqualFold(name, "NUMERIC"),
		strings.EqualFold(name, "NEWDECIMAL"),
		strings.EqualFold(name, "FLOAT"),
		strings.EqualFold(name, "DOUBLE"),
		strings.EqualFold(name, "REAL"):
		return true
	default:
		return false
	}
}

func numericJSON(text []byte) any {
	raw := string(text)
	if raw == "" {
		return nil
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return json.Number(raw)
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return json.Number(raw)
	}
	return raw
}

// quoteIdentifier writes a MySQL identifier in back quotes.
func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
