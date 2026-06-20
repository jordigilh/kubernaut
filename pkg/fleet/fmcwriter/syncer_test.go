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

package fmcwriter_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmcwriter"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// mockMCPLister simulates MCP Gateway responses for list_resources.
type mockMCPLister struct {
	responses map[string]string // key: "clusterID:kind" -> response JSON
	calls     []string
	mu        sync.Mutex
}

func (m *mockMCPLister) List(_ context.Context, clusterID, kind, _ string) (string, error) {
	m.mu.Lock()
	m.calls = append(m.calls, clusterID+":"+kind)
	m.mu.Unlock()
	key := clusterID + ":" + kind
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return "[]", nil
}

// memoryCacheWriter is an in-memory CacheWriter for testing.
type memoryCacheWriter struct {
	keys map[string]time.Duration
	mu   sync.Mutex
}

func newMemoryCacheWriter() *memoryCacheWriter {
	return &memoryCacheWriter{keys: make(map[string]time.Duration)}
}

func (w *memoryCacheWriter) Set(_ context.Context, key string, ttl time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.keys[key] = ttl
	return nil
}

func (w *memoryCacheWriter) Close() error { return nil }

func (w *memoryCacheWriter) HasKey(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.keys[key]
	return ok
}

func (w *memoryCacheWriter) KeyCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.keys)
}

// mockClusterRegistry provides a static cluster list for testing.
type mockClusterRegistry struct {
	clusters []registry.ClusterInfo
	eventCh  chan registry.ClusterEvent
}

func newMockRegistry(clusters ...registry.ClusterInfo) *mockClusterRegistry {
	return &mockClusterRegistry{
		clusters: clusters,
		eventCh:  make(chan registry.ClusterEvent, 10),
	}
}

func (r *mockClusterRegistry) List() []registry.ClusterInfo         { return r.clusters }
func (r *mockClusterRegistry) Get(id string) (registry.ClusterInfo, bool) {
	for _, c := range r.clusters {
		if c.ID == id {
			return c, true
		}
	}
	return registry.ClusterInfo{}, false
}
func (r *mockClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return r.eventCh }
func (r *mockClusterRegistry) Ready() bool                                 { return true }
func (r *mockClusterRegistry) Start(_ context.Context) error               { return nil }
func (r *mockClusterRegistry) Stop()                                       { close(r.eventCh) }

