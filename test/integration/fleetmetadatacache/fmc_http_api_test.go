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

package fleetmetadatacache_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// IT-FMC-054-001, IT-FMC-054-010, IT-FMC-054-020
//
// Pyramid Invariant: IT proves wiring.
// These tests construct the same FMC HTTP stack as cmd/fleetmetadatacache/main.go
// and exercise it through real HTTP, real Valkey, and a real EAIGWRegistry
// connected to envtest.
//
// Wiring Manifest:
//
//	fmc.Handler + RegisterRoutes  -> cmd/fleetmetadatacache/main.go:135-137  -> IT-FMC-054-001
//	fmc.HTTPClient                -> pkg/fleet/scope_factory.go:54 -> IT-FMC-054-010
//	registry.Get guard            -> pkg/fleet/fmc/handler.go:89   -> IT-FMC-054-020
var _ = Describe("FMC HTTP API Integration (BR-INTEGRATION-065)", Ordered, Label("fmc", "integration"), func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		writer      *fmc.ValkeyWriter
		cacheReader *scopecache.ValkeyCacheReader
		clusterReg  *registry.EAIGWRegistry
		server      *httptest.Server
		redisClient *redis.Client
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.Background())

		By("Creating real Valkey writer and cache reader")
		writer = fmc.NewValkeyWriter(valkeyAddr)
		cacheReader = scopecache.NewValkeyCacheReader(valkeyAddr)
		redisClient = redis.NewClient(&redis.Options{Addr: valkeyAddr})

		By("Creating real EAIGWRegistry from envtest")
		clusterReg = registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		Expect(clusterReg.Start(ctx)).To(Succeed(), "EAIGWRegistry should start against envtest")

		By("Creating Backend 'it-cluster' in envtest")
		createBackend(ctx, "it-cluster")
		Eventually(func() bool {
			_, ok := clusterReg.Get("it-cluster")
			return ok
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"EAIGWRegistry should discover it-cluster")

		By("Starting httptest.Server with real FMC handler stack")
		scopeClient := scopecache.NewClient(cacheReader)
		handler := fmc.NewHandler(scopeClient, clusterReg, logr.Discard())
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		server = httptest.NewServer(mux)
	})

	AfterAll(func() {
		cancel()
		if server != nil {
			server.Close()
		}
		if clusterReg != nil {
			clusterReg.Stop()
		}
		if cacheReader != nil {
			_ = cacheReader.Close()
		}
		if writer != nil {
			_ = writer.Close()
		}
		if redisClient != nil {
			_ = redisClient.Close()
		}
		deleteBackend(context.Background(), "it-cluster")
	})

	Describe("IT-FMC-054-001 [AC-4]: FMC HTTP API serves scope check through production router", func() {
		var valkeyKey string

		BeforeEach(func() {
			var err error
			valkeyKey, err = scopecache.BuildKey("it-cluster", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, valkeyKey, 30*time.Second)).To(Succeed())
		})

		AfterEach(func() {
			redisClient.Del(ctx, valkeyKey)
		})

		It("returns managed=true for a resource seeded in Valkey on a known cluster", func() {
			resp, err := http.Get(server.URL + "/api/v1/scope/check?cluster=it-cluster&group=apps&version=v1&kind=Deployment&namespace=default&name=nginx")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var result fmc.ScopeCheckResponse
			Expect(json.Unmarshal(body, &result)).To(Succeed())
			Expect(result.Managed).To(BeTrue(),
				"IT-FMC-054-001: resource seeded in Valkey on a known cluster must return managed=true through the production HTTP path")
		})
	})

	Describe("IT-FMC-054-010 [AC-4, SC-7]: HTTPClient round-trips through real FMC server", func() {
		var valkeyKey string

		BeforeEach(func() {
			var err error
			valkeyKey, err = scopecache.BuildKey("it-cluster", "apps", "v1", "StatefulSet", "data", "redis-primary")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, valkeyKey, 30*time.Second)).To(Succeed())
		})

		AfterEach(func() {
			redisClient.Del(ctx, valkeyKey)
		})

		It("returns managed=true when resource exists in Valkey", func() {
			httpClient := fmc.NewHTTPClient(server.URL)

			managed, err := httpClient.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "it-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "StatefulSet",
				Namespace: "data",
				Name:      "redis-primary",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"IT-FMC-054-010: HTTPClient must return managed=true for a resource seeded in Valkey through the real FMC HTTP stack")
		})

		It("returns managed=false when resource does not exist in Valkey", func() {
			httpClient := fmc.NewHTTPClient(server.URL)

			managed, err := httpClient.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "it-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "orphan-ns",
				Name:      "no-such-deploy",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(),
				"IT-FMC-054-010: HTTPClient must return managed=false for an absent resource")
		})
	})

	Describe("IT-FMC-054-020 [SC-7]: ClusterID validation rejects unknown cluster through HTTP", func() {
		var valkeyKey string

		BeforeEach(func() {
			var err error
			valkeyKey, err = scopecache.BuildKey("unknown-cluster", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, valkeyKey, 30*time.Second)).To(Succeed())
		})

		AfterEach(func() {
			redisClient.Del(ctx, valkeyKey)
		})

		It("returns managed=false for unknown cluster even when Valkey key exists", func() {
			httpClient := fmc.NewHTTPClient(server.URL)

			managed, err := httpClient.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "unknown-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(),
				"IT-FMC-054-020: unknown cluster must be rejected by EAIGWRegistry.Get() before reaching Valkey cache")

			By("Verifying the Valkey key still exists (cache was not the rejection reason)")
			exists, err := redisClient.Exists(ctx, valkeyKey).Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(int64(1)),
				"Valkey key must still exist, proving the registry guard rejected the request before consulting the cache")
		})
	})
})

