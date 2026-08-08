package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

// Insert writes one or more rows as the database role. Column names come from
// the JSON objects; generated columns are refused.
func (p *Pool) Insert(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	bodyRows []map[string]any,
	options writequery.Options,
) (writequery.Result, error) {
	return p.withWriteConn(ctx, role, func(ctx context.Context, conn *sql.Conn) (writequery.Result, error) {
		return insertRows(ctx, conn, table, bodyRows, options)
	})
}

// Update changes matching rows as the database role.
func (p *Pool) Update(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	return p.withWriteConn(ctx, role, func(ctx context.Context, conn *sql.Conn) (writequery.Result, error) {
		return updateRows(ctx, conn, table, patch, query, options)
	})
}

// Delete removes matching rows as the database role.
func (p *Pool) Delete(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	return p.withWriteConn(ctx, role, func(ctx context.Context, conn *sql.Conn) (writequery.Result, error) {
		return deleteRows(ctx, conn, table, query, options)
	})
}

func (p *Pool) withWriteConn(
	ctx context.Context,
	role schemacache.Role,
	work func(context.Context, *sql.Conn) (writequery.Result, error),
) (writequery.Result, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return writequery.Result{}, err
	}
	var result writequery.Result
	err = p.onRequest(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var workErr error
		result, workErr = work(ctx, conn)
		return workErr
	})
	return result, err
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
	bodyRows []map[string]any,
	options writequery.Options,
) (writequery.Result, error) {
	if len(bodyRows) == 0 {
		return writequery.Result{}, fmt.Errorf("insert needs at least one row")
	}
	columns, err := insertColumns(table, bodyRows)
	if err != nil {
		return writequery.Result{}, err
	}
	parts, err := buildInsert(table, columns, bodyRows, options.MissingDefault)
	if err != nil {
		return writequery.Result{}, err
	}
	if !options.ReturnRepresentation && !options.ReturnKeys {
		return execWrite(ctx, conn, parts)
	}
	return withTx(ctx, conn, func(ctx context.Context, tx *sql.Tx) (writequery.Result, error) {
		return finishInsert(ctx, tx, table, bodyRows, options, parts)
	})
}

func finishInsert(
	ctx context.Context,
	tx *sql.Tx,
	table schemacache.Table,
	bodyRows []map[string]any,
	options writequery.Options,
	parts sqlParts,
) (writequery.Result, error) {
	execResult, err := tx.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return writequery.Result{}, err
	}
	affected, err := execResult.RowsAffected()
	if err != nil {
		return writequery.Result{}, err
	}
	keys, err := insertedKeys(table, bodyRows, options.PrimaryKey, execResult)
	if err != nil {
		return writequery.Result{}, err
	}
	result := writequery.Result{Affected: affected, Keys: keys}
	if !options.ReturnRepresentation {
		return result, nil
	}
	result.Rows, err = selectByKeys(ctx, tx, table, options.PrimaryKey, keys)
	return result, err
}

func updateRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	patch map[string]any,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	parts, err := buildUpdate(table, patch, query)
	if err != nil {
		return writequery.Result{}, err
	}
	if !options.ReturnRepresentation && options.MaxAffected == nil {
		return execWrite(ctx, conn, parts)
	}
	return withTx(ctx, conn, func(ctx context.Context, tx *sql.Tx) (writequery.Result, error) {
		var keys []map[string]any
		if options.ReturnRepresentation {
			keys, err = selectKeyMaps(ctx, tx, table, options.PrimaryKey, query)
			if err != nil {
				return writequery.Result{}, err
			}
		}
		result, err := execTxWrite(ctx, tx, parts, options.MaxAffected)
		if err != nil {
			return writequery.Result{}, err
		}
		result.Keys = keys
		if options.ReturnRepresentation {
			result.Rows, err = selectByKeys(ctx, tx, table, options.PrimaryKey, keys)
			if err != nil {
				return writequery.Result{}, err
			}
		}
		return result, nil
	})
}

