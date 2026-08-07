package httpapi

import (
	"strings"

	"github.com/jonbaldie/myrest/internal/config"
)

// ReportedBaseURL is the public base URL myrest uses when it reports an
// absolute URL. When openapi-server-proxy-uri is set, that value wins.
// Otherwise the listen URL of the process wins. myrest does not read
// X-Forwarded-Host, X-Forwarded-Proto, or Forwarded for this value — the
// parity target does not either.
func ReportedBaseURL(settings config.Settings, listenURL string) string {
	if settings.OpenAPI.ServerProxyURI != "" {
		return strings.TrimRight(settings.OpenAPI.ServerProxyURI, "/")
	}
	return strings.TrimRight(listenURL, "/")
}
