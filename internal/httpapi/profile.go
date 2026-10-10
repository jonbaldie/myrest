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

// profileHeader is the request header that names the database for a method,
// as the parity target reads it: Content-Profile for the writing methods,
// Accept-Profile for every other method.
func profileHeader(method string) string {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return headerContentProfile
	default:
		return headerAcceptProfile
	}
}

// requestProfile is the trimmed profile header value of the request.
func requestProfile(request *http.Request) string {
	return strings.TrimSpace(request.Header.Get(profileHeader(request.Method)))
}

// requestDatabase returns the MySQL database the request names. With no
// profile header it is the default database. A value outside db-schemas
// refuses in the PostgREST shape (PGRST106).
func (s *Service) requestDatabase(writer http.ResponseWriter, request *http.Request) (string, bool) {
	profile := requestProfile(request)
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

// setContentProfile names the request database in the Content-Profile
// response header when the database was negotiated by profile: the client
// sent a profile header, or db-schemas lists more than one database. The
// parity target sends it only with the Content-Type of a successful body, so
// call it there; failures drop it again (see writeFailureExtra).
func (s *Service) setContentProfile(writer http.ResponseWriter, request *http.Request, database string) {
	if requestProfile(request) == "" && len(s.settings.DB.Schemas) < 2 {
		return
	}
	writer.Header().Set(headerContentProfile, database)
}

// badProfileMessage lists the configured databases the way the parity target
// does for PGRST106.
func badProfileMessage(databases []string) string {
	return "The schema must be one of the following: " + strings.Join(databases, ", ")
}
