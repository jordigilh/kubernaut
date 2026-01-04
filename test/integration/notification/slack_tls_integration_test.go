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

package notification

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

// ========================================
// TLS Certificate Validation Integration Tests
// Moved from unit tests per triage recommendation
// BR-NOT-058: Security Error Handling & Policy
// ========================================
//
// These tests validate that TLS certificate errors are classified as
// permanent failures (non-retryable) per security policy.
//
// Security Rationale:
// - TLS errors indicate misconfiguration or security issues
// - Automatic retry would bypass security validation
// - Operations team should be alerted immediately
//
// Test Infrastructure:
// - Uses httptest with custom TLS configurations
// - Simulates real TLS error scenarios
// - Validates error classification behavior
// ========================================

var _ = Describe("Slack Delivery TLS Certificate Validation (Integration)", Label("integration", "slack", "tls", "security"), func() {
	var (
		ctx          context.Context
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create test notification fixture
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tls-test-%d", time.Now().UnixNano()),
				Namespace: testNamespace,
				Generation: 1, // K8s increments on create/update
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "TLS Certificate Test",
				Body:     "Testing TLS certificate validation behavior",
				Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelSlack},
				Recipients: []notificationv1alpha1.Recipient{
					{Slack: "#tls-test"},
				},
			},
		}
	})

	// ===== INTEGRATION TEST 1: Self-Signed Certificate =====
	Context("when Slack webhook uses self-signed certificate", func() {
		It("should classify TLS error as permanent failure (BR-NOT-058: Security Error)", func() {
			// BR-NOT-058: Security Error Handling
			// Self-signed certificates indicate:
			// - Development/test environment misconfiguration
			// - Potential MITM attack in production
			// - Must NOT be retried automatically

			// Create self-signed certificate
			tlsCert, err := createSelfSignedCert()
			Expect(err).ToNot(HaveOccurred(), "Failed to create self-signed certificate")

			// Create httptest server with self-signed certificate
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Slack API successful response (won't be reached due to TLS error)
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))
			server.TLS = &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
			}
			server.StartTLS()
			defer server.Close()

			// Create Slack delivery service with self-signed TLS endpoint
			service := delivery.NewSlackDeliveryService(server.URL)

			// Attempt delivery - should fail with TLS error
			err = service.Deliver(ctx, notification)

			// Verify error occurred
			Expect(err).To(HaveOccurred(),
				"Delivery should fail with self-signed certificate")

			// Verify error is NOT retryable (BR-NOT-058: Security Policy)
			Expect(delivery.IsRetryableError(err)).To(BeFalse(),
				"Self-signed certificate errors should NOT be retryable (BR-NOT-058)")

			// Verify error message indicates TLS failure
			Expect(err.Error()).To(ContainSubstring("TLS certificate validation failed"),
				"Error should indicate TLS certificate validation failure")
		})
	})

	// ===== INTEGRATION TEST 2: Expired Certificate =====
	Context("when Slack webhook certificate is expired", func() {
		It("should classify TLS error as permanent failure (BR-NOT-058: Security Error)", func() {
			// BR-NOT-058: Security Error Handling
			// Expired certificates indicate:
			// - Certificate not renewed properly
			// - Security vulnerability (expired cert = no validation)
			// - Must NOT be retried automatically

			// Create expired certificate
			tlsCert, err := createExpiredCert()
			Expect(err).ToNot(HaveOccurred(), "Failed to create expired certificate")

			// Create httptest server with expired certificate
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Slack API successful response (won't be reached due to TLS error)
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))
			server.TLS = &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
			}
			server.StartTLS()
			defer server.Close()

			// Create Slack delivery service with expired TLS endpoint
			service := delivery.NewSlackDeliveryService(server.URL)

			// Attempt delivery - should fail with TLS error
			err = service.Deliver(ctx, notification)

			// Verify error occurred
			Expect(err).To(HaveOccurred(),
				"Delivery should fail with expired certificate")

			// Verify error is NOT retryable (BR-NOT-058: Security Policy)
			Expect(delivery.IsRetryableError(err)).To(BeFalse(),
				"Expired certificate errors should NOT be retryable (BR-NOT-058)")

			// Verify error message indicates TLS failure
			Expect(err.Error()).To(ContainSubstring("TLS certificate validation failed"),
				"Error should indicate TLS certificate validation failure")
		})
	})

	// ===== INTEGRATION TEST 3: Certificate Hostname Mismatch =====
	Context("when Slack webhook certificate hostname doesn't match", func() {
		It("should classify TLS error as permanent failure (BR-NOT-058: Security Error)", func() {
			// BR-NOT-058: Security Error Handling
			// Hostname mismatch indicates:
			// - Certificate issued for wrong domain
			// - Potential MITM attack
			// - DNS misconfiguration
			// - Must NOT be retried automatically

			// Create certificate with wrong hostname
			tlsCert, err := createCertWithWrongHostname("localhost")
			Expect(err).ToNot(HaveOccurred(), "Failed to create certificate with wrong hostname")

			// Create httptest server with mismatched hostname certificate
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Slack API successful response (won't be reached due to TLS error)
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))
			server.TLS = &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
			}
			server.StartTLS()
			defer server.Close()

			// Create Slack delivery service with mismatched hostname TLS endpoint
			service := delivery.NewSlackDeliveryService(server.URL)

			// Attempt delivery - should fail with TLS error
			err = service.Deliver(ctx, notification)

			// Verify error occurred
			Expect(err).To(HaveOccurred(),
				"Delivery should fail with hostname mismatch")

			// Verify error is NOT retryable (BR-NOT-058: Security Policy)
			Expect(delivery.IsRetryableError(err)).To(BeFalse(),
				"Hostname mismatch errors should NOT be retryable (BR-NOT-058)")

			// Verify error message indicates TLS failure
			Expect(err.Error()).To(ContainSubstring("TLS certificate validation failed"),
				"Error should indicate TLS certificate validation failure")
		})
	})

	// ===== INTEGRATION TEST 4: Valid TLS Certificate (Control Test) =====
	Context("when Slack webhook uses valid TLS certificate", func() {
		It("should successfully deliver notification (BR-NOT-058: Security Baseline)", func() {
			// BR-NOT-058: Security Baseline
			// Valid TLS certificates should allow normal operation
			// This is the control test to verify TLS validation doesn't
			// break legitimate HTTPS connections

			// Create httptest TLS server (uses valid localhost certificate)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Slack API successful response
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))
			defer server.Close()

			// Create Slack delivery service with client that trusts httptest certificate
			// Note: In production, we use system trust store. For testing with httptest,
			// we configure the client to trust the test server's certificate.
			service := delivery.NewSlackDeliveryService(server.URL)

			// Configure the service's HTTP client to trust the test server's certificate
			// This simulates a legitimate HTTPS connection in production
			service.SetHTTPClient(server.Client())

			// Attempt delivery - should succeed with valid certificate
			err := service.Deliver(ctx, notification)

			// Verify successful delivery
			Expect(err).ToNot(HaveOccurred(),
				"Delivery should succeed with valid TLS certificate (BR-NOT-058)")
		})
	})

	// ===== INTEGRATION TEST 5: TLS Handshake Failure =====
	Context("when TLS handshake fails", func() {
		It("should classify TLS handshake error as permanent failure (BR-NOT-058: Security Error)", func() {
			// BR-NOT-058: Security Error Handling
			// TLS handshake failures indicate:
			// - Incompatible TLS versions
			// - Cipher suite mismatch
			// - Protocol errors
			// - Must NOT be retried automatically

			// Note: This test simulates a TLS handshake failure scenario
			// In Go 1.21+, creating truly incompatible TLS configs is challenging
			// This test validates that if a handshake fails for any reason,
			// the error is classified as non-retryable

			// Create self-signed certificate (will cause handshake issues)
			tlsCert, err := createSelfSignedCert()
			Expect(err).ToNot(HaveOccurred(), "Failed to create certificate")

			// Create httptest server with restrictive TLS config
			// This creates conditions that may cause handshake issues
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
					GinkgoWriter.Printf("Mock server failed to write response: %v\n", err)
				}
			}))
			server.TLS = &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
				MinVersion:   tls.VersionTLS13,
				MaxVersion:   tls.VersionTLS13,
			}
			server.StartTLS()
			defer server.Close()

			// Create Slack delivery service
			service := delivery.NewSlackDeliveryService(server.URL)

			// Attempt delivery - should fail with TLS error
			err = service.Deliver(ctx, notification)

			// Verify error occurred
			Expect(err).To(HaveOccurred(),
				"Delivery should fail with TLS handshake/validation error")

			// Verify error is NOT retryable (BR-NOT-058: Security Policy)
			Expect(delivery.IsRetryableError(err)).To(BeFalse(),
				"TLS handshake/validation errors should NOT be retryable (BR-NOT-058)")

			// Verify error message indicates TLS failure
			Expect(err.Error()).To(ContainSubstring("TLS certificate validation failed"),
				"Error should indicate TLS certificate validation failure")
		})
	})
})