func deleteRows(
	ctx context.Context,
	conn *sql.Conn,
	table schemacache.Table,
	query readquery.Query,
	options writequery.Options,
) (writequery.Result, error) {
	parts, err := buildDelete(table, query)
	if err != nil {
		return writequery.Result{}, err
	}
	if !options.ReturnRepresentation && options.MaxAffected == nil {
		return execWrite(ctx, conn, parts)
	}
	return withTx(ctx, conn, func(ctx context.Context, tx *sql.Tx) (writequery.Result, error) {
		var readRows []rows.Row
		if options.ReturnRepresentation {
			// Re-read every column. The client select list is applied after the
			// write for representation embeds, and join columns must remain.
			readQuery := readquery.Query{
				SelectAll: true,
				Filters:   query.Filters,
				Groups:    query.Groups,
			}
			readRows, err = selectMatching(ctx, tx, table, readQuery)
			if err != nil {
				return writequery.Result{}, err
			}
		}
		result, err := execTxWrite(ctx, tx, parts, options.MaxAffected)
		if err != nil {
			return writequery.Result{}, err
		}
		result.Rows = readRows
		return result, nil
	})
}

func execWrite(ctx context.Context, conn *sql.Conn, parts sqlParts) (writequery.Result, error) {
	execResult, err := conn.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return writequery.Result{}, err
	}
	affected, err := execResult.RowsAffected()
	if err != nil {
		return writequery.Result{}, err
	}
	return writequery.Result{Affected: affected}, nil
}

func execTxWrite(
	ctx context.Context,
	tx *sql.Tx,
	parts sqlParts,
	maxAffected *int64,
) (writequery.Result, error) {
	execResult, err := tx.ExecContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return writequery.Result{}, err
	}
	affected, err := execResult.RowsAffected()
	if err != nil {
		return writequery.Result{}, err
	}
	if err := checkMaxAffected(affected, maxAffected); err != nil {
		return writequery.Result{}, err
	}
	return writequery.Result{Affected: affected}, nil
}

