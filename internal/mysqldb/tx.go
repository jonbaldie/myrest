package mysqldb

import (
	"context"
	"database/sql"

	"github.com/jonbaldie/myrest/internal/config"
)

// SetTxEnd sets how write and RPC request transactions end (knob db-tx-end).
func (p *Pool) SetTxEnd(txEnd config.TxEnd) {
	p.txEnd = txEnd
}

func (p *Pool) resolvedTxEnd() config.TxEnd {
	if p.txEnd == "" {
		return config.TxEndCommit
	}
	return p.txEnd
}

// withRequestTx runs pre-request and work inside one READ COMMITTED
// transaction, then commits or rolls back per db-tx-end and Prefer: tx=.
func (p *Pool) withRequestTx(
	ctx context.Context,
	roleSwitch string,
	preferTx string,
	work func(context.Context, *sql.Tx) error,
) error {
	return p.onConnection(ctx, roleSwitch, func(ctx context.Context, conn *sql.Conn) error {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		if err := p.runPreRequest(ctx, tx); err != nil {
			return err
		}
		if err := work(ctx, tx); err != nil {
			return err
		}
		commit, _ := config.DecideTxEnd(p.resolvedTxEnd(), preferTx)
		if !commit {
			return nil
		}
		return tx.Commit()
	})
}
