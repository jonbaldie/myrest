// Package prefer parses the Prefer request header once per request. Every
// request surface (table reads, RPC, and writes) reads the same parsed value,
// so one list of known tokens and one handling=strict rule apply everywhere.
package prefer

import (
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/readquery"
)

// Prefer return values of the parity target.
const (
	ReturnMinimal        = "minimal"
	ReturnHeadersOnly    = "headers-only"
	ReturnRepresentation = "representation"
)

// Prefer resolution values of the parity target.
const (
	ResolutionMergeDuplicates  = "merge-duplicates"
	ResolutionIgnoreDuplicates = "ignore-duplicates"
)

// Surface is the kind of request that reads the preferences. The strict rule
// can name different invalid tokens on different surfaces.
type Surface int

const (
	// SurfaceRead is GET and HEAD on a table.
	SurfaceRead Surface = iota
	// SurfaceRPC is GET and POST on /rpc.
	SurfaceRPC
	// SurfaceWrite is POST, PATCH, PUT, and DELETE on a table.
	SurfaceWrite
)

// Preferences is the parsed Prefer header of one request. A field holds a
// value only when the client sent a valid token for it. When a token occurs
// more than once, the last one wins.
type Preferences struct {
	// Strict is Prefer: handling=strict.
	Strict bool
	// Count is the Prefer count value.
	Count readquery.CountMode
	// Return is one of the Return* values, or "" when absent or invalid.
	Return string
	// MissingDefault is Prefer: missing=default.
	MissingDefault bool
	// MaxAffected is a valid Prefer max-affected value.
	MaxAffected *int64
	// Tx is config.PreferTxCommit or config.PreferTxRollback, or "".
	Tx string
	// Resolution is one of the Resolution* values, or "" when absent or invalid.
	Resolution string
	// BadResolution is true when a non-empty resolution value is not valid.
	BadResolution bool
	// AllRows is the bare Prefer: all-rows flag.
	AllRows bool

	// RowSecurity, JWTClaims, and Timezone ask for Postgres-only features.
	// They are known tokens: the auth check refuses them with a stable
	// message, not the strict rule.
	RowSecurity bool
	JWTClaims   bool
	Timezone    bool

	invalid []invalidToken
}

// invalidToken is one token that handling=strict refuses. writeOnly marks a
// token that only the write surface refuses; reads and RPC refuse it another
// way.
type invalidToken struct {
	raw       string
	writeOnly bool
}

// knownNames are the Prefer names myrest recognises. Other names are invalid
// under handling=strict.
var knownNames = map[string]bool{
	"return":       true,
	"missing":      true,
	"max-affected": true,
	"handling":     true,
	"all-rows":     true,
	"count":        true,
	"resolution":   true,
	"tx":           true,
	"row-security": true,
	"jwt-claims":   true,
	"timezone":     true,
}

// tokens holds the last value of each known name, before validation.
type tokens struct {
	values map[string]string
	// general holds unknown and empty-valued tokens in header order.
	general []string
	allRows bool
}

// Parse reads every Prefer header value of one request.
func Parse(headers []string) Preferences {
	collected := collect(headers)
	var preferences Preferences
	for _, raw := range collected.general {
		preferences.invalid = append(preferences.invalid, invalidToken{raw: raw})
	}
	preferences.AllRows = collected.allRows
	_, preferences.RowSecurity = collected.values["row-security"]
	_, preferences.JWTClaims = collected.values["jwt-claims"]
	_, preferences.Timezone = collected.values["timezone"]
	// The rule order keeps the PGRST122 details stable.
	for _, rule := range rules {
		value, held := collected.values[rule.name]
		if !held {
			continue
		}
		if invalid, bad := rule.apply(&preferences, value); bad {
			preferences.invalid = append(preferences.invalid, invalid)
		}
	}
	return preferences
}

func collect(headers []string) tokens {
	collected := tokens{values: map[string]string{}}
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			token := strings.TrimSpace(part)
			if token == "" {
				continue
			}
			name, value, hasValue := strings.Cut(token, "=")
			name = strings.ToLower(strings.TrimSpace(name))
			value = strings.TrimSpace(value)
			collectToken(&collected, name, value, hasValue, token)
		}
	}
	return collected
}

