package mysqldb

import (
	"bytes"
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
		converted, err := jsonValue(value, columnTypeName(columnType))
		if err != nil {
			return nil, err
		}
		scanned[i] = converted
	}
	return scanned, nil
}

func columnTypeName(columnType *sql.ColumnType) string {
	if columnType == nil {
		return ""
	}
	return columnType.DatabaseTypeName()
}

// jsonValue turns a MySQL driver value into a JSON-ready value. Text and blob
// columns arrive as bytes (JSON would otherwise write base64). Decimal, float,
// and BIGINT types from parameterized reads also arrive as bytes and become
// JSON numbers. JSON columns and JSON extracts arrive as bytes and stay JSON
// values. Issue #287: UNSIGNED BIGINT bytes above max int64 become numbers.
func jsonValue(value any, typeName string) (any, error) {
	text, isBytes := value.([]byte)
	if !isBytes {
		return value, nil
	}
	if isNumericDBType(typeName) {
		return numericJSON(text), nil
	}
	if isJSONDataType(typeName) {
		return jsonColumnValue(text)
	}
	return string(text), nil
}

func jsonColumnValue(text []byte) (any, error) {
	if !json.Valid(text) {
		return nil, rows.InvalidJSON{}
	}
	return json.RawMessage(bytes.Clone(text)), nil
}

func isNumericDBType(name string) bool {
	switch numericBaseType(name) {
	case "DECIMAL", "NUMERIC", "NEWDECIMAL", "FLOAT", "DOUBLE", "REAL", "BIGINT", "LONGLONG":
		return true
	default:
		return false
	}
}

// numericBaseType returns the core MySQL type of a driver or catalog name.
// It drops display width, unsigned, and zerofill. Driver names put unsigned
// first (UNSIGNED BIGINT). Catalog DTD identifiers put it last.
func numericBaseType(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if at := strings.Index(upper, "("); at >= 0 {
		upper = strings.TrimSpace(upper[:at])
	}
	for _, field := range strings.Fields(upper) {
		if field == "UNSIGNED" || field == "ZEROFILL" {
			continue
		}
		return field
	}
	return ""
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
