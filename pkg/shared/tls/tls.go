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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// TLSConfig holds TLS configuration shared across services.
type TLSConfig struct {
	// CertDir is the directory containing tls.crt and tls.key files. An empty
	// value is invalid for a Kubernaut API listener.
	CertDir string `yaml:"certDir,omitempty"`

	// CAFile is the path to the CA certificate for client trust.
	// Used by services that make outbound HTTPS calls.
	CAFile string `yaml:"caFile,omitempty"`
}

// Enabled returns true when a certificate directory is configured. API
// startup still validates that both certificate files are present and valid.
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

// ConfigureRequiredTLS configures the server for TLS and fails when the
// certificate material is unavailable or invalid. API listeners must not
// silently downgrade to plaintext while a Secret is missing or delayed.
//
// Issue #756: Returns a CertReloader that can be wired to a FileWatcher for
// zero-downtime certificate rotation.
func ConfigureRequiredTLS(server *http.Server, certDir string) (bool, *CertReloader, error) {
	if certDir == "" {
		return false, nil, fmt.Errorf("tls certificate directory is required")
	}

	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	for _, file := range []string{certFile, keyFile} {
		if _, err := os.Stat(file); err != nil {
			return false, nil, fmt.Errorf("required tls file %s is unavailable: %w", file, err)
		}
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

// defaultSystemCertDirCandidates mirrors the Linux directories checked by
// crypto/x509. Keeping them in SSL_CERT_DIR preserves system roots on Linux
// distributions whose trust store is directory-based, while the normal
// system bundle remains selected because SSL_CERT_FILE is cleared.
var defaultSystemCertDirCandidates = []string{
	"/etc/ssl/certs",
	"/etc/pki/tls/certs",
}

// InjectAmbientCACerts configures process-wide Linux trust discovery without
// writing a generated trust bundle. The configured CA must live in a dedicated
// read-only directory. TLS_CA_FILE remains the source for explicit reloadable
// transports; SSL_CERT_DIR augments Go's normal system trust discovery.
//
// The runtime owns SSL_CERT_FILE and SSL_CERT_DIR. Static deployment overrides
// are not supported and are cleared/replaced here. No-op (fail-open, matching
// BuildClientTLSConfig's empty-caFile precedent) when caFile is empty.
// MUST be called before any TLS handshake: x509.SystemCertPool() is
// process-cached. Call it immediately after config load and before any client,
// logger, or OTel setup that can perform outbound TLS.
func InjectAmbientCACerts(logger logr.Logger, caFile string) error {
	if caFile == "" {
		return nil
	}

	caDir, err := validateAmbientCAFile(caFile)
	if err != nil {
		return err
	}
	sslCertDirs := buildAmbientCACertDirs(caDir)
	if err := configureAmbientCAEnvironment(caFile, sslCertDirs); err != nil {
		return err
	}

	logger.Info("ambient CA trust injected", "caFile", caFile, "sslCertDir", sslCertDirs)
	return nil
}

func validateAmbientCAFile(caFile string) (string, error) {
	if !filepath.IsAbs(caFile) {
		return "", fmt.Errorf("ambient CA file path must be absolute: %s", caFile)
	}

	customPEM, err := os.ReadFile(caFile)
	if err != nil {
		return "", fmt.Errorf("failed to read CA file %s: %w", caFile, err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(customPEM) {
		return "", fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	caDir := filepath.Dir(caFile)
	if strings.Contains(caDir, string(filepath.ListSeparator)) {
		return "", fmt.Errorf("ambient CA directory path contains path-list separator: %s", caDir)
	}
	info, err := os.Stat(caDir)
	if err != nil {
		return "", fmt.Errorf("failed to stat ambient CA directory %s: %w", caDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("ambient CA parent path is not a directory: %s", caDir)
	}
	return caDir, nil
}

func buildAmbientCACertDirs(caDir string) string {
	dirs := make([]string, 0, len(defaultSystemCertDirCandidates)+1)
	appendDir := func(path string) {
		cleanPath := filepath.Clean(path)
		for _, existing := range dirs {
			if existing == cleanPath {
				return
			}
		}
		dirs = append(dirs, cleanPath)
	}

	appendDir(caDir)
	for _, systemDir := range defaultSystemCertDirCandidates {
		appendDir(systemDir)
	}
	return strings.Join(dirs, string(filepath.ListSeparator))
}

func configureAmbientCAEnvironment(caFile, sslCertDirs string) error {
	oldSSLFile, hadSSLFile := os.LookupEnv("SSL_CERT_FILE")
	oldSSLDirs, hadSSLDirs := os.LookupEnv("SSL_CERT_DIR")
	oldTLSCAFile, hadTLSCAFile := os.LookupEnv("TLS_CA_FILE")
	restore := func() {
		if hadSSLFile {
			_ = os.Setenv("SSL_CERT_FILE", oldSSLFile)
		} else {
			_ = os.Unsetenv("SSL_CERT_FILE")
		}
		if hadSSLDirs {
			_ = os.Setenv("SSL_CERT_DIR", oldSSLDirs)
		} else {
			_ = os.Unsetenv("SSL_CERT_DIR")
		}
		if hadTLSCAFile {
			_ = os.Setenv("TLS_CA_FILE", oldTLSCAFile)
		} else {
			_ = os.Unsetenv("TLS_CA_FILE")
		}
	}

	if err := os.Unsetenv("SSL_CERT_FILE"); err != nil {
		return fmt.Errorf("failed to clear SSL_CERT_FILE: %w", err)
	}
	if err := os.Setenv("SSL_CERT_DIR", sslCertDirs); err != nil {
		restore()
		return fmt.Errorf("failed to set SSL_CERT_DIR: %w", err)
	}
	if err := os.Setenv("TLS_CA_FILE", caFile); err != nil {
		restore()
		return fmt.Errorf("failed to set TLS_CA_FILE: %w", err)
	}
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
