/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tls provides shared TLS utilities for Kubernaut services.
// Issue #493: TLS for inter-pod HTTP communication.
package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// TLSConfig holds TLS configuration shared across services.
type TLSConfig struct {
	// CertDir is the directory containing tls.crt and tls.key files.
	// When empty, TLS is disabled.
	CertDir string `yaml:"certDir,omitempty"`

	// CAFile is the path to the CA certificate for client trust.
	// Used by services that make outbound HTTPS calls.
	CAFile string `yaml:"caFile,omitempty"`
}

// Enabled returns true when a cert directory is configured.
func (c TLSConfig) Enabled() bool {
	return c.CertDir != ""
}

// CertPath returns the full path to the TLS certificate file.
func (c TLSConfig) CertPath() string {
	return filepath.Join(c.CertDir, "tls.crt")
}

// KeyPath returns the full path to the TLS private key file.
func (c TLSConfig) KeyPath() string {
	return filepath.Join(c.CertDir, "tls.key")
}

// ConfigureConditionalTLS configures the server for TLS if cert files exist in certDir.
// Returns (true, reloader, nil) if TLS was configured with hot-reload support,
// (false, nil, nil) if no certs found (plain HTTP),
// or (false, nil, error) if certs exist but are invalid.
//
// Issue #756: Returns a CertReloader that can be wired to a FileWatcher for
// zero-downtime certificate rotation.
func ConfigureConditionalTLS(server *http.Server, certDir string) (bool, *CertReloader, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return false, nil, nil
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return false, nil, nil
	}

	reloader, err := NewCertReloader(certFile, keyFile)
	if err != nil {
		return false, nil, fmt.Errorf("failed to load TLS certificate from %s: %w", certDir, err)
	}

	server.TLSConfig = &tls.Config{
		GetCertificate: reloader.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
	ApplyProfile(server.TLSConfig, getDefaultSecurityProfile())

	return true, reloader, nil
}

// LoadCACert loads a PEM-encoded CA certificate from the given file path
// and returns an x509.CertPool containing the CA.
func LoadCACert(caFile string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate %s: %w", caFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	return pool, nil
}

// Singleton CAReloader for process-wide client TLS.
// Issue #753: Replaced sync.Once with mutex-guarded lazy init that retries
// on error -- prevents permanent failure if CA file isn't yet mounted.
var (
	caReloaderInstance *CAReloader
	singletonMu        sync.Mutex
)

// DefaultBaseTransport returns an http.RoundTripper pre-configured with the CA
// certificate at $TLS_CA_FILE (if set). When TLS_CA_FILE points to a valid CA
// file, a process-level CAReloader is initialized and returned as the
// RoundTripper — this enables hot-reload when the CA file is rotated.
//
// When TLS_CA_FILE is unset or empty, returns a plain http.Transport.
//
// Issue #753: Uses retry-capable lazy init instead of sync.Once. If the CA
// file is not yet available (e.g., Secret not mounted), subsequent calls will
// retry instead of failing permanently.
func DefaultBaseTransport() (http.RoundTripper, error) {
	caFile := os.Getenv("TLS_CA_FILE")
	if caFile == "" {
		return &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     15 * time.Second, // Issue #853: reduced from 90s — prevents stale connection reuse after pod rescheduling
		}, nil
	}

	singletonMu.Lock()
	defer singletonMu.Unlock()

	if caReloaderInstance != nil {
		return caReloaderInstance, nil
	}

	instance, err := NewCAReloaderFromFile(caFile)
	if err != nil {
		return nil, err
	}
	caReloaderInstance = instance
	return instance, nil
}

// DefaultBaseTransportWithRetry returns DefaultBaseTransport wrapped with a
// RetryTransport using default retry configuration (3 attempts, exponential
// backoff with 20% jitter). Use this for inter-service HTTP clients that
// should survive transient failures (connection reset, 502/503/504).
//
// IMPORTANT: Do NOT use for the audit client — it has its own application-level
// retry in BufferedAuditStore.writeBatchWithRetry (DD-AUDIT-003).
//
// Issue #853: Inter-service HTTP clients lack retry/circuit-breaker.
func DefaultBaseTransportWithRetry() (http.RoundTripper, error) {
	base, err := DefaultBaseTransport()
	if err != nil {
		return nil, err
	}
	return transport.NewRetryTransport(base, transport.DefaultRetryConfig()), nil
}

// ResetDefaultTransportForTesting resets the singleton CAReloader so that
// tests run with a clean slate. Must only be called from test code.
func ResetDefaultTransportForTesting() {
	singletonMu.Lock()
	defer singletonMu.Unlock()
	caReloaderInstance = nil
}