// IT-FMC-1993-001/002/003 (ADR-068 gap closure, IA-2/AC-3): unlike the
// "own mux" Describe block above (constructs a bare fmc.Handler mux, no
// auth), this block wraps the same handler stack with auth.NewMiddleware --
// the real production wiring added to buildFMCServers
// (cmd/fleetmetadatacache/main.go) -- and exercises it with real envtest
// ServiceAccount tokens (TokenReview + SAR), mirroring
// test/integration/gateway/security_suite_setup_test.go's SecurityTestTokens
// pattern.
//
// Wiring Manifest: auth.NewMiddleware(...).Handler(apiMux) -> buildFMCServers
// (cmd/fleetmetadatacache/main.go:265-276) -> IT-FMC-1993-001/002/003.
var _ = Describe("FMC HTTP API AuthN/Z (Issue #1993, ADR-068)", Ordered, Label("fmc", "integration", "auth"), func() {
	const (
		itNamespace     = "default"
		authorizedSA    = "fmc-it-authorized-caller"
		unauthorizedSA  = "fmc-it-unauthorized-caller"
		fmcResourceName = "fleetmetadatacache-service"
	)

	var (
		server            *httptest.Server
		k8sClientset      kubernetes.Interface
		authorizedToken   string
		unauthorizedToken string
	)

	BeforeAll(func() {
		ctx := context.Background()

		By("Creating a Kubernetes clientset from the envtest admin rest.Config")
		var err error
		k8sClientset, err = kubernetes.NewForConfig(restConfig)
		Expect(err).ToNot(HaveOccurred(), "kubernetes clientset should be created from the shared envtest rest.Config")

		By("Minting an authorized caller ServiceAccount (bound to fmc-scope-check-client-it, mirrors gateway/remediationorchestrator-controller)")
		authorizedToken = createServiceAccountWithToken(ctx, k8sClientset, itNamespace, authorizedSA)
		bindServiceAccountToFMCScopeCheckClient(ctx, k8sClientset, itNamespace, authorizedSA)

		By("Minting an unauthorized caller ServiceAccount (no RBAC binding)")
		unauthorizedToken = createServiceAccountWithToken(ctx, k8sClientset, itNamespace, unauthorizedSA)

		By("Starting httptest.Server with the real FMC handler stack wrapped in auth.NewMiddleware")
		handler := fmc.NewHandler(scopecache.NewClient(nil), &fakeEmptyRegistry{}, logr.Discard())
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		authenticator := auth.NewK8sAuthenticator(k8sClientset)
		authorizer := auth.NewK8sAuthorizer(k8sClientset)
		authMiddleware := auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace:    itNamespace,
			Resource:     "services",
			ResourceName: fmcResourceName,
			Verb:         "get",
		}, logr.Discard())

		// DD-PLATFORM-010 (Issue #2169): mirror buildFMCServers' real
		// production wiring (cmd/fleetmetadatacache/main.go) exactly --
		// /readyz is registered at the top level, outside authMiddleware's
		// wrap, reusing fmc.ReadyzHandler against a real, reachable Valkey
		// (the suite's shared valkeyAddr container) so the ping succeeds.
		cacheReader := scopecache.NewValkeyCacheReader(valkeyAddr)
		topMux := http.NewServeMux()
		topMux.HandleFunc(fmc.ReadyzPath, fmc.ReadyzHandler(func() bool { return true }, cacheReader))
		topMux.Handle("/", authMiddleware.Handler(mux))

		server = httptest.NewServer(topMux)
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
	})

	It("IT-FMC-1993-001 [IA-2]: a request with no Authorization header is rejected with 401", func() {
		resp, err := http.Get(server.URL + fmc.ClustersPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
			"IA-2: a token-less GW/RO -> FMC scope-check request must be rejected before reaching the handler")
	})

	It("IT-FMC-1993-002 [AC-3]: a valid but unauthorized ServiceAccount token is rejected with 403", func() {
		req, err := http.NewRequest(http.MethodGet, server.URL+fmc.ClustersPath, nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+unauthorizedToken)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
			"AC-3: a real, TokenReview-valid ServiceAccount without the fmc-scope-check-client binding "+
				"must be denied by SAR, not merely by an invalid token")
	})

	It("IT-FMC-1993-003 [IA-2,AC-3]: an authorized ServiceAccount token (gateway/remediationorchestrator-controller-equivalent) succeeds with 200", func() {
		req, err := http.NewRequest(http.MethodGet, server.URL+fmc.ClustersPath, nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+authorizedToken)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"IA-2/AC-3: a ServiceAccount bound to fmc-scope-check-client (mirrors gateway/"+
				"remediationorchestrator-controller's production ClusterRoleBinding) must reach the handler")
	})

	It("IT-FMC-2169-001 [AC-4, DD-PLATFORM-010]: /readyz bypasses auth.NewMiddleware entirely, on the real production wiring shape", func() {
		resp, err := http.Get(server.URL + fmc.ReadyzPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"DD-PLATFORM-010/#2169: /readyz must be reachable with no Authorization header at all, since it "+
				"is registered outside auth.NewMiddleware's wrap -- exactly mirroring buildFMCServers' "+
				"production topMux/apiMux split")
	})

	It("IT-FMC-2169-002 [IA-2, AC-4]: /api/v1/clusters on the same server still requires auth (no accidental exemption of the whole router)", func() {
		resp, err := http.Get(server.URL + fmc.ClustersPath) //nolint:gosec,noctx // test-only probe
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
			"DD-PLATFORM-010 must scope the /readyz auth exemption narrowly -- the real scope-check/clusters "+
				"API on the same httptest.Server must still reject an unauthenticated request")
	})
})

// fakeEmptyRegistry is a minimal registry.ClusterRegistry stub for the
// auth-focused Describe block above -- these tests only prove
// auth.NewMiddleware's TokenReview/SAR gating in front of the mux, never
// cluster-list business logic (covered by the "own mux" Describe block
// above and pkg/fleet/fmc/handler_test.go).
type fakeEmptyRegistry struct{}

func (f *fakeEmptyRegistry) List() []registry.ClusterInfo { return nil }
func (f *fakeEmptyRegistry) Get(string) (registry.ClusterInfo, bool) {
	return registry.ClusterInfo{}, false
}
func (f *fakeEmptyRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (f *fakeEmptyRegistry) Ready() bool                                 { return true }
func (f *fakeEmptyRegistry) Start(context.Context) error                 { return nil }
func (f *fakeEmptyRegistry) Stop()                                       {}
