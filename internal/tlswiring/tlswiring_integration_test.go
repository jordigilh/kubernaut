//go:build integration

package tlswiring_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tlswiring"
)

func TestTLSHandshake_EndToEnd(t *testing.T) {
	t.Parallel()
	certDir := generateTestCerts(t)
	caFile := filepath.Join(certDir, "tls.crt")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	srv := &http.Server{Handler: mux}
	enabled, _, err := tlswiring.ConfigureServer(srv, certDir)
	if err != nil {
		t.Fatalf("ConfigureServer: %v", err)
	}
	if !enabled {
		t.Fatal("expected TLS to be enabled")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, srv.TLSConfig)
	defer func() { _ = srv.Close() }()

	go func() { _ = srv.Serve(tlsLn) }()

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("https://%s/healthz", ln.Addr().String()))
	if err != nil {
		t.Fatalf("TLS GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("expected 'ok', got %q", body)
	}
}

func TestOutboundTransport_ConnectsToTLSServer(t *testing.T) {
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

	rt, err := tlswiring.OutboundTransport(caFile)
	if err != nil {
		t.Fatalf("OutboundTransport: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil transport")
	}

	client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://%s/ping", ln.Addr().String()))
	if err != nil {
		t.Fatalf("outbound TLS GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("expected 'pong', got %q", body)
	}
}

func TestOutboundTransport_RejectsUntrustedServer(t *testing.T) {
	t.Parallel()
	serverCertDir := generateTestCerts(t)
	differentCA := generateTestCA(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "should not reach")
	})

	srv := &http.Server{Handler: mux}
	enabled, _, err := tlswiring.ConfigureServer(srv, serverCertDir)
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

	rt, err := tlswiring.OutboundTransport(differentCA)
	if err != nil {
		t.Fatalf("OutboundTransport: %v", err)
	}

	client := &http.Client{Transport: rt, Timeout: 5 * time.Second}
	_, err = client.Get(fmt.Sprintf("https://%s/", ln.Addr().String()))
	if err == nil {
		t.Fatal("expected TLS verification failure when CA doesn't match")
	}
}

func TestStartCertFileWatcher_NilReloader(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher, err := tlswiring.StartCertFileWatcher(ctx, "/some/dir", nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if watcher != nil {
		t.Fatal("expected nil watcher with nil reloader")
	}
}

func TestStartCertFileWatcher_EmptyCertDir(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher, err := tlswiring.StartCertFileWatcher(ctx, "", nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if watcher != nil {
		t.Fatal("expected nil watcher with empty certDir")
	}
}
