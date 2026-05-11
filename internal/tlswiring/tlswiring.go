// Package tlswiring provides TLS configuration helpers for the API Frontend.
// It wraps the kubernaut shared TLS package with AF-specific logic for
// conditional server TLS and per-dependency outbound transports.
package tlswiring

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

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

// CheckPartialTLSMaterial returns a warning message if exactly one of tls.crt
// or tls.key exists in certDir. Returns "" when both or neither exist.
func CheckPartialTLSMaterial(certDir string) string {
	if certDir == "" {
		return ""
	}
	certExists := fileExists(filepath.Join(certDir, "tls.crt"))
	keyExists := fileExists(filepath.Join(certDir, "tls.key"))
	if certExists && !keyExists {
		return fmt.Sprintf("tls.crt exists in %s but tls.key is missing — TLS will be disabled; provide both files or remove tls.crt", certDir)
	}
	if !certExists && keyExists {
		return fmt.Sprintf("tls.key exists in %s but tls.crt is missing — TLS will be disabled; provide both files or remove tls.key", certDir)
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// OutboundTransport returns a TLS-configured http.RoundTripper if caFile is
// non-empty, or nil (meaning "use default transport") otherwise.
func OutboundTransport(caFile string) (http.RoundTripper, error) {
	if caFile == "" {
		return nil, nil
	}
	return sharedtls.NewTLSTransport(caFile)
}

// CAReloadableTransport returns an http.RoundTripper whose CA trust is
// updated whenever the underlying CA file changes on disk. The returned
// watcher must be started by the caller via Start(ctx).
func CAReloadableTransport(caFile string, logger logr.Logger) (http.RoundTripper, *hotreload.FileWatcher, error) {
	if caFile == "" {
		return nil, nil, nil
	}

	rt := &reloadableCATransport{caFile: caFile}
	if err := rt.reload(); err != nil {
		return nil, nil, fmt.Errorf("initial CA load from %s: %w", caFile, err)
	}

	watcher, err := hotreload.NewFileWatcher(caFile, func(_ string) error {
		if reloadErr := rt.reload(); reloadErr != nil {
			logger.Error(reloadErr, "CA reload failed, keeping previous trust", "file", caFile)
			return reloadErr
		}
		logger.Info("outbound CA trust reloaded", "file", caFile)
		return nil
	}, logger.WithName("ca-reloader"))
	if err != nil {
		return nil, nil, err
	}

	return rt, watcher, nil
}

type reloadableCATransport struct {
	caFile string
	mu     sync.RWMutex
	inner  *http.Transport
}

// RoundTrip delegates to the current inner transport under a read lock.
func (t *reloadableCATransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.RLock()
	transport := t.inner
	t.mu.RUnlock()
	return transport.RoundTrip(req)
}

func (t *reloadableCATransport) reload() error {
	caPEM, err := os.ReadFile(t.caFile)
	if err != nil {
		return fmt.Errorf("reading CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("no valid certificates found in %s", t.caFile)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return fmt.Errorf("http.DefaultTransport is not *http.Transport")
	}
	transport := base.Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}

	t.mu.Lock()
	t.inner = transport
	t.mu.Unlock()
	return nil
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

// ValidateCAFilePath checks that a CA file path, if non-empty, points to a
// readable file containing at least one valid PEM certificate.
func ValidateCAFilePath(field, path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is from operator-controlled config, not user input
	if err != nil {
		return fmt.Errorf("%s: cannot read CA file %q: %w", field, path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return fmt.Errorf("%s: no valid PEM certificates in %q", field, path)
	}
	return nil
}
