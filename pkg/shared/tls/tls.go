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
	singletonMu       sync.Mutex
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
			IdleConnTimeout:     90 * time.Second,
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
func StartCAFileWatcher(ctx context.Context, logger logr.Logger) (*hotreload.FileWatcher, error) {
	caFile := os.Getenv("TLS_CA_FILE")
	if caFile == "" {
		return nil, nil
	}

	rt, err := DefaultBaseTransport()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize CA reloader: %w", err)
	}

	reloader, ok := rt.(*CAReloader)
	if !ok {
		return nil, nil
	}

	watcher, err := hotreload.NewFileWatcher(
		caFile,
		reloader.ReloadCallback,
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

// NewTLSTransport creates an http.Transport configured with a custom CA pool
// for verifying server certificates on outbound HTTPS calls.
func NewTLSTransport(caFile string) (*http.Transport, error) {
	pool, err := LoadCACert(caFile)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	ApplyProfile(tlsCfg, getDefaultSecurityProfile())
	return &http.Transport{TLSClientConfig: tlsCfg}, nil
}
