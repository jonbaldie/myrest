package config

import (
	"fmt"
	"strings"
)

// anyAuth is the one part of the minimum run set that two knobs can satisfy.
const anyAuth = "jwt-secret and/or db-anon-role"

// IncompleteRunSetError says which parts of the minimum run set are missing.
type IncompleteRunSetError struct {
	// Needs holds one entry for each unmet part of the minimum run set, in
	// surface order: the name of a missing knob, or "jwt-secret and/or
	// db-anon-role" when the settings hold neither of those two.
	Needs []string
}

// Error reports the unmet parts of the minimum run set to the operator.
func (e *IncompleteRunSetError) Error() string {
	return fmt.Sprintf(
		"myrest must not serve the API: the minimum run set needs %s",
		strings.Join(e.Needs, ", "),
	)
}

// ServeGate reports nil when the settings complete the minimum run set, and an
// *IncompleteRunSetError when they do not. A process that gets an error must
// not serve the API.
func (s Settings) ServeGate() error {
	var needs []string
	if s.DB.URI == "" {
		needs = append(needs, "db-uri")
	}
	if len(s.DB.Schemas) == 0 {
		needs = append(needs, "db-schemas")
	}
	if s.JWT.Secret == "" && s.DB.AnonRole == "" {
		needs = append(needs, anyAuth)
	}
	if len(needs) == 0 {
		return nil
	}
	return &IncompleteRunSetError{Needs: needs}
}
