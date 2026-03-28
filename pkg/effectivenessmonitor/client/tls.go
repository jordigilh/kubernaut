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

package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// BuildCertPool creates an x509.CertPool from PEM-encoded certificate data.
// The custom CA is appended to the system cert pool so that both
// system-trusted and custom CAs are honored. Returns an error if pemData
// contains no valid PEM certificates (no silent fallback).
func BuildCertPool(pemData []byte) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("no valid PEM certificates found in CA data (%d bytes)", len(pemData))
	}
	return pool, nil
}

// NewHTTPClientWithCA creates an HTTP client configured with a custom CA
// certificate pool for TLS connections. The custom CA is appended to the
// system cert pool so that both system-trusted and custom CAs are honored.
//
// Issue #452: Enables EM to connect to Prometheus/AlertManager over HTTPS
// when endpoints use certificates signed by a non-system CA (e.g., OCP
// service-serving signer).
//
// The returned client has TLS configured but no bearer token injection.
// Callers can wrap client.Transport with auth.NewServiceAccountTransportWithBase
// to add SA token authentication.
func NewHTTPClientWithCA(caFile string, timeout time.Duration) (*http.Client, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %q: %w", caFile, err)
	}

	pool, err := BuildCertPool(caCert)
	if err != nil {
		return nil, fmt.Errorf("CA file %q: %w", caFile, err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
