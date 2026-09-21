// Package readexec runs the read side of a request that can nest embeds: it
// plans the embed tree against the schema cache, loads the related rows in
// batches through a Reader, nests them into their parent rows, and shapes the
// result. Ordinary read, write representation, and RPC row sets share it.
package readexec

import (
	"context"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Reader reads a resource as one database role under an ordinary-read query.
type Reader interface {
	Read(
		ctx context.Context,
		role schemacache.Role,
		table schemacache.Table,
		query readquery.Query,
	) (readquery.Result, error)
}

// Executor reads and shapes row sets with their embeds.
type Executor struct {
	cache  *schemacache.Cache
	reader Reader
}

// New returns an Executor that resolves relationships in cache and reads
// rows through reader.
func New(cache *schemacache.Cache, reader Reader) *Executor {
	return &Executor{cache: cache, reader: reader}
}

// Execute reads table under query and nests the embeds of query into the
// rows. It adds the origin join columns the embeds need to the read, and
// drops them again from the result when the client did not select them.
func (e *Executor) Execute(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	query readquery.Query,
) (readquery.Result, error) {
	plan, err := e.Plan(role, table.ID, query.Embeds)
	if err != nil {
		return readquery.Result{}, err
	}
	query, injected := withJoinColumns(table, query, plan.embeds)
	read, err := e.reader.Read(ctx, role, table, query)
	if err != nil {
		return readquery.Result{}, err
	}
	nested, err := e.nest(ctx, role, read.Rows, plan.embeds)
	if err != nil {
		return readquery.Result{}, err
	}
	read.Rows = dropColumns(nested, injected)
	return read, nil
}

// Shape filters, orders, and pages rows the caller already holds, nests the
// embeds of query with table as their origin, and keeps the selected columns.
// It adds no join columns: the rows must hold the origin keys of the embeds.
// When query has no embeds, table is not used and can be the zero Table.
func (e *Executor) Shape(
	ctx context.Context,
	role schemacache.Role,
	table schemacache.Table,
	set []rows.Row,
	query readquery.Query,
) (readquery.Result, error) {
	plan, err := e.Plan(role, table.ID, query.Embeds)
	if err != nil {
		return readquery.Result{}, err
	}
	return e.shapeRows(ctx, role, plan, set, query)
}

// ShapePlanned is Shape with an embed plan the caller made before, so a write
// nests the embeds it checked before the mutation.
func (e *Executor) ShapePlanned(
	ctx context.Context,
	role schemacache.Role,
	plan Plan,
	set []rows.Row,
	query readquery.Query,
) (readquery.Result, error) {
	return e.shapeRows(ctx, role, plan, set, query)
}
