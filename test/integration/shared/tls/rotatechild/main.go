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

// Command rotatechild is the IT-TLS-2545-002 child process (BR-SECURITY-2545).
// It runs inside a read-only Linux container with a memory-backed /tmp and
// proves, through real HTTPS round trips, that the explicit TLS_CA_FILE
// transport contract hot-reloads a valid CA rotation and retains the
// last-known-good transport after an invalid rotation. Linux fsnotify is
// required (macOS kqueue directory watches do not attribute events to file
// names), which is why this proof runs under podman rather than in-process.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type caMaterial struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pem  []byte
}

func run() error {
	dir, err := os.MkdirTemp("", "tls-rotate-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}

	oldCA, err := generateCA("IT-TLS-2545-002 old CA")
	if err != nil {
		return err
	}
	newCA, err := generateCA("IT-TLS-2545-002 new CA")
	if err != nil {
		return err
	}
	thirdCA, err := generateCA("IT-TLS-2545-002 third CA")
	if err != nil {
		return err
	}

	serverOld, urlOld, err := startServer(dir, "old", oldCA)
	if err != nil {
		return fmt.Errorf("start old-CA server: %w", err)
	}
	defer func() { _ = serverOld.Close() }()
	serverNew, urlNew, err := startServer(dir, "new", newCA)
	if err != nil {
		return fmt.Errorf("start new-CA server: %w", err)
	}
	defer func() { _ = serverNew.Close() }()
	serverThird, urlThird, err := startServer(dir, "third", thirdCA)
	if err != nil {
		return fmt.Errorf("start third-CA server: %w", err)
	}
	defer func() { _ = serverThird.Close() }()

	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, oldCA.pem, 0644); err != nil {
		return fmt.Errorf("write initial CA: %w", err)
	}

	transport, stopWatcher, err := startExplicitTransport(caPath)
	if err != nil {
		return err
	}
	defer stopWatcher()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	if err := get(ctx, client, urlOld); err != nil {
		return fmt.Errorf("old-CA server must be trusted before rotation: %w", err)
	}
	if err := get(ctx, client, urlNew); err == nil {
		return errors.New("new-CA server must not be trusted before rotation")
	}

	return verifyRotationLifecycle(ctx, client, caPath, newCA.pem, urlNew, thirdCA.pem, urlThird)
}

// verifyRotationLifecycle proves, through real HTTPS round trips, that a
// valid CA rotation hot-reloads, an invalid rotation retains the
// last-known-good transport, and the watcher keeps processing events after
// rejecting the garbage PEM (the second valid rotation makes the rejection
// check non-vacuous).
func verifyRotationLifecycle(ctx context.Context, client *http.Client, caPath string,
	newCAPEM []byte, urlNew string, thirdCAPEM []byte, urlThird string) error {
	// Valid rotation via atomic rename (the ConfigMap update pattern).
	if err := rotate(caPath, newCAPEM); err != nil {
		return err
	}
	if err := waitForTrust(ctx, client, urlNew); err != nil {
		return fmt.Errorf("valid CA rotation did not hot-reload: %w", err)
	}

	// Invalid rotation: the watcher must reject the garbage PEM and keep
	// the last-known-good trust.
	if err := rotate(caPath, []byte("not a certificate")); err != nil {
		return err
	}
	time.Sleep(1500 * time.Millisecond)
	if err := get(ctx, client, urlNew); err != nil {
		return fmt.Errorf("last-known-good trust lost after invalid rotation: %w", err)
	}

	// Second valid rotation (to a third CA).
	if err := rotate(caPath, thirdCAPEM); err != nil {
		return err
	}
	if err := waitForTrust(ctx, client, urlThird); err != nil {
		return fmt.Errorf("watcher stopped processing events after invalid rotation: %w", err)
	}
	return nil
}

// startExplicitTransport wires the production TLS_CA_FILE contract: env ->
// DefaultBaseTransport (CAReloader singleton) -> StartCAFileWatcher. The
// returned stop function stops the watcher and its context.
func startExplicitTransport(caPath string) (rt http.RoundTripper, stop func(), err error) {
	if err := os.Setenv("TLS_CA_FILE", caPath); err != nil {
		return nil, nil, fmt.Errorf("set TLS_CA_FILE: %w", err)
	}
	transport, err := sharedtls.DefaultBaseTransport()
	if err != nil {
		return nil, nil, fmt.Errorf("default base transport: %w", err)
	}
	if _, ok := transport.(*sharedtls.CAReloader); !ok {
		return nil, nil, errors.New("TLS_CA_FILE transport is not a CAReloader")
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	watcher, err := sharedtls.StartCAFileWatcher(watchCtx, logr.Discard())
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("start CA file watcher: %w", err)
	}
	if watcher == nil {
		cancel()
		return nil, nil, errors.New("CA file watcher did not start")
	}
	return transport, func() {
		watcher.Stop()
		cancel()
	}, nil
}

// waitForTrust polls until the client successfully reaches url, proving the
// reloaded pool is live on real HTTPS round trips.
func waitForTrust(ctx context.Context, client *http.Client, url string) error {
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := get(ctx, client, url)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func generateCA(commonName string) (caMaterial, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return caMaterial{}, fmt.Errorf("generate CA key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return caMaterial{}, fmt.Errorf("create CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return caMaterial{}, fmt.Errorf("parse CA certificate: %w", err)
	}
	return caMaterial{key: key, cert: cert, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

func startServer(dir, name string, ca caMaterial) (*http.Server, string, error) {
	certDir := filepath.Join(dir, name+"-server")
	if err := os.Mkdir(certDir, 0755); err != nil {
		return nil, "", fmt.Errorf("create server cert dir: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generate server key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, "", fmt.Errorf("create server certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, "", fmt.Errorf("marshal server key: %w", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return nil, "", fmt.Errorf("write server certificate: %w", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return nil, "", fmt.Errorf("write server key: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	server := &http.Server{Handler: mux}
	isTLS, _, err := sharedtls.ConfigureRequiredTLS(server, certDir)
	if err != nil {
		return nil, "", fmt.Errorf("configure server TLS: %w", err)
	}
	if !isTLS {
		return nil, "", errors.New("server TLS not enabled")
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", server.TLSConfig)
	if err != nil {
		return nil, "", fmt.Errorf("listen: %w", err)
	}
	go func() { _ = server.Serve(listener) }()
	addr := listener.Addr().(*net.TCPAddr)
	return server, fmt.Sprintf("https://127.0.0.1:%d", addr.Port), nil
}

func rotate(path string, pemBytes []byte) error {
	staged := path + ".staged"
	if err := os.WriteFile(staged, pemBytes, 0644); err != nil {
		return fmt.Errorf("stage rotated CA: %w", err)
	}
	if err := os.Rename(staged, path); err != nil {
		return fmt.Errorf("rename rotated CA into place: %w", err)
	}
	return nil
}

func get(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
