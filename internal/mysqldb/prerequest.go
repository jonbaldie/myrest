package mysqldb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SetPreRequest names the zero-argument procedure myrest calls after the role
// switch and before the main statement of a request. An empty name turns the
// hook off.
func (p *Pool) SetPreRequest(name string) {
	p.preRequest = strings.TrimSpace(name)
}

// onRequest takes one authenticator connection, activates the request role,
// runs the optional db-pre-request hook, then runs work.
func (p *Pool) onRequest(
	ctx context.Context,
	roleSwitch string,
	work func(context.Context, *sql.Conn) error,
) error {
	return p.onConnection(ctx, roleSwitch, func(ctx context.Context, conn *sql.Conn) error {
		if err := p.runPreRequest(ctx, conn); err != nil {
			return err
		}
		return work(ctx, conn)
	})
}

func (p *Pool) runPreRequest(ctx context.Context, conn *sql.Conn) error {
	if p.preRequest == "" {
		return nil
	}
	statement, err := preRequestCallStatement(p.preRequest)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		return err
	}
	return nil
}

// preRequestCallStatement is the CALL for a zero-argument procedure named as
// database.routine (the PostgREST db-pre-request shape on MySQL).
func preRequestCallStatement(name string) (string, error) {
	database, routine, err := splitPreRequest(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"CALL %s.%s()",
		quoteIdentifier(database),
		quoteIdentifier(routine),
	), nil
}

func splitPreRequest(name string) (database, routine string, err error) {
	parts := strings.Split(name, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("db-pre-request %q must be database.routine", name)
	}
	return parts[0], parts[1], nil
}