// ========================================
// Helper Functions for TLS Integration Tests
// ========================================

// createSelfSignedCert creates a self-signed certificate for testing
// This certificate will trigger x509.UnknownAuthorityError because it's not signed by a trusted CA
func createSelfSignedCert() (*tls.Certificate, error) {
	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Self-Signed Org"},
			CommonName:   "localhost",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Create self-signed certificate (both issuer and subject are the same)
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate from PEM-encoded cert and key
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 key pair: %w", err)
	}

	return &tlsCert, nil
}

// createExpiredCert creates an expired certificate for testing
// This certificate will trigger x509.CertificateInvalidError due to expiration
func createExpiredCert() (*tls.Certificate, error) {
	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template with dates in the past
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Expired Cert Org"},
			CommonName:   "localhost",
		},
		// Certificate expired 30 days ago
		NotBefore: time.Now().Add(-60 * 24 * time.Hour), // Started 60 days ago
		NotAfter:  time.Now().Add(-30 * 24 * time.Hour), // Expired 30 days ago

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Create self-signed expired certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate from PEM-encoded cert and key
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 key pair: %w", err)
	}

	return &tlsCert, nil
}

// createCertWithWrongHostname creates a certificate for a different hostname
// This certificate will trigger x509.HostnameError when used with a different hostname
func createCertWithWrongHostname(correctHostname string) (*tls.Certificate, error) {
	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template with WRONG hostname
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Use a completely different hostname than what will be requested
	wrongHostname := "wrong.example.com"
	if correctHostname == "wrong.example.com" {
		wrongHostname = "different.example.com"
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Wrong Hostname Org"},
			CommonName:   wrongHostname,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		// Only include the WRONG hostname, not the correct one
		DNSNames: []string{wrongHostname},
	}

	// Create self-signed certificate with wrong hostname
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Create tls.Certificate from PEM-encoded cert and key
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 key pair: %w", err)
	}

	return &tlsCert, nil
}
