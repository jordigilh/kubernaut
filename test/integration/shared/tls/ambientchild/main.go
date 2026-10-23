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
	"time"

	"github.com/go-logr/logr"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("expected private CA, private key, and public HTTPS endpoint")
	}

	if err := sharedtls.InjectAmbientCACerts(logr.Discard(), args[0]); err != nil {
		return fmt.Errorf("ambient CA bootstrap failed: %w", err)
	}
	if os.Getenv("SSL_CERT_FILE") != "" {
		return fmt.Errorf("SSL_CERT_FILE must be empty after ambient bootstrap")
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("load system roots: %w", err)
	}
	if len(roots.Subjects()) == 0 {
		return fmt.Errorf("system root pool is empty")
	}
	custom, err := firstCertificate(args[0])
	if err != nil {
		return fmt.Errorf("read custom CA: %w", err)
	}
	if _, err := custom.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return fmt.Errorf("custom CA is not trusted: %w", err)
	}
	server, endpoint, err := startPrivateServer(custom, args[1])
	if err != nil {
		return fmt.Errorf("start private HTTPS server: %w", err)
	}
	defer func() { _ = server.Close() }()

	client := &http.Client{Timeout: 20 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	privateResponse, err := getHTTPS(ctx, client, endpoint)
	if err != nil {
		return fmt.Errorf("private HTTPS request failed: %w", err)
	}
	_ = privateResponse.Body.Close()
	if privateResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("private HTTPS request returned status %d", privateResponse.StatusCode)
	}

	publicResponse, err := getHTTPS(ctx, client, args[2])
	if err != nil {
		return fmt.Errorf("public HTTPS request failed: %w", err)
	}
	_ = publicResponse.Body.Close()
	if publicResponse.StatusCode < http.StatusOK || publicResponse.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("public HTTPS request returned status %d", publicResponse.StatusCode)
	}
	return nil
}

func getHTTPS(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create HTTPS request: %w", err)
	}
	return client.Do(request)
}

func startPrivateServer(ca *x509.Certificate, keyPath string) (*http.Server, string, error) {
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, "", fmt.Errorf("read private CA key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, "", fmt.Errorf("private CA key has no PEM block")
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse private CA key: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generate server key: %w", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, "", fmt.Errorf("create server certificate: %w", err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal server key: %w", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	keyPair, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, "", fmt.Errorf("load server key pair: %w", err)
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		return nil, "", fmt.Errorf("listen for private HTTPS server: %w", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "private HTTPS server stopped unexpectedly: %v\n", err)
		}
	}()
	return server, "https://" + listener.Addr().String(), nil
}

func firstCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM certificate in %s", path)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate in %s: %w", path, err)
	}
	return certificate, nil
}
