/*
Copyright 2025 Jordi Gil.

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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
// Returns (true, nil) if TLS was configured, (false, nil) if no certs found (plain HTTP),
// or (false, error) if certs exist but are invalid.
func ConfigureConditionalTLS(server *http.Server, certDir string) (bool, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return false, nil
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return false, nil
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return false, fmt.Errorf("failed to load TLS certificate from %s: %w", certDir, err)
	}

	server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return true, nil
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

// NewTLSTransport creates an http.Transport configured with a custom CA pool
// for verifying server certificates on outbound HTTPS calls.
func NewTLSTransport(caFile string) (*http.Transport, error) {
	pool, err := LoadCACert(caFile)
	if err != nil {
		return nil, err
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		},
	}, nil
}
