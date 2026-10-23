package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

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
	if len(args) != 2 {
		return fmt.Errorf("expected private CA and system bundle paths")
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
	system, err := firstCertificate(args[1])
	if err != nil {
		return fmt.Errorf("read system CA: %w", err)
	}
	if _, err := system.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return fmt.Errorf("system CA is not trusted: %w", err)
	}
	return nil
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
