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
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// GAP-08 (kubernaut#1505): buildReplayCache selects the distributed Valkey
// backend when configured, and must fall back to the in-memory cache rather
// than disabling replay protection outright when the backend is unreachable.

func TestBuildReplayCache_NilConfigLegacyDisabled_ReturnsNil(t *testing.T) {
	rc := buildReplayCache(nil, false, logr.Discard())
	if rc != nil {
		t.Errorf("expected nil replay cache when config is nil and legacy flag disabled, got %T", rc)
	}
}

func TestBuildReplayCache_NilConfigLegacyEnabled_ReturnsInMemory(t *testing.T) {
	rc := buildReplayCache(nil, true, logr.Discard())
	defer rc.Stop()
	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected legacy enableReplayProtection=true to construct an in-memory ReplayCache, got %T", rc)
	}
}

func TestBuildReplayCache_MemoryBackend_ReturnsInMemory(t *testing.T) {
	rc := buildReplayCache(&config.ReplayCacheConfig{Backend: "memory"}, false, logr.Discard())
	defer rc.Stop()
	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected backend=memory to construct an in-memory ReplayCache, got %T", rc)
	}
}

func TestBuildReplayCache_RedisBackend_ReturnsValkeyReplayCache(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ValkeyReplayCache); !ok {
		t.Errorf("expected backend=redis to construct a ValkeyReplayCache, got %T", rc)
	}
	// Sanity check the constructed cache is actually wired to the running instance.
	if rc.Seen("jti-wiring-check") {
		t.Error("expected a fresh jti to not be seen yet")
	}
	if !rc.Seen("jti-wiring-check") {
		t.Error("expected the same jti to be detected as seen on the second call")
	}
}

func TestBuildReplayCache_RedisBackendUnreachable_FallsBackToInMemory(t *testing.T) {
	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: "127.0.0.1:1", // nothing listens here — connection refused
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected an unreachable redis backend to fall back to in-memory ReplayCache, got %T", rc)
	}
}

func TestLoadReplayCachePassword_EmptyPath_ReturnsEmptyNoError(t *testing.T) {
	password, err := loadReplayCachePassword("")
	if err != nil {
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if password != "" {
		t.Errorf("expected empty password for empty path, got %q", password)
	}
}

func TestLoadReplayCachePassword_ValidFile_ReturnsPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valkey-secrets.yaml")
	if err := os.WriteFile(path, []byte("password: s3cr3t\n"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	password, err := loadReplayCachePassword(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if password != "s3cr3t" {
		t.Errorf("expected password %q, got %q", "s3cr3t", password)
	}
}

func TestLoadReplayCachePassword_MissingPasswordKey_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valkey-secrets.yaml")
	if err := os.WriteFile(path, []byte("username: admin\n"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if _, err := loadReplayCachePassword(path); err == nil {
		t.Error("expected an error when the password key is missing")
	}
}

func TestLoadReplayCachePassword_MissingFile_ReturnsError(t *testing.T) {
	if _, err := loadReplayCachePassword(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Error("expected an error for a missing credentials file")
	}
}

// generateSelfSignedCertPair writes a self-signed CA/leaf cert (usable as
// both server cert and its own trust root, mirroring the pattern in
// pkg/shared/tls/tls_test.go) to certFile/keyFile.
func generateSelfSignedCertPair(t *testing.T, certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("failed to write cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
}

// DD-PLATFORM-006 DA9: Decision Area 8 makes Valkey TLS-only, but the
// replay-cache Valkey client had zero TLS support -- these tests prove the
// new config.ReplayCacheConfig.TLS block is actually wired into a real TLS
// handshake against a TLS-only Valkey, not just parsed and dropped.

func TestBuildReplayCache_TLSEnabled_ConnectsOverTLS(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	generateSelfSignedCertPair(t, certFile, keyFile)

	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("failed to load server cert: %v", err)
	}
	mr, err := miniredis.RunTLS(&tls.Config{Certificates: []tls.Certificate{serverCert}}) //nolint:gosec // test-only min version default is fine
	if err != nil {
		t.Fatalf("failed to start TLS miniredis: %v", err)
	}
	defer mr.Close()

	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
		TLS: &config.ReplayCacheTLSConfig{
			Enabled: true,
			CAFile:  certFile, // self-signed cert doubles as its own CA
		},
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ValkeyReplayCache); !ok {
		t.Fatalf("expected a TLS-enabled config to still construct a ValkeyReplayCache (successful TLS handshake), got %T", rc)
	}
	if rc.Seen("jti-tls-wiring-check") {
		t.Error("expected a fresh jti to not be seen yet")
	}
	if !rc.Seen("jti-tls-wiring-check") {
		t.Error("expected the same jti to be detected as seen on the second call")
	}
}

func TestBuildReplayCache_TLSEnabledWrongCA_FallsBackToInMemory(t *testing.T) {
	serverDir := t.TempDir()
	serverCertFile := filepath.Join(serverDir, "tls.crt")
	serverKeyFile := filepath.Join(serverDir, "tls.key")
	generateSelfSignedCertPair(t, serverCertFile, serverKeyFile)

	otherDir := t.TempDir()
	wrongCAFile := filepath.Join(otherDir, "other.crt")
	wrongKeyFile := filepath.Join(otherDir, "other.key")
	generateSelfSignedCertPair(t, wrongCAFile, wrongKeyFile)

	serverCert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		t.Fatalf("failed to load server cert: %v", err)
	}
	mr, err := miniredis.RunTLS(&tls.Config{Certificates: []tls.Certificate{serverCert}}) //nolint:gosec // test-only min version default is fine
	if err != nil {
		t.Fatalf("failed to start TLS miniredis: %v", err)
	}
	defer mr.Close()

	// wrongCAFile does not trust the server's actual cert, so the TLS
	// handshake must fail closed into the in-memory fallback -- exactly like
	// the existing "unreachable backend" fallback test above.
	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
		TLS: &config.ReplayCacheTLSConfig{
			Enabled: true,
			CAFile:  wrongCAFile,
		},
	}, false, logr.Discard())
	defer rc.Stop()

	if _, ok := rc.(*auth.ReplayCache); !ok {
		t.Errorf("expected a TLS handshake failure (untrusted CA) to fall back to in-memory ReplayCache, got %T", rc)
	}
}

func TestBuildReplayCache_UnauthenticatedValkeyStillWorks(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rc := buildReplayCache(&config.ReplayCacheConfig{
		Backend:   "redis",
		RedisAddr: mr.Addr(),
		RedisDB:   0,
	}, false, logr.Discard())
	defer rc.Stop()

	if rc.MissingJTI("") != true {
		t.Error("expected MissingJTI(\"\") to be true")
	}
}
