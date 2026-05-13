package tlswiring_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
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

func TestCheckPartialTLSMaterial_BothPresent(t *testing.T) {
	t.Parallel()
	dir := generateTestCerts(t)
	if warn := tlswiring.CheckPartialTLSMaterial(dir); warn != "" {
		t.Fatalf("expected no warning with both files, got: %s", warn)
	}
}

func TestCheckPartialTLSMaterial_NeitherPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if warn := tlswiring.CheckPartialTLSMaterial(dir); warn != "" {
		t.Fatalf("expected no warning with neither file, got: %s", warn)
	}
}

func TestCheckPartialTLSMaterial_OnlyCert(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	warn := tlswiring.CheckPartialTLSMaterial(dir)
	if warn == "" {
		t.Fatal("expected warning when only tls.crt exists")
	}
}

func TestCheckPartialTLSMaterial_OnlyKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	warn := tlswiring.CheckPartialTLSMaterial(dir)
	if warn == "" {
		t.Fatal("expected warning when only tls.key exists")
	}
}

func TestCheckPartialTLSMaterial_EmptyDir(t *testing.T) {
	t.Parallel()
	if warn := tlswiring.CheckPartialTLSMaterial(""); warn != "" {
		t.Fatalf("expected no warning with empty dir, got: %s", warn)
	}
}

func TestCAReloadableTransport_EmptyCA(t *testing.T) {
	t.Parallel()
	rt, watcher, err := tlswiring.CAReloadableTransport("", testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if rt != nil {
		t.Fatal("expected nil transport")
	}
	if watcher != nil {
		t.Fatal("expected nil watcher")
	}
}

func TestCAReloadableTransport_ValidCA(t *testing.T) {
	t.Parallel()
	caFile := generateTestCA(t)
	rt, watcher, err := tlswiring.CAReloadableTransport(caFile, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil transport")
	}
	if watcher == nil {
		t.Fatal("expected non-nil watcher")
	}
}

func TestCAReloadableTransport_InvalidCA(t *testing.T) {
	t.Parallel()
	caFile := filepath.Join(t.TempDir(), "bad-ca.crt")
	if err := os.WriteFile(caFile, []byte("not a cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := tlswiring.CAReloadableTransport(caFile, testLogger())
	if err == nil {
		t.Fatal("expected error with invalid CA")
	}
}

func TestCAReloadableTransport_NonExistentFile(t *testing.T) {
	t.Parallel()
	_, _, err := tlswiring.CAReloadableTransport("/nonexistent/ca.crt", testLogger())
	if err == nil {
		t.Fatal("expected error with non-existent file")
	}
}

func TestCAReloadableTransport_RoundTrip(t *testing.T) {
	// Business outcome: an HTTP request goes through the CAReloadableTransport
	// and reaches the target TLS server using the loaded CA certificate.
	t.Parallel()
	certDir := generateTestCerts(t)
	caFile := filepath.Join(certDir, "tls.crt")

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "pong")
	})

	srv := &http.Server{Handler: mux}
	enabled, _, err := tlswiring.ConfigureServer(srv, certDir)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("expected TLS enabled")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, srv.TLSConfig)
	defer func() { _ = srv.Close() }()
	go func() { _ = srv.Serve(tlsLn) }()

	rt, watcher, err := tlswiring.CAReloadableTransport(caFile, testLogger())
	if err != nil {
		t.Fatalf("CAReloadableTransport: %v", err)
	}
	if watcher == nil {
		t.Fatal("expected non-nil watcher")
	}

	client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://%s/ping", ln.Addr().String()))
	if err != nil {
		t.Fatalf("GET through CAReloadableTransport failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("expected 'pong', got %q", body)
	}
}

func TestValidateCAFilePath_Empty(t *testing.T) {
	t.Parallel()
	if err := tlswiring.ValidateCAFilePath("test", ""); err != nil {
		t.Fatalf("expected nil error for empty path, got: %v", err)
	}
}

func TestValidateCAFilePath_Valid(t *testing.T) {
	t.Parallel()
	caFile := generateTestCA(t)
	if err := tlswiring.ValidateCAFilePath("test", caFile); err != nil {
		t.Fatalf("expected nil error for valid CA, got: %v", err)
	}
}

func TestValidateCAFilePath_Invalid(t *testing.T) {
	t.Parallel()
	caFile := filepath.Join(t.TempDir(), "bad.crt")
	if err := os.WriteFile(caFile, []byte("not a cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := tlswiring.ValidateCAFilePath("test", caFile); err == nil {
		t.Fatal("expected error for invalid CA")
	}
}

func TestValidateCAFilePath_NonExistent(t *testing.T) {
	t.Parallel()
	if err := tlswiring.ValidateCAFilePath("test", "/nonexistent/ca.crt"); err == nil {
		t.Fatal("expected error for non-existent file")
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
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
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

func testLogger() logr.Logger {
	return logr.Discard()
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
