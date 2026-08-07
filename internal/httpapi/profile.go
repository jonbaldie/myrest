package httpapi

import (
	"net/http"
	"strings"
)

const (
	// headerAcceptProfile selects the database for a read (GET/HEAD).
	headerAcceptProfile = "Accept-Profile"
	// headerContentProfile selects the database for a write (POST/PATCH/DELETE).
	headerContentProfile = "Content-Profile"
	// codeBadProfile is the parity-target code when a profile is outside
	// db-schemas.
	codeBadProfile = "PGRST106"
)

// requestDatabase returns the MySQL database the request names. With no
// profile header it is the default database. A value outside db-schemas
// refuses in the PostgREST shape (PGRST106).
func (s *Service) requestDatabase(
	writer http.ResponseWriter,
	request *http.Request,
	header string,
) (string, bool) {
	profile := strings.TrimSpace(request.Header.Get(header))
	if profile == "" {
		return s.settings.DefaultDatabase(), true
	}
	if s.settings.HasDatabase(profile) {
		return profile, true
	}
	writeFailure(
		writer,
		http.StatusNotAcceptable,
		codeBadProfile,
		badProfileMessage(s.settings.DB.Schemas),
	)
	return "", false
}

// badProfileMessage lists the configured databases the way the parity target
// does for PGRST106.
func badProfileMessage(databases []string) string {
	return "The schema must be one of the following: " + strings.Join(databases, ", ")
}
