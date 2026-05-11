package tlswiring_test

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
	"testing"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tlswiring"
)

func TestConfigureServer_NoCertDir(t *testing.T) {
	t.Parallel()
	srv := &http.Server{}
	enabled, reloader, err := tlswiring.ConfigureServer(srv, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected TLS disabled when certDir is empty")
	}
	if reloader != nil {
		t.Fatal("expected nil reloader when TLS disabled")
	}
	if srv.TLSConfig != nil {
		t.Fatal("expected nil TLSConfig when TLS disabled")
	}
}

func TestConfigureServer_NonExistentDir(t *testing.T) {
	t.Parallel()
	srv := &http.Server{}
	enabled, reloader, err := tlswiring.ConfigureServer(srv, "/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected TLS disabled when cert files don't exist")
	}
	if reloader != nil {
		t.Fatal("expected nil reloader when certs missing")
	}
}

func TestConfigureServer_ValidCerts(t *testing.T) {
	t.Parallel()
	certDir := generateTestCerts(t)

	srv := &http.Server{}
	enabled, reloader, err := tlswiring.ConfigureServer(srv, certDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected TLS enabled with valid certificates")
	}
	if reloader == nil {
		t.Fatal("expected non-nil reloader")
	}
	if srv.TLSConfig == nil {
		t.Fatal("expected TLSConfig to be set")
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected MinVersion TLS 1.2, got %d", srv.TLSConfig.MinVersion)
	}
}

func TestConfigureServer_InvalidCerts(t *testing.T) {
	t.Parallel()
	certDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(certDir, "tls.crt"), []byte("not a cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "tls.key"), []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := &http.Server{}
	_, _, err := tlswiring.ConfigureServer(srv, certDir)
	if err == nil {
		t.Fatal("expected error with invalid certificates")
	}
}

func TestOutboundTransport_Empty(t *testing.T) {
	t.Parallel()
	rt, err := tlswiring.OutboundTransport("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != nil {
		t.Fatal("expected nil transport when caFile is empty")
	}
}

func TestOutboundTransport_ValidCA(t *testing.T) {
	t.Parallel()
	caFile := generateTestCA(t)

	rt, err := tlswiring.OutboundTransport(caFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil transport with valid CA")
	}
	transport, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", rt)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set")
	}
	if transport.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be set")
	}
}

func TestOutboundTransport_InvalidCA(t *testing.T) {
	t.Parallel()
	caFile := filepath.Join(t.TempDir(), "bad-ca.crt")
	if err := os.WriteFile(caFile, []byte("not a CA"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := tlswiring.OutboundTransport(caFile)
	if err == nil {
		t.Fatal("expected error with invalid CA file")
	}
}

func TestOutboundTransport_NonExistentFile(t *testing.T) {
	t.Parallel()
	_, err := tlswiring.OutboundTransport("/nonexistent/ca.crt")
	if err == nil {
		t.Fatal("expected error with non-existent CA file")
	}
}

func TestConfigureServer_Concurrent(t *testing.T) {
	t.Parallel()
	certDir := generateTestCerts(t)
	const goroutines = 10

	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			srv := &http.Server{}
			_, _, err := tlswiring.ConfigureServer(srv, certDir)
			errCh <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

func TestOutboundTransport_Concurrent(t *testing.T) {
	t.Parallel()
	caFile := generateTestCA(t)
	const goroutines = 10

	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := tlswiring.OutboundTransport(caFile)
			errCh <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// generateTestCerts creates a self-signed cert+key pair in a temp directory.
func generateTestCerts(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// generateTestCA creates a self-signed CA certificate in a temp file.
func generateTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return caFile
}
