// Package tlswiring provides TLS configuration helpers for the API Frontend.
// It wraps the kubernaut shared TLS package with AF-specific logic for
// conditional server TLS and per-dependency outbound transports.
package tlswiring

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
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

// StartCertFileWatcher starts a file watcher on the TLS certificate directory
// that triggers hot-reload of the server certificate when the file changes.
// Returns nil watcher if reloader is nil (TLS disabled).
func StartCertFileWatcher(ctx context.Context, certDir string, reloader *sharedtls.CertReloader, logger logr.Logger) (*hotreload.FileWatcher, error) {
	if reloader == nil || certDir == "" {
		return nil, nil
	}
	certFile := filepath.Join(certDir, "tls.crt")
	watcher, err := hotreload.NewFileWatcher(certFile, reloader.ReloadCallback, logger.WithName("cert-reloader"))
	if err != nil {
		return nil, err
	}
	if err := watcher.Start(ctx); err != nil {
		return nil, err
	}
	return watcher, nil
}

// StartCAFileWatcher starts a file watcher for the outbound CA certificate.
// Delegates to sharedtls.StartCAFileWatcher which reads $TLS_CA_FILE.
// Returns nil watcher if TLS_CA_FILE is not set.
func StartCAFileWatcher(ctx context.Context, logger logr.Logger) (*hotreload.FileWatcher, error) {
	return sharedtls.StartCAFileWatcher(ctx, logger)
}
