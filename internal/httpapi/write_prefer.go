package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	// applied lists Preference-Applied tokens in stable order.
	applied []string
}

// knownPreferNames are Prefer names myrest recognises on a write. Unknown
// names are invalid under handling=strict. Deferred Prefer values stay in
// the known set so a strict client does not fail on them until their ticket.
var knownPreferNames = map[string]bool{
	"return":       true,
	"missing":      true,
	"max-affected": true,
	"handling":     true,
	"all-rows":     true,
	"count":        true,
	"resolution":   true,
	"tx":           true,
	"timezone":     true,
}

type preferTokens struct {
	returnValue   string
	returnSet     bool
	missingValue  string
	missingSet    bool
	maxRaw        string
	maxSet        bool
	handlingValue string
	handlingSet   bool
	allRows       bool
	invalid       []string
}

func parseWritePrefer(headers []string) (writePrefer, error) {
	tokens := collectPreferTokens(headers)
	prefer, invalid := applyPreferTokens(tokens)
	if prefer.Strict && len(invalid) > 0 {
		return writePrefer{}, invalidPreferError{tokens: invalid}
	}
	prefer.applied = preferenceApplied(prefer, tokens)
	return prefer, nil
}

func collectPreferTokens(headers []string) preferTokens {
	var tokens preferTokens
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
		tokens.allRows = true
		return
	}
	if !hasValue || value == "" {
		tokens.invalid = append(tokens.invalid, raw)
		return
	}
	switch name {
	case "return":
		tokens.returnValue = strings.ToLower(value)
		tokens.returnSet = true
	case "missing":
		tokens.missingValue = strings.ToLower(value)
		tokens.missingSet = true
	case "max-affected":
		tokens.maxRaw = value
		tokens.maxSet = true
	case "handling":
		tokens.handlingValue = strings.ToLower(value)
		tokens.handlingSet = true
	}
}

func applyPreferTokens(tokens preferTokens) (writePrefer, []string) {
	prefer := writePrefer{Return: returnMinimal, AllRows: tokens.allRows}
	invalid := append([]string(nil), tokens.invalid...)
	invalid = append(invalid, applyHandling(&prefer, tokens)...)
	invalid = append(invalid, applyReturn(&prefer, tokens)...)
	invalid = append(invalid, applyMissing(&prefer, tokens)...)
	invalid = append(invalid, applyMaxAffected(&prefer, tokens)...)
	return prefer, invalid
}

func applyHandling(prefer *writePrefer, tokens preferTokens) []string {
	if !tokens.handlingSet {
		return nil
	}
	switch tokens.handlingValue {
	case "strict":
		prefer.Strict = true
	case "lenient":
		prefer.Strict = false
	default:
		return []string{"handling=" + tokens.handlingValue}
	}
	return nil
}

func applyReturn(prefer *writePrefer, tokens preferTokens) []string {
	if !tokens.returnSet {
		return nil
	}
	switch tokens.returnValue {
	case returnMinimal, returnHeadersOnly, returnRepresentation:
		prefer.Return = tokens.returnValue
		return nil
	default:
		return []string{"return=" + tokens.returnValue}
	}
}

func applyMissing(prefer *writePrefer, tokens preferTokens) []string {
	if !tokens.missingSet {
		return nil
	}
	if tokens.missingValue == "default" {
		prefer.MissingDefault = true
		return nil
	}
	return []string{"missing=" + tokens.missingValue}
}

func applyMaxAffected(prefer *writePrefer, tokens preferTokens) []string {
	if !tokens.maxSet {
		return nil
	}
	maxValue, err := strconv.ParseInt(tokens.maxRaw, 10, 64)
	if err != nil || maxValue < 0 {
		return []string{"max-affected=" + tokens.maxRaw}
	}
	prefer.MaxAffected = &maxValue
	return nil
}

func preferenceApplied(prefer writePrefer, tokens preferTokens) []string {
	var applied []string
	if prefer.Strict {
		applied = append(applied, "handling=strict")
	}
	if tokens.returnSet && prefer.Return == tokens.returnValue {
		applied = append(applied, "return="+prefer.Return)
	}
	if prefer.MissingDefault {
		applied = append(applied, "missing=default")
	}
	if prefer.Strict && prefer.MaxAffected != nil {
		applied = append(
			applied,
			"max-affected="+strconv.FormatInt(*prefer.MaxAffected, 10),
		)
	}
	return applied
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