// StartCAFileWatcher initializes the CA reloader singleton and starts a
// FileWatcher on $TLS_CA_FILE. Returns nil watcher if TLS_CA_FILE is unset.
// The returned watcher must be stopped by the caller (defer watcher.Stop()).
// StartCAFileWatcher watches TLS_CA_FILE for changes and hot-reloads the
// shared CA transport. onReload, if provided, is invoked after every reload
// attempt (initial load and every subsequent change) with the callback's
// error (nil on success) — letting callers emit an audit event without this
// widely-shared helper depending on any particular audit implementation
// (GAP-11, Issue #1505). At most the first onReload function is used.
func StartCAFileWatcher(ctx context.Context, logger logr.Logger, onReload ...func(error)) (*hotreload.FileWatcher, error) {
	caFile := os.Getenv("TLS_CA_FILE")
	if caFile == "" {
		// nolint:nilnil // intentional "not configured" sentinel, not an error.
		// All 10+ cmd/*/main.go callers guard with `if caWatcher != nil` before
		// use, and *hotreload.FileWatcher.Stop() is itself nil-receiver-safe
		// as defense in depth (Issue #1546 Tier 2).
		return nil, nil
	}

	rt, err := DefaultBaseTransport()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize CA reloader: %w", err)
	}

	reloader, ok := rt.(*CAReloader)
	if !ok {
		// nolint:nilnil // intentional "not applicable" sentinel (base transport
		// isn't a *CAReloader), not an error. Same caller-safety guarantees as
		// the caFile=="" case above (Issue #1546 Tier 2).
		return nil, nil
	}

	callback := reloader.ReloadCallback
	if len(onReload) > 0 && onReload[0] != nil {
		notify := onReload[0]
		callback = func(newContent string) error {
			err := reloader.ReloadCallback(newContent)
			notify(err)
			return err
		}
	}

	watcher, err := hotreload.NewFileWatcher(
		caFile,
		callback,
		logger.WithName("ca-reloader"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA file watcher: %w", err)
	}
	if err := watcher.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start CA file watcher: %w", err)
	}
	return watcher, nil
}

