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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// IT-FMC-054-001, IT-FMC-054-010, IT-FMC-054-020
//
// Pyramid Invariant: IT proves wiring.
// These tests construct the same FMC HTTP stack as cmd/fmc/main.go
// and exercise it through real HTTP, real Valkey, and a real CRDWatcher
// connected to envtest.
//
// Wiring Manifest:
//
//	fmc.Handler + RegisterRoutes  -> cmd/fmc/main.go:135-137  -> IT-FMC-054-001
//	fmc.HTTPClient                -> pkg/fleet/scope_factory.go:54 -> IT-FMC-054-010
//	registry.Get guard            -> pkg/fleet/fmc/handler.go:89   -> IT-FMC-054-020
var _ = Describe("FMC HTTP API Integration (BR-INTEGRATION-065)", Ordered, Label("fmc", "integration"), func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		writer      *fmc.ValkeyWriter
		cacheReader *scopecache.ValkeyCacheReader
		clusterReg  *registry.CRDWatcher
		server      *httptest.Server
		redisClient *redis.Client
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.Background())

		By("Creating real Valkey writer and cache reader")
		writer = fmc.NewValkeyWriter(valkeyAddr)
		cacheReader = scopecache.NewValkeyCacheReader(valkeyAddr)
		redisClient = redis.NewClient(&redis.Options{Addr: valkeyAddr})

		By("Creating real CRDWatcher from envtest")
		clusterReg = registry.NewCRDWatcher(dynClient, registry.CRDWatcherConfig{}, nil, logr.Discard())
		Expect(clusterReg.Start(ctx)).To(Succeed(), "CRDWatcher should start against envtest")

		By("Creating MCPServerRegistration 'it-cluster' in envtest")
		createMCPServerRegistration(ctx, "it-cluster", "IT Cluster")
		Eventually(func() bool {
			_, ok := clusterReg.Get("it-cluster")
			return ok
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"CRDWatcher should discover it-cluster")

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
		deleteMCPServerRegistration(context.Background(), "it-cluster")
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
				"IT-FMC-054-020: unknown cluster must be rejected by CRDWatcher.Get() before reaching Valkey cache")

			By("Verifying the Valkey key still exists (cache was not the rejection reason)")
			exists, err := redisClient.Exists(ctx, valkeyKey).Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(Equal(int64(1)),
				"Valkey key must still exist, proving the registry guard rejected the request before consulting the cache")
		})
	})
})

