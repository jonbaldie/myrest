package mysqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

// Catalog reads the catalog data of the configured MySQL databases, with every
// role of the authenticator active (see ADR 0010).
func (p *Pool) Catalog(ctx context.Context, databases []string) (schemacache.Catalog, error) {
	var catalog schemacache.Catalog
	err := p.onConnection(ctx, activateAllRoles, func(ctx context.Context, conn *sql.Conn) error {
		var readErr error
		catalog, readErr = readCatalog(ctx, conn, databases)
		return readErr
	})
	return catalog, err
}

// Read gives back every row of the table, as the database role. MySQL applies
// the grants of that role, so a table the role lost SELECT on gives an error
// even when the schema cache still holds it.
func (p *Pool) Read(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
) ([]rows.Row, error) {
	statement, err := roleSwitchStatement(role)
	if err != nil {
		return nil, err
	}

	var read []rows.Row
	err = p.onConnection(ctx, statement, func(ctx context.Context, conn *sql.Conn) error {
		var readErr error
		read, readErr = readTable(ctx, conn, table)
		return readErr
	})
	return read, err
}

// onConnection takes one authenticator connection, activates roles on it with
// the given SET ROLE statement, and runs work.
//
// No request can read with the roles of another request: MySQL keeps the
// active roles for the session, and every use of a connection comes through
// here, where SET ROLE replaces them. Clearing the roles afterwards is what
// keeps a connection that waits in the pool powerless; a connection that
// cannot be cleared leaves the pool instead.
func (p *Pool) onConnection(
	ctx context.Context,
	roleSwitch string,
	work func(context.Context, *sql.Conn) error,
) error {
	conn, err := p.connections.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, roleSwitch); err != nil {
		return fmt.Errorf("%s: %w", roleSwitch, err)
	}
	defer clearRolesOrDrop(conn)

	return work(ctx, conn)
}

// clearRolesOrDrop takes the roles off the connection. A connection that keeps
// its roles must not go back to the pool, so it is marked unusable and the
// pool throws it away.
func clearRolesOrDrop(conn *sql.Conn) {
	if _, err := conn.ExecContext(context.Background(), clearRoles); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
}
