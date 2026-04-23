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

package authwebhook

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("BR-INTEGRATION-750: authwebhook DSClientAdapter inter-service TLS trust (#750)", func() {

	var (
		origTLSCAFile string
		hadTLSCAFile  bool
	)

	BeforeEach(func() {
		sharedtls.ResetDefaultTransportForTesting()
		origTLSCAFile, hadTLSCAFile = os.LookupEnv("TLS_CA_FILE")
	})

	AfterEach(func() {
		if hadTLSCAFile {
			os.Setenv("TLS_CA_FILE", origTLSCAFile)
		} else {
			os.Unsetenv("TLS_CA_FILE")
		}
		sharedtls.ResetDefaultTransportForTesting()
	})

	generateSelfSignedCA := func(certFile string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "test-ca"},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		Expect(err).ToNot(HaveOccurred())

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
		Expect(os.WriteFile(certFile, certPEM, 0644)).To(Succeed())
	}

	// UT-AW-750-001: Constructor must fail when TLS_CA_FILE points to a nonexistent path.
	// This proves the constructor reads the env var (plain http.Transport would ignore it).
	It("UT-AW-750-001: returns error when TLS_CA_FILE points to invalid path", func() {
		os.Setenv("TLS_CA_FILE", "/nonexistent/ca.crt")

		adapter, err := authwebhook.NewDSClientAdapter(
			"https://localhost:9999", 5*time.Second, logr.Discard(),
		)
		Expect(err).To(HaveOccurred(), "constructor must propagate DefaultBaseTransport error")
		Expect(err.Error()).To(ContainSubstring("failed to read CA certificate"),
			"error should originate from TLS CA loading")
		Expect(adapter).To(BeNil())
	})

	// UT-AW-750-002: Constructor must succeed when TLS_CA_FILE points to a valid CA cert.
	It("UT-AW-750-002: succeeds when TLS_CA_FILE points to valid CA cert", func() {
		tmpDir, err := os.MkdirTemp("", "tls-test-750-*")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		caFile := filepath.Join(tmpDir, "ca.crt")
		generateSelfSignedCA(caFile)
		os.Setenv("TLS_CA_FILE", caFile)

		adapter, err := authwebhook.NewDSClientAdapter(
			"https://localhost:9999", 5*time.Second, logr.Discard(),
		)
		Expect(err).ToNot(HaveOccurred(), "constructor must succeed with valid CA cert")
		Expect(adapter).NotTo(BeNil(), "adapter must be returned when TLS_CA_FILE is valid")
	})

	// UT-AW-750-003: Constructor must continue to work when TLS_CA_FILE is unset (regression guard).
	It("UT-AW-750-003: succeeds when TLS_CA_FILE is unset (regression guard)", func() {
		os.Unsetenv("TLS_CA_FILE")

		adapter, err := authwebhook.NewDSClientAdapter(
			"http://localhost:9999", 5*time.Second, logr.Discard(),
		)
		Expect(err).ToNot(HaveOccurred(), "constructor must work without TLS_CA_FILE (plain HTTP)")
		Expect(adapter).NotTo(BeNil(), "adapter must be returned when TLS_CA_FILE is unset")
	})
})
