package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Insert writes one or more rows as the database role. Column names come from
// the JSON objects; generated columns are refused.
func (p *Pool) Insert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	rows []map[string]any,
) (int, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return 0, err
	}

	var inserted int
	err = p.onRequest(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var insertErr error
		inserted, insertErr = insertRows(ctx, conn, table, rows)
		return insertErr
	})
	return inserted, err
}

// Update changes matching rows as the database role.
func (p *Pool) Update(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
) (int64, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return 0, err
	}

	var updated int64
	err = p.onRequest(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var updateErr error
		updated, updateErr = updateRows(ctx, conn, table, patch, query)
		return updateErr
	})
	return updated, err
}

// Delete removes matching rows as the database role.
func (p *Pool) Delete(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (int64, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return 0, err
	}

	var deleted int64
	err = p.onRequest(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var deleteErr error
		deleted, deleteErr = deleteRows(ctx, conn, table, query)
		return deleteErr
	})
	return deleted, err
}

// Upsert writes one row by primary key as the database role.
// merge-duplicates uses INSERT ... AS new ON DUPLICATE KEY UPDATE.
// ignore-duplicates uses INSERT IGNORE.
// inserted is true when MySQL created the row.
func (p *Pool) Upsert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	row map[string]any,
	primaryKey []string,
	resolution httpapi.UpsertResolution,
) (bool, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return false, err
	}

	var inserted bool
	err = p.onRequest(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var upsertErr error
		inserted, upsertErr = upsertRow(ctx, conn, table, row, primaryKey, resolution)
		return upsertErr
	})
	return inserted, err
}

func insertRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	rows []map[string]any,
) (int, error) {
	if len(rows) == 0 {
		return 0, fmt.Errorf("insert needs at least one row")
	}
	columns, err := insertColumns(table, rows)
	if err != nil {
		return 0, err
	}
	parts, err := buildInsert(table, columns, rows)
	if err != nil {
		return 0, err
	}
	result, err := conn.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func updateRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
) (int64, error) {
	parts, err := buildUpdate(table, patch, query)
	if err != nil {
		return 0, err
	}
	result, err := conn.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func deleteRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	query readquery.Query,
) (int64, error) {
	parts, err := buildDelete(table, query)
	if err != nil {
		return 0, err
	}
	result, err := conn.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func insertColumns(table schemacache.Table, rows []map[string]any) ([]string, error) {
	seen := make(map[string]bool)
	var columns []string
	for _, row := range rows {
		for name := range row {
			if seen[name] {
				continue
			}
			column, ok := tableColumn(table, name)
			if !ok {
				return nil, unknownColumn(name)
			}
			if column.Generated {
				return nil, readquery.UnsupportedFeature{
					Message: "Cannot insert into generated column " + name,
				}
			}
			seen[name] = true
			columns = append(columns, name)
		}
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("insert needs at least one column")
	}
	sort.Strings(columns)
	return columns, nil
}

func buildInsert(table schemacache.Table, columns []string, rows []map[string]any) (sqlParts, error) {
	quoted := make([]string, len(columns))
	for i, name := range columns {
		quoted[i] = quoteIdentifier(name)
	}
	placeholders := "(" + strings.TrimRight(strings.Repeat("?,", len(columns)), ",") + ")"
	values := make([]string, len(rows))
	args := make([]any, 0, len(rows)*len(columns))
	for i, row := range rows {
		values[i] = placeholders
		for _, name := range columns {
			args = append(args, row[name])
		}
	}
	return sqlParts{
		statement: fmt.Sprintf(
			"INSERT INTO %s.%s (%s) VALUES %s",
			quoteIdentifier(table.ID.Database),
			quoteIdentifier(table.ID.Name),
			strings.Join(quoted, ", "),
			strings.Join(values, ", "),
		),
		args: args,
	}, nil
}

func buildUpdate(table schemacache.Table, patch map[string]any, query readquery.Query) (sqlParts, error) {
	if len(patch) == 0 {
		return sqlParts{}, fmt.Errorf("update needs at least one column")
	}
	names := make([]string, 0, len(patch))
	for name := range patch {
		names = append(names, name)
	}
	sort.Strings(names)

	sets := make([]string, 0, len(names))
	args := make([]any, 0, len(names))
	for _, name := range names {
		column, ok := tableColumn(table, name)
		if !ok {
			return sqlParts{}, unknownColumn(name)
		}
		if column.Generated {
			return sqlParts{}, readquery.UnsupportedFeature{
				Message: "Cannot update generated column " + name,
			}
		}
		sets = append(sets, quoteIdentifier(name)+" = ?")
		args = append(args, patch[name])
	}
	where, whereArgs, err := buildWhere(table, query)
	if err != nil {
		return sqlParts{}, err
	}
	statement := fmt.Sprintf(
		"UPDATE %s.%s SET %s",
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
		strings.Join(sets, ", "),
	)
	if where != "" {
		statement += " WHERE " + where
		args = append(args, whereArgs...)
	}
	return sqlParts{statement: statement, args: args}, nil
}

func buildDelete(table schemacache.Table, query readquery.Query) (sqlParts, error) {
	where, args, err := buildWhere(table, query)
	if err != nil {
		return sqlParts{}, err
	}
	statement := fmt.Sprintf(
		"DELETE FROM %s.%s",
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
	)
	if where != "" {
		statement += " WHERE " + where
	}
	return sqlParts{statement: statement, args: args}, nil
}

func upsertRow(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	row map[string]any,
	primaryKey []string,
	resolution httpapi.UpsertResolution,
) (bool, error) {
	parts, err := buildUpsert(table, row, primaryKey, resolution)
	if err != nil {
		return false, err
	}
	result, err := conn.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	// INSERT and INSERT IGNORE report 1 for a new row. ON DUPLICATE KEY UPDATE
	// reports 1 for insert, 2 for update, and 0 when values did not change.
	return affected == 1, nil
}

func buildUpsert(
	table schemacache.Table,
	row map[string]any,
	primaryKey []string,
	resolution httpapi.UpsertResolution,
) (sqlParts, error) {
	if len(row) == 0 {
		return sqlParts{}, fmt.Errorf("upsert needs at least one column")
	}
	columns, err := insertColumns(table, []map[string]any{row})
	if err != nil {
		return sqlParts{}, err
	}
	quoted := make([]string, len(columns))
	args := make([]any, 0, len(columns))
	for i, name := range columns {
		quoted[i] = quoteIdentifier(name)
		args = append(args, row[name])
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(columns)), ",")
	target := fmt.Sprintf(
		"%s.%s (%s)",
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
		strings.Join(quoted, ", "),
	)

	switch resolution {
	case httpapi.UpsertIgnoreDuplicates:
		return sqlParts{
			statement: fmt.Sprintf(
				"INSERT IGNORE INTO %s VALUES (%s)",
				target,
				placeholders,
			),
			args: args,
		}, nil
	case httpapi.UpsertMergeDuplicates:
		updates := upsertUpdateSets(columns, primaryKey)
		return sqlParts{
			statement: fmt.Sprintf(
				"INSERT INTO %s VALUES (%s) AS %s ON DUPLICATE KEY UPDATE %s",
				target,
				placeholders,
				quoteIdentifier("new"),
				updates,
			),
			args: args,
		}, nil
	default:
		return sqlParts{}, fmt.Errorf("unknown upsert resolution %v", resolution)
	}
}

// upsertUpdateSets builds the ON DUPLICATE KEY UPDATE assignments. Non-primary
// key columns take the inserted values. When the body holds only primary key
// columns, the first primary key column is assigned to itself so MySQL accepts
// the statement.
func upsertUpdateSets(columns, primaryKey []string) string {
	pk := make(map[string]bool, len(primaryKey))
	for _, name := range primaryKey {
		pk[name] = true
	}
	var sets []string
	for _, name := range columns {
		if pk[name] {
			continue
		}
		sets = append(sets, fmt.Sprintf(
			"%s = %s.%s",
			quoteIdentifier(name),
			quoteIdentifier("new"),
			quoteIdentifier(name),
		))
	}
	if len(sets) == 0 {
		name := primaryKey[0]
		if len(columns) > 0 {
			name = columns[0]
		}
		sets = append(sets, fmt.Sprintf(
			"%s = %s.%s",
			quoteIdentifier(name),
			quoteIdentifier("new"),
			quoteIdentifier(name),
		))
	}
	return strings.Join(sets, ", ")
}
