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
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
)

// ========================================
// SOC2 Day 9.1: Digital Signature Implementation
// Authority: SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md - Day 9.1
// ========================================
//
// Signs audit export data with RSA-SHA256 for tamper detection.
//
// SOC2 Requirements:
// - CC8.1: Audit Export with digital signatures
// - AU-9: Protection of Audit Information (tamper-evident)
//
// Signature Algorithm: SHA256withRSA (industry standard)
// Key Size: 2048-bit RSA (NIST recommended minimum)
//
// ========================================

// Signer provides digital signature capabilities for audit exports
type Signer struct {
	cert       *x509.Certificate
	privateKey *rsa.PrivateKey
}

// NewSignerFromTLSCertificate creates a Signer from a tls.Certificate
// Used in production when loading from cert-manager managed Secret
func NewSignerFromTLSCertificate(tlsCert *tls.Certificate) (*Signer, error) {
	if len(tlsCert.Certificate) == 0 {
		return nil, fmt.Errorf("TLS certificate has no certificate chain")
	}

	// Parse X.509 certificate
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse X.509 certificate: %w", err)
	}

	// Extract RSA private key
	privateKey, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("certificate private key is not RSA (got %T)", tlsCert.PrivateKey)
	}

	return &Signer{
		cert:       cert,
		privateKey: privateKey,
	}, nil
}

// NewSignerFromPEM creates a Signer from PEM-encoded certificate and key
// Used in testing and development
func NewSignerFromPEM(certPEM, keyPEM []byte) (*Signer, error) {
	// Parse certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
	}

	return &Signer{
		cert:       cert,
		privateKey: privateKey,
	}, nil
}

// Sign signs the provided data with SHA256-RSA
// Returns base64-encoded signature
//
// BR-AUDIT-007: Digital signature support for audit exports
// SOC2 CC8.1: Tamper-evident audit logs
func (s *Signer) Sign(data interface{}) (string, error) {
	// Serialize data to JSON (canonical form for consistent signing)
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data for signing: %w", err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(dataJSON)

	// Sign hash with RSA private key
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign data: %w", err)
	}

	// Encode signature as base64 (standard format for HTTP/JSON)
	signatureBase64 := base64.StdEncoding.EncodeToString(signature)

	return signatureBase64, nil
}

// Verify verifies a signature against the provided data
// Used for signature verification tools (Day 9.2)
func (s *Signer) Verify(data interface{}, signatureBase64 string) error {
	// Serialize data to JSON (must match signing serialization)
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data for verification: %w", err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(dataJSON)

	// Decode base64 signature
	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Verify signature using public key
	err = rsa.VerifyPKCS1v15(&s.privateKey.PublicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	return nil
}

// GetCertificateFingerprint returns SHA256 fingerprint of the signing certificate
// Used for export metadata
func (s *Signer) GetCertificateFingerprint() string {
	fingerprint := sha256.Sum256(s.cert.Raw)

	// Format as colon-separated hex (standard fingerprint format)
	formatted := ""
	for i, b := range fingerprint {
		if i > 0 {
			formatted += ":"
		}
		formatted += fmt.Sprintf("%02x", b)
	}

	return formatted
}

// GetAlgorithm returns the signature algorithm name
func (s *Signer) GetAlgorithm() string {
	return "SHA256withRSA"
}

// GetCertificate returns the X.509 certificate used for signing
func (s *Signer) GetCertificate() *x509.Certificate {
	return s.cert
}

