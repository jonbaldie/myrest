package mysqldb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"

	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Pool holds MySQL connections that log in as the authenticator. Every
// connection starts with no database role, so a request reads only what the
// role switch gives it.
type Pool struct {
	connections *sql.DB
}

// Open starts a pool from a db-uri and proves that the authenticator can log
// in. The caller must Close the pool.
func Open(uri string) (*Pool, error) {
	name, err := dataSourceName(uri)
	if err != nil {
		return nil, err
	}
	connections, err := sql.Open("mysql", name)
	if err != nil {
		return nil, err
	}
	if err := connections.Ping(); err != nil {
		_ = connections.Close()
		return nil, fmt.Errorf("log in as the authenticator: %w", err)
	}
	return &Pool{connections: connections}, nil
}

// Close ends every connection of the pool.
func (p *Pool) Close() error {
	return p.connections.Close()
}

// Catalog reads the catalog data of the configured MySQL databases. It
// activates every role of the authenticator first, because MySQL hides
// catalog rows from an account that holds no privilege on them.
func (p *Pool) Catalog(ctx context.Context, schemas []string) (schemacache.Catalog, error) {
	var catalog schemacache.Catalog
	err := p.withRole(ctx, allRoles, func(ctx context.Context, conn *sql.Conn) error {
		var readErr error
		catalog, readErr = readCatalog(ctx, conn, schemas)
		return readErr
	})
	return catalog, err
}

// Read gives back every row of the table, as the database role. MySQL applies
// the grants of that role, so a table the role lost SELECT on gives an error
// even when the schema cache still holds it.
func (p *Pool) Read(ctx context.Context, role string, table schemacache.Table) ([]rows.Row, error) {
	var read []rows.Row
	err := p.withRole(ctx, role, func(ctx context.Context, conn *sql.Conn) error {
		var readErr error
		read, readErr = readTable(ctx, conn, table)
		return readErr
	})
	return read, err
}

// withRole takes one authenticator connection, activates the database role on
// it, runs work, and then clears the role so that the next request that takes
// the connection starts with no privileges.
func (p *Pool) withRole(
	ctx context.Context,
	role string,
	work func(context.Context, *sql.Conn) error,
) error {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return err
	}
	conn, err := p.connections.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("role switch to %s: %w", role, err)
	}
	defer func() { _, _ = conn.ExecContext(context.WithoutCancel(ctx), "SET ROLE NONE") }()

	return work(ctx, conn)
}
