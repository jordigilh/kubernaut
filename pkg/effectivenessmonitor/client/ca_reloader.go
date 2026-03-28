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
	"sync"
)

// CAReloader is an http.RoundTripper that supports hot-reloading of
// the TLS CA certificate pool. When the OCP service-ca operator
// updates the mounted ConfigMap, the FileWatcher calls ReloadCallback
// which builds a new http.Transport with the fresh cert pool and swaps
// it atomically. Existing in-flight requests complete on the old
// transport; new requests use the new one.
//
// Issue #484: Resolves the race where EM starts before the service-ca
// ConfigMap is populated by allowing the cert pool to be replaced at
// runtime without restarting the pod.
//
// Thread safety: all public methods are safe for concurrent use.
type CAReloader struct {
	mu        sync.RWMutex
	transport *http.Transport
	pool      *x509.CertPool
}

// NewCAReloader creates a CAReloader initialized with the given PEM
// certificate data. Returns an error if pemData contains no valid PEM
// certificates.
func NewCAReloader(pemData []byte) (*CAReloader, error) {
	pool, err := BuildCertPool(pemData)
	if err != nil {
		return nil, err
	}
	t := buildTransport(pool)
	return &CAReloader{
		transport: t,
		pool:      pool,
	}, nil
}

// ReloadCallback is compatible with hotreload.ReloadCallback. It parses
// newContent as PEM, builds a fresh cert pool, and atomically replaces
// the underlying http.Transport. If the PEM is invalid the previous
// transport is preserved and an error is returned.
func (r *CAReloader) ReloadCallback(newContent string) error {
	pool, err := BuildCertPool([]byte(newContent))
	if err != nil {
		return fmt.Errorf("CA reload rejected: %w", err)
	}
	t := buildTransport(pool)

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

func buildTransport(pool *x509.CertPool) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}
	return t
}
