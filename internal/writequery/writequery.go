// Package writequery holds the options and result of an ordinary table write.
// HTTP and SQL stay outside this package.
package writequery

import "github.com/jonbaldie/myrest/internal/rows"

// Options carries Prefer-driven write behaviour into the database layer.
type Options struct {
	// PrimaryKey is the PRIMARY KEY column list in key order. Empty when the
	// table has no primary key.
	PrimaryKey []string
	// ReturnRepresentation asks for the affected rows after the write.
	ReturnRepresentation bool
	// ReturnKeys asks for primary-key values of inserted rows (Location).
	ReturnKeys bool
	// MissingDefault uses SQL DEFAULT for columns omitted from an insert row.
	MissingDefault bool
	// MaxAffected, when set, refuses the write when more rows would change.
	MaxAffected *int64
	// PreferTx is Prefer: tx=commit|rollback when the client sent it.
	PreferTx string
}

// Result is what a write produced for the HTTP response.
type Result struct {
	Affected int64
	Rows     []rows.Row
	// Keys holds one primary-key map per inserted or updated row when the
	// options asked for keys.
	Keys []map[string]any
}

// MaxAffectedExceeded is Prefer max-affected under handling=strict.
type MaxAffectedExceeded struct {
	Affected int64
	Max      int64
}

func (e MaxAffectedExceeded) Error() string {
	return "Query result exceeds max-affected preference constraint"
}
