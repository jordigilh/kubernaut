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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("CertReloader (#756)", func() {

	var (
		certDir  string
		certPath string
		keyPath  string
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "cert-reloader-test-*")
		Expect(err).ToNot(HaveOccurred())
		certPath = filepath.Join(certDir, "tls.crt")
		keyPath = filepath.Join(certDir, "tls.key")
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	generateCert := func(cn string) (*ecdsa.PrivateKey, []byte) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		template := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{"localhost"},
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		Expect(err).ToNot(HaveOccurred())
		return key, certDER
	}

	writeCertAndKey := func(cn string) {
		key, certDER := generateCert(cn)

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		Expect(os.WriteFile(certPath, certPEM, 0644)).To(Succeed())

		keyDER, err := x509.MarshalECPrivateKey(key)
		Expect(err).ToNot(HaveOccurred())
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		Expect(os.WriteFile(keyPath, keyPEM, 0600)).To(Succeed())
	}

	// UT-TLS-756-001: NewCertReloader loads initial certificate from disk
	// BR-SECURITY-756: Server certificates must be loaded at startup
	It("UT-TLS-756-001: should load initial certificate from disk", func() {
		writeCertAndKey("initial-cert")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		Expect(cert.Certificate).To(HaveLen(1),
			"newly created reloader must serve exactly one certificate")
	})

	// UT-TLS-756-002: GetCertificate returns loaded certificate for TLS handshake
	// BR-SECURITY-756: tls.Config.GetCertificate must serve current cert
	It("UT-TLS-756-002: should return certificate via GetCertificate", func() {
		writeCertAndKey("get-cert-test")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		Expect(cert.Certificate).ToNot(BeEmpty(), "certificate chain must contain at least one cert")

		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(leaf.Subject.CommonName).To(Equal("get-cert-test"))
	})

	// UT-TLS-756-003: ReloadCallback re-reads both files from disk after rotation
	// BR-SECURITY-756: Certificate rotation must be picked up without restart
	It("UT-TLS-756-003: should reload rotated certificate from disk", func() {
		writeCertAndKey("before-rotation")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(leaf.Subject.CommonName).To(Equal("before-rotation"))

		writeCertAndKey("after-rotation")

		err = reloader.ReloadCallback("ignored-content")
		Expect(err).ToNot(HaveOccurred())

		cert, err = reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		leaf, err = x509.ParseCertificate(cert.Certificate[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(leaf.Subject.CommonName).To(Equal("after-rotation"),
			"GetCertificate must serve the rotated certificate after ReloadCallback")
	})

	// UT-TLS-756-004: ReloadCallback preserves previous cert on invalid new cert
	// BR-SECURITY-756: Graceful degradation -- bad cert must not break TLS
	It("UT-TLS-756-004: should keep previous cert when reload fails", func() {
		writeCertAndKey("good-cert")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		Expect(os.WriteFile(certPath, []byte("not-a-cert"), 0644)).To(Succeed())
		Expect(os.WriteFile(keyPath, []byte("not-a-key"), 0600)).To(Succeed())

		err = reloader.ReloadCallback("ignored")
		Expect(err).To(HaveOccurred(), "ReloadCallback must return error for invalid cert")

		cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(leaf.Subject.CommonName).To(Equal("good-cert"),
			"must preserve previous good certificate after failed reload")
	})

	// UT-TLS-756-005: NewCertReloader fails on missing cert file
	// BR-SECURITY-756: Startup must fail fast if certs are missing
	It("UT-TLS-756-005: should fail on missing cert file", func() {
		_, err := sharedtls.NewCertReloader("/nonexistent/tls.crt", "/nonexistent/tls.key")
		Expect(err).To(HaveOccurred())
	})

	// UT-TLS-756-006: NewCertReloader fails on invalid cert content
	// BR-SECURITY-756: Startup must fail fast on corrupt certificates
	It("UT-TLS-756-006: should fail on invalid cert content at startup", func() {
		Expect(os.WriteFile(certPath, []byte("garbage"), 0644)).To(Succeed())
		Expect(os.WriteFile(keyPath, []byte("garbage"), 0600)).To(Succeed())

		_, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).To(HaveOccurred())
	})

	// UT-TLS-756-007: Concurrent GetCertificate and ReloadCallback are race-free
	// BR-SECURITY-756: Must be safe for concurrent TLS handshakes during rotation
	It("UT-TLS-756-007: should be safe for concurrent access", func() {
		writeCertAndKey("concurrent-test")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		for i := 0; i < goroutines; i++ {
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
				Expect(err).ToNot(HaveOccurred())
				Expect(cert.Certificate).ToNot(BeEmpty(),
					"concurrent GetCertificate must always return a valid certificate chain")
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				_ = reloader.ReloadCallback("ignored")
			}()
		}

		wg.Wait()
	})

	// UT-TLS-756-008: ReloadCallback preserves previous cert when cert file disappears
	// BR-SECURITY-756: Transient file absence during rotation must not break TLS
	It("UT-TLS-756-008: should preserve cert when file temporarily disappears", func() {
		writeCertAndKey("stable-cert")

		reloader, err := sharedtls.NewCertReloader(certPath, keyPath)
		Expect(err).ToNot(HaveOccurred())

		Expect(os.Remove(certPath)).To(Succeed())

		err = reloader.ReloadCallback("ignored")
		Expect(err).To(HaveOccurred(), "ReloadCallback must error when cert file is missing")

		cert, err := reloader.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).ToNot(HaveOccurred())
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(leaf.Subject.CommonName).To(Equal("stable-cert"),
			"must serve previous cert when file disappears")
	})
})
