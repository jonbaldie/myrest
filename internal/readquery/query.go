// Package readquery holds the PostgREST-shaped ordinary-read query: column
// select, full-match and named partial-match filters, JSON path fields, order,
// page, and exact count. HTTP and SQL stay outside this package.
package readquery

import "github.com/jonbaldie/myrest/internal/rows"

// Query is one ordinary read of a resource.
type Query struct {
	// Columns are the selected columns. Empty means every column of the
	// resource, in catalog order.
	Columns []Column
	// Embeds are nested related resources asked for in select.
	Embeds []Embed
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
	// SelectAll is true when the client omitted select or used *.
	SelectAll bool
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

// Column is one selected column, optionally renamed, optionally a JSON path.
type Column struct {
	Name  string
	Alias string
	Path  *JSONPath
}

// Embed is one nested resource asked for in select.
type Embed struct {
	// Resource is the related table name the client asked for.
	Resource string
	// Alias is the JSON key when the client renamed the embed. Empty means
	// the resource name.
	Alias string
	// Hint picks one relationship when more than one applies (!fk_name or
	// !column).
	Hint string
	// Columns of the nested resource. Empty means every column.
	Columns []Column
	// Embeds are nested embeds inside this one.
	Embeds []Embed
	// Filters, Groups, Order, Limit, and Offset shape the nested rows.
	Filters []Filter
	Groups  []Group
	Order   []Order
	Limit   *uint64
	Offset  uint64
}

// Key is the JSON object key of the embed.
func (e Embed) Key() string {
	if e.Alias != "" {
		return e.Alias
	}
	return e.Resource
}

// Operator is a filter operator of an ordinary read.
type Operator string

// Filter operators claimed as full match, plus the text-case partial match.
const (
	OpEq         Operator = "eq"
	OpNeq        Operator = "neq"
	OpGt         Operator = "gt"
	OpGte        Operator = "gte"
	OpLt         Operator = "lt"
	OpLte        Operator = "lte"
	OpLike       Operator = "like"
	OpILike      Operator = "ilike"
	OpIn         Operator = "in"
	OpIs         Operator = "is"
	OpIsDistinct Operator = "isdistinct"
)

// FullMatchOperators lists every filter operator the ordinary-read ticket
// claims as a full match.
var FullMatchOperators = []Operator{
	OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpLike, OpIn, OpIs, OpIsDistinct,
}

// PartialMatchOperators lists filter operators claimed as a named MySQL
// partial match (see docs/read-parity-boundaries.md).
var PartialMatchOperators = []Operator{
	OpILike,
}

// Filter is one column comparison, optionally on a JSON path.
type Filter struct {
	Column  string
	Path    *JSONPath
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

// Order is one sort key. Ascending is the default. Path holds a JSON path when
// the client orders by an extracted JSON value.
type Order struct {
	Column string
	Path   *JSONPath
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
