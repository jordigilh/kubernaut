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

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
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

// unreachableValkeyAddr is a deliberately-unreachable address for tests
// that don't care whether /readyz's backend ping succeeds.
const unreachableValkeyAddr = "127.0.0.1:1"

// testFMCDeps builds the minimal *fmcDeps needed to exercise
// buildFMCServers' HTTP/TLS wiring without a real MCP Gateway. syncer/
// mcpClient/writer are deliberately left nil -- buildFMCServers never
// touches them; only runFMCServers' background syncer goroutine would.
//
// valkeyAddr backs deps.cacheReader (DD-PLATFORM-010, Issue #2169: /readyz
// -- and therefore fmc.HTTPClient.Ping() -- now pings this address for
// real, unlike the pre-#2169 ClustersPath target). Pass
// unreachableValkeyAddr for tests that don't care whether /readyz's
// backend ping succeeds; pass a real miniredis address (see
// IT-FMC-1683-A-002 below) for tests that need Ping()/readyz to succeed.
//
// Issue #1993: buildFMCServers now wraps apiMux with auth.NewMiddleware, so
// every test below needs a k8sClientset (for the middleware's TokenReview/SAR
// calls) and a releaseNamespace (the SAR check's target namespace). This
// package has no envtest (unlike test/integration/fleetmetadatacache), so
// k8sClientset is always the fake clientset from fakeAuthorizedK8sClient/
// fakeUnauthorizedK8sClient below -- these tests prove buildFMCServers' TLS
// and routing wiring, not auth.Middleware's own TokenReview/SAR logic
// (covered by pkg/shared/auth/k8s_auth_test.go and the real envtest-backed
// IT-FMC-1993-* cases in test/integration/fleetmetadatacache).
func testFMCDeps(k8sClientset kubernetes.Interface, valkeyAddr string) *fmcDeps {
	return &fmcDeps{
		cacheReader:      scopecache.NewValkeyCacheReader(valkeyAddr),
		clusterRegistry:  &fakeClusterRegistry{},
		k8sClientset:     k8sClientset,
		releaseNamespace: "kubernaut-system",
	}
}

// fakeAuthorizedK8sClient returns a fake kubernetes.Interface whose
// TokenReview always authenticates any non-empty Bearer token as username,
// and whose SubjectAccessReview always returns Allowed. Mirrors the
// PrependReactor pattern in pkg/shared/auth/k8s_auth_test.go -- this
// package has no envtest, so the real TokenReview/SAR API isn't available;
// the fake clientset instead lets these tests exercise the real
// auth.NewMiddleware wiring path (buildFMCServers) end-to-end.
func fakeAuthorizedK8sClient(username string) kubernetes.Interface {
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenReview) //nolint:forcetypeassert // test-only reactor, type is fixed by the fake clientset's dispatch
		review.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User:          authenticationv1.UserInfo{Username: username},
		}
		return true, review, nil
	})
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sar := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview) //nolint:forcetypeassert // test-only reactor, type is fixed by the fake clientset's dispatch
		sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	})
	return client
}

