package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/prefer"
)

// Write preference tokens claimed by this service for ordinary writes.
const (
	returnMinimal        = prefer.ReturnMinimal
	returnHeadersOnly    = prefer.ReturnHeadersOnly
	returnRepresentation = prefer.ReturnRepresentation

	// codeInvalidPrefer is Prefer handling=strict with an invalid token.
	codeInvalidPrefer = "PGRST122"
	// codeMaxAffected is Prefer max-affected under handling=strict.
	codeMaxAffected = "PGRST124"
)

// writePrefer is the Prefer control surface for ordinary writes and RPC: the
// parsed preferences of the request and the Preference-Applied tokens of the
// write kind.
type writePrefer struct {
	prefer.Preferences
	// applied lists Preference-Applied tokens in stable order.
	applied []string
}

// newWritePrefer works out which preferences the write kind applies. The
// caller has already refused invalid preferences (refuseInvalidPrefer).
func newWritePrefer(preferences prefer.Preferences, txEnd config.TxEnd, kind writeKind) writePrefer {
	applied := preferenceApplied(preferences, txEnd, kind)
	if preferences.Return == "" {
		preferences.Return = returnMinimal
	}
	return writePrefer{Preferences: preferences, applied: applied}
}

func preferenceApplied(preferences prefer.Preferences, txEnd config.TxEnd, kind writeKind) []string {
	var applied []string
	if preferences.Strict {
		applied = append(applied, "handling=strict")
	}
	if preferences.Return != "" {
		applied = append(applied, "return="+preferences.Return)
	}
	// missing=default changes omitted columns only for inserts.
	if preferences.MissingDefault && kind == writeKindInsert {
		applied = append(applied, "missing=default")
	}
	// max-affected is an update, delete, and upsert preference. Inserts
	// write normally, so they must not echo the limit as applied.
	if preferences.Strict && preferences.MaxAffected != nil && honoursMaxAffected(kind) {
		applied = append(
			applied,
			"max-affected="+strconv.FormatInt(*preferences.MaxAffected, 10),
		)
	}
	if _, txApplied := config.DecideTxEnd(txEnd, preferences.Tx); txApplied {
		applied = append(applied, "tx="+preferences.Tx)
	}
	return applied
}

// refuseInvalidPrefer answers PGRST122 when handling=strict meets an invalid
// token on the surface.
func refuseInvalidPrefer(writer http.ResponseWriter, preferences prefer.Preferences, surface prefer.Surface) bool {
	var invalid prefer.InvalidError
	if !errors.As(preferences.Refusal(surface), &invalid) {
		return false
	}
	writeInvalidPrefer(writer, invalid)
	return true
}

func writeInvalidPrefer(writer http.ResponseWriter, err prefer.InvalidError) {
	writeFailureExtra(
		writer,
		http.StatusBadRequest,
		codeInvalidPrefer,
		err.Error(),
		err.Details(),
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
