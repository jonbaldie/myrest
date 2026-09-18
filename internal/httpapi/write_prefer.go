package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
)

// Write preference tokens claimed by this service for ordinary writes.
const (
	returnMinimal        = "minimal"
	returnHeadersOnly    = "headers-only"
	returnRepresentation = "representation"

	// codeInvalidPrefer is Prefer handling=strict with an invalid token.
	codeInvalidPrefer = "PGRST122"
	// codeMaxAffected is Prefer max-affected under handling=strict.
	codeMaxAffected = "PGRST124"
)

// writePrefer is the Prefer control surface for ordinary writes.
type writePrefer struct {
	Return         string
	MissingDefault bool
	MaxAffected    *int64
	Strict         bool
	AllRows        bool
	// Tx is Prefer: tx=commit|rollback when the client sent a valid value.
	Tx string
	// applied lists Preference-Applied tokens in stable order.
	applied []string
}

// knownPreferNames are Prefer names myrest recognises on a write. Unknown
// names are invalid under handling=strict. Prefer timezone is refused before
// this map runs (see preferAsksForTimezone).
var knownPreferNames = map[string]bool{
	"return":       true,
	"missing":      true,
	"max-affected": true,
	"handling":     true,
	"all-rows":     true,
	"count":        true,
	"resolution":   true,
	"tx":           true,
}

type preferTokens struct {
	values  map[string]string
	allRows bool
	invalid []string
}

func parseWritePrefer(headers []string, txEnd config.TxEnd, kind writeKind) (writePrefer, error) {
	tokens := collectPreferTokens(headers)
	prefer, invalid := applyPreferTokens(tokens)
	if prefer.Strict && len(invalid) > 0 {
		return writePrefer{}, invalidPreferError{tokens: invalid}
	}
	prefer.applied = preferenceApplied(prefer, tokens, txEnd, kind)
	return prefer, nil
}

func collectPreferTokens(headers []string) preferTokens {
	tokens := preferTokens{values: make(map[string]string)}
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			token := strings.TrimSpace(part)
			if token == "" {
				continue
			}
			name, value, hasValue := strings.Cut(token, "=")
			name = strings.ToLower(strings.TrimSpace(name))
			value = strings.TrimSpace(value)
			if !knownPreferNames[name] {
				tokens.invalid = append(tokens.invalid, token)
				continue
			}
			collectKnownToken(&tokens, name, value, hasValue, token)
		}
	}
	return tokens
}

func collectKnownToken(tokens *preferTokens, name, value string, hasValue bool, raw string) {
	if name == "all-rows" {
		// Only the bare flag unlocks an all-rows write. A valued form
		// (all-rows=false, all-rows=true, all-rows=) is not the flag, so it
		// never sets the option and is invalid under handling=strict.
		if hasValue {
			tokens.invalid = append(tokens.invalid, raw)
			return
		}
		tokens.allRows = true
		return
	}
	if !hasValue || value == "" {
		tokens.invalid = append(tokens.invalid, raw)
		return
	}
	tokens.values[name] = value
}

func applyPreferTokens(tokens preferTokens) (writePrefer, []string) {
	prefer := writePrefer{Return: returnMinimal, AllRows: tokens.allRows}
	invalid := append([]string(nil), tokens.invalid...)
	invalid = append(invalid, applyHandling(&prefer, tokens)...)
	invalid = append(invalid, applyReturn(&prefer, tokens)...)
	invalid = append(invalid, applyMissing(&prefer, tokens)...)
	invalid = append(invalid, applyMaxAffected(&prefer, tokens)...)
	invalid = append(invalid, applyTx(&prefer, tokens)...)
	invalid = append(invalid, applyCount(&prefer, tokens)...)
	invalid = append(invalid, applyResolution(&prefer, tokens)...)
	return prefer, invalid
}

