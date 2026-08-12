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

package telemetry

// White-box (package telemetry, not telemetry_test) coverage for
// TLSConfig.buildTLSConfig, since NewTracerProvider does not expose the
// built *tls.Config for black-box inspection. GAP-14 / Issue #1519.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"encoding/pem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// No TestXxx/RunSpecs here: this file's package (telemetry, internal --
// needed for white-box access to buildTLSConfig) compiles into the same
// test binary as telemetry_test.go's external "telemetry_test" package.
// These Describe/It blocks register into the one global Ginkgo suite tree
// that telemetry_test.go's TestTelemetry -> RunSpecs already drives.

var _ = Describe("TLSConfig.buildTLSConfig", func() {

	var (
		certDir  string
		certPath string
		keyPath  string
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "telemetry-tls-test-*")
		Expect(err).ToNot(HaveOccurred())
		certPath = filepath.Join(certDir, "tls.crt")
		keyPath = filepath.Join(certDir, "tls.key")
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	generateSelfSignedCert := func(certFile, keyFile string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())

		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "collector.example.com"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{"collector.example.com"},
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

	// UT-1519-005: Enabled=false is the default and returns a nil *tls.Config
	// (no TLS), regardless of what the other fields hold.
	It("UT-1519-005: returns nil when TLS is disabled", func() {
		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: false, CAFile: "/should/be/ignored"})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).To(BeNil())
	})

	// UT-1519-006: Enabled=true with CAFile empty trusts the system CA pool
	// -- the vendor-collector case (e.g. Datadog, Grafana Cloud) where there
	// is no private CA to load.
	It("UT-1519-006: trusts the system CA pool when CAFile is empty", func() {
		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())
		Expect(cfg.RootCAs).To(BeNil(), "nil RootCAs means Go falls back to the system trust store")
	})

	// UT-1519-007: Enabled=true with CAFile set loads that CA into the pool
	// -- the self-signed-collector case.
	It("UT-1519-007: loads a custom CA pool when CAFile is set", func() {
		generateSelfSignedCert(certPath, keyPath)

		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true, CAFile: certPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.RootCAs.Subjects()).ToNot(BeEmpty()) //nolint:staticcheck // no alternative for validating cert pool content
	})

	// UT-1519-008: an unreadable CAFile surfaces a clear, named error instead
	// of silently falling back to the system pool or an opaque SDK failure.
	It("UT-1519-008: returns a named error for an unreadable CAFile", func() {
		_, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true, CAFile: "/nonexistent/ca.pem"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/ca.pem"))
	})

	// UT-1519-009: CertFile+KeyFile together enable mTLS to the collector.
	It("UT-1519-009: loads a client certificate when CertFile and KeyFile are both set", func() {
		generateSelfSignedCert(certPath, keyPath)

		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true, CertFile: certPath, KeyFile: keyPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.Certificates).To(HaveLen(1))
	})

	// UT-1519-010: CertFile without a matching KeyFile is a no-op for mTLS
	// (both must be set together) rather than a partial/broken client cert.
	It("UT-1519-010: does not attempt mTLS when only CertFile is set", func() {
		generateSelfSignedCert(certPath, keyPath)

		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true, CertFile: certPath})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.Certificates).To(BeEmpty())
	})

	// UT-1519-011: the process-wide SecurityProfile (cipher suites, TLS
	// version floor -- Issue #748) is applied to the OTLP exporter's TLS
	// config exactly like every other outbound TLS client in Kubernaut.
	// This is the behavior that reusing pkg/shared/tls.BuildClientTLSConfig
	// (instead of hand-rolling *tls.Config construction) buys for free.
	It("UT-1519-011: applies the process-wide security profile, including cipher suites", func() {
		sharedtls.SetDefaultSecurityProfile(sharedtls.IntermediateProfile())
		defer sharedtls.ResetDefaultSecurityProfileForTesting()

		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
		Expect(cfg.CipherSuites).To(HaveLen(6), "Intermediate profile must set its 6 AEAD ECDHE cipher suites")
	})

	// UT-1519-012: a Modern (TLS 1.3-only) profile raises MinVersion on the
	// OTLP exporter's TLS config too, not just on inter-service transports.
	It("UT-1519-012: applies a Modern (TLS 1.3) security profile", func() {
		sharedtls.SetDefaultSecurityProfile(sharedtls.ModernProfile())
		defer sharedtls.ResetDefaultSecurityProfileForTesting()

		cfg, err := buildTLSConfig(internalconfig.TelemetryTLSConfig{Enabled: true})
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
	})
})
