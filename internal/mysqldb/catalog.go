package mysqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

const (
	tableQuery = `SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_COMMENT
	FROM information_schema.TABLES
	WHERE TABLE_TYPE = 'BASE TABLE' AND TABLE_SCHEMA IN (%s)
	ORDER BY TABLE_SCHEMA, TABLE_NAME`

	viewQuery = `SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_COMMENT
	FROM information_schema.TABLES
	WHERE TABLE_TYPE = 'VIEW' AND TABLE_SCHEMA IN (%s)
	ORDER BY TABLE_SCHEMA, TABLE_NAME`

	columnQuery = `SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE,
		COLLATION_NAME, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_COMMENT, EXTRA
	FROM information_schema.COLUMNS
	WHERE TABLE_SCHEMA IN (%s)
	ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`

	keyQuery = `SELECT tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_NAME, tc.CONSTRAINT_TYPE,
		kcu.COLUMN_NAME, kcu.ORDINAL_POSITION
	FROM information_schema.TABLE_CONSTRAINTS tc
	JOIN information_schema.KEY_COLUMN_USAGE kcu
		ON tc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA
		AND tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
		AND tc.TABLE_NAME = kcu.TABLE_NAME
	WHERE tc.CONSTRAINT_TYPE IN ('PRIMARY KEY', 'UNIQUE')
		AND tc.TABLE_SCHEMA IN (%s)
	ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

	foreignKeyQuery = `SELECT kcu.CONSTRAINT_NAME, kcu.TABLE_SCHEMA, kcu.TABLE_NAME,
		kcu.COLUMN_NAME, kcu.REFERENCED_TABLE_SCHEMA, kcu.REFERENCED_TABLE_NAME,
		kcu.REFERENCED_COLUMN_NAME, kcu.ORDINAL_POSITION,
		rc.UPDATE_RULE, rc.DELETE_RULE
	FROM information_schema.KEY_COLUMN_USAGE kcu
	JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
		ON kcu.CONSTRAINT_SCHEMA = rc.CONSTRAINT_SCHEMA
		AND kcu.CONSTRAINT_NAME = rc.CONSTRAINT_NAME
	WHERE kcu.REFERENCED_TABLE_NAME IS NOT NULL
		AND kcu.TABLE_SCHEMA IN (%s)
	ORDER BY kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

	routineQuery = `SELECT ROUTINE_SCHEMA, ROUTINE_NAME, ROUTINE_TYPE, ROUTINE_COMMENT,
		DTD_IDENTIFIER, SQL_DATA_ACCESS
	FROM information_schema.ROUTINES
	WHERE ROUTINE_SCHEMA IN (%s)
	ORDER BY ROUTINE_SCHEMA, ROUTINE_NAME`

	parameterQuery = `SELECT SPECIFIC_SCHEMA, SPECIFIC_NAME, PARAMETER_NAME, PARAMETER_MODE,
		ORDINAL_POSITION, DTD_IDENTIFIER
	FROM information_schema.PARAMETERS
	WHERE SPECIFIC_SCHEMA IN (%s)
	ORDER BY SPECIFIC_SCHEMA, SPECIFIC_NAME, ORDINAL_POSITION`

	tablePrivilegeQuery = `SELECT GRANTEE, TABLE_SCHEMA, TABLE_NAME, PRIVILEGE_TYPE
	FROM information_schema.%s
	WHERE TABLE_SCHEMA IN (%s)`

	routinePrivilegeQuery = `SELECT GRANTEE, ROUTINE_SCHEMA, ROUTINE_NAME, PRIVILEGE_TYPE
	FROM information_schema.%s
	WHERE ROUTINE_SCHEMA IN (%s)`

	roleQuery = `SELECT GRANTEE, ROLE_NAME FROM information_schema.APPLICABLE_ROLES`
)

var (
	tablePrivilegeSources   = []string{"ROLE_TABLE_GRANTS", "TABLE_PRIVILEGES"}
	routinePrivilegeSources = []string{"ROLE_ROUTINE_GRANTS"}
)

// readCatalog reads the catalog data of the configured MySQL databases. The
// caller must have activated the roles of the authenticator.
func readCatalog(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) (schemacache.Catalog, error) {
	if len(databases) == 0 {
		return schemacache.Catalog{}, errors.New("db-schemas holds no MySQL database")
	}

	objects, err := readCatalogObjects(ctx, conn, databases)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	privileges, err := readCatalogPrivileges(ctx, conn, databases)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	return schemacache.Catalog{
		Tables:            objects.tables,
		Views:             objects.views,
		RelationComments:  objects.comments,
		Columns:           objects.columns,
		Keys:              objects.keys,
		ForeignKeys:       objects.foreignKeys,
		Routines:          objects.routines,
		Selects:           privileges.selects,
		TablePrivileges:   privileges.tablePrivileges,
		RoutinePrivileges: privileges.routinePrivileges,
		Roles:             privileges.roles,
	}, nil
}