// bearerHTTPClient wraps base with auth.NewAuthTransport so every request
// carries "Authorization: Bearer <token>" -- mirrors production's
// BackendFMC client (pkg/fleet/scope_factory.go) and IT-FLEET-1993-010's
// test pattern (pkg/fleet/scope_factory_test.go), writing token to a
// GinkgoT().TempDir() file consumed by auth.NewTokenSource.
func bearerHTTPClient(token string, base http.RoundTripper) *http.Client {
	tokenPath := filepath.Join(GinkgoT().TempDir(), "token")
	Expect(os.WriteFile(tokenPath, []byte(token), 0o600)).To(Succeed())
	return &http.Client{Transport: auth.NewAuthTransport(auth.NewTokenSource(tokenPath), base)}
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

// DD-PLATFORM-006 Decision Area 13 follow-up (round-16 RCA, PR #1790): proves
// buildValkeyTLSConfig -- the wiring point between fmcconfig.ValkeyTLSConfig
// and pkg/shared/tls.BuildTLSConfig -- actually constructs a usable
// *tls.Config from the on-disk CA file, not just that it compiles/is called.
var _ = Describe("buildValkeyTLSConfig (DD-PLATFORM-006 Decision Area 13 follow-up)", func() {
	var certDir string

	BeforeEach(func() {
		certDir = GinkgoT().TempDir()
	})

	It("IT-FMC-VALKEYTLS-001 [SC-8]: returns nil (plaintext) when TLS is disabled", func() {
		got := buildValkeyTLSConfig(fmcconfig.ValkeyTLSConfig{Enabled: false}, logr.Discard())
		Expect(got).To(BeNil())
	})

	It("IT-FMC-VALKEYTLS-002 [SC-8]: returns a *tls.Config trusting the configured CA when TLS is enabled", func() {
		certFile := filepath.Join(certDir, "tls.crt")
		keyFile := filepath.Join(certDir, "tls.key")
		generateSelfSignedCert(certFile, keyFile)

		got := buildValkeyTLSConfig(fmcconfig.ValkeyTLSConfig{Enabled: true, CAFile: certFile}, logr.Discard())
		Expect(got).ToNot(BeNil())
		Expect(got.RootCAs).ToNot(BeNil(), "the configured caFile must be loaded into RootCAs so the server cert can be verified")
	})
})

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
		deps = testFMCDeps(fakeAuthorizedK8sClient("system:serviceaccount:kubernaut-system:test-caller"), unreachableValkeyAddr)
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

		httpsClient := bearerHTTPClient("test-caller-token", &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: caPoolFromCert(filepath.Join(certDir, "tls.crt"))}, //nolint:gosec // MinVersion inherited from default; test dials with modern Go defaults
		})
		resp, err := httpsClient.Get("https://" + addr + fmc.ClustersPath)
		Expect(err).ToNot(HaveOccurred(), "a CA-trusting HTTPS client must complete the handshake")
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// A plaintext HTTP request against a TLS-only listener must never
		// reach the API handler -- proves plaintext is rejected, not
		// silently accepted alongside TLS. (Go's http.Server detects the
		// non-TLS ClientHello and replies with a plain-text 400 explaining
		// the mismatch rather than serving the handler -- it does not
		// return a transport-level error to the plaintext client.)
		plainResp, plainErr := http.Get("http://" + addr + fmc.ClustersPath) //nolint:gosec,noctx // deliberate plaintext probe against a TLS-only listener
		if plainErr == nil {
			defer func() { _ = plainResp.Body.Close() }()
			Expect(plainResp.StatusCode).ToNot(Equal(http.StatusOK),
				"SC-8: a plaintext request must never reach the API handler behind the TLS-only listener")
		}
	})

	It("IT-FMC-1683-A-001b: falls back to plain HTTP when no cert is mounted (fail-open bootstrap, matches DataStorage/Gateway)", func() {
		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())
		Expect(servers.api.TLSConfig).To(BeNil(),
			"no cert mounted -- API server must remain plain HTTP (ConfigureConditionalTLS fail-open)")
	})

	It("IT-FMC-1683-A-002 [SC-8, AC-4, DD-PLATFORM-010]: fmc.HTTPClient.Ping() succeeds against the TLS-protected API port via the unauthenticated /readyz route, reusing fmc.ReadyzHandler rather than a duplicated liveness handler", func() {
		mr, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred(), "miniredis should start so /readyz's backend ping has something real to succeed against")
		defer mr.Close()

		generateSelfSignedCert(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))
		cfg.Server.TLS.CertDir = certDir
		readyDeps := testFMCDeps(fakeAuthorizedK8sClient("system:serviceaccount:kubernaut-system:test-caller"), mr.Addr())

		servers := buildFMCServers(cfg, readyDeps, &ready, logr.Discard())
		ln, addr := listenOn()
		go func() { _ = servers.api.ServeTLS(ln, "", "") }()
		defer func() { _ = servers.api.Close() }()

		// Deliberately no bearer token: DD-PLATFORM-010/#2169's whole point is
		// that Ping() no longer needs one, unlike a real scope-check/clusters call.
		caOnlyClient := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: caPoolFromCert(filepath.Join(certDir, "tls.crt"))}, //nolint:gosec // test dials with modern Go defaults
			},
		}
		fmcClient := fmc.NewHTTPClient("https://"+addr, fmc.WithHTTPClient(caOnlyClient))

		Expect(fmcClient.Ping(context.Background())).To(Succeed(),
			"DD-PLATFORM-010/#2169: Ping() must succeed against the unauthenticated /readyz route on the "+
				"TLS-protected API port, with no bearer token and no TokenReview/SAR round-trip, reusing the "+
				"same fmc.ReadyzHandler that also backs the kubelet probe rather than a duplicated implementation")
	})

	It("IT-FMC-1683-A-003 [AC-4, DD-PLATFORM-010]: /readyz on the API port is unauthenticated, while /api/v1/clusters on the same port still requires auth", func() {
		mr, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred(), "miniredis should start so /readyz's backend ping has something real to succeed against")
		defer mr.Close()
		readyDeps := testFMCDeps(fakeAuthorizedK8sClient("system:serviceaccount:kubernaut-system:test-caller"), mr.Addr())

		servers := buildFMCServers(cfg, readyDeps, &ready, logr.Discard())

		apiLn, apiAddr := listenOn()
		go func() { _ = servers.api.Serve(apiLn) }()
		defer func() { _ = servers.api.Close() }()

		healthLn, healthAddr := listenOn()
		go func() { _ = servers.health.Serve(healthLn) }()
		defer func() { _ = servers.health.Close() }()

		// DD-PLATFORM-010/#2169: /readyz is now ALSO registered, deliberately
		// unauthenticated, on the API port -- a second registration of the
		// same fmc.ReadyzHandler, not a leak of the whole router.
		apiResp, err := http.Get("http://" + apiAddr + fmc.ReadyzPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = apiResp.Body.Close() }()
		Expect(apiResp.StatusCode).To(Equal(http.StatusOK),
			"DD-PLATFORM-010: /readyz on the API port must be reachable with no Authorization header, "+
				"exactly like the dedicated health port it mirrors")

		// Guard: the auth exemption must be scoped to /readyz only -- the
		// real business-data route on the same port must still reject an
		// unauthenticated request (no accidental exemption of the whole router).
		clustersResp, err := http.Get("http://" + apiAddr + fmc.ClustersPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = clustersResp.Body.Close() }()
		Expect(clustersResp.StatusCode).To(Equal(http.StatusUnauthorized),
			"DD-PLATFORM-010 must scope the auth exemption to /readyz only -- /api/v1/clusters must still "+
				"reject unauthenticated requests")

		healthResp, err := http.Get("http://" + healthAddr + fmc.ReadyzPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = healthResp.Body.Close() }()
		Expect(healthResp.StatusCode).To(BeNumerically(">=", http.StatusOK),
			"/readyz must remain routed (not 404) on the dedicated health port -- unaffected by this change")
		body, _ := io.ReadAll(healthResp.Body)
		Expect(string(body)).ToNot(BeEmpty())
	})

	It("IT-FMC-1683-A-004 [AC-4, DD-FLEET-004]: /healthz (liveness) is served exclusively on the dedicated health port, not the API port", func() {
		servers := buildFMCServers(cfg, deps, &ready, logr.Discard())

		apiLn, apiAddr := listenOn()
		go func() { _ = servers.api.Serve(apiLn) }()
		defer func() { _ = servers.api.Close() }()

		healthLn, healthAddr := listenOn()
		go func() { _ = servers.health.Serve(healthLn) }()
		defer func() { _ = servers.health.Close() }()

		apiResp, err := http.Get("http://" + apiAddr + fmc.HealthzPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = apiResp.Body.Close() }()
		// Issue #1993: see the matching comment in IT-FMC-1683-A-003 above --
		// auth middleware now intercepts with 401 before the mux's own 404.
		Expect(apiResp.StatusCode).To(Equal(http.StatusUnauthorized),
			"DD-FLEET-004: /healthz must not be registered on the API mux -- FMC's cross-service health "+
				"signal is /readyz now (DD-PLATFORM-010), and /healthz remains kubelet-only on the health port "+
				"(auth middleware intercepts with 401 before the mux's own 404 is reached)")

		healthResp, err := http.Get("http://" + healthAddr + fmc.HealthzPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = healthResp.Body.Close() }()
		Expect(healthResp.StatusCode).To(Equal(http.StatusOK),
			"/healthz must remain reachable on the dedicated health port (kubelet liveness probe)")
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
			// codeql[go/insecure-tls]: deliberate adversarial client config -- proves the real
			// server *rejects* a TLS 1.1 handshake (SC-13), not a production listener/dialer setting.
			TLSClientConfig: &tls.Config{RootCAs: pool, MaxVersion: tls.VersionTLS11}, //nolint:gosec // deliberately testing a below-floor TLS version
		}}
		_, err := downgradedClient.Get("https://" + addr + fmc.ClustersPath)
		Expect(err).To(HaveOccurred(),
			"SC-13: Intermediate profile floors at TLS 1.2 -- a TLS 1.1-only client must be rejected")

		compliantClient := bearerHTTPClient("test-caller-token", &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		})
		resp, err := compliantClient.Get("https://" + addr + fmc.ClustersPath)
		Expect(err).ToNot(HaveOccurred(),
			"SC-13: a TLS 1.2+ client with default (AEAD) ciphers must be accepted by the Intermediate profile")
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})
