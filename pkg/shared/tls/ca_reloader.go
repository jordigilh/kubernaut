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

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// CAReloader is an http.RoundTripper that supports hot-reloading of the TLS
// CA certificate pool. When the CA file is rotated (e.g., by the OCP service-ca
// operator), the FileWatcher calls ReloadCallback which builds a new
// http.Transport with the fresh cert pool and swaps it atomically.
//
// Existing in-flight requests complete on the old transport; new requests
// use the updated one.
//
// Issue #756: Generalized from pkg/effectivenessmonitor/client for all
// inter-service TLS communication.
//
// Thread safety: all public methods are safe for concurrent use.
type CAReloader struct {
	mu        sync.RWMutex
	transport *http.Transport
	pool      *x509.CertPool
}

// NewCAReloader creates a CAReloader initialized with the given PEM
// certificate data. Returns an error if pemData contains no valid PEM
// certificates or is empty.
func NewCAReloader(pemData []byte) (*CAReloader, error) {
	if len(pemData) == 0 {
		return nil, fmt.Errorf("CA PEM data is empty")
	}
	pool, err := buildCertPool(pemData)
	if err != nil {
		return nil, err
	}
	t := buildCATransport(pool)
	return &CAReloader{
		transport: t,
		pool:      pool,
	}, nil
}

// NewCAReloaderFromFile creates a CAReloader by reading PEM data from a file.
// Error message preserves compatibility with existing test expectations.
func NewCAReloaderFromFile(path string) (*CAReloader, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate %s: %w", path, err)
	}
	return NewCAReloader(data)
}

// ReloadCallback parses newContent as PEM, builds a fresh cert pool, and
// atomically replaces the underlying http.Transport. If the PEM is invalid,
// the previous transport is preserved and an error is returned.
//
// This function satisfies the hotreload.ReloadCallback signature.
func (r *CAReloader) ReloadCallback(newContent string) error {
	if newContent == "" {
		return fmt.Errorf("CA reload rejected: empty content")
	}
	pool, err := buildCertPool([]byte(newContent))
	if err != nil {
		return fmt.Errorf("CA reload rejected: %w", err)
	}
	t := buildCATransport(pool)

	r.mu.Lock()
	r.transport = t
	r.pool = pool
	r.mu.Unlock()
	return nil
}

// RoundTrip implements http.RoundTripper. Each call reads the current
// transport under a read lock, then delegates. The lock is held only
// for the pointer copy, not for the network I/O.
func (r *CAReloader) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.RLock()
	t := r.transport
	r.mu.RUnlock()
	return t.RoundTrip(req)
}

// GetCertPool returns the currently active certificate pool (snapshot).
func (r *CAReloader) GetCertPool() *x509.CertPool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pool
}

// CurrentTransport returns the currently active http.Transport (snapshot).
func (r *CAReloader) CurrentTransport() *http.Transport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.transport
}

// buildCertPool creates an x509.CertPool from PEM-encoded certificate data.
// Issue #753: Uses a file-only pool (no system roots) to enforce private PKI
// isolation -- prevents public CAs from verifying internal service certs.
func buildCertPool(pemData []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("no valid PEM certificates found in CA data (%d bytes)", len(pemData))
	}
	return pool, nil
}

// buildCATransport creates an http.Transport configured with the given CA pool.
// Issue #748: Applies the process-wide SecurityProfile when set (OCP deployments).
func buildCATransport(pool *x509.CertPool) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.IdleConnTimeout = 15 * time.Second // Issue #853: prevents stale connection reuse after pod rescheduling
	t.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	ApplyProfile(t.TLSClientConfig, getDefaultSecurityProfile())
	return t
}
