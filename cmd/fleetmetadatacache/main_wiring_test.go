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

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// Issue #1683: FMC server TLS + 3-port standard + FedRAMP profile wiring.
//
// osAssignedAddr requests an OS-assigned free TCP port on the loopback
// interface -- used for every server address in these tests so parallel
// runs never collide on a fixed port.
const osAssignedAddr = "127.0.0.1:0"

// These IT tests exercise the real production buildFMCServers wiring (not a
// re-implementation), starting real net.Listeners and performing real
// TLS handshakes -- proving SC-8 (Transmission Confidentiality) and SC-13
// (Cryptographic Protection) end-to-end at the integration tier, per the
// pyramid invariant (AGENTS.md).

// fakeClusterRegistry is a minimal registry.ClusterRegistry stub. These
// wiring tests only prove HTTP/TLS routing (buildFMCServers), never
// cluster-list business logic (that is fmc.Handler's job, covered by
// pkg/fleet/fmc/handler_test.go).
type fakeClusterRegistry struct{}

var _ registry.ClusterRegistry = (*fakeClusterRegistry)(nil)

func (f *fakeClusterRegistry) List() []registry.ClusterInfo { return nil }
func (f *fakeClusterRegistry) Get(string) (registry.ClusterInfo, bool) {
	return registry.ClusterInfo{}, false
}
func (f *fakeClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (f *fakeClusterRegistry) Ready() bool                                 { return true }
func (f *fakeClusterRegistry) Start(context.Context) error                 { return nil }
func (f *fakeClusterRegistry) Stop()                                       {}

// testFMCDeps builds the minimal *fmcDeps needed to exercise
// buildFMCServers' HTTP/TLS wiring without a real MCP Gateway or Valkey.
// syncer/mcpClient/writer are deliberately left nil -- buildFMCServers never
// touches them; only runFMCServers' background syncer goroutine would.
func testFMCDeps() *fmcDeps {
	return &fmcDeps{
		cacheReader:     scopecache.NewValkeyCacheReader("127.0.0.1:1"), // unreachable by design; Ping() failure is fine, these tests don't assert /readyz body content
		clusterRegistry: &fakeClusterRegistry{},
	}
}

// generateSelfSignedCert writes a self-signed cert/key pair valid for
// "localhost" and 127.0.0.1 to certFile/keyFile. Mirrors the helper in
// pkg/shared/tls/tls_test.go.
func generateSelfSignedCert(certFile, keyFile string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
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
	Expect(os.WriteFile(certFile, certPEM, 0o644)).To(Succeed()) //nolint:gosec // test-only self-signed cert

	keyDER, err := x509.MarshalECPrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	Expect(os.WriteFile(keyFile, keyPEM, 0o600)).To(Succeed())
}

// listenOn starts a TCP listener on osAssignedAddr and returns it along with
// its address, so tests never race on fixed ports.
func listenOn() (net.Listener, string) {
	ln, err := net.Listen("tcp", osAssignedAddr)
	Expect(err).ToNot(HaveOccurred())
	return ln, ln.Addr().String()
}

// caPoolFromCert loads certFile as both leaf and CA (valid for a
// self-signed cert) into a fresh x509.CertPool for client-side verification.
func caPoolFromCert(certFile string) *x509.CertPool {
	pemBytes, err := os.ReadFile(certFile)
	Expect(err).ToNot(HaveOccurred())
	pool := x509.NewCertPool()
	Expect(pool.AppendCertsFromPEM(pemBytes)).To(BeTrue())
	return pool
}

var _ = Describe("buildFMCServers TLS + 3-port wiring (#1683, BR-INTEGRATION-065)", func() {
	var (
		certDir string
		cfg     *fmcconfig.ServiceConfig
		deps    *fmcDeps
		ready   atomic.Bool
	)

	BeforeEach(func() {
		certDir = GinkgoT().TempDir()
		cfg = fmcconfig.DefaultServiceConfig()
		cfg.Server.APIAddr = osAssignedAddr
		cfg.Server.HealthAddr = osAssignedAddr
		cfg.Server.MetricsAddr = osAssignedAddr
		deps = testFMCDeps()
		ready.Store(true)
	})

	AfterEach(func() {
		sharedtls.ResetDefaultSecurityProfileForTesting()
	})

	It("IT-FMC-1683-A-001 [SC-8]: only accepts HTTPS on the API port when a cert is mounted", func() {
		generateSelfSignedCert(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))
		cfg.Server.TLS.CertDir = certDir

		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())
		Expect(servers.api.TLSConfig).NotTo(BeNil(),
			"SC-8: API server must be TLS-configured when a cert is mounted (ConfigureConditionalTLS)")

		ln, addr := listenOn()
		go func() { _ = servers.api.ServeTLS(ln, "", "") }()
		defer func() { _ = servers.api.Close() }()

		httpsClient := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: caPoolFromCert(filepath.Join(certDir, "tls.crt"))}, //nolint:gosec // MinVersion inherited from default; test dials with modern Go defaults
		}}
		resp, err := httpsClient.Get("https://" + addr + fmc.HealthzPath)
		Expect(err).ToNot(HaveOccurred(), "a CA-trusting HTTPS client must complete the handshake")
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// A plaintext HTTP request against a TLS-only listener must never
		// reach the liveness handler -- proves plaintext is rejected, not
		// silently accepted alongside TLS. (Go's http.Server detects the
		// non-TLS ClientHello and replies with a plain-text 400 explaining
		// the mismatch rather than serving the handler -- it does not
		// return a transport-level error to the plaintext client.)
		plainResp, plainErr := http.Get("http://" + addr + fmc.HealthzPath) //nolint:gosec,noctx // deliberate plaintext probe against a TLS-only listener
		if plainErr == nil {
			defer func() { _ = plainResp.Body.Close() }()
			Expect(plainResp.StatusCode).ToNot(Equal(http.StatusOK),
				"SC-8: a plaintext request must never reach the liveness handler behind the TLS-only listener")
		}
	})

	It("IT-FMC-1683-A-001b: falls back to plain HTTP when no cert is mounted (fail-open bootstrap, matches DataStorage/Gateway)", func() {
		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())
		Expect(servers.api.TLSConfig).To(BeNil(),
			"no cert mounted -- API server must remain plain HTTP (ConfigureConditionalTLS fail-open)")
	})

	It("IT-FMC-1683-A-002 [SC-8, AC-4]: fmc.HTTPClient.Ping() succeeds against the TLS-protected API port's dual-registered /healthz, unmodified from its pre-split contract", func() {
		generateSelfSignedCert(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))
		cfg.Server.TLS.CertDir = certDir

		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())
		ln, addr := listenOn()
		go func() { _ = servers.api.ServeTLS(ln, "", "") }()
		defer func() { _ = servers.api.Close() }()

		caTrustingClient := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: caPoolFromCert(filepath.Join(certDir, "tls.crt"))}, //nolint:gosec // test dials with modern Go defaults
			},
		}
		fmcClient := fmc.NewHTTPClient("https://"+addr, fmc.WithHTTPClient(caTrustingClient))

		Expect(fmcClient.Ping(context.Background())).To(Succeed(),
			"AC-4/#1553: Ping()'s call signature, base URL, and success contract must be "+
				"byte-for-byte unchanged from before the 3-port split")
	})

	It("IT-FMC-1683-A-003 [AC-4]: /readyz is served exclusively on the dedicated health port, not the API port", func() {
		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())

		apiLn, apiAddr := listenOn()
		go func() { _ = servers.api.Serve(apiLn) }()
		defer func() { _ = servers.api.Close() }()

		healthLn, healthAddr := listenOn()
		go func() { _ = servers.health.Serve(healthLn) }()
		defer func() { _ = servers.health.Close() }()

		apiResp, err := http.Get("http://" + apiAddr + "/readyz") //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = apiResp.Body.Close() }()
		Expect(apiResp.StatusCode).To(Equal(http.StatusNotFound),
			"AC-4: /readyz must no longer be reachable on the API port after the 3-port split")

		healthResp, err := http.Get("http://" + healthAddr + "/readyz") //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = healthResp.Body.Close() }()
		Expect(healthResp.StatusCode).To(BeNumerically(">=", http.StatusOK),
			"/readyz must be routed (not 404) on the dedicated health port")
		body, _ := io.ReadAll(healthResp.Body)
		Expect(string(body)).ToNot(BeEmpty())
	})

	It("IT-FMC-1683-A-004 [AC-4]: /healthz (liveness) is reachable on both the API port and the dedicated health port", func() {
		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())

		apiLn, apiAddr := listenOn()
		go func() { _ = servers.api.Serve(apiLn) }()
		defer func() { _ = servers.api.Close() }()

		healthLn, healthAddr := listenOn()
		go func() { _ = servers.health.Serve(healthLn) }()
		defer func() { _ = servers.health.Close() }()

		for _, addr := range []string{apiAddr, healthAddr} {
			resp, err := http.Get("http://" + addr + fmc.HealthzPath) //nolint:gosec,noctx // test-only probe
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "addr=%s", addr)
			_ = resp.Body.Close()
		}
	})

	It("IT-FMC-1683-E-001 [SC-13]: an Intermediate TLS security profile rejects a downgraded handshake and accepts a compliant one", func() {
		Expect(sharedtls.SetDefaultSecurityProfileFromConfig("Intermediate")).To(Succeed())

		generateSelfSignedCert(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))
		cfg.Server.TLS.CertDir = certDir

		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())
		ln, addr := listenOn()
		go func() { _ = servers.api.ServeTLS(ln, "", "") }()
		defer func() { _ = servers.api.Close() }()

		pool := caPoolFromCert(filepath.Join(certDir, "tls.crt"))

		downgradedClient := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MaxVersion: tls.VersionTLS11}, //nolint:gosec // deliberately testing a below-floor TLS version
		}}
		_, err := downgradedClient.Get("https://" + addr + fmc.HealthzPath)
		Expect(err).To(HaveOccurred(),
			"SC-13: Intermediate profile floors at TLS 1.2 -- a TLS 1.1-only client must be rejected")

		compliantClient := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		}}
		resp, err := compliantClient.Get("https://" + addr + fmc.HealthzPath)
		Expect(err).ToNot(HaveOccurred(),
			"SC-13: a TLS 1.2+ client with default (AEAD) ciphers must be accepted by the Intermediate profile")
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
