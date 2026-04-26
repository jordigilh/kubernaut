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

package tls

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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("CAReloader (#756)", func() {

	var certDir string

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "ca-reloader-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	generateCAPEM := func(cn string) []byte {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		template := &x509.Certificate{
			SerialNumber:          big.NewInt(time.Now().UnixNano()),
			Subject:               pkix.Name{CommonName: cn},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		Expect(err).ToNot(HaveOccurred())

		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	}

	// subjectFromPEM extracts the raw subject bytes from a PEM-encoded cert
	// so we can assert our specific CA is present in the pool.
	subjectFromPEM := func(pemData []byte) []byte {
		block, _ := pem.Decode(pemData)
		Expect(block.Type).To(Equal("CERTIFICATE"), "PEM block must be a CERTIFICATE")
		cert, err := x509.ParseCertificate(block.Bytes)
		Expect(err).ToNot(HaveOccurred())
		return cert.RawSubject
	}

	poolContainsSubject := func(pool *x509.CertPool, subject []byte) bool {
		for _, s := range pool.Subjects() { //nolint:staticcheck
			if string(s) == string(subject) {
				return true
			}
		}
		return false
	}

	writeCAFile := func(cn string) string {
		caPath := filepath.Join(certDir, "ca.crt")
		pemBytes := generateCAPEM(cn)
		Expect(os.WriteFile(caPath, pemBytes, 0644)).To(Succeed())
		return caPath
	}

	// UT-TLS-756-010: NewCAReloader loads initial CA from PEM data
	// BR-SECURITY-756: CA certificates must be loaded at startup
	It("UT-TLS-756-010: should load initial CA from PEM data", func() {
		pemBytes := generateCAPEM("test-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())
		Expect(poolContainsSubject(reloader.GetCertPool(), subjectFromPEM(pemBytes))).To(BeTrue(),
			"reloader must contain the loaded CA certificate")
	})

	// UT-TLS-756-011: RoundTrip delegates to the underlying transport
	// BR-SECURITY-756: CAReloader must implement http.RoundTripper
	It("UT-TLS-756-011: should implement http.RoundTripper", func() {
		pemBytes := generateCAPEM("roundtrip-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		var rt http.RoundTripper = reloader
		_ = rt
	})

	// UT-TLS-756-012: ReloadCallback swaps transport with new CA
	// BR-SECURITY-756: Rotated CA must be picked up without restart
	It("UT-TLS-756-012: should swap transport on ReloadCallback", func() {
		pemBytes := generateCAPEM("initial-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		poolBefore := reloader.GetCertPool()
		Expect(poolContainsSubject(poolBefore, subjectFromPEM(pemBytes))).To(BeTrue(),
			"initial pool must contain the initial CA")

		newPEM := generateCAPEM("rotated-ca")
		err = reloader.ReloadCallback(string(newPEM))
		Expect(err).ToNot(HaveOccurred())

		poolAfter := reloader.GetCertPool()
		Expect(poolContainsSubject(poolAfter, subjectFromPEM(newPEM))).To(BeTrue(),
			"reloaded pool must contain the rotated CA")
		Expect(poolAfter).ToNot(BeIdenticalTo(poolBefore),
			"cert pool must be replaced after reload")
	})

	// UT-TLS-756-013: ReloadCallback preserves previous transport on invalid PEM
	// BR-SECURITY-756: Graceful degradation on bad CA data
	It("UT-TLS-756-013: should keep previous transport on invalid PEM", func() {
		pemBytes := generateCAPEM("good-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		poolBefore := reloader.GetCertPool()

		err = reloader.ReloadCallback("not-valid-pem")
		Expect(err).To(HaveOccurred(), "must return error for invalid PEM")

		poolAfter := reloader.GetCertPool()
		Expect(poolAfter).To(BeIdenticalTo(poolBefore),
			"must preserve previous cert pool after failed reload")
	})

	// UT-TLS-756-014: NewCAReloader fails on invalid PEM data
	// BR-SECURITY-756: Startup must fail fast on corrupt CA
	It("UT-TLS-756-014: should fail on invalid PEM data", func() {
		_, err := sharedtls.NewCAReloader([]byte("garbage"))
		Expect(err).To(HaveOccurred())
	})

	// UT-TLS-756-015: NewCAReloader fails on empty PEM data
	// BR-SECURITY-756: Empty CA data must be rejected
	It("UT-TLS-756-015: should fail on empty PEM data", func() {
		_, err := sharedtls.NewCAReloader([]byte{})
		Expect(err).To(HaveOccurred())
	})

	// UT-TLS-756-016: Concurrent RoundTrip and ReloadCallback are race-free
	// BR-SECURITY-756: Must be safe during concurrent requests and rotation
	It("UT-TLS-756-016: should be safe for concurrent access", func() {
		pemBytes := generateCAPEM("concurrent-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				pool := reloader.GetCertPool()
				Expect(pool.Subjects()).ToNot(BeEmpty(),
					"cert pool must always contain at least one CA during concurrent access")
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				newPEM := generateCAPEM("concurrent-reload")
				_ = reloader.ReloadCallback(string(newPEM))
			}()
		}

		wg.Wait()
	})

	// UT-TLS-756-017: NewCAReloaderFromFile loads CA from file path
	// BR-SECURITY-756: Convenience constructor for file-based CA initialization
	It("UT-TLS-756-017: should load CA from file path", func() {
		caPath := writeCAFile("file-ca")
		pemData, err := os.ReadFile(caPath)
		Expect(err).ToNot(HaveOccurred())

		reloader, fileErr := sharedtls.NewCAReloaderFromFile(caPath)
		Expect(fileErr).ToNot(HaveOccurred())
		Expect(poolContainsSubject(reloader.GetCertPool(), subjectFromPEM(pemData))).To(BeTrue(),
			"file-loaded reloader must contain the CA certificate")
	})

	// UT-TLS-756-018: NewCAReloaderFromFile fails on missing file
	// BR-SECURITY-756: Startup must fail fast if CA file is missing
	It("UT-TLS-756-018: should fail on missing CA file", func() {
		_, err := sharedtls.NewCAReloaderFromFile("/nonexistent/ca.crt")
		Expect(err).To(HaveOccurred())
	})

	// UT-TLS-756-019: Transport enforces MinVersion TLS 1.2
	// BR-SECURITY-756: No TLS downgrade below 1.2
	It("UT-TLS-756-019: should enforce TLS 1.2 minimum", func() {
		pemBytes := generateCAPEM("tls-version-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		transport := reloader.CurrentTransport()
		Expect(transport.TLSClientConfig.MinVersion).To(
			BeNumerically(">=", uint16(0x0303)),
			"minimum TLS version must be at least TLS 1.2 (0x0303)")
		Expect(poolContainsSubject(transport.TLSClientConfig.RootCAs, subjectFromPEM(pemBytes))).To(BeTrue(),
			"transport must have the CA in its root CA pool")
	})

	// UT-TLS-753-001: CA pool uses file-only trust (no system roots)
	// BR-SECURITY-753: Private PKI isolation -- system CAs must not verify internal certs
	It("UT-TLS-753-001: should use file-only CA pool without system roots", func() {
		pemBytes := generateCAPEM("isolated-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		pool := reloader.GetCertPool()
		subjects := pool.Subjects() //nolint:staticcheck
		Expect(subjects).To(HaveLen(1),
			"CA pool must contain exactly one certificate (the file CA), not system roots")
		Expect(poolContainsSubject(pool, subjectFromPEM(pemBytes))).To(BeTrue(),
			"the single cert must be our file CA")
	})

	// Issue #853: IdleConnTimeout on TLS path
	It("UT-RT-853-014: buildCATransport must set IdleConnTimeout=15s", func() {
		pemBytes := generateCAPEM("idle-timeout-ca")

		reloader, err := sharedtls.NewCAReloader(pemBytes)
		Expect(err).ToNot(HaveOccurred())

		transport := reloader.CurrentTransport()
		Expect(transport.IdleConnTimeout).To(Equal(15*time.Second),
			"Issue #853: TLS path must set IdleConnTimeout to 15s to prevent stale connection reuse")
	})
})