type catalogObjects struct {
	tables      []schemacache.TableID
	views       []schemacache.TableID
	comments    []schemacache.CommentFact
	columns     []schemacache.ColumnFact
	keys        []schemacache.KeyFact
	foreignKeys []schemacache.ForeignKeyFact
	routines    []schemacache.RoutineFact
}

func readCatalogObjects(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) (catalogObjects, error) {
	tables, tableComments, err := readRelations(ctx, conn, tableQuery, databases)
	if err != nil {
		return catalogObjects{}, fmt.Errorf("read the tables: %w", err)
	}
	views, viewComments, err := readRelations(ctx, conn, viewQuery, databases)
	if err != nil {
		return catalogObjects{}, fmt.Errorf("read the views: %w", err)
	}
	columns, err := read(ctx, conn, fmt.Sprintf(columnQuery, placeholders(databases)), databases, scanColumn)
	if err != nil {
		return catalogObjects{}, fmt.Errorf("read the columns: %w", err)
	}
	keys, err := readKeys(ctx, conn, databases)
	if err != nil {
		return catalogObjects{}, fmt.Errorf("read the keys: %w", err)
	}
	foreignKeys, err := readForeignKeys(ctx, conn, databases)
	if err != nil {
		return catalogObjects{}, fmt.Errorf("read the foreign keys: %w", err)
	}
	routines, err := readRoutines(ctx, conn, databases)
	if err != nil {
		return catalogObjects{}, err
	}
	return catalogObjects{
		tables:      tables,
		views:       views,
		comments:    append(tableComments, viewComments...),
		columns:     columns,
		keys:        keys,
		foreignKeys: foreignKeys,
		routines:    routines,
	}, nil
}

type catalogPrivileges struct {
	selects           []schemacache.SelectFact
	tablePrivileges   []schemacache.TablePrivilegeFact
	routinePrivileges []schemacache.RoutinePrivilegeFact
	roles             []schemacache.RoleFact
}

func readCatalogPrivileges(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) (catalogPrivileges, error) {
	selects, tablePrivileges, err := readTablePrivileges(ctx, conn, databases)
	if err != nil {
		return catalogPrivileges{}, err
	}
	routinePrivileges, err := readRoutinePrivileges(ctx, conn, databases)
	if err != nil {
		return catalogPrivileges{}, err
	}
	roles, err := read(ctx, conn, roleQuery, nil, scanRole)
	if err != nil {
		return catalogPrivileges{}, fmt.Errorf("read the role grants: %w", err)
	}
	return catalogPrivileges{
		selects:           selects,
		tablePrivileges:   tablePrivileges,
		routinePrivileges: routinePrivileges,
		roles:             roles,
	}, nil
}

func readRelations(
	ctx context.Context,
	conn *sql.Conn,
	query string,
	databases []string,
) ([]schemacache.TableID, []schemacache.CommentFact, error) {
	result, err := conn.QueryContext(ctx, fmt.Sprintf(query, placeholders(databases)), asArguments(databases)...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = result.Close() }()

	var relations []schemacache.TableID
	var comments []schemacache.CommentFact
	for result.Next() {
		var id schemacache.TableID
		var comment string
		if err := result.Scan(&id.Database, &id.Name, &comment); err != nil {
			return nil, nil, err
		}
		relations = append(relations, id)
		if comment != "" && comment != "VIEW" {
			comments = append(comments, schemacache.CommentFact{Relation: id, Comment: comment})
		}
	}
	return relations, comments, result.Err()
}

