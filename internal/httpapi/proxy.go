package httpapi

import "strings"

// reportedBaseURL is the public base URL myrest uses when it reports an
// absolute URL. When openapi-server-proxy-uri is set, that value wins.
// Otherwise the listen URL of the process wins. myrest does not read
// X-Forwarded-Host, X-Forwarded-Proto, or Forwarded for this value — the
// parity target does not either. OpenAPI emission of this URL is ticket #44.
func reportedBaseURL(proxyURI, listenURL string) string {
	if proxyURI != "" {
		return strings.TrimRight(proxyURI, "/")
	}
	return strings.TrimRight(listenURL, "/")
}