func collectToken(collected *tokens, name, value string, hasValue bool, raw string) {
	switch {
	case !knownNames[name]:
		collected.general = append(collected.general, raw)
	case name == "row-security" || name == "jwt-claims" || name == "timezone":
		collected.values[name] = value
	case name == "all-rows":
		// Only the bare flag unlocks an all-rows write. A valued form
		// (all-rows=false, all-rows=true, all-rows=) is not the flag, so it
		// never sets the option and is invalid under handling=strict.
		if hasValue {
			collected.general = append(collected.general, raw)
			return
		}
		collected.allRows = true
	case !hasValue || value == "":
		collected.general = append(collected.general, raw)
	default:
		collected.values[name] = strings.ToLower(value)
	}
}

// rule validates the last value of one known name.
type rule struct {
	name  string
	apply func(preferences *Preferences, value string) (invalidToken, bool)
}

var rules = []rule{
	{name: "handling", apply: applyHandling},
	{name: "return", apply: applyReturn},
	{name: "missing", apply: applyMissing},
	{name: "max-affected", apply: applyMaxAffected},
	{name: "tx", apply: applyTx},
	{name: "count", apply: applyCount},
	{name: "resolution", apply: applyResolution},
}

func refused(name, value string) (invalidToken, bool) {
	return invalidToken{raw: name + "=" + value}, true
}

func applyHandling(preferences *Preferences, value string) (invalidToken, bool) {
	switch value {
	case "strict":
		preferences.Strict = true
	case "lenient":
		preferences.Strict = false
	default:
		return refused("handling", value)
	}
	return invalidToken{}, false
}

func applyReturn(preferences *Preferences, value string) (invalidToken, bool) {
	switch value {
	case ReturnMinimal, ReturnHeadersOnly, ReturnRepresentation:
		preferences.Return = value
		return invalidToken{}, false
	default:
		return refused("return", value)
	}
}

func applyMissing(preferences *Preferences, value string) (invalidToken, bool) {
	if value != "default" {
		return refused("missing", value)
	}
	preferences.MissingDefault = true
	return invalidToken{}, false
}

func applyMaxAffected(preferences *Preferences, value string) (invalidToken, bool) {
	maxValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil || maxValue < 0 {
		return refused("max-affected", value)
	}
	preferences.MaxAffected = &maxValue
	return invalidToken{}, false
}

func applyTx(preferences *Preferences, value string) (invalidToken, bool) {
	switch value {
	case config.PreferTxCommit, config.PreferTxRollback:
		preferences.Tx = value
		return invalidToken{}, false
	default:
		return refused("tx", value)
	}
}

// applyCount keeps count=planned and count=estimated for reads and RPC, which
// refuse them as a MySQL gap. Writes do not count, so there they are invalid.
func applyCount(preferences *Preferences, value string) (invalidToken, bool) {
	switch value {
	case "exact":
		preferences.Count = readquery.CountExact
		return invalidToken{}, false
	case "planned":
		preferences.Count = readquery.CountPlanned
		return invalidToken{raw: "count=planned", writeOnly: true}, true
	case "estimated":
		preferences.Count = readquery.CountEstimated
		return invalidToken{raw: "count=estimated", writeOnly: true}, true
	default:
		return refused("count", value)
	}
}

func applyResolution(preferences *Preferences, value string) (invalidToken, bool) {
	switch value {
	case ResolutionMergeDuplicates, ResolutionIgnoreDuplicates:
		preferences.Resolution = value
		return invalidToken{}, false
	default:
		preferences.BadResolution = true
		return refused("resolution", value)
	}
}

// Invalid lists the tokens that handling=strict refuses on the surface.
func (p Preferences) Invalid(surface Surface) []string {
	var invalid []string
	for _, token := range p.invalid {
		if token.writeOnly && surface != SurfaceWrite {
			continue
		}
		invalid = append(invalid, token.raw)
	}
	return invalid
}

// Refusal is an InvalidError when handling=strict is set and the surface has
// an invalid token, and nil otherwise.
func (p Preferences) Refusal(surface Surface) error {
	if !p.Strict {
		return nil
	}
	invalid := p.Invalid(surface)
	if len(invalid) == 0 {
		return nil
	}
	return InvalidError{Tokens: invalid}
}

// InvalidError is PGRST122: Prefer handling=strict with invalid tokens.
type InvalidError struct {
	Tokens []string
}

func (e InvalidError) Error() string {
	return "Invalid preferences given with handling=strict"
}

// Details names the invalid tokens for the error envelope.
func (e InvalidError) Details() string {
	return "Invalid preferences: " + strings.Join(e.Tokens, ", ")
}
