package config

import (
	"fmt"
	"strings"
)

// IncompleteRunSetError says which part of the minimum run set is missing.
type IncompleteRunSetError struct {
	// Missing names the absent knobs, in normative surface order.
	Missing []string
}

// Error reports the missing knobs to the operator.
func (e *IncompleteRunSetError) Error() string {
	return fmt.Sprintf(
		"myrest must not serve the API: the minimum run set needs %s",
		strings.Join(e.Missing, ", "),
	)
}

// Gate reports nil when settings complete the minimum run set, and an
// *IncompleteRunSetError when they do not.
func Gate(settings Settings) error {
	var missing []string
	if settings.DB.URI == "" {
		missing = append(missing, "db-uri")
	}
	if len(settings.DB.Schemas) == 0 {
		missing = append(missing, "db-schemas")
	}
	if settings.JWT.Secret == "" && settings.DB.AnonRole == "" {
		missing = append(missing, "jwt-secret and/or db-anon-role")
	}
	if len(missing) == 0 {
		return nil
	}
	return &IncompleteRunSetError{Missing: missing}
}
