package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BR-SECURITY-042: AF must trust non-public CAs for OIDC provider TLS.

func writeTestCA(t *testing.T, dir string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ingress-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	path := filepath.Join(dir, "ca.crt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode PEM: %v", err)
	}
	return path
}

func TestBuildOIDCHTTPClient_ValidCA(t *testing.T) {
	// UT-AF-1245-001: Valid CA file produces a working HTTP client with custom root pool.
	dir := t.TempDir()
	caPath := writeTestCA(t, dir)

	client, err := buildOIDCHTTPClient(caPath)
	if err != nil {
		t.Fatalf("buildOIDCHTTPClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("RootCAs is nil, expected custom CA pool")
	}
}

func TestBuildOIDCHTTPClient_MissingFile(t *testing.T) {
	// UT-AF-1245-002: Missing CA file returns a clear error.
	_, err := buildOIDCHTTPClient("/nonexistent/path/ca.crt")
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
	if !strings.Contains(err.Error(), "reading OIDC CA file") {
		t.Errorf("error = %q, want to contain 'reading OIDC CA file'", err.Error())
	}
}

func TestBuildOIDCHTTPClient_InvalidPEM(t *testing.T) {
	// UT-AF-1245-003: File exists but contains no valid PEM certs.
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.crt")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := buildOIDCHTTPClient(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if !strings.Contains(err.Error(), "no valid certificates") {
		t.Errorf("error = %q, want to contain 'no valid certificates'", err.Error())
	}
}

func TestBuildOIDCHTTPClient_TLSMinVersion(t *testing.T) {
	// UT-AF-1245-004: TLS config enforces minimum TLS 1.2.
	dir := t.TempDir()
	caPath := writeTestCA(t, dir)

	client, err := buildOIDCHTTPClient(caPath)
	if err != nil {
		t.Fatalf("buildOIDCHTTPClient() error = %v", err)
	}
	tr := client.Transport.(*http.Transport)
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d (TLS 1.2)", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
}

func TestBuildOIDCHTTPClient_EmptyFile(t *testing.T) {
	// UT-AF-1245-005: Zero-byte file returns a clear error.
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.crt")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := buildOIDCHTTPClient(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}
