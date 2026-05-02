/*
Copyright 2026 Jordi Gil.

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

package workflowexecution

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// Issue #902: AWX TLS CA Transport Tests
// ========================================
// Authority: Issue #902 (WFE: AWX HTTP client does not use TLS_CA_FILE CA)
// These tests verify that NewAWXHTTPClient uses sharedtls.DefaultBaseTransport()
// (which picks up TLS_CA_FILE) instead of cloning http.DefaultTransport directly.
// ========================================

var _ = Describe("AWXHTTPClient TLS CA (Issue #902)", func() {

	Context("NewAWXHTTPClient constructor", func() {
		It("UT-WE-902-001: should return client and nil error for valid inputs", func() {
			client, err := executor.NewAWXHTTPClient("https://awx.example.com", "test-token")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).To(BeAssignableToTypeOf(&executor.AWXHTTPClient{}))
		})

		It("UT-WE-902-002: should return error for empty base URL", func() {
			client, err := executor.NewAWXHTTPClient("", "test-token")
			Expect(err).To(HaveOccurred())
			Expect(client).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("baseURL"))
		})

		It("UT-WE-902-003: should return error for empty token", func() {
			client, err := executor.NewAWXHTTPClient("https://awx.example.com", "")
			Expect(err).To(HaveOccurred())
			Expect(client).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("token"))
		})
	})

	Context("TLS CA file integration", func() {
		var (
			caDir  string
			caPath string
			caKey  *ecdsa.PrivateKey
			caCert *x509.Certificate
		)

		BeforeEach(func() {
			var err error
			caDir, err = os.MkdirTemp("", "awx-tls-test-*")
			Expect(err).ToNot(HaveOccurred())
			caPath = filepath.Join(caDir, "ca.crt")

			caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).ToNot(HaveOccurred())

			caTemplate := &x509.Certificate{
				SerialNumber:          big.NewInt(1),
				Subject:               pkix.Name{CommonName: "Test CA"},
				NotBefore:             time.Now().Add(-1 * time.Hour),
				NotAfter:              time.Now().Add(24 * time.Hour),
				KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
				BasicConstraintsValid: true,
				IsCA:                  true,
			}

			caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
			Expect(err).ToNot(HaveOccurred())

			caCert, err = x509.ParseCertificate(caCertDER)
			Expect(err).ToNot(HaveOccurred())

			caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
			Expect(os.WriteFile(caPath, caPEM, 0644)).To(Succeed())
		})

		AfterEach(func() {
			_ = os.RemoveAll(caDir)
		})

		It("UT-WE-902-004: should connect to TLS server using TLS_CA_FILE CA", func() {
			serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).ToNot(HaveOccurred())

			serverTemplate := &x509.Certificate{
				SerialNumber: big.NewInt(2),
				Subject:      pkix.Name{CommonName: "localhost"},
				NotBefore:    time.Now().Add(-1 * time.Hour),
				NotAfter:     time.Now().Add(24 * time.Hour),
				KeyUsage:     x509.KeyUsageDigitalSignature,
				ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
				DNSNames:     []string{"localhost"},
			}

			serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
			Expect(err).ToNot(HaveOccurred())

			serverTLSCert := tls.Certificate{
				Certificate: [][]byte{serverCertDER},
				PrivateKey:  serverKey,
			}

			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"results": [{"id": 42}], "count": 1}`))
			}))
			server.TLS = &tls.Config{
				Certificates: []tls.Certificate{serverTLSCert},
				MinVersion:   tls.VersionTLS12,
			}
			server.StartTLS()
			defer server.Close()

			// Set TLS_CA_FILE before constructing the client so that
			// DefaultBaseTransport picks up our test CA via CAReloader.
			origVal, wasSet := os.LookupEnv("TLS_CA_FILE")
			Expect(os.Setenv("TLS_CA_FILE", caPath)).To(Succeed())
			DeferCleanup(func() {
				if wasSet {
					Expect(os.Setenv("TLS_CA_FILE", origVal)).To(Succeed())
				} else {
					Expect(os.Unsetenv("TLS_CA_FILE")).To(Succeed())
				}
			})

			client, err := executor.NewAWXHTTPClient(server.URL, "test-token")
			Expect(err).ToNot(HaveOccurred())

			templateID, err := client.FindJobTemplateByName(context.Background(), "test-template")
			Expect(err).ToNot(HaveOccurred())
			Expect(templateID).To(Equal(42))
		})
	})

	Context("HTTP compatibility", func() {
		It("UT-WE-902-005: should work with plain HTTP servers", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v2/job_templates/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"results": [{"id": 42, "name": "test-template"}], "count": 1}`))
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			client, err := executor.NewAWXHTTPClient(server.URL, "test-token")
			Expect(err).ToNot(HaveOccurred())

			templateID, err := client.FindJobTemplateByName(context.Background(), "test-template")
			Expect(err).ToNot(HaveOccurred())
			Expect(templateID).To(Equal(42))
		})
	})
})
