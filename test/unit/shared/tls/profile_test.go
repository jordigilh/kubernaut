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
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("TLS Security Profiles (#748)", Label("BR-SECURITY-748"), func() {

	var (
		certDir  string
		certPath string
		keyPath  string
	)

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

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "tls-profile-test-*")
		Expect(err).ToNot(HaveOccurred())
		certPath = filepath.Join(certDir, "tls.crt")
		keyPath = filepath.Join(certDir, "tls.key")
	})

	AfterEach(func() {
		sharedtls.ResetDefaultSecurityProfileForTesting()
		_ = os.RemoveAll(certDir)
	})

	// ──────────────────────────────────────────────────────────────
	// AC-1: Profile mapping produces correct TLS constraints
	// ──────────────────────────────────────────────────────────────

	Describe("Intermediate profile maps to AEAD-only TLS 1.2", func() {

		It("UT-TLS-748-002: cipher suites contain exactly the 6 AEAD ECDHE suites", func() {
			p := sharedtls.IntermediateProfile()
			Expect(p.CipherSuites).To(ConsistOf(
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			))
			Expect(p.MinTLSVersion).To(Equal(uint16(tls.VersionTLS12)))
		})
	})

	Describe("Old profile is a strict superset of Intermediate", func() {

		It("UT-TLS-748-003: includes all Intermediate ciphers plus CBC/RSA fallbacks", func() {
			old := sharedtls.OldProfile()
			intermediate := sharedtls.IntermediateProfile()

			for _, c := range intermediate.CipherSuites {
				Expect(old.CipherSuites).To(ContainElement(c),
					"Old profile must contain every Intermediate cipher")
			}
			Expect(len(old.CipherSuites)).To(BeNumerically(">", len(intermediate.CipherSuites)),
				"Old profile must have additional backward-compat ciphers beyond Intermediate")
			Expect(old.MinTLSVersion).To(Equal(uint16(tls.VersionTLS10)))
		})
	})

	Describe("Modern profile enforces TLS 1.3 with auto cipher selection", func() {

		It("UT-TLS-748-004: MinTLSVersion is 1.3 and CipherSuites is empty", func() {
			p := sharedtls.ModernProfile()
			Expect(p.CipherSuites).To(BeEmpty(),
				"Go auto-selects TLS 1.3 ciphers; explicit list must be empty")
			Expect(p.MinTLSVersion).To(Equal(uint16(tls.VersionTLS13)))
		})
	})

	Describe("All profiles prefer X25519 as the primary curve", func() {

		It("UT-TLS-748-005: X25519 is first, followed by P-256 and P-384", func() {
			expected := []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384}
			for _, constructor := range []func() *sharedtls.SecurityProfile{
				sharedtls.OldProfile, sharedtls.IntermediateProfile, sharedtls.ModernProfile,
			} {
				p := constructor()
				Expect(p.CurvePreferences).To(Equal(expected),
					"profile %s must have [X25519, P-256, P-384]", p.Type)
			}
		})
	})

	// ──────────────────────────────────────────────────────────────
	// AC-2: Server TLS applies the stored profile
	// ──────────────────────────────────────────────────────────────

	Describe("ConfigureConditionalTLS applies the stored profile to the server", func() {

		It("UT-TLS-748-050: Modern profile upgrades server MinVersion to TLS 1.3", func() {
			generateSelfSignedCert(certPath, keyPath)
			sharedtls.SetDefaultSecurityProfile(sharedtls.ModernProfile())

			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS13)),
				"server must enforce TLS 1.3 when Modern profile is active")
			Expect(server.TLSConfig.CurvePreferences[0]).To(Equal(tls.X25519))
		})

		It("UT-TLS-748-051: Intermediate profile sets cipher suites on server", func() {
			generateSelfSignedCert(certPath, keyPath)
			sharedtls.SetDefaultSecurityProfile(sharedtls.IntermediateProfile())

			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)),
				"server must enforce TLS 1.2 when Intermediate profile is active")
			Expect(server.TLSConfig.CipherSuites).To(HaveLen(6),
				"server must have exactly 6 AEAD ECDHE cipher suites")
			Expect(server.TLSConfig.CurvePreferences).To(HaveLen(3))
		})

		It("UT-TLS-748-052: Old profile lowers server MinVersion to TLS 1.0", func() {
			generateSelfSignedCert(certPath, keyPath)
			sharedtls.SetDefaultSecurityProfile(sharedtls.OldProfile())

			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS10)),
				"server must allow TLS 1.0 when Old profile is active")
			Expect(len(server.TLSConfig.CipherSuites)).To(BeNumerically(">=", 12),
				"Old profile must include CBC/RSA fallback ciphers")
		})
	})

	// ──────────────────────────────────────────────────────────────
	// AC-3: Vanilla K8s / Kind — no regression
	// ──────────────────────────────────────────────────────────────

	Describe("No profile set preserves existing TLS 1.2 behavior", func() {

		It("UT-TLS-748-060: server uses TLS 1.2 with no cipher restriction when profile is absent", func() {
			generateSelfSignedCert(certPath, keyPath)
			// No SetDefaultSecurityProfile call — simulates vanilla K8s

			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)),
				"vanilla K8s must retain hardcoded TLS 1.2 minimum")
			Expect(server.TLSConfig.CipherSuites).To(BeNil(),
				"vanilla K8s must not restrict cipher suites (Go default negotiation)")
			Expect(server.TLSConfig.CurvePreferences).To(BeNil(),
				"vanilla K8s must not restrict curves (Go default negotiation)")
		})

		It("UT-TLS-748-061: empty config string is a no-op (no profile stored)", func() {
			sharedtls.SetDefaultSecurityProfileFromConfig("")

			generateSelfSignedCert(certPath, keyPath)
			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)),
				"empty config must not alter the TLS 1.2 default")
			Expect(server.TLSConfig.CipherSuites).To(BeNil())
		})
	})

	// ──────────────────────────────────────────────────────────────
	// AC-4: Config-to-profile resolution (operator integration)
	// ──────────────────────────────────────────────────────────────

	Describe("SetDefaultSecurityProfileFromConfig resolves YAML value to active profile", func() {

		It("UT-TLS-748-070: 'Intermediate' config value produces TLS 1.2 AEAD on server", func() {
			sharedtls.SetDefaultSecurityProfileFromConfig("Intermediate")

			generateSelfSignedCert(certPath, keyPath)
			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(server.TLSConfig.CipherSuites).To(HaveLen(6))
		})

		It("UT-TLS-748-071: 'Modern' config value produces TLS 1.3 on server", func() {
			sharedtls.SetDefaultSecurityProfileFromConfig("Modern")

			generateSelfSignedCert(certPath, keyPath)
			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
			Expect(server.TLSConfig.CipherSuites).To(BeEmpty(),
				"Modern profile must not set TLS 1.2 cipher suites")
		})
	})

	// ──────────────────────────────────────────────────────────────
	// AC-5: Graceful degradation on invalid config
	// ──────────────────────────────────────────────────────────────

	Describe("Invalid profile names degrade gracefully to TLS 1.2 default", func() {

		It("UT-TLS-748-080: unknown profile name preserves TLS 1.2 default", func() {
			sharedtls.SetDefaultSecurityProfileFromConfig("InvalidProfile")

			generateSelfSignedCert(certPath, keyPath)
			server := &http.Server{Addr: ":0"}
			isTLS, _, err := sharedtls.ConfigureConditionalTLS(server, certDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(isTLS).To(BeTrue())

			Expect(server.TLSConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)),
				"invalid profile must fall back to TLS 1.2")
			Expect(server.TLSConfig.CipherSuites).To(BeNil(),
				"invalid profile must not restrict ciphers")
		})
	})

	// ──────────────────────────────────────────────────────────────
	// AC-6: Client-side transport applies the stored profile
	// ──────────────────────────────────────────────────────────────

	Describe("NewTLSTransport applies the stored profile to client transport", func() {

		It("UT-TLS-748-090: Modern profile upgrades client transport to TLS 1.3", func() {
			generateSelfSignedCert(certPath, keyPath)
			sharedtls.SetDefaultSecurityProfile(sharedtls.ModernProfile())

			transport, err := sharedtls.NewTLSTransport(certPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(transport.TLSClientConfig.MinVersion).To(Equal(uint16(tls.VersionTLS13)),
				"client transport must enforce TLS 1.3 when Modern profile is active")
		})

		It("UT-TLS-748-091: no profile preserves TLS 1.2 on client transport", func() {
			generateSelfSignedCert(certPath, keyPath)
			// No profile set — vanilla K8s

			transport, err := sharedtls.NewTLSTransport(certPath)
			Expect(err).ToNot(HaveOccurred())

			Expect(transport.TLSClientConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)),
				"vanilla K8s client transport must retain TLS 1.2")
			Expect(transport.TLSClientConfig.CipherSuites).To(BeNil(),
				"vanilla K8s client must not restrict ciphers")
		})
	})

	// ──────────────────────────────────────────────────────────────
	// ApplyProfile — unit-level overlay mechanics
	// ──────────────────────────────────────────────────────────────

	Describe("ApplyProfile overlay mechanics", func() {

		It("UT-TLS-748-010: overlays MinVersion, CipherSuites, and CurvePreferences", func() {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			sharedtls.ApplyProfile(cfg, sharedtls.IntermediateProfile())

			Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(cfg.CipherSuites).To(HaveLen(6))
			Expect(cfg.CurvePreferences).To(HaveLen(3))
		})

		It("UT-TLS-748-011: overrides MinVersion when profile requires higher", func() {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			sharedtls.ApplyProfile(cfg, sharedtls.ModernProfile())

			Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
		})

		It("UT-TLS-748-012: nil profile leaves config unchanged", func() {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			sharedtls.ApplyProfile(cfg, nil)

			Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(cfg.CipherSuites).To(BeNil())
		})

		It("UT-TLS-748-013: nil config does not panic", func() {
			Expect(func() {
				sharedtls.ApplyProfile(nil, sharedtls.IntermediateProfile())
			}).ToNot(Panic())
		})

		It("UT-TLS-748-014: MaxTLSVersion is applied for Custom profiles", func() {
			cfg := &tls.Config{MinVersion: tls.VersionTLS10}
			sharedtls.ApplyProfile(cfg, &sharedtls.SecurityProfile{
				Type:          sharedtls.ProfileCustom,
				MinTLSVersion: tls.VersionTLS12,
				MaxTLSVersion: tls.VersionTLS12,
			})

			Expect(cfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(cfg.MaxVersion).To(Equal(uint16(tls.VersionTLS12)))
		})
	})

	// ──────────────────────────────────────────────────────────────
	// ProfileForType — lookup correctness
	// ──────────────────────────────────────────────────────────────

	Describe("ProfileForType resolves built-in types", func() {

		It("UT-TLS-748-020: returns matching profile for each known type", func() {
			for _, tc := range []struct {
				pt       sharedtls.ProfileType
				expected uint16
			}{
				{sharedtls.ProfileOld, tls.VersionTLS10},
				{sharedtls.ProfileIntermediate, tls.VersionTLS12},
				{sharedtls.ProfileModern, tls.VersionTLS13},
			} {
				p := sharedtls.ProfileForType(tc.pt)
				Expect(p.Type).To(Equal(tc.pt))
				Expect(p.MinTLSVersion).To(Equal(tc.expected))
			}
		})

		It("UT-TLS-748-021: returns nil for Custom (requires explicit construction)", func() {
			Expect(sharedtls.ProfileForType(sharedtls.ProfileCustom)).To(BeNil())
		})

		It("UT-TLS-748-022: returns nil for unrecognized type", func() {
			Expect(sharedtls.ProfileForType("UnknownProfile")).To(BeNil())
		})
	})
})
