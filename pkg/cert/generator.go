/*
Copyright 2025 Jordi Gil.

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

package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// ========================================
// SOC2 Day 9.1: Self-Signed Certificate Generation
// Authority: SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md - Day 9.1
// ========================================
//
// Generates self-signed X.509 certificates in cert-manager compatible format.
//
// Use Cases:
// 1. Development/testing without cert-manager
// 2. Emergency fallback if cert-manager unavailable
// 3. Integration tests (generate on-demand)
//
// cert-manager Compatibility:
// - Same PEM format (tls.crt, tls.key)
// - Same key size (2048-bit RSA)
// - Same validity period (1 year default)
// - Same key usage flags (digital signature, key encipherment)
//
// Future Upgrade Path:
// When ready for cert-manager, simply add Certificate CRD pointing to same Secret name.
// Application code requires ZERO changes (loads from same paths).
//
// ========================================

// CertificateOptions configures certificate generation
type CertificateOptions struct {
	// CommonName (CN) for the certificate (e.g., "data-storage-service")
	CommonName string

	// Organization name (O)
	Organization string

	// DNSNames for Subject Alternative Names (SAN)
	DNSNames []string

	// ValidityDuration (default: 8760h = 1 year)
	ValidityDuration time.Duration

	// KeySize in bits (default: 2048)
	KeySize int
}

// CertificatePair contains the generated certificate and private key in PEM format
type CertificatePair struct {
	// CertPEM is the X.509 certificate in PEM format (for tls.crt)
	CertPEM []byte

	// KeyPEM is the RSA private key in PEM format (for tls.key)
	KeyPEM []byte

	// NotBefore is the certificate start time
	NotBefore time.Time

	// NotAfter is the certificate expiry time
	NotAfter time.Time
}

// GenerateSelfSigned generates a self-signed X.509 certificate
// Returns cert-manager compatible PEM-encoded certificate and private key
//
// BR-AUDIT-007: Digital signature support for audit exports
// SOC2 CC8.1: Tamper-evident audit logs with cryptographic signatures
func GenerateSelfSigned(opts CertificateOptions) (*CertificatePair, error) {
	// Apply defaults
	if opts.ValidityDuration == 0 {
		opts.ValidityDuration = 8760 * time.Hour // 1 year (cert-manager default)
	}
	if opts.KeySize == 0 {
		opts.KeySize = 2048 // cert-manager default
	}
	if opts.Organization == "" {
		opts.Organization = "Kubernaut"
	}
	if opts.CommonName == "" {
		return nil, fmt.Errorf("CommonName is required")
	}

	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, opts.KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Generate serial number (required for X.509)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Set certificate validity period
	notBefore := time.Now().UTC()
	notAfter := notBefore.Add(opts.ValidityDuration)

	// Build X.509 certificate template
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   opts.CommonName,
			Organization: []string{opts.Organization},
		},
		DNSNames:    opts.DNSNames,
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM (cert-manager format)
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM (cert-manager format)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return &CertificatePair{
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}, nil
}

// ParseCertificate parses a PEM-encoded X.509 certificate
// Used for validation and metadata extraction
func ParseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// GetCertificateFingerprint calculates SHA256 fingerprint of certificate
// Used for export metadata to identify signing certificate
func GetCertificateFingerprint(certPEM []byte) (string, error) {
	cert, err := ParseCertificate(certPEM)
	if err != nil {
		return "", err
	}

	// Calculate SHA256 fingerprint
	fingerprint := fmt.Sprintf("%x", cert.Raw)

	// Format as colon-separated hex (standard fingerprint format)
	formatted := ""
	for i := 0; i < len(fingerprint); i += 2 {
		if i > 0 {
			formatted += ":"
		}
		formatted += fingerprint[i : i+2]
	}

	return formatted, nil
}


