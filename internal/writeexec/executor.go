package writeexec

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/readquery"
	"github.com/jonbaldie/myrest/internal/representation"
	"github.com/jonbaldie/myrest/internal/rows"
	"github.com/jonbaldie/myrest/internal/schemacache"
	"github.com/jonbaldie/myrest/internal/writequery"
)

type Action int

const (
	Insert Action = iota
	Update
	Delete
	Upsert
)

type Intent struct {
	Action         Action
	Role           schemacache.Role
	Target         schemacache.Table
	Payload        []map[string]any
	Filter         readquery.Query
	Preferences    prefer.Preferences
	PrimaryKey     []string
	CanSelect      bool
	Representation representation.Spec
}

type Outcome struct {
	Affected int64
	Rows     []rows.Row
	Keys     []map[string]any
	Applied  []string
	Inserted bool
}

type Executor interface {
	Execute(context.Context, Intent) (Outcome, error)
}

type Transaction interface {
	Mutate(context.Context, Intent, writequery.Options) (Outcome, error)
	ReadDelete(context.Context, Intent) ([]rows.Row, error)
}
type Database interface {
	WithinTransaction(context.Context, schemacache.Role, string, func(context.Context, Transaction) error) error
}
type Service struct {
	db    Database
	txEnd config.TxEnd
}

func New(db Database, txEnd config.TxEnd) *Service { return &Service{db: db, txEnd: txEnd} }

type Unbounded struct{}

func (Unbounded) Error() string { return "PATCH/DELETE needs a filter or Prefer: all-rows" }

type BadResolution struct{}

func (BadResolution) Error() string {
	return "Prefer resolution must be merge-duplicates or ignore-duplicates"
}

func (s *Service) Execute(ctx context.Context, in Intent) (Outcome, error) {
	if err := checkIntent(in); err != nil {
		return Outcome{}, err
	}
	opt := writeOptions(in)
	var outcome Outcome
	err := s.db.WithinTransaction(ctx, in.Role, in.Preferences.Tx, func(ctx context.Context, tx Transaction) error {
		var err error
		outcome, err = executeTransaction(ctx, tx, in, opt)
		return err
	})
	if err != nil {
		return Outcome{}, err
	}
	outcome.Applied = applied(in.Preferences, s.txEnd, in.Action)
	return outcome, nil
}

func checkIntent(in Intent) error {
	p := in.Preferences
	if err := p.Refusal(prefer.SurfaceWrite); err != nil {
		return err
	}
	if in.Action < Insert || in.Action > Upsert {
		return fmt.Errorf("unknown write action %d", in.Action)
	}
	if err := checkAction(in); err != nil {
		return err
	}
	return CheckRepresentation(in)
}

func checkAction(in Intent) error {
	switch in.Action {
	case Update, Delete:
		if UnboundedFilter(in.Filter) && !in.Preferences.AllRows {
			return Unbounded{}
		}
	}
	switch in.Action {
	case Insert, Update, Upsert:
		if len(in.Payload) == 0 {
			return fmt.Errorf("write action %d needs a payload", in.Action)
		}
	}
	if (in.Action == Insert || in.Action == Upsert) && in.Preferences.BadResolution {
		return BadResolution{}
	}
	return nil
}

func writeOptions(in Intent) writequery.Options {
	p := in.Preferences
	opt := writequery.Options{PrimaryKey: in.PrimaryKey, MissingDefault: p.MissingDefault, PreferTx: p.Tx}
	if p.Strict && in.Action != Insert {
		opt.MaxAffected = p.MaxAffected
	}
	if p.Return == prefer.ReturnHeadersOnly {
		opt.ReturnKeys = true
	}
	if p.Return == prefer.ReturnRepresentation {
		opt.ReturnRepresentation, opt.ReturnKeys = true, true
	}
	if p.Resolution == prefer.ResolutionIgnoreDuplicates {
		opt.OnDuplicate = writequery.DuplicateIgnored
	} else if p.Resolution == prefer.ResolutionMergeDuplicates || in.Action == Upsert {
		opt.OnDuplicate = writequery.DuplicateMerges
	}
	return opt
}

func executeTransaction(ctx context.Context, tx Transaction, in Intent, opt writequery.Options) (Outcome, error) {
	var before []rows.Row
	if in.Action == Delete && opt.ReturnRepresentation {
		var err error
		before, err = tx.ReadDelete(ctx, in)
		if err != nil {
			return Outcome{}, err
		}
		if err = validate(in, before); err != nil {
			return Outcome{}, err
		}
		opt.ReturnRepresentation = false
	}
	outcome, err := tx.Mutate(ctx, in, opt)
	if err != nil {
		return Outcome{}, err
	}
	if in.Action == Delete && in.Preferences.Return == prefer.ReturnRepresentation {
		outcome.Rows = before
	}
	if opt.ReturnRepresentation {
		return outcome, validate(in, outcome.Rows)
	}
	return outcome, nil
}

// UnboundedFilter reports whether a query has no filter.
func UnboundedFilter(query readquery.Query) bool {
	return len(query.Filters) == 0 && len(query.Groups) == 0
}

func CheckRepresentation(in Intent) error {
	p := in.Preferences
	if p.Return != prefer.ReturnRepresentation {
		return nil
	}
	msg := representationProblem(in)
	if msg != "" {
		return readquery.UnsupportedFeature{Message: msg}
	}
	return nil
}

func representationProblem(in Intent) string {
	p := in.Preferences
	switch {
	case in.Action == Upsert:
		return "Prefer return=representation cannot return affected rows honestly"
	case len(in.PrimaryKey) == 0 && (in.Action == Insert || in.Action == Update):
		return missingKeyMessage(in.Action)
	case !in.CanSelect:
		return "Prefer return=representation needs SELECT to return affected rows honestly"
	case in.Action == Insert && p.Resolution == prefer.ResolutionIgnoreDuplicates:
		return "Prefer return=representation cannot tell inserted rows from ignored rows honestly"
	}
	return ""
}

func missingKeyMessage(action Action) string {
	verb := "inserted"
	if action == Update {
		verb = "updated"
	}
	return "Prefer return=representation needs a primary key to return " + verb + " rows honestly"
}

func validate(in Intent, result []rows.Row) error {
	if err := representation.ValidateCardinality(in.Representation, len(result)); err != nil {
		return err
	}
	_, err := readquery.Project(result, in.Filter)
	return err
}
func applied(p prefer.Preferences, txEnd config.TxEnd, action Action) []string {
	var a []string
	if p.Strict {
		a = append(a, "handling=strict")
	}
	if p.Return != "" {
		a = append(a, "return="+p.Return)
	}
	a = appendActionPreferences(a, p, action)
	if _, ok := config.DecideTxEnd(txEnd, p.Tx); ok {
		a = append(a, "tx="+p.Tx)
	}
	if (action == Insert || action == Upsert) && p.Resolution != "" {
		a = append(a, "resolution="+p.Resolution)
	}
	return a
}

func appendActionPreferences(applied []string, p prefer.Preferences, action Action) []string {
	if p.MissingDefault && action == Insert {
		applied = append(applied, "missing=default")
	}
	if p.Strict && p.MaxAffected != nil && action != Insert {
		applied = append(applied, "max-affected="+strconv.FormatInt(*p.MaxAffected, 10))
	}
	return applied
}
