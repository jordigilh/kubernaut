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
	"fmt"
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

var _ = Describe("TLS Integration Tests (#493)", func() {

	var (
		certDir    string
		caCertPath string
		caKeyPath  string
		caKey      *ecdsa.PrivateKey
		caCert     *x509.Certificate
	)

	// Generate a self-signed CA and server cert for each test
	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "tls-it-*")
		Expect(err).ToNot(HaveOccurred())

		// Generate CA
		caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		caTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}

		caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		Expect(err).ToNot(HaveOccurred())
		caCert, err = x509.ParseCertificate(caCertDER)
		Expect(err).ToNot(HaveOccurred())

		caCertPath = filepath.Join(certDir, "ca.crt")
		caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
		Expect(os.WriteFile(caCertPath, caCertPEM, 0644)).To(Succeed())

		caKeyDER, err := x509.MarshalECPrivateKey(caKey)
		Expect(err).ToNot(HaveOccurred())
		caKeyPath = filepath.Join(certDir, "ca.key")
		caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})
		Expect(os.WriteFile(caKeyPath, caKeyPEM, 0600)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	// Helper: generate server cert signed by the CA
	generateServerCert := func() (certFile, keyFile string) {
		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "localhost"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{"localhost"},
		}

		srvCertDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)
		Expect(err).ToNot(HaveOccurred())

		certFile = filepath.Join(certDir, "tls.crt")
		srvCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvCertDER})
		Expect(os.WriteFile(certFile, srvCertPEM, 0644)).To(Succeed())

		keyFile = filepath.Join(certDir, "tls.key")
		srvKeyDER, err := x509.MarshalECPrivateKey(srvKey)
		Expect(err).ToNot(HaveOccurred())
		srvKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: srvKeyDER})
		Expect(os.WriteFile(keyFile, srvKeyPEM, 0600)).To(Succeed())

		return certFile, keyFile
	}

	// Helper: start a TLS server on a random port and return its URL
	startTLSServer := func() (*http.Server, string) {
		generateServerCert()

		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		server := &http.Server{Handler: mux}
		isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(isTLS).To(BeTrue())

		listener, err := tls.Listen("tcp", "127.0.0.1:0", server.TLSConfig)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			_ = server.Serve(listener)
		}()

		addr := listener.Addr().(*net.TCPAddr)
		return server, fmt.Sprintf("https://127.0.0.1:%d", addr.Port)
	}

	// IT-TLS-493-001: HTTPS request to server with valid cert succeeds
	It("IT-TLS-493-001: should accept HTTPS request with valid client CA", func() {
		server, url := startTLSServer()
		defer func() { _ = server.Close() }()

		transport, err := sharedtls.NewTLSTransport(caCertPath)
		Expect(err).ToNot(HaveOccurred())

		client := &http.Client{Transport: transport}
		resp, err := client.Get(url + "/health")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	// IT-TLS-493-002: Plain HTTP fails when server only speaks TLS
	It("IT-TLS-493-002: should reject plain HTTP when server runs TLS", func() {
		server, url := startTLSServer()
		defer func() { _ = server.Close() }()

		plainURL := "http" + url[5:] // Replace "https" with "http"
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(plainURL + "/health")
		if err != nil {
			// Connection-level failure: error must relate to TLS/HTTP protocol mismatch
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("tls"),
				ContainSubstring("http"),
				ContainSubstring("EOF"),
				ContainSubstring("connection reset"),
			), "error should indicate protocol mismatch")
		} else {
			// Server responded with HTTP 400 ("Client sent an HTTP request to an HTTPS server")
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		}
	})

	// IT-TLS-493-003: HTTPS request with correct CA succeeds (same as 001 but verifies handshake)
	It("IT-TLS-493-003: should complete TLS handshake with correct CA", func() {
		server, url := startTLSServer()
		defer func() { _ = server.Close() }()

		pool, err := sharedtls.LoadCACert(caCertPath)
		Expect(err).ToNot(HaveOccurred())

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		}
		client := &http.Client{Transport: transport}
		resp, err := client.Get(url + "/health")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	// IT-TLS-493-004: Client with wrong CA cert fails TLS handshake
	It("IT-TLS-493-004: should fail TLS handshake with wrong CA", func() {
		server, url := startTLSServer()
		defer func() { _ = server.Close() }()

		// Generate a different CA (not the one that signed the server cert)
		wrongKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		wrongTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(99),
			Subject:               pkix.Name{CommonName: "Wrong CA"},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
		}
		wrongCertDER, err := x509.CreateCertificate(rand.Reader, wrongTemplate, wrongTemplate, &wrongKey.PublicKey, wrongKey)
		Expect(err).ToNot(HaveOccurred())
		wrongCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: wrongCertDER})
		wrongCAPath := filepath.Join(certDir, "wrong-ca.crt")
		Expect(os.WriteFile(wrongCAPath, wrongCertPEM, 0644)).To(Succeed())

		transport, err := sharedtls.NewTLSTransport(wrongCAPath)
		Expect(err).ToNot(HaveOccurred())
		client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

		_, err = client.Get(url + "/health")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
	})

	// IT-TLS-493-005: Client using default system trust rejects server cert signed by private CA
	It("IT-TLS-493-005: should fail when client uses system trust for private CA server", func() {
		server, url := startTLSServer()
		defer func() { _ = server.Close() }()

		client := &http.Client{Timeout: 2 * time.Second}
		_, err := client.Get(url + "/health")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("certificate"))
	})
})
