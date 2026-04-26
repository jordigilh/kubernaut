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

package tls

import (
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
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("Shared TLS Helper (#493)", func() {

	var (
		certDir  string
		certPath string
		keyPath  string
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "tls-test-*")
		Expect(err).ToNot(HaveOccurred())
		certPath = filepath.Join(certDir, "tls.crt")
		keyPath = filepath.Join(certDir, "tls.key")
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	// Helper: generate self-signed cert for tests
	generateSelfSignedCert := func(certFile, keyFile string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "localhost"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{"localhost"},
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		Expect(err).ToNot(HaveOccurred())

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		Expect(os.WriteFile(certFile, certPEM, 0644)).To(Succeed())

		keyDER, err := x509.MarshalECPrivateKey(key)
		Expect(err).ToNot(HaveOccurred())
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		Expect(os.WriteFile(keyFile, keyPEM, 0600)).To(Succeed())
	}

	Describe("ConditionalTLS", func() {

		// UT-TLS-493-001: ConditionalTLS starts HTTPS when cert files exist
		It("UT-TLS-493-001: should start HTTPS when cert files exist", func() {
			generateSelfSignedCert(certPath, keyPath)

			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue(), "should detect TLS certs and configure TLS")

			cert, certErr := server.TLSConfig.GetCertificate(&tls.ClientHelloInfo{})
			Expect(certErr).ToNot(HaveOccurred())
			Expect(cert.Certificate).To(HaveLen(1), "TLS cert chain should contain exactly one certificate")
		})

		// UT-TLS-493-002: ConditionalTLS starts HTTP when cert files don't exist
		It("UT-TLS-493-002: should start HTTP when cert files don't exist", func() {
			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeFalse(), "no certs, should remain plain HTTP")
			Expect(server.TLSConfig).To(BeNil(), "TLSConfig must remain nil when no certs exist")
		})

		// UT-TLS-493-003: ConditionalTLS fails gracefully on invalid cert
		It("UT-TLS-493-003: should return error on invalid cert content", func() {
			Expect(os.WriteFile(certPath, []byte("not-a-cert"), 0644)).To(Succeed())
			Expect(os.WriteFile(keyPath, []byte("not-a-key"), 0600)).To(Succeed())

			server := &http.Server{Addr: ":0"}
			_, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("LoadCACert", func() {

		// UT-TLS-493-004: LoadCACert loads valid PEM file
		It("UT-TLS-493-004: should load a valid CA PEM file", func() {
			generateSelfSignedCert(certPath, keyPath)

			pool, err := sharedtls.LoadCACert(certPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(pool.Subjects()).ToNot(BeEmpty(), "CA pool should contain at least one certificate subject")
		})

		// UT-TLS-493-005: LoadCACert returns error on missing file
		It("UT-TLS-493-005: should return error for missing CA file", func() {
			_, err := sharedtls.LoadCACert("/nonexistent/ca.crt")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("NewTLSTransport", func() {

		// UT-TLS-493-006: NewTLSTransport builds transport with custom CA pool
		It("UT-TLS-493-006: should build transport with custom CA pool", func() {
			generateSelfSignedCert(certPath, keyPath)

			transport, err := sharedtls.NewTLSTransport(certPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(transport.TLSClientConfig.RootCAs.Subjects()).ToNot(BeEmpty(),
				"transport CA pool should contain the loaded CA certificate")
		})
	})

	Describe("DefaultBaseTransport (#753)", func() {

		AfterEach(func() {
			sharedtls.ResetDefaultTransportForTesting()
			os.Unsetenv("TLS_CA_FILE")
		})

		// UT-TLS-753-002: DefaultBaseTransport retries after initial failure
		// BR-SECURITY-753: Transient CA file unavailability must not cause permanent failure
		It("UT-TLS-753-002: should retry after initial CA file failure", func() {
			os.Setenv("TLS_CA_FILE", "/nonexistent/ca.crt")

			_, err := sharedtls.DefaultBaseTransport()
			Expect(err).To(HaveOccurred(), "first call must fail when CA file is missing")

			generateSelfSignedCert(certPath, keyPath)
			os.Setenv("TLS_CA_FILE", certPath)

			rt, err := sharedtls.DefaultBaseTransport()
			Expect(err).ToNot(HaveOccurred(), "second call must succeed after CA file becomes available")
			_, isCAReloader := rt.(*sharedtls.CAReloader)
			Expect(isCAReloader).To(BeTrue(), "must return *CAReloader when TLS_CA_FILE is valid")
		})

		// UT-TLS-753-003: DefaultBaseTransport returns plain transport when TLS_CA_FILE unset
		It("UT-TLS-753-003: should return plain transport when TLS_CA_FILE is unset", func() {
			os.Unsetenv("TLS_CA_FILE")

			rt, err := sharedtls.DefaultBaseTransport()
			Expect(err).ToNot(HaveOccurred())
			_, isPlain := rt.(*http.Transport)
			Expect(isPlain).To(BeTrue(), "must return plain *http.Transport when no CA file")
		})
	})

	Describe("Config Parsing", func() {

		// UT-TLS-493-007: TLSConfig accessors (Enabled, CertPath, KeyPath)
		It("UT-TLS-493-007: should report Enabled and return correct CertPath/KeyPath", func() {
			cfg := sharedtls.TLSConfig{
				CertDir: "/etc/kubernaut-tls",
			}
			Expect(cfg.Enabled()).To(BeTrue())
			Expect(cfg.CertPath()).To(Equal("/etc/kubernaut-tls/tls.crt"))
			Expect(cfg.KeyPath()).To(Equal("/etc/kubernaut-tls/tls.key"))
		})

		// UT-TLS-493-008: TLSConfig with empty CertDir means disabled
		It("UT-TLS-493-008: should report disabled when CertDir is empty", func() {
			cfg := sharedtls.TLSConfig{}
			Expect(cfg.Enabled()).To(BeFalse())
		})
	})

	// Issue #853: IdleConnTimeout reduction from 90s to 15s
	Describe("IdleConnTimeout (#853)", func() {
		It("UT-RT-853-011: DefaultBaseTransport returns IdleConnTimeout=15s (non-TLS)", func() {
			os.Unsetenv("TLS_CA_FILE")
			sharedtls.ResetDefaultTransportForTesting()

			rt, err := sharedtls.DefaultBaseTransport()
			Expect(err).ToNot(HaveOccurred())

			plainTransport, ok := rt.(*http.Transport)
			Expect(ok).To(BeTrue(), "non-TLS path must return *http.Transport")
			Expect(plainTransport.IdleConnTimeout).To(Equal(15*time.Second),
				"Issue #853: IdleConnTimeout must be 15s to prevent stale connection reuse after pod rescheduling")
		})
	})

	Describe("DefaultBaseTransportWithRetry (#853)", func() {
		It("UT-RT-853-016: wraps DefaultBaseTransport with RetryTransport", func() {
			os.Unsetenv("TLS_CA_FILE")
			sharedtls.ResetDefaultTransportForTesting()

			rt, err := sharedtls.DefaultBaseTransportWithRetry()
			Expect(err).ToNot(HaveOccurred())
			Expect(rt).ToNot(BeNil(), "must return a non-nil RoundTripper")

			// The returned transport should NOT be a plain *http.Transport
			// (it should be wrapped by RetryTransport)
			_, isPlain := rt.(*http.Transport)
			Expect(isPlain).To(BeFalse(),
				"DefaultBaseTransportWithRetry must wrap with RetryTransport, not return plain transport")
		})
	})
})