func applyCount(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["count"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	switch lower {
	case "exact":
		return nil
	default:
		return []string{"count=" + lower}
	}
}

func applyResolution(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["resolution"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	switch lower {
	case "merge-duplicates", "ignore-duplicates":
		return nil
	default:
		return []string{"resolution=" + lower}
	}
}

func applyTx(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["tx"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	switch lower {
	case config.PreferTxCommit, config.PreferTxRollback:
		prefer.Tx = lower
		return nil
	default:
		return []string{"tx=" + lower}
	}
}

func applyHandling(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["handling"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	switch lower {
	case "strict":
		prefer.Strict = true
	case "lenient":
		prefer.Strict = false
	default:
		return []string{"handling=" + lower}
	}
	return nil
}

func applyReturn(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["return"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	switch lower {
	case returnMinimal, returnHeadersOnly, returnRepresentation:
		prefer.Return = lower
		return nil
	default:
		return []string{"return=" + lower}
	}
}

func applyMissing(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["missing"]
	if !ok {
		return nil
	}
	lower := strings.ToLower(val)
	if lower == "default" {
		prefer.MissingDefault = true
		return nil
	}
	return []string{"missing=" + lower}
}

func applyMaxAffected(prefer *writePrefer, tokens preferTokens) []string {
	val, ok := tokens.values["max-affected"]
	if !ok {
		return nil
	}
	maxValue, err := strconv.ParseInt(val, 10, 64)
	if err != nil || maxValue < 0 {
		return []string{"max-affected=" + val}
	}
	prefer.MaxAffected = &maxValue
	return nil
}

func preferenceApplied(prefer writePrefer, tokens preferTokens, txEnd config.TxEnd, kind writeKind) []string {
	var applied []string
	if prefer.Strict {
		applied = append(applied, "handling=strict")
	}
	if val, ok := tokens.values["return"]; ok && prefer.Return == strings.ToLower(val) {
		applied = append(applied, "return="+prefer.Return)
	}
	// missing=default changes omitted columns only for inserts.
	if shouldApplyMissingDefault(prefer, kind) {
		applied = append(applied, "missing=default")
	}
	// max-affected is an update, delete, and upsert preference. Inserts
	// write normally, so they must not echo the limit as applied.
	if prefer.Strict && prefer.MaxAffected != nil && honoursMaxAffected(kind) {
		applied = append(
			applied,
			"max-affected="+strconv.FormatInt(*prefer.MaxAffected, 10),
		)
	}
	if _, txApplied := config.DecideTxEnd(txEnd, prefer.Tx); txApplied {
		applied = append(applied, "tx="+prefer.Tx)
	}
	return applied
}

func shouldApplyMissingDefault(prefer writePrefer, kind writeKind) bool {
	if !prefer.MissingDefault {
		return false
	}
	return kind == writeKindInsert
}

type invalidPreferError struct {
	tokens []string
}

func (e invalidPreferError) Error() string {
	return "Invalid preferences given with handling=strict"
}

func (e invalidPreferError) details() string {
	return "Invalid preferences: " + strings.Join(e.tokens, ", ")
}

func writeInvalidPrefer(writer http.ResponseWriter, err invalidPreferError) {
	writeFailureExtra(
		writer,
		http.StatusBadRequest,
		codeInvalidPrefer,
		err.Error(),
		err.details(),
		nil,
	)
}

// maxAffectedError is Prefer max-affected under handling=strict.
type maxAffectedError struct {
	Affected int64
	Max      int64
}

func (e maxAffectedError) Error() string {
	return "Query result exceeds max-affected preference constraint"
}

func (e maxAffectedError) details() string {
	return fmt.Sprintf("The query affects %d rows", e.Affected)
}

func writeMaxAffected(writer http.ResponseWriter, err maxAffectedError) {
	writeFailureExtra(
		writer,
		http.StatusBadRequest,
		codeMaxAffected,
		err.Error(),
		err.details(),
		nil,
	)
}

func setPreferenceApplied(writer http.ResponseWriter, prefer writePrefer) {
	if len(prefer.applied) == 0 {
		return
	}
	writer.Header().Set("Preference-Applied", strings.Join(prefer.applied, ", "))
}

// setTxPreferenceApplied sets Preference-Applied only for an applied Prefer: tx=.
func setTxPreferenceApplied(writer http.ResponseWriter, preferTx string, txEnd config.TxEnd) {
	if _, applied := config.DecideTxEnd(txEnd, preferTx); !applied {
		return
	}
	writer.Header().Set("Preference-Applied", "tx="+preferTx)
}