// defaultSystemCertFileCandidates mirrors the unexported certFiles list in
// Go's crypto/x509/root_linux.go -- the stdlib has no exported API to read
// the system trust store back out as PEM bytes (x509.CertPool cannot be
// exported), so InjectAmbientCACerts locates the same well-known bundle
// file directly in order to build a combined SSL_CERT_FILE. Order matches
// upstream; the lookup stops at the first file that exists.
var defaultSystemCertFileCandidates = []string{
	"/etc/ssl/certs/ca-certificates.crt",                // Debian/Ubuntu/Gentoo
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora/RHEL (kubernaut's UBI base image)
	"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
	"/etc/pki/tls/cacert.pem",                           // OpenELEC
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS/RHEL 7+ / UBI
	"/etc/ssl/cert.pem",                                 // Alpine Linux
}

var systemCertFileCandidates = defaultSystemCertFileCandidates

// SetSystemCertFileCandidatesForTesting overrides the well-known system CA
// bundle paths InjectAmbientCACerts checks. Must only be called from test
// code (mirrors ResetDefaultTransportForTesting's test-hook pattern).
func SetSystemCertFileCandidatesForTesting(paths []string) {
	systemCertFileCandidates = paths
}

// ResetSystemCertFileCandidatesForTesting restores the production list of
// system CA bundle candidate paths. Must only be called from test code.
func ResetSystemCertFileCandidatesForTesting() {
	systemCertFileCandidates = defaultSystemCertFileCandidates
}

func readSystemCertBundle() ([]byte, string, error) {
	for _, f := range systemCertFileCandidates {
		if b, err := os.ReadFile(f); err == nil {
			return b, f, nil
		}
	}
	return nil, "", fmt.Errorf("no system CA bundle found in known locations")
}

// InjectAmbientCACerts derives the ambient SSL_CERT_FILE trust bundle
// in-process from a service's own resolved config, so no deployer needs to
// declare SSL_CERT_FILE as a static Pod-spec env: entry (Issue #2276) --
// eliminating the class of bug where two independently chart-rendered
// components declare the same env var name and the Pod spec is rejected.
//
// Combines the process's default system CA bundle (located via the same
// well-known paths crypto/x509 itself checks, see
// defaultSystemCertFileCandidates) with caFile's PEM content into one temp
// file, then sets SSL_CERT_FILE to that combined path. This step is
// necessary because setting SSL_CERT_FILE REPLACES -- not merges with --
// the default system trust store (crypto/x509/root_unix.go), and
// x509.CertPool has no API to export the system pool back to PEM for
// combination; reading the well-known bundle file directly is the only way
// to reproduce the "combined bundle" the reporter otherwise had to build by
// hand.
//
// Also sets TLS_CA_FILE=caFile so every existing
// DefaultBaseTransport/StartCAFileWatcher/NewTLSTransport call site (which
// already reads TLS_CA_FILE via os.Getenv) keeps working unmodified -- only
// the *source* of that env var moves from a static Pod-spec env: entry to
// an in-process, config-derived os.Setenv.
//
// No-op (fail-open, matching BuildClientTLSConfig's empty-caFile precedent)
// when caFile is empty. MUST be called before any TLS handshake in the
// process: x509.SystemCertPool() is sync.Once-cached process-wide, so
// calling this after the first outbound HTTPS call has no effect (spike-
// verified, Issue #2276 preflight). Call as the first statement of run(),
// immediately after config load and before any client/logger/OTel setup.
func InjectAmbientCACerts(logger logr.Logger, caFile string) error {
	if caFile == "" {
		return nil
	}

	customPEM, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("failed to read CA file %s: %w", caFile, err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(customPEM) {
		return fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	combined := make([]byte, 0, len(customPEM)+1)
	combined = append(combined, customPEM...)
	if systemPEM, systemFile, sysErr := readSystemCertBundle(); sysErr == nil {
		combined = append(combined, '\n')
		combined = append(combined, systemPEM...)
		logger.V(1).Info("combined ambient CA bundle with system trust store", "systemBundle", systemFile)
	} else {
		logger.Info("no system CA bundle found in known locations; SSL_CERT_FILE will trust only the configured CA", "caFile", caFile)
	}

	tmpFile, err := os.CreateTemp("", "kubernaut-ambient-ca-*.pem")
	if err != nil {
		return fmt.Errorf("failed to create ambient CA bundle temp file: %w", err)
	}
	if _, err := tmpFile.Write(combined); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write ambient CA bundle: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close ambient CA bundle temp file: %w", err)
	}

	if err := os.Setenv("SSL_CERT_FILE", tmpFile.Name()); err != nil {
		return fmt.Errorf("failed to set SSL_CERT_FILE: %w", err)
	}
	if err := os.Setenv("TLS_CA_FILE", caFile); err != nil {
		return fmt.Errorf("failed to set TLS_CA_FILE: %w", err)
	}

	logger.Info("ambient CA trust injected", "caFile", caFile, "sslCertFile", tmpFile.Name())
	return nil
}

// TLSTransportOption configures optional features on transports created by
// NewTLSTransport. Use WithClientCert to enable mTLS.
type TLSTransportOption func(*tlsTransportOpts)

type tlsTransportOpts struct {
	certFile string
	keyFile  string
}

// WithClientCert enables mutual TLS (mTLS) by loading a client certificate
// and private key that will be presented during the TLS handshake.
// Both certFile and keyFile must be valid PEM-encoded files.
// BR-NET-002: Certificate-based authentication for enterprise LLM gateways.
func WithClientCert(certFile, keyFile string) TLSTransportOption {
	return func(o *tlsTransportOpts) {
		o.certFile = certFile
		o.keyFile = keyFile
	}
}

// BuildTLSConfig builds a *tls.Config with a custom CA pool for verifying
// server certificates, the fleet's default security profile (min TLS
// version, cipher suites, curve preferences), and optional mTLS via
// WithClientCert. This is the building block NewTLSTransport wraps in an
// *http.Transport; callers that need a raw *tls.Config directly (e.g.
// go-redis's redis.Options.TLSConfig) should call this instead of
// constructing their own ad-hoc tls.Config (DD-PLATFORM-006 DA9).
//
// caFile is REQUIRED here (LoadCACert errors on empty/unreadable paths) --
// existing callers (cmd/apifrontend, cmd/fleetmetadatacache) always
// validate a non-empty CAFile before calling this. For callers that need
// an OPTIONAL CAFile (empty = trust the system CA pool, e.g. a
// publicly-trusted vendor OTel collector), use BuildClientTLSConfig instead.
func BuildTLSConfig(caFile string, opts ...TLSTransportOption) (*tls.Config, error) {
	pool, err := LoadCACert(caFile)
	if err != nil {
		return nil, err
	}

	var o tlsTransportOpts
	for _, fn := range opts {
		fn(&o)
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}

	if o.certFile != "" && o.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(o.certFile, o.keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate (%s, %s): %w", o.certFile, o.keyFile, err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	ApplyProfile(tlsCfg, getDefaultSecurityProfile())
	return tlsCfg, nil
}

// BuildClientTLSConfig builds a *tls.Config for outbound HTTPS calls, with
// the process-wide SecurityProfile (see profile.go) applied. Unlike
// BuildTLSConfig, caFile is OPTIONAL here: when empty, the system CA pool
// is trusted (e.g. a publicly-trusted vendor OTel collector); when set, it
// is loaded as the sole trust anchor via LoadCACert. Optional
// TLSTransportOption values (e.g. WithClientCert) add mTLS.
func BuildClientTLSConfig(caFile string, opts ...TLSTransportOption) (*tls.Config, error) {
	var o tlsTransportOpts
	for _, fn := range opts {
		fn(&o)
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if caFile != "" {
		pool, err := LoadCACert(caFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}

	if o.certFile != "" && o.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(o.certFile, o.keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate (%s, %s): %w", o.certFile, o.keyFile, err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	ApplyProfile(tlsCfg, getDefaultSecurityProfile())
	return tlsCfg, nil
}

// NewTLSTransport creates an http.Transport configured with a custom CA pool
// for verifying server certificates on outbound HTTPS calls.
// Optional TLSTransportOption values (e.g. WithClientCert) extend the transport
// with mTLS or other features without breaking existing callers.
func NewTLSTransport(caFile string, opts ...TLSTransportOption) (*http.Transport, error) {
	tlsCfg, err := BuildTLSConfig(caFile, opts...)
	if err != nil {
		return nil, err
	}
	return &http.Transport{TLSClientConfig: tlsCfg}, nil
}
