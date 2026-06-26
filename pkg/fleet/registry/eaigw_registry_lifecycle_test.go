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

package registry

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func newTestWatcher() *EAIGWRegistry {
	logger := zap.New(zap.UseDevMode(true))
	return NewEAIGWRegistry(nil, EAIGWRegistryConfig{
		ChannelSize:  8,
		ResyncPeriod: time.Minute,
	}, nil, logger)
}

func newBackendUnstructured(name, namespace, endpoint string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "Backend",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					ManagedLabel: "true",
				},
			},
			"status": map[string]interface{}{
				"endpoint": endpoint,
			},
		},
	}
}

// UT-REG-EAIGW: EAIGWRegistry lifecycle unit tests
// Authority: BR-INTEGRATION-065 (Multi-Cluster Fleet Registry)
// FedRAMP: SI-4 (Information System Monitoring) -- cluster event tracking
var _ = Describe("UT-REG-EAIGW: EAIGWRegistry lifecycle", func() {
	var w *EAIGWRegistry

	BeforeEach(func() {
		w = newTestWatcher()
	})

	AfterEach(func() {
		w.Stop()
	})

	Describe("Initial state", func() {
		It("UT-REG-EAIGW-002: should start with empty cluster list and not ready", func() {
			Expect(w.List()).To(BeEmpty())
			Expect(w.Ready()).To(BeFalse())
		})
	})

	Describe("onAdd", func() {
		It("UT-REG-EAIGW-003: should add cluster to registry and emit Added event", func() {
			u := newBackendUnstructured("prod-east", "kubernaut-system",
				"https://mcp.example.com/prod-east")

			w.onAdd(u)

			clusters := w.List()
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].ID).To(Equal("prod-east"))
			Expect(clusters[0].MCPEndpoint).To(Equal("https://mcp.example.com/prod-east"))

			info, found := w.Get("prod-east")
			Expect(found).To(BeTrue())
			Expect(info.ID).To(Equal("prod-east"))

			Eventually(w.WatchClusters()).Should(Receive(Equal(ClusterEvent{
				Type:    EventAdded,
				Cluster: info,
			})))
		})

		It("UT-REG-EAIGW-004: should ignore non-Unstructured objects", func() {
			w.onAdd("not-an-unstructured")
			Expect(w.List()).To(BeEmpty())
		})
	})

	Describe("onUpdate", func() {
		It("UT-REG-EAIGW-005: should update existing cluster and emit Updated event", func() {
			u1 := newBackendUnstructured("staging", "kubernaut-system",
				"https://old-endpoint.com/staging")
			w.onAdd(u1)
			// Drain the add event
			Eventually(w.WatchClusters()).Should(Receive())

			u2 := newBackendUnstructured("staging", "kubernaut-system",
				"https://new-endpoint.com/staging")
			w.onUpdate(u1, u2)

			info, found := w.Get("staging")
			Expect(found).To(BeTrue())
			Expect(info.MCPEndpoint).To(Equal("https://new-endpoint.com/staging"))

			Eventually(w.WatchClusters()).Should(Receive(Equal(ClusterEvent{
				Type:    EventUpdated,
				Cluster: info,
			})))
		})
	})

	Describe("onDelete", func() {
		It("UT-REG-EAIGW-006: should remove cluster from registry and emit Deleted event", func() {
			u := newBackendUnstructured("dev-west", "kubernaut-system",
				"https://mcp.example.com/dev-west")
			w.onAdd(u)
			Eventually(w.WatchClusters()).Should(Receive())

			w.onDelete(u)

			Expect(w.List()).To(BeEmpty())
			_, found := w.Get("dev-west")
			Expect(found).To(BeFalse())

			Eventually(w.WatchClusters()).Should(Receive(WithTransform(
				func(e ClusterEvent) EventType { return e.Type },
				Equal(EventDeleted),
			)))
		})

		It("UT-REG-EAIGW-007: should handle tombstone objects on delete", func() {
			u := newBackendUnstructured("tombstone-cluster", "kubernaut-system",
				"https://mcp.example.com/tombstone")
			w.onAdd(u)
			Eventually(w.WatchClusters()).Should(Receive())

			tombstone := cache.DeletedFinalStateUnknown{
				Key: "kubernaut-system/tombstone-cluster",
				Obj: u,
			}
			w.onDelete(tombstone)

			Expect(w.List()).To(BeEmpty())
		})

		It("UT-REG-EAIGW-008: should no-op when deleting unknown cluster", func() {
			u := newBackendUnstructured("unknown", "kubernaut-system", "")
			w.onDelete(u)
			Expect(w.List()).To(BeEmpty())
		})
	})

	Describe("emit", func() {
		It("UT-REG-EAIGW-009: should not panic after Stop is called", func() {
			w.Stop()
			Expect(func() {
				w.emit(ClusterEvent{Type: EventAdded, Cluster: ClusterInfo{ID: "post-stop"}})
			}).ToNot(Panic())
		})
	})
})