func withTx(
	ctx context.Context,
	conn *sql.Conn,
	work func(context.Context, *sql.Tx) (writequery.Result, error),
) (writequery.Result, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return writequery.Result{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := work(ctx, tx)
	if err != nil {
		return writequery.Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return writequery.Result{}, err
	}
	return result, nil
}

func checkMaxAffected(affected int64, max *int64) error {
	if max == nil || affected <= *max {
		return nil
	}
	return writequery.MaxAffectedExceeded{Affected: affected, Max: *max}
}

func insertColumns(table schemacache.Table, bodyRows []map[string]any) ([]string, error) {
	seen := make(map[string]bool)
	var columns []string
	for _, row := range bodyRows {
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

func buildInsert(
	table schemacache.Table,
	columns []string,
	bodyRows []map[string]any,
	missingDefault bool,
) (sqlParts, error) {
	quoted := make([]string, len(columns))
	for i, name := range columns {
		quoted[i] = quoteIdentifier(name)
	}
	values := make([]string, len(bodyRows))
	args := make([]any, 0, len(bodyRows)*len(columns))
	for i, row := range bodyRows {
		placeholders := make([]string, len(columns))
		for j, name := range columns {
			if _, held := row[name]; !held && missingDefault {
				placeholders[j] = "DEFAULT"
				continue
			}
			placeholders[j] = "?"
			args = append(args, row[name])
		}
		values[i] = "(" + strings.Join(placeholders, ", ") + ")"
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

func insertedKeys(
	table schemacache.Table,
	bodyRows []map[string]any,
	primaryKey []string,
	execResult sql.Result,
) ([]map[string]any, error) {
	if len(primaryKey) == 0 {
		return nil, nil
	}
	if keysFromPayload, ok := keysFromBody(bodyRows, primaryKey); ok {
		return keysFromPayload, nil
	}
	if len(primaryKey) == 1 {
		column, ok := tableColumn(table, primaryKey[0])
		if ok && column.AutoIncrement {
			first, err := execResult.LastInsertId()
			if err != nil {
				return nil, err
			}
			keys := make([]map[string]any, len(bodyRows))
			for i := range bodyRows {
				keys[i] = map[string]any{primaryKey[0]: first + int64(i)}
			}
			return keys, nil
		}
	}
	return nil, readquery.UnsupportedFeature{
		Message: "Prefer return cannot identify inserted primary key values honestly",
	}
}

func keysFromBody(bodyRows []map[string]any, primaryKey []string) ([]map[string]any, bool) {
	keys := make([]map[string]any, len(bodyRows))
	for i, row := range bodyRows {
		key := make(map[string]any, len(primaryKey))
		for _, column := range primaryKey {
			value, held := row[column]
			if !held || value == nil {
				return nil, false
			}
			key[column] = value
		}
		keys[i] = key
	}
	return keys, true
}

func selectKeyMaps(
	ctx context.Context,
	tx *sql.Tx,
	table schemacache.Table,
	primaryKey []string,
	query readquery.Query,
) ([]map[string]any, error) {
	if len(primaryKey) == 0 {
		return nil, readquery.UnsupportedFeature{
			Message: "Prefer return=representation needs a primary key to return updated rows honestly",
		}
	}
	parts, err := buildSelectColumns(table, primaryKey, query)
	if err != nil {
		return nil, err
	}
	result, err := tx.QueryContext(ctx, parts.statement, parts.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	var keys []map[string]any
	for result.Next() {
		values := make([]any, len(primaryKey))
		dest := make([]any, len(primaryKey))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := result.Scan(dest...); err != nil {
			return nil, err
		}
		key := make(map[string]any, len(primaryKey))
		for i, column := range primaryKey {
			key[column] = unwrapValue(values[i])
		}
		keys = append(keys, key)
	}
	return keys, result.Err()
}

func selectByKeys(
	ctx context.Context,
	tx *sql.Tx,
	table schemacache.Table,
	primaryKey []string,
	keys []map[string]any,
) ([]rows.Row, error) {
	if len(keys) == 0 {
		return []rows.Row{}, nil
	}
	if len(primaryKey) != 1 {
		return selectByMultiKeys(ctx, tx, table, primaryKey, keys)
	}
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = fmt.Sprint(key[primaryKey[0]])
	}
	query := readquery.Query{
		SelectAll: true,
		Filters: []readquery.Filter{{
			Column: primaryKey[0],
			Op:     readquery.OpIn,
			Values: values,
		}},
	}
	return selectMatching(ctx, tx, table, query)
}

func selectByMultiKeys(
	ctx context.Context,
	tx *sql.Tx,
	table schemacache.Table,
	primaryKey []string,
	keys []map[string]any,
) ([]rows.Row, error) {
	columnList, err := resolveColumns(table, readquery.Query{SelectAll: true})
	if err != nil {
		return nil, err
	}
	orParts := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*len(primaryKey))
	for _, key := range keys {
		andParts := make([]string, 0, len(primaryKey))
		for _, column := range primaryKey {
			andParts = append(andParts, quoteIdentifier(column)+" = ?")
			args = append(args, key[column])
		}
		orParts = append(orParts, "("+strings.Join(andParts, " AND ")+")")
	}
	statement := fmt.Sprintf(
		"SELECT %s FROM %s.%s WHERE %s",
		selectList(columnList),
		quoteIdentifier(table.ID.Database),
		quoteIdentifier(table.ID.Name),
		strings.Join(orParts, " OR "),
	)
	result, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	names := outputNames(columnList)
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

func selectMatching(
	ctx context.Context,
	tx *sql.Tx,
	table schemacache.Table,
	query readquery.Query,
) ([]rows.Row, error) {
	if !query.SelectAll && len(query.Columns) == 0 {
		query.SelectAll = true
	}
	parts, err := buildSelect(table, query)
	if err != nil {
		return nil, err
	}
	result, err := tx.QueryContext(ctx, parts.statement, parts.args...)
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

func buildSelectColumns(
	table schemacache.Table,
	columns []string,
	query readquery.Query,
) (sqlParts, error) {
	selectQuery := readquery.Query{
		Filters: query.Filters,
		Groups:  query.Groups,
	}
	for _, name := range columns {
		selectQuery.Columns = append(selectQuery.Columns, readquery.Column{Name: name})
	}
	return buildSelect(table, selectQuery)
}

func unwrapValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
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
