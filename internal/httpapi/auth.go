package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jonbaldie/myrest/internal/jwt"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// The JWT error codes of the parity target.
const (
	codeJWTSecretMissing = "PGRST300"
	codeInvalidJWT       = "PGRST301"
	codeJWTRequired      = "PGRST302"
	codeJWTClaimsErr     = "PGRST303"
)

// requestRole picks the database role of the request: the JWT role claim on a
// valid Bearer token, or the anonymous database role when there is no usable
// JWT. Non-Bearer schemes and Postgres-only authz Prefer values are refused.
func (s *Service) requestRole(writer http.ResponseWriter, request *http.Request) (schemacache.Role, bool) {
	if refused := refuseUnsupportedAuth(writer, request); refused {
		return "", false
	}

	token, hasBearer, ok := bearerToken(writer, request)
	if !ok {
		return "", false
	}
	if !hasBearer {
		return s.anonymousRole(writer)
	}
	return s.roleFromBearer(writer, token)
}

func refuseUnsupportedAuth(writer http.ResponseWriter, request *http.Request) bool {
	if preferAsksForRowSecurity(request) {
		writeUnsupportedFeature(
			writer,
			"Postgres row-level security is not available with MySQL",
		)
		return true
	}
	if preferAsksForRequestGUCs(request) {
		writeUnsupportedFeature(
			writer,
			"Request GUCs and request.jwt.claims are not available with MySQL",
		)
		return true
	}
	return false
}

// bearerToken reads a Bearer credential. hasBearer is false when the header is
// absent. ok is false when the header is present but not a Bearer scheme.
func bearerToken(writer http.ResponseWriter, request *http.Request) (token string, hasBearer, ok bool) {
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return "", false, true
	}

	scheme, value, found := strings.Cut(authorization, " ")
	if !found || scheme == "" || !strings.EqualFold(scheme, "Bearer") {
		writeUnsupportedFeature(writer, "Only Bearer JWT credentials are supported")
		return "", false, false
	}
	return strings.TrimSpace(value), true, true
}

func (s *Service) roleFromBearer(writer http.ResponseWriter, token string) (schemacache.Role, bool) {
	if s.verifier == nil {
		writeFailure(
			writer, http.StatusInternalServerError, codeJWTSecretMissing,
			"Server lacks JWT secret",
		)
		return "", false
	}

	role, err := s.verifier.Role(token)
	if err == nil {
		return role, true
	}
	if errors.Is(err, jwt.ErrNoRole) {
		return s.anonymousRole(writer)
	}
	var claims jwt.ClaimsFailure
	if errors.As(err, &claims) {
		writeAuthFailure(writer, codeJWTClaimsErr, claims.Message)
		return "", false
	}
	var decode jwt.DecodeFailure
	if errors.As(err, &decode) {
		writeAuthFailure(writer, codeInvalidJWT, decode.Message)
		return "", false
	}
	if errors.Is(err, jwt.ErrClaimsFailed) {
		writeAuthFailure(writer, codeJWTClaimsErr, "Parsing claims failed")
		return "", false
	}
	writeAuthFailure(writer, codeInvalidJWT, "JWT cryptographic operation failed")
	return "", false
}

func (s *Service) anonymousRole(writer http.ResponseWriter) (schemacache.Role, bool) {
	role := schemacache.Role(s.settings.DB.AnonRole)
	if role == "" {
		writeAuthFailure(writer, codeJWTRequired, "Anonymous access is disabled")
		return "", false
	}
	return role, true
}

func writeAuthFailure(writer http.ResponseWriter, code, message string) {
	writer.Header().Set("WWW-Authenticate", `Bearer`)
	if code == codeInvalidJWT || code == codeJWTClaimsErr {
		writer.Header().Set(
			"WWW-Authenticate",
			`Bearer error="invalid_token", error_description="`+message+`"`,
		)
	}
	writeFailure(writer, http.StatusUnauthorized, code, message)
}

// preferAsksForRowSecurity finds a Prefer token that asks for Postgres
// row-level security. myrest refuses it: MySQL has no RLS, and myrest offers
// no fake row policy layer.
func preferAsksForRowSecurity(request *http.Request) bool {
	return preferHolds(request, "row-security")
}

// preferAsksForRequestGUCs finds a Prefer token that asks for claim or header
// injection as request GUCs in SQL. myrest refuses it: MySQL has no GUCs.
func preferAsksForRequestGUCs(request *http.Request) bool {
	return preferHolds(request, "jwt-claims")
}

func preferHolds(request *http.Request, token string) bool {
	for _, header := range request.Header.Values("Prefer") {
		for _, part := range strings.Split(header, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			if strings.EqualFold(name, token) {
				return true
			}
		}
	}
	return false
}
