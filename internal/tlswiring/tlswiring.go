// Package tlswiring provides TLS configuration helpers for the API Frontend.
// It wraps the kubernaut shared TLS package with AF-specific logic for
// conditional server TLS and per-dependency outbound transports.
package tlswiring

import (
	"net/http"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// ConfigureServer sets up conditional TLS on the server based on certDir.
// Returns (tlsEnabled, certReloader, error). When certDir is empty or cert
// files don't exist, returns (false, nil, nil) — the server serves plain HTTP.
func ConfigureServer(server *http.Server, certDir string) (bool, *sharedtls.CertReloader, error) {
	if certDir == "" {
		return false, nil, nil
	}
	return sharedtls.ConfigureConditionalTLS(server, certDir)
}

// OutboundTransport returns a TLS-configured http.RoundTripper if caFile is
// non-empty, or nil (meaning "use default transport") otherwise.
func OutboundTransport(caFile string) (http.RoundTripper, error) {
	if caFile == "" {
		return nil, nil
	}
	return sharedtls.NewTLSTransport(caFile)
}
