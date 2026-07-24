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

package fleet_test

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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// generateTestCA creates a self-signed CA cert/key pair for the FMC
// CA-verified transport tests below (#1683).
func generateTestCA() (*x509.Certificate, []byte, *ecdsa.PrivateKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "fmc-test-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).ToNot(HaveOccurred())
	cert, err := x509.ParseCertificate(der)
	Expect(err).ToNot(HaveOccurred())
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return cert, pemBytes, key
}

// generateLeafSignedByCA creates a "localhost"/127.0.0.1 leaf certificate
// signed by the given CA, suitable for httptest.Server.TLS.Certificates.
func generateLeafSignedByCA(ca *x509.Certificate, caKey *ecdsa.PrivateKey) tls.Certificate {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &leafKey.PublicKey, caKey)
	Expect(err).ToNot(HaveOccurred())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: leafKey}
}

var _ = Describe("NewScopeChecker factory (BR-INTEGRATION-065)", func() {
	var local *mockLocalChecker

	BeforeEach(func() {
		local = &mockLocalChecker{managed: map[string]bool{}}
	})

	It("UT-FLEET-FAC-001: disabled fleet returns local checker unchanged", func() {
		cfg := fleet.FleetConfig{Enabled: false}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).To(BeIdenticalTo(local),
			"disabled fleet must return the exact same local checker instance")
	})

	It("UT-FLEET-FAC-002: enabled fleet with empty endpoint and non-FMC backend returns local checker", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "acm", Endpoint: ""}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).To(BeIdenticalTo(local),
			"empty endpoint for non-FMC backend must fall back to local checker")
	})

	It("UT-FLEET-FAC-003 [AC-4]: BackendFMC with HTTP endpoint returns FederatedScopeChecker using FMC HTTP client", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "fleetmetadatacache", Endpoint: "http://fmc:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).ToNot(BeIdenticalTo(local),
			"fmc backend must wrap local checker with FederatedScopeChecker")

		_, isFederated := checker.(*fleet.FederatedScopeChecker)
		Expect(isFederated).To(BeTrue(),
			"factory must return *fleet.FederatedScopeChecker for fmc backend")
	})

	It("UT-FLEET-FAC-004 [CM-6]: BackendValkey is rejected as unsupported", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "valkey", Endpoint: "valkey:6379"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
		Expect(checker).To(BeNil(),
			"factory must reject legacy valkey backend")
	})

	It("UT-FLEET-FAC-005 [AC-4]: BackendACM with endpoint returns FederatedScopeChecker using ACM client", func() {
		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  "https://search-api:4010",
			TokenPath: "/etc/gateway/acm-token/token",
		}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		Expect(checker).ToNot(BeIdenticalTo(local),
			"acm backend must wrap local checker with FederatedScopeChecker")

		_, isFederated := checker.(*fleet.FederatedScopeChecker)
		Expect(isFederated).To(BeTrue(),
			"factory must return *fleet.FederatedScopeChecker for acm backend")
	})

	It("UT-FLEET-FAC-006: unknown backend returns error", func() {
		cfg := fleet.FleetConfig{Enabled: true, Backend: "unknown", Endpoint: "http://example:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
		Expect(checker).To(BeNil())
	})

	It("UT-FLEET-FAC-007 [CM-6]: empty backend with endpoint returns unsupported backend error", func() {
		cfg := fleet.FleetConfig{Enabled: true, Endpoint: "http://fmc:8080"}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).To(HaveOccurred(),
			"empty backend with endpoint must fail validation in factory")
		Expect(checker).To(BeNil())
	})

	// #1556: proves the factory actually composes the bearer-token transport,
	// not just that it returns a FederatedScopeChecker (UT-FLEET-FAC-005 only
	// checks the type, not the wire behavior).
	It("UT-FLEET-FAC-008 [SC-7,IA-5]: BackendACM with TokenPath set sends Authorization: Bearer <token> to the ACM endpoint", func() {
		var gotAuthHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		defer server.Close()

		tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
		Expect(os.WriteFile(tokenPath, []byte("test-acm-token"), 0o600)).To(Succeed())

		cfg := fleet.FleetConfig{
			Enabled:   true,
			Backend:   "acm",
			Endpoint:  server.URL,
			TokenPath: tokenPath,
		}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		_, err = checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(gotAuthHeader).To(Equal("Bearer test-acm-token"),
			"SC-7/IA-5: factory-composed ACM client must authenticate every request "+
				"with the configured bearer token")
	})

	// #1556 defense-in-depth: FleetConfig.Validate() hard-requires TokenPath for
	// BackendACM (UT-FLEET-CFG-070), so this state is unreachable through normal
	// config loading. This test locks in the factory's own fail-safe behavior
	// for direct struct construction that bypasses Validate() (e.g. a future
	// caller or test helper) — it must never send a partial/malformed
	// Authorization header, only either a correct one or none at all.
	It("UT-FLEET-FAC-009 [AC-4]: BackendACM without TokenPath (Validate bypassed) sends no Authorization header", func() {
		var gotAuthHeader string
		sawRequest := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawRequest = true
			gotAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"searchResult":[{"count":0}]}}`))
		}))
		defer server.Close()

		cfg := fleet.FleetConfig{Enabled: true, Backend: "acm", Endpoint: server.URL}
		checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())

		_, err = checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(sawRequest).To(BeTrue(), "request must still reach the ACM endpoint")
		Expect(gotAuthHeader).To(BeEmpty(),
			"AC-4: without a configured TokenPath the factory must never send a "+
				"partial or malformed Authorization header")
	})

	// Issue #1683: FMC branch must build a CA-verified transport from
	// FleetConfig.TLSCAFile, mirroring the existing ACM branch (SC-8, IA-5).
	Describe("BackendFMC TLS (Issue #1683)", func() {
		It("IT-FMC-1683-B-001 [SC-8,IA-5]: TLSCAFile builds a transport that trusts a server cert signed by that CA and rejects one that isn't", func() {
			ca, caPEM, caKey := generateTestCA()
			leafCert := generateLeafSignedByCA(ca, caKey)

			trustedServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			trustedServer.TLS = &tls.Config{Certificates: []tls.Certificate{leafCert}} //nolint:gosec // test-only, MinVersion inherited
			trustedServer.StartTLS()
			defer trustedServer.Close()

			caFile := filepath.Join(GinkgoT().TempDir(), "ca.crt")
			Expect(os.WriteFile(caFile, caPEM, 0o600)).To(Succeed())

			cfg := fleet.FleetConfig{
				Enabled:   true,
				Backend:   "fleetmetadatacache",
				Endpoint:  trustedServer.URL,
				TLSCAFile: caFile,
			}
			checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())

			managed, err := checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"SC-8/IA-5: the factory-built CA-verified transport must trust a server cert "+
					"signed by the configured CA and successfully read the managed:true response")

			// MITM rejection: a server presenting a cert NOT signed by the
			// configured CA -- even though it returns the identical
			// managed:true JSON body -- must be rejected at the TLS layer.
			// Since IsManagedResource fails safe (swallows transport errors,
			// UT-FMC-HC-003..005), a false result here can only be explained
			// by the CA verification correctly rejecting the untrusted cert
			// (the happy-path assertion above proves the JSON-decode path
			// would otherwise return true for this exact body).
			untrustedServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			defer untrustedServer.Close()

			cfgUntrusted := fleet.FleetConfig{
				Enabled:   true,
				Backend:   "fleetmetadatacache",
				Endpoint:  untrustedServer.URL,
				TLSCAFile: caFile,
			}
			checkerUntrusted, err := fleet.NewScopeChecker(local, cfgUntrusted, logr.Discard())
			Expect(err).ToNot(HaveOccurred())

			managedUntrusted, err := checkerUntrusted.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred(), "fail-safe: transport/cert errors must not propagate")
			Expect(managedUntrusted).To(BeFalse(),
				"SC-8/IA-5: a server cert not signed by the configured CA must be rejected "+
					"(MITM protection), even though the response body is identical to the trusted case")
		})

		It("IT-FMC-1683-B-002 [AC-4]: without TLSCAFile, the FMC branch uses a plain (non-CA-pinned) client, matching pre-#1683 behavior", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"managed":true}`))
			}))
			defer server.Close()

			cfg := fleet.FleetConfig{Enabled: true, Backend: "fleetmetadatacache", Endpoint: server.URL}
			checker, err := fleet.NewScopeChecker(local, cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())

			managed, err := checker.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"AC-4: without TLSCAFile configured, the FMC branch must remain backward-compatible "+
					"with plain-HTTP FMC endpoints (no regression for existing non-TLS deployments)")
		})
	})
})