func readKeys(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.KeyFact, error) {
	result, err := conn.QueryContext(ctx, fmt.Sprintf(keyQuery, placeholders(databases)), asArguments(databases)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	var keys []schemacache.KeyFact
	index := map[string]int{}
	for result.Next() {
		var table schemacache.TableID
		var name, kind, column string
		var ordinal int
		if err := result.Scan(&table.Database, &table.Name, &name, &kind, &column, &ordinal); err != nil {
			return nil, err
		}
		if kind == "PRIMARY KEY" {
			kind = "PRIMARY"
		}
		keyID := table.Database + "\x00" + table.Name + "\x00" + name
		if at, held := index[keyID]; held {
			keys[at].Columns = append(keys[at].Columns, column)
			continue
		}
		index[keyID] = len(keys)
		keys = append(keys, schemacache.KeyFact{
			Table:   table,
			Name:    name,
			Kind:    kind,
			Columns: []string{column},
		})
	}
	return keys, result.Err()
}

func readForeignKeys(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.ForeignKeyFact, error) {
	result, err := conn.QueryContext(ctx, fmt.Sprintf(foreignKeyQuery, placeholders(databases)), asArguments(databases)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	var foreignKeys []schemacache.ForeignKeyFact
	index := map[string]int{}
	for result.Next() {
		var fact schemacache.ForeignKeyFact
		var ordinal int
		var fromColumn, toColumn string
		if err := result.Scan(
			&fact.Name,
			&fact.Table.Database,
			&fact.Table.Name,
			&fromColumn,
			&fact.ReferencedTable.Database,
			&fact.ReferencedTable.Name,
			&toColumn,
			&ordinal,
			&fact.UpdateRule,
			&fact.DeleteRule,
		); err != nil {
			return nil, err
		}
		keyID := fact.Table.Database + "\x00" + fact.Name
		if at, held := index[keyID]; held {
			foreignKeys[at].Columns = append(foreignKeys[at].Columns, fromColumn)
			foreignKeys[at].ReferencedColumns = append(foreignKeys[at].ReferencedColumns, toColumn)
			continue
		}
		index[keyID] = len(foreignKeys)
		fact.Columns = []string{fromColumn}
		fact.ReferencedColumns = []string{toColumn}
		foreignKeys = append(foreignKeys, fact)
	}
	return foreignKeys, result.Err()
}

func readRoutines(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.RoutineFact, error) {
	routines, err := read(ctx, conn, fmt.Sprintf(routineQuery, placeholders(databases)), databases, scanRoutine)
	if err != nil {
		return nil, fmt.Errorf("read the routines: %w", err)
	}
	parameters, err := read(ctx, conn, fmt.Sprintf(parameterQuery, placeholders(databases)), databases, scanParameter)
	if err != nil {
		return nil, fmt.Errorf("read the routine parameters: %w", err)
	}

	grouped := map[schemacache.RoutineID][]schemacache.ParameterFact{}
	for _, parameter := range parameters {
		grouped[parameter.routine] = append(grouped[parameter.routine], parameter.fact)
	}
	for i := range routines {
		routines[i].Parameters = grouped[routines[i].ID]
	}
	return routines, nil
}

type parameterRow struct {
	routine schemacache.RoutineID
	fact    schemacache.ParameterFact
}

func scanRoutine(result *sql.Rows) (schemacache.RoutineFact, bool, error) {
	var fact schemacache.RoutineFact
	var returnType sql.NullString
	err := result.Scan(
		&fact.ID.Database,
		&fact.ID.Name,
		&fact.Kind,
		&fact.Comment,
		&returnType,
		&fact.SQLDataAccess,
	)
	if err != nil {
		return fact, false, err
	}
	if returnType.Valid {
		fact.ReturnType = returnType.String
	}
	return fact, true, nil
}

func scanParameter(result *sql.Rows) (parameterRow, bool, error) {
	var row parameterRow
	var name, mode sql.NullString
	err := result.Scan(
		&row.routine.Database,
		&row.routine.Name,
		&name,
		&mode,
		&row.fact.Ordinal,
		&row.fact.DataType,
	)
	if err != nil {
		return row, false, err
	}
	if name.Valid {
		row.fact.Name = name.String
	}
	if mode.Valid {
		row.fact.Mode = mode.String
	}
	return row, true, nil
}

func readTablePrivileges(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.SelectFact, []schemacache.TablePrivilegeFact, error) {
	var selects []schemacache.SelectFact
	var privileges []schemacache.TablePrivilegeFact
	for _, source := range tablePrivilegeSources {
		query := fmt.Sprintf(tablePrivilegeQuery, source, placeholders(databases))
		found, err := read(ctx, conn, query, databases, scanTablePrivilege)
		if err != nil {
			return nil, nil, fmt.Errorf("read the table privileges of %s: %w", source, err)
		}
		for _, fact := range found {
			for _, privilege := range splitPrivileges(fact.Privilege) {
				if !isTableExposurePrivilege(privilege) {
					continue
				}
				one := fact
				one.Privilege = privilege
				privileges = append(privileges, one)
				if privilege == "SELECT" {
					selects = append(selects, schemacache.SelectFact{Role: one.Role, Table: one.Table})
				}
			}
		}
	}
	return selects, privileges, nil
}

func readRoutinePrivileges(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.RoutinePrivilegeFact, error) {
	var privileges []schemacache.RoutinePrivilegeFact
	for _, source := range routinePrivilegeSources {
		query := fmt.Sprintf(routinePrivilegeQuery, source, placeholders(databases))
		found, err := read(ctx, conn, query, databases, scanRoutinePrivilege)
		if err != nil {
			return nil, fmt.Errorf("read the routine privileges of %s: %w", source, err)
		}
		for _, fact := range found {
			for _, privilege := range splitPrivileges(fact.Privilege) {
				if privilege != "EXECUTE" {
					continue
				}
				one := fact
				one.Privilege = privilege
				privileges = append(privileges, one)
			}
		}
	}
	return privileges, nil
}

// splitPrivileges turns a MySQL privilege SET value into one uppercase privilege
// name per grant. ROLE_*_GRANTS can pack several privileges into one cell.
func splitPrivileges(value string) []string {
	parts := strings.Split(value, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.ToUpper(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func isTableExposurePrivilege(privilege string) bool {
	switch privilege {
	case "SELECT", "INSERT", "UPDATE", "DELETE":
		return true
	default:
		return false
	}
}

// read runs one catalog query over the configured databases and reads every
// row of the answer with scan. A row that scan gives no fact for is left out.
func read[Fact any](
	ctx context.Context,
	conn *sql.Conn,
	query string,
	databases []string,
	scan func(*sql.Rows) (Fact, bool, error),
) ([]Fact, error) {
	result, err := conn.QueryContext(ctx, query, asArguments(databases)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Close() }()

	var facts []Fact
	for result.Next() {
		fact, held, err := scan(result)
		if err != nil {
			return nil, err
		}
		if held {
			facts = append(facts, fact)
		}
	}
	return facts, result.Err()
}

func scanColumn(result *sql.Rows) (schemacache.ColumnFact, bool, error) {
	var fact schemacache.ColumnFact
	var collation sql.NullString
	var nullable string
	var defaultValue sql.NullString
	var extra string
	err := result.Scan(
		&fact.Table.Database,
		&fact.Table.Name,
		&fact.Name,
		&fact.DataType,
		&collation,
		&nullable,
		&defaultValue,
		&fact.Comment,
		&extra,
	)
	if err != nil {
		return fact, false, err
	}
	if collation.Valid {
		fact.Collation = collation.String
	}
	fact.Nullable = strings.EqualFold(nullable, "YES")
	if defaultValue.Valid {
		value := defaultValue.String
		fact.Default = &value
	}
	lowerExtra := strings.ToLower(extra)
	fact.Generated = strings.Contains(lowerExtra, "generated")
	fact.AutoIncrement = strings.Contains(lowerExtra, "auto_increment")
	return fact, true, nil
}

func scanTablePrivilege(result *sql.Rows) (schemacache.TablePrivilegeFact, bool, error) {
	var grantee string
	var fact schemacache.TablePrivilegeFact
	if err := result.Scan(&grantee, &fact.Table.Database, &fact.Table.Name, &fact.Privilege); err != nil {
		return fact, false, err
	}
	fact.Role = roleOfGrantee(grantee)
	return fact, fact.Role != "", nil
}

func scanRoutinePrivilege(result *sql.Rows) (schemacache.RoutinePrivilegeFact, bool, error) {
	var grantee string
	var fact schemacache.RoutinePrivilegeFact
	if err := result.Scan(&grantee, &fact.Routine.Database, &fact.Routine.Name, &fact.Privilege); err != nil {
		return fact, false, err
	}
	fact.Role = roleOfGrantee(grantee)
	return fact, fact.Role != "", nil
}

// scanRole reads a role grant. A grantee myrest cannot read a role name out
// of gives no fact.
func scanRole(result *sql.Rows) (schemacache.RoleFact, bool, error) {
	var holder, granted string
	if err := result.Scan(&holder, &granted); err != nil {
		return schemacache.RoleFact{}, false, err
	}
	fact := schemacache.RoleFact{
		Holder:  roleOfGrantee(holder),
		Granted: roleOfGrantee(granted),
	}
	return fact, fact.Holder != "" && fact.Granted != "", nil
}

// roleOfGrantee reads the role name out of a MySQL grantee. TABLE_PRIVILEGES
// writes `'name'@'host'`; ROLE_TABLE_GRANTS writes the bare name.
func roleOfGrantee(grantee string) schemacache.Role {
	if quoted, isQuoted := strings.CutPrefix(grantee, "'"); isQuoted {
		name, _, closed := strings.Cut(quoted, "'")
		if !closed {
			return ""
		}
		return schemacache.Role(name)
	}
	name, _, _ := strings.Cut(grantee, "@")
	return schemacache.Role(name)
}

// placeholders builds the `?, ?` list an IN clause needs for the databases.
func placeholders(databases []string) string {
	return strings.TrimSuffix(strings.Repeat("?, ", len(databases)), ", ")
}

// asArguments passes the database names as query arguments.
func asArguments(databases []string) []any {
	passed := make([]any, len(databases))
	for i, database := range databases {
		passed[i] = database
	}
	return passed
}
