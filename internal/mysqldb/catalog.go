package mysqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

// tableQuery finds the tables of the configured MySQL databases.
const tableQuery = `SELECT TABLE_SCHEMA, TABLE_NAME
	FROM information_schema.TABLES
	WHERE TABLE_TYPE = 'BASE TABLE' AND TABLE_SCHEMA IN (%s)`

// columnQuery finds the columns of those tables, in catalog order.
const columnQuery = `SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME
	FROM information_schema.COLUMNS
	WHERE TABLE_SCHEMA IN (%s)
	ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`

// grantQuery finds who holds SELECT on those tables. A grant to a database
// role is in ROLE_TABLE_GRANTS; a grant to an account is in TABLE_PRIVILEGES.
const grantQuery = `SELECT GRANTEE, TABLE_SCHEMA, TABLE_NAME
	FROM information_schema.%s
	WHERE PRIVILEGE_TYPE = 'SELECT' AND TABLE_SCHEMA IN (%s)`

// roleQuery finds which role MySQL granted to which other role, so that the
// cache can follow a SELECT privilege that reaches a role through another one.
// The view holds the roles the authenticator can reach, which are the roles
// myrest can activate (ADR 0010).
const roleQuery = `SELECT GRANTEE, ROLE_NAME FROM information_schema.APPLICABLE_ROLES`

// grantSources are the catalog views that report a SELECT privilege.
var grantSources = []string{"ROLE_TABLE_GRANTS", "TABLE_PRIVILEGES"}

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

	tables, err := read(ctx, conn, fmt.Sprintf(tableQuery, placeholders(databases)), databases, scanTable)
	if err != nil {
		return schemacache.Catalog{}, fmt.Errorf("read the tables: %w", err)
	}
	columns, err := read(ctx, conn, fmt.Sprintf(columnQuery, placeholders(databases)), databases, scanColumn)
	if err != nil {
		return schemacache.Catalog{}, fmt.Errorf("read the columns: %w", err)
	}
	selects, err := readSelectFacts(ctx, conn, databases)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	roles, err := read(ctx, conn, roleQuery, nil, scanRole)
	if err != nil {
		return schemacache.Catalog{}, fmt.Errorf("read the role grants: %w", err)
	}
	return schemacache.Catalog{
		Tables:  tables,
		Columns: columns,
		Selects: selects,
		Roles:   roles,
	}, nil
}

// readSelectFacts reads the SELECT grants out of every catalog view that
// reports one.
func readSelectFacts(
	ctx context.Context,
	conn *sql.Conn,
	databases []string,
) ([]schemacache.SelectFact, error) {
	var facts []schemacache.SelectFact
	for _, source := range grantSources {
		query := fmt.Sprintf(grantQuery, source, placeholders(databases))
		found, err := read(ctx, conn, query, databases, scanSelect)
		if err != nil {
			return nil, fmt.Errorf("read the SELECT grants of %s: %w", source, err)
		}
		facts = append(facts, found...)
	}
	return facts, nil
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

func scanTable(result *sql.Rows) (schemacache.TableID, bool, error) {
	var table schemacache.TableID
	err := result.Scan(&table.Database, &table.Name)
	return table, err == nil, err
}

func scanColumn(result *sql.Rows) (schemacache.ColumnFact, bool, error) {
	var fact schemacache.ColumnFact
	err := result.Scan(&fact.Table.Database, &fact.Table.Name, &fact.Name)
	return fact, err == nil, err
}

// scanSelect reads a grant row. A grantee myrest cannot read a role name out
// of gives no fact.
func scanSelect(result *sql.Rows) (schemacache.SelectFact, bool, error) {
	var grantee string
	var fact schemacache.SelectFact
	if err := result.Scan(&grantee, &fact.Table.Database, &fact.Table.Name); err != nil {
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
