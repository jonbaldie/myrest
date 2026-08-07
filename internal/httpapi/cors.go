package httpapi

import (
	"net/http"
	"strings"
)

// CORS headers of the parity target (PostgREST v14.16).
const (
	corsAllowMethods  = "GET, POST, PATCH, PUT, DELETE, OPTIONS, HEAD"
	corsExposeHeaders = "Content-Encoding, Content-Location, Content-Range, Content-Type, Date, Location, Server, Transfer-Encoding, Range-Unit"
	corsMaxAge        = "86400"
)

// withCORS applies the server-cors-allowed-origins policy of the parity target.
// An empty allow list accepts every origin.
func withCORS(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(writer, request)
			return
		}

		allowedOrigin, ok := corsAllowOrigin(allowed, origin)
		if isCORSPreflight(request) {
			if ok {
				writeCORSPreflight(writer, request, allowedOrigin)
				return
			}
			// The parity target ignores a refused origin and lets the request
			// continue without CORS headers.
			next.ServeHTTP(writer, request)
			return
		}

		if ok {
			writeCORSActual(writer, allowedOrigin)
		}
		next.ServeHTTP(writer, request)
	})
}

// corsAllowOrigin gives the Access-Control-Allow-Origin value for a request.
// An empty allow list means every origin, as "*". A listed match reflects the
// request Origin. An origin outside the list is refused.
func corsAllowOrigin(allowed []string, origin string) (string, bool) {
	if len(allowed) == 0 {
		return "*", true
	}
	for _, candidate := range allowed {
		if candidate == origin {
			return origin, true
		}
	}
	return "", false
}

func isCORSPreflight(request *http.Request) bool {
	return request.Method == http.MethodOptions &&
		request.Header.Get("Access-Control-Request-Method") != ""
}

func writeCORSPreflight(writer http.ResponseWriter, request *http.Request, allowOrigin string) {
	setAllowOrigin(writer, allowOrigin)
	writer.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
	writer.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders(request))
	writer.Header().Set("Access-Control-Max-Age", corsMaxAge)
	writer.WriteHeader(http.StatusOK)
}

func corsAllowHeaders(request *http.Request) string {
	parts := []string{"Authorization"}
	if asked := request.Header.Get("Access-Control-Request-Headers"); asked != "" {
		for _, header := range strings.Split(asked, ",") {
			header = strings.TrimSpace(header)
			if header == "" || strings.EqualFold(header, "Authorization") {
				continue
			}
			parts = append(parts, header)
		}
	}
	parts = append(parts, "Accept", "Accept-Language", "Content-Language")
	return strings.Join(parts, ", ")
}

func writeCORSActual(writer http.ResponseWriter, allowOrigin string) {
	setAllowOrigin(writer, allowOrigin)
	writer.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)
}

func setAllowOrigin(writer http.ResponseWriter, allowOrigin string) {
	writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	if allowOrigin != "*" {
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}
