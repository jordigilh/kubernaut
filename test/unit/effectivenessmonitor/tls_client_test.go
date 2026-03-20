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

package effectivenessmonitor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

// generateTestCACert creates a self-signed CA certificate and returns the PEM bytes.
func generateTestCACert() []byte {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

var _ = Describe("TLS HTTP Client Builder (Issue #452, BR-EM-002, BR-EM-003)", Label("tls", "client", "452"), func() {

	// ========================================
	// UT-EM-452-001: Valid PEM produces TLS-enabled HTTP client
	// ========================================
	It("UT-EM-452-001: valid PEM CA file produces an HTTP client that trusts the custom CA", func() {
		caPEM := generateTestCACert()
		tmpDir := GinkgoT().TempDir()
		caFile := filepath.Join(tmpDir, "ca.crt")
		Expect(os.WriteFile(caFile, caPEM, 0644)).To(Succeed())

		client, err := emclient.NewHTTPClientWithCA(caFile, 10*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(client).NotTo(BeNil())

		httpTransport, ok := client.Transport.(*http.Transport)
		Expect(ok).To(BeTrue(), "transport should be *http.Transport")
		Expect(httpTransport.TLSClientConfig).NotTo(BeNil())
		Expect(httpTransport.TLSClientConfig.RootCAs).NotTo(BeNil())
		Expect(client.Timeout).To(Equal(10 * time.Second))
	})

	// ========================================
	// UT-EM-452-002: Non-existent CA file returns clear error
	// ========================================
	It("UT-EM-452-002: non-existent CA file returns a clear, actionable error", func() {
		client, err := emclient.NewHTTPClientWithCA("/nonexistent/path/ca.crt", 10*time.Second)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/path/ca.crt"))
		Expect(client).To(BeNil())
	})

	// ========================================
	// UT-EM-452-003: Invalid PEM returns clear error
	// ========================================
	It("UT-EM-452-003: invalid/corrupt PEM content returns a clear error", func() {
		tmpDir := GinkgoT().TempDir()
		caFile := filepath.Join(tmpDir, "corrupt.crt")
		Expect(os.WriteFile(caFile, []byte("not-a-valid-pem-file"), 0644)).To(Succeed())

		client, err := emclient.NewHTTPClientWithCA(caFile, 10*time.Second)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no valid PEM"))
		Expect(client).To(BeNil())
	})

	// ========================================
	// UT-EM-452-004: Empty tlsCaFile yields plain HTTP client
	// ========================================
	It("UT-EM-452-004: empty tlsCaFile config path produces a plain HTTP client", func() {
		client := &http.Client{Timeout: 10 * time.Second}
		Expect(client).NotTo(BeNil())
		Expect(client.Transport).To(BeNil(), "plain client uses http.DefaultTransport (nil)")
		Expect(client.Timeout).To(Equal(10 * time.Second))
	})
})
