// Package rpcexec coordinates the execution of database routines (functions and
// stored procedures) behind a typed interface. It validates routine signatures,
// enforces GET read-safety, coordinates transaction boundaries, and discriminates
// typed routine outcomes.
package rpcexec

import (
	"context"
	"fmt"

	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// CallMode identifies the HTTP method or caller mode used for the RPC call.
type CallMode int

const (
	// CallModePost indicates a POST routine call (e.g. POST /rpc/<name>).
	CallModePost CallMode = iota
	// CallModeGet indicates a GET routine call (e.g. GET /rpc/<name>).
	CallModeGet
)

// RepresentationConstraint specifies representation constraints on the RPC call.
type RepresentationConstraint int

const (
	// RepresentationDefault indicates default JSON representation.
	RepresentationDefault RepresentationConstraint = iota
	// RepresentationSingularObject indicates application/vnd.pgrst.object+json.
	RepresentationSingularObject
)

// ResultKind identifies the kind of result produced by a routine.
type ResultKind int

const (
	// ResultKindScalar indicates a scalar result from a SQL function.
	ResultKindScalar ResultKind = iota
	// ResultKindObject indicates output parameter values from a stored procedure without a result set.
	ResultKindObject
	// ResultKindRowSet indicates a tabular result set from a stored procedure.
	ResultKindRowSet
)

// TxOutcome records whether the transaction was committed and whether Prefer: tx was applied.
type TxOutcome struct {
	Committed     bool
	PreferApplied bool
}

// Intent specifies everything needed to execute a routine.
type Intent struct {
	Routine        schemacache.RoutineFact
	Role           schemacache.Role
	Args           map[string]any
	CallMode       CallMode
	PreferTx       string
	Representation RepresentationConstraint
	Query          readquery.Query
}

// Outcome represents the result of executing a routine.
type Outcome struct {
	Kind      ResultKind
	Data      any
	Rows      []rows.Row
	TxOutcome TxOutcome
}

// CallOptions carries Prefer-driven RPC behaviour into the database layer.
type CallOptions struct {
	// PreferTx is Prefer: tx=commit|rollback when the client sent it.
	PreferTx string
	// Validate runs inside the routine unit before commit, so a refused
	// representation rolls the unit back (issue #175). A nil Validate
	// validates nothing.
	Validate func(any) error
}

// Caller runs a routine as one database role with named JSON arguments.
type Caller interface {
	Call(
		ctx context.Context,
		role schemacache.Role,
		routine schemacache.RoutineFact,
		args map[string]any,
		options CallOptions,
	) (any, error)
}

// Executor executes a routine intent and yields a typed outcome.
type Executor interface {
	Execute(ctx context.Context, intent Intent) (Outcome, error)
}

// SignatureMismatch reports a missing IN/INOUT argument or an unknown argument.
type SignatureMismatch struct {
	Routine schemacache.RoutineID
	Missing string
	Unknown string
}

func (e SignatureMismatch) Error() string {
	if e.Missing != "" {
		return fmt.Sprintf("missing required argument %q for %s.%s", e.Missing, e.Routine.Database, e.Routine.Name)
	}
	if e.Unknown != "" {
		return fmt.Sprintf("unknown argument %q for %s.%s", e.Unknown, e.Routine.Database, e.Routine.Name)
	}
	return fmt.Sprintf("signature mismatch for %s.%s", e.Routine.Database, e.Routine.Name)
}

// ReadSafetyViolation reports a GET call on a non-read-safe routine.
type ReadSafetyViolation struct{}

func (ReadSafetyViolation) Error() string {
	return "Only a read-safe routine can be called with GET"
}

// SingularObjectRefusal reports that a singular JSON object was requested
// but the tabular result set contained zero or multiple rows.
type SingularObjectRefusal struct {
	RowCount int
}

func (e SingularObjectRefusal) Error() string {
	return fmt.Sprintf("The result contains %d rows, while 1 was expected", e.RowCount)
}

// RowSetFeaturesRefusal reports that read query features (filter, order, range, embed)
// were supplied for a non-tabular routine result.
type RowSetFeaturesRefusal struct{}

func (RowSetFeaturesRefusal) Error() string {
	return "Filter, order, pagination, and embed need a row-set RPC result"
}

