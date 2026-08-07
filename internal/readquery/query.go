// Package readquery holds the PostgREST-shaped ordinary-read query: column
// select, full-match filters, order, page, and exact count. HTTP and SQL stay
// outside this package.
package readquery

import "github.com/jonbaldie/myrest/internal/rows"

// Query is one ordinary read of a resource.
type Query struct {
	// Columns are the selected columns. Empty means every column of the
	// resource, in catalog order.
	Columns []Column
	// Filters are AND conditions on columns.
	Filters []Filter
	// Groups hold top-level or=(...) and and=(...) trees.
	Groups []Group
	// Order sorts the rows.
	Order []Order
	// Limit caps how many rows to return. Nil means no client limit.
	Limit *uint64
	// Offset skips that many matching rows.
	Offset uint64
	// ExactCount asks for the total matching row count.
	ExactCount bool
	// MaxRows is the hard row cap from db-max-rows. Nil means no hard cap.
	MaxRows *uint64
}

// Result is the answer of one ordinary read.
type Result struct {
	Rows  []rows.Row
	Total *int64
}

// ColumnNotFound is a select, filter, or order column the resource does not hold.
type ColumnNotFound struct {
	Name string
}

func (e ColumnNotFound) Error() string {
	return "column not found: " + e.Name
}

// Column is one selected column, optionally renamed.
type Column struct {
	Name  string
	Alias string
}

// Operator is a full-match filter operator of this ticket.
type Operator string

// Full-match filter operators claimed by the ordinary-read ticket.
const (
	OpEq         Operator = "eq"
	OpNeq        Operator = "neq"
	OpGt         Operator = "gt"
	OpGte        Operator = "gte"
	OpLt         Operator = "lt"
	OpLte        Operator = "lte"
	OpLike       Operator = "like"
	OpIn         Operator = "in"
	OpIs         Operator = "is"
	OpIsDistinct Operator = "isdistinct"
)

// FullMatchOperators lists every filter operator this ticket claims as a
// full match. Ticket #28 owns text-case, JSON path, FTS, and array/range.
var FullMatchOperators = []Operator{
	OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpLike, OpIn, OpIs, OpIsDistinct,
}

// Filter is one column comparison.
type Filter struct {
	Column  string
	Op      Operator
	Value   string
	Values  []string // for in.(...)
	Negated bool
}

// Group is a logical and/or tree of filters.
type Group struct {
	Or      bool
	Filters []Filter
	Groups  []Group
	Negated bool
}

// Order is one sort key. Ascending is the default.
type Order struct {
	Column string
	Desc   bool
}

// EffectiveLimit is the row cap after db-max-rows. Nil means no cap.
func (q Query) EffectiveLimit() *uint64 {
	switch {
	case q.Limit == nil && q.MaxRows == nil:
		return nil
	case q.Limit == nil:
		return q.MaxRows
	case q.MaxRows == nil:
		return q.Limit
	case *q.Limit < *q.MaxRows:
		return q.Limit
	default:
		return q.MaxRows
	}
}
