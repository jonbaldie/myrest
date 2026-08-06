package mysqldb

import (
	"context"
	"database/sql"
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

// grantSources are the catalog views that report a SELECT privilege.
var grantSources = []string{"ROLE_TABLE_GRANTS", "TABLE_PRIVILEGES"}

// readCatalog reads the catalog data of the configured MySQL databases. The
// caller must have activated the roles of the authenticator.
func readCatalog(ctx context.Context, conn *sql.Conn, schemas []string) (schemacache.Catalog, error) {
	if len(schemas) == 0 {
		return schemacache.Catalog{}, fmt.Errorf("db-schemas holds no MySQL database")
	}

	tables, err := readTableFacts(ctx, conn, schemas)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	columns, err := readColumnFacts(ctx, conn, schemas)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	selects, err := readSelectFacts(ctx, conn, schemas)
	if err != nil {
		return schemacache.Catalog{}, err
	}
	return schemacache.Catalog{Tables: tables, Columns: columns, Selects: selects}, nil
}

func readTableFacts(ctx context.Context, conn *sql.Conn, schemas []string) ([]schemacache.TableFact, error) {
	query := fmt.Sprintf(tableQuery, placeholders(len(schemas)))
	result, err := conn.QueryContext(ctx, query, arguments(schemas)...)
	if err != nil {
		return nil, fmt.Errorf("read the tables: %w", err)
	}
	defer func() { _ = result.Close() }()

	var facts []schemacache.TableFact
	for result.Next() {
		var fact schemacache.TableFact
		if err := result.Scan(&fact.Schema, &fact.Name); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, result.Err()
}

func readColumnFacts(ctx context.Context, conn *sql.Conn, schemas []string) ([]schemacache.ColumnFact, error) {
	query := fmt.Sprintf(columnQuery, placeholders(len(schemas)))
	result, err := conn.QueryContext(ctx, query, arguments(schemas)...)
	if err != nil {
		return nil, fmt.Errorf("read the columns: %w", err)
	}
	defer func() { _ = result.Close() }()

	var facts []schemacache.ColumnFact
	for result.Next() {
		var fact schemacache.ColumnFact
		if err := result.Scan(&fact.Schema, &fact.Table, &fact.Name); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, result.Err()
}

func readSelectFacts(ctx context.Context, conn *sql.Conn, schemas []string) ([]schemacache.SelectFact, error) {
	var facts []schemacache.SelectFact
	for _, source := range grantSources {
		query := fmt.Sprintf(grantQuery, source, placeholders(len(schemas)))
		result, err := conn.QueryContext(ctx, query, arguments(schemas)...)
		if err != nil {
			return nil, fmt.Errorf("read the SELECT grants of %s: %w", source, err)
		}
		found, err := scanSelectFacts(result)
		_ = result.Close()
		if err != nil {
			return nil, err
		}
		facts = append(facts, found...)
	}
	return facts, nil
}

func scanSelectFacts(result *sql.Rows) ([]schemacache.SelectFact, error) {
	var facts []schemacache.SelectFact
	for result.Next() {
		var grantee string
		var fact schemacache.SelectFact
		if err := result.Scan(&grantee, &fact.Schema, &fact.Table); err != nil {
			return nil, err
		}
		fact.Role = roleOfGrantee(grantee)
		if fact.Role == "" {
			continue
		}
		facts = append(facts, fact)
	}
	return facts, result.Err()
}

// roleOfGrantee reads the role name out of a MySQL grantee. TABLE_PRIVILEGES
// writes `'name'@'host'`; ROLE_TABLE_GRANTS writes the bare name.
func roleOfGrantee(grantee string) string {
	name := grantee
	if quoted := strings.HasPrefix(name, "'"); quoted {
		end := strings.Index(name[1:], "'")
		if end < 0 {
			return ""
		}
		return name[1 : 1+end]
	}
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	return name
}

// placeholders builds the `?, ?` list of an IN clause.
func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", count), ", ")
}

// arguments passes the configured databases as query arguments.
func arguments(values []string) []any {
	passed := make([]any, len(values))
	for i, value := range values {
		passed[i] = value
	}
	return passed
}
