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
	"fmt"
	"sync"
)

// CertReloader supports hot-reloading of TLS server certificates.
// It implements the tls.Config.GetCertificate callback pattern, allowing
// Kubernetes Secret rotation to be picked up without restarting the server.
// Thread-safe: concurrent GetCertificate calls and ReloadCallback are serialized
// via RWMutex.
//
// Issue #756: TLS certificate rotation for inter-service communication.
type CertReloader struct {
	certFile string
	keyFile  string
	mu       sync.RWMutex
	cert     *tls.Certificate
}

// NewCertReloader creates a CertReloader that loads the initial certificate
// from disk. Returns error if the initial load fails (fail-fast at startup).
func NewCertReloader(certFile, keyFile string) (*CertReloader, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS key pair (%s, %s): %w", certFile, keyFile, err)
	}

	return &CertReloader{
		certFile: certFile,
		keyFile:  keyFile,
		cert:     &cert,
	}, nil
}

// GetCertificate returns the current certificate for TLS handshakes.
// Safe for concurrent use from multiple goroutines (TLS accept loop).
func (r *CertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert, nil
}

// ReloadCallback re-reads both cert and key files from disk.
// The content argument is ignored because Kubernetes Secret updates are atomic
// (symlink swap of the entire directory), so we must re-read both files together
// rather than using the single-file content provided by FileWatcher.
//
// On failure, the previous certificate is preserved (graceful degradation).
// This function satisfies the hotreload.ReloadCallback signature.
func (r *CertReloader) ReloadCallback(_ string) error {
	newCert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("cert reload failed (%s, %s): %w", r.certFile, r.keyFile, err)
	}

	r.mu.Lock()
	r.cert = &newCert
	r.mu.Unlock()
	return nil
}
