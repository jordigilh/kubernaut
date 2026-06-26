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

package fmc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// E2E-FMC-054-001
//
// Pyramid Invariant: E2E proves the journey.
// This test exercises the complete fleet federation path from factory to Valkey:
//
//   fleet.NewScopeChecker (factory)
//     -> FederatedScopeChecker (local/remote router)
//       -> fmc.HTTPClient (production HTTP client)
//         -> real HTTP transport
//           -> fmc.Handler (production handler)
//             -> registry.BackendInformerRegistry.Get (real envtest K8s API)
//             -> scopecache.Client (real business logic)
//               -> ValkeyCacheReader (real Valkey)
//
// All components are real production code. The only stub is localAlwaysFalse
// which isolates the remote (Valkey) path.
//
// Wiring Manifest:
//
//	fleet.NewScopeChecker factory -> pkg/fleet/scope_factory.go   -> E2E-FMC-054-001
//	FederatedScopeChecker         -> pkg/fleet/federated_checker.go -> E2E-FMC-054-001
//	fmc.HTTPClient                -> pkg/fleet/fmc/http_client.go  -> E2E-FMC-054-001
//	fmc.Handler + RegisterRoutes  -> cmd/fmc/main.go               -> E2E-FMC-054-001
//	registry.BackendInformerRegistry.Get       -> pkg/fleet/fmc/handler.go      -> E2E-FMC-054-001
//	scopecache.Client             -> pkg/fleet/scopecache/client.go -> E2E-FMC-054-001
var _ = Describe("Fleet Federation E2E: Factory -> FMC -> Valkey (BR-INTEGRATION-065)", Ordered, Label("fmc", "e2e"), func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		writer      *fmc.ValkeyWriter
		cacheReader *scopecache.ValkeyCacheReader
		clusterReg  *registry.BackendInformerRegistry
		fmcServer   *httptest.Server
		redisClient *redis.Client
		fedChecker  scope.ScopeChecker
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.Background())

		By("Setting up real FMC server stack (mirrors cmd/fmc/main.go)")
		writer = fmc.NewValkeyWriter(valkeyAddr)
		cacheReader = scopecache.NewValkeyCacheReader(valkeyAddr)
		redisClient = redis.NewClient(&redis.Options{Addr: valkeyAddr})

		scopeClient := scopecache.NewClient(cacheReader)
		clusterReg = registry.NewBackendInformerRegistry(dynClient, registry.BackendInformerRegistryConfig{}, nil, logr.Discard())
		Expect(clusterReg.Start(ctx)).To(Succeed(), "BackendInformerRegistry should start against envtest")

		By("Creating Backend 'e2e-cluster' in envtest")
		createBackend(ctx, "e2e-cluster", "E2E Cluster")
		Eventually(func() bool {
			_, ok := clusterReg.Get("e2e-cluster")
			return ok
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"BackendInformerRegistry should discover e2e-cluster")

		handler := fmc.NewHandler(scopeClient, clusterReg, logr.Discard())
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		fmcServer = httptest.NewServer(mux)

		By("Creating FederatedScopeChecker via production factory")
		cfg := fleet.FleetConfig{
			Enabled:  true,
			Backend:  fleet.BackendFMC,
			Endpoint: fmcServer.URL,
		}
		var err error
		fedChecker, err = fleet.NewScopeChecker(&localAlwaysFalse{}, cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred(), "fleet.NewScopeChecker must create a FederatedScopeChecker")
	})

	AfterAll(func() {
		cancel()
		if fmcServer != nil {
			fmcServer.Close()
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
		deleteBackend(context.Background(), "e2e-cluster")
	})

	Describe("E2E-FMC-054-001 [AC-4, SC-7]: Full factory-to-Valkey journey", func() {
		It("Sub-case A: managed resource (Valkey key exists, cluster known) returns managed=true", func() {
			key, err := scopecache.BuildKey("e2e-cluster", "apps", "v1", "Deployment", "default", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, key, 30*time.Second)).To(Succeed())
			defer redisClient.Del(ctx, key)

			managed, err := fedChecker.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "e2e-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "default",
				Name:      "nginx",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue(),
				"E2E-FMC-054-001A: managed resource on a known cluster must return true "+
					"through the full factory -> FederatedScopeChecker -> HTTPClient -> FMC handler -> BackendInformerRegistry -> Valkey path")
		})

		It("Sub-case B: unmanaged resource (Valkey key absent) returns managed=false", func() {
			managed, err := fedChecker.IsManagedResource(ctx, scope.ResourceIdentity{
				ClusterID: "e2e-cluster",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "orphan-ns",
				Name:      "no-such-deploy",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse(),
				"E2E-FMC-054-001B: absent resource must return false through the full production path")
		})
	})
})