var _ = Describe("FMC Writer Syncer (BR-INTEGRATION-065)", func() {
	var (
		testLogger logr.Logger
		lister     *mockMCPLister
		writer     *memoryCacheWriter
		reg        *mockClusterRegistry
		syncer     *fmcwriter.Syncer
	)

	BeforeEach(func() {
		testLogger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))

		lister = &mockMCPLister{responses: make(map[string]string)}
		writer = newMemoryCacheWriter()

		promReg := prometheus.NewRegistry()
		metrics := fmcwriter.NewMetrics(promReg)

		reg = newMockRegistry(registry.ClusterInfo{
			ID:          "prod-east",
			Name:        "Production East",
			MCPEndpoint: "http://mcp-gateway:8080/mcp",
		})

		syncer = fmcwriter.NewSyncer(reg, lister, writer, fmcwriter.Config{
			SyncInterval:  100 * time.Millisecond,
			KeyTTL:        45 * time.Second,
			ResourceKinds: []string{"Deployment", "Pod"},
		}, testLogger, metrics)
	})

	It("IT-FMC-002: SyncCluster writes Valkey keys from MCP list_resources response", func() {
		deployments := []map[string]interface{}{
			{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "nginx",
					"namespace": "default",
				},
			},
			{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "redis",
					"namespace": "cache",
				},
			},
		}
		data, _ := json.Marshal(deployments)
		lister.responses["prod-east:Deployment"] = string(data)

		err := syncer.SyncCluster(context.Background(), registry.ClusterInfo{
			ID:          "prod-east",
			MCPEndpoint: "http://mcp-gateway:8080/mcp",
		})
		Expect(err).ToNot(HaveOccurred())

		expectedKey1 := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
		expectedKey2 := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "cache", "redis")

		Expect(writer.HasKey(expectedKey1)).To(BeTrue(),
			"IT-FMC-002: nginx Deployment key must be written to cache")
		Expect(writer.HasKey(expectedKey2)).To(BeTrue(),
			"IT-FMC-002: redis Deployment key must be written to cache")
	})

	It("IT-FMC-003: ValkeyWriter.Set writes key with correct TTL", func() {
		deployments := []map[string]interface{}{
			{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "worker-1",
					"namespace": "jobs",
				},
			},
		}
		data, _ := json.Marshal(deployments)
		lister.responses["prod-east:Pod"] = string(data)

		err := syncer.SyncCluster(context.Background(), registry.ClusterInfo{
			ID:          "prod-east",
			MCPEndpoint: "http://mcp-gateway:8080/mcp",
		})
		Expect(err).ToNot(HaveOccurred())

		expectedKey := scopecache.BuildKey("prod-east", "", "v1", "Pod", "jobs", "worker-1")
		Expect(writer.HasKey(expectedKey)).To(BeTrue(),
			"IT-FMC-003: Pod key must be written")

		writer.mu.Lock()
		ttl := writer.keys[expectedKey]
		writer.mu.Unlock()
		Expect(ttl).To(Equal(45 * time.Second),
			"IT-FMC-003: Key TTL must match configured value (45s)")
	})

	It("IT-FMC-004: Run reacts to cluster add event with immediate sync", func() {
		newCluster := registry.ClusterInfo{
			ID:          "staging-west",
			Name:        "Staging West",
			MCPEndpoint: "http://mcp-gateway:8080/mcp",
		}

		pods := []map[string]interface{}{
			{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "api-pod",
					"namespace": "staging",
				},
			},
		}
		data, _ := json.Marshal(pods)
		lister.responses["staging-west:Pod"] = string(data)
		lister.responses["staging-west:Deployment"] = "[]"

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_ = syncer.Run(ctx)
		}()

		time.Sleep(50 * time.Millisecond)

		reg.eventCh <- registry.ClusterEvent{
			Type:    registry.EventAdded,
			Cluster: newCluster,
		}

		Eventually(func() bool {
			key := scopecache.BuildKey("staging-west", "", "v1", "Pod", "staging", "api-pod")
			return writer.HasKey(key)
		}, 2*time.Second, 50*time.Millisecond).Should(BeTrue(),
			"IT-FMC-004: New cluster event must trigger immediate sync and write keys")

		cancel()
	})

	It("IT-FMC-001: Run syncs all clusters on interval", func() {
		deployments := []map[string]interface{}{
			{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "app",
					"namespace": "prod",
				},
			},
		}
		data, _ := json.Marshal(deployments)
		lister.responses["prod-east:Deployment"] = string(data)
		lister.responses["prod-east:Pod"] = "[]"

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_ = syncer.Run(ctx)
		}()

		Eventually(func() int {
			return writer.KeyCount()
		}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1),
			"IT-FMC-001: Main loop must sync clusters and write keys")

		expectedKey := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "prod", "app")
		Expect(writer.HasKey(expectedKey)).To(BeTrue(),
			"IT-FMC-001: Expected deployment key must exist after sync")

		cancel()
	})

	It("handles K8s List format with items array", func() {
		listResponse := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PodList",
			"items": []interface{}{
				map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "pod-a",
						"namespace": "ns1",
					},
				},
				map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      "pod-b",
						"namespace": "ns2",
					},
				},
			},
		}
		data, _ := json.Marshal(listResponse)
		lister.responses["prod-east:Pod"] = string(data)
		lister.responses["prod-east:Deployment"] = "[]"

		err := syncer.SyncCluster(context.Background(), registry.ClusterInfo{
			ID: "prod-east",
		})
		Expect(err).ToNot(HaveOccurred())

		key1 := scopecache.BuildKey("prod-east", "", "v1", "Pod", "ns1", "pod-a")
		key2 := scopecache.BuildKey("prod-east", "", "v1", "Pod", "ns2", "pod-b")
		Expect(writer.HasKey(key1)).To(BeTrue())
		Expect(writer.HasKey(key2)).To(BeTrue())
	})

	It("handles empty response gracefully", func() {
		lister.responses["prod-east:Deployment"] = ""
		lister.responses["prod-east:Pod"] = ""

		err := syncer.SyncCluster(context.Background(), registry.ClusterInfo{
			ID: "prod-east",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.KeyCount()).To(Equal(0))
	})
})
