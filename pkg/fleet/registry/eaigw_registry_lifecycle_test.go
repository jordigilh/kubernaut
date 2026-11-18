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
	"context"
	"errors"
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

func newBackendUnstructured(name, endpoint string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "Backend",
			"metadata": map[string]interface{}{
				"name":      name,
				"uid":       "backend-" + name,
				"namespace": "kubernaut-system",
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

	Describe("Session publication", func() {
		It("W05 UT-REG-REVIEW-EA-003 [BR-INTEGRATION-065]: stale LIST cannot resurrect an intervening deletion", func() {
			old := newBackendUnstructured("east", "https://old")
			w.onAdd(old)
			generation, err := w.begin()
			Expect(err).NotTo(HaveOccurred())
			info, err := ExtractClusterInfo(old)
			Expect(err).NotTo(HaveOccurred())
			w.onDelete(old)
			Expect(w.install(context.Background(), generation, map[string]ClusterInfo{"east": info})).NotTo(Succeed())
			Expect(w.List()).To(BeEmpty())
		})

		DescribeTable("W05 UT-REG-REVIEW-EA-004/005 [BR-INTEGRATION-065]: stale publication cannot undo lifecycle retirement", func(stop bool) {
			generation, err := w.begin()
			Expect(err).NotTo(HaveOccurred())
			if stop {
				w.Stop()
			} else {
				w.invalidate(errors.New("watch failed"))
			}
			Expect(w.install(context.Background(), generation, map[string]ClusterInfo{})).NotTo(Succeed())
			Expect(w.Ready()).To(BeFalse())
		}, Entry("stop", true), Entry("watch failure", false))

		It("W05 UT-REG-REVIEW-EA-001 [BR-INTEGRATION-065]: stale LIST cannot overwrite an intervening update", func() {
			old := newBackendUnstructured("east", "https://old")
			updated := newBackendUnstructured("east", "https://new")
			w.onAdd(old)
			generation, err := w.begin()
			Expect(err).NotTo(HaveOccurred())
			info, err := ExtractClusterInfo(old)
			Expect(err).NotTo(HaveOccurred())
			w.onUpdate(old, updated)
			Expect(w.install(context.Background(), generation, map[string]ClusterInfo{"east": info})).NotTo(Succeed())
			current, found := w.Get("east")
			Expect(found).To(BeTrue())
			Expect(current.MCPEndpoint).To(Equal("https://new"))
		})

		It("UT-REG-REVIEW-EA-002 [BR-INTEGRATION-065]: unrelated and identical updates preserve a registration proof", func() {
			east := newBackendUnstructured("east", "https://east")
			west := newBackendUnstructured("west", "https://west")
			w.onAdd(east)
			w.ready = true // Handler-unit fixture; external HTTP tests prove health.
			snapshot, found := w.Snapshot("east")
			Expect(found).To(BeTrue())
			w.onAdd(west)
			w.onUpdate(east, east.DeepCopy())
			Expect(w.WithCurrent(context.Background(), "east", snapshot.Info.RegistrationUID, snapshot.Revision, func(ClusterInfo) error { return nil })).To(Succeed())
		})
		// Former LIST-only Probe recovery/startup specs are replaced by
		// IT-FMC-WATCH-2637-001/002/004 against the real external HTTP boundary.

		It("UT-REG-EAIGW-015 [AC-3]: rejects stale delete and old revision commits after same-name recreation", func() {
			oldRegistration := newBackendUnstructured("prod-east", "https://old.example")
			newRegistration := newBackendUnstructured("prod-east", "https://new.example")
			newRegistration.SetUID("backend-prod-east-recreated")

			w.onAdd(oldRegistration)
			w.ready = true
			oldSnapshot, found := w.Snapshot("prod-east")
			Expect(found).To(BeTrue())

			w.onUpdate(oldRegistration, newRegistration)
			w.onDelete(oldRegistration)

			current, found := w.Get("prod-east")
			Expect(found).To(BeTrue())
			Expect(current.RegistrationUID).To(Equal("backend-prod-east-recreated"))
			Expect(w.WithCurrent(context.Background(), "prod-east", oldSnapshot.Info.RegistrationUID, oldSnapshot.Revision, func(ClusterInfo) error {
				return nil
			})).To(MatchError(ContainSubstring("registration changed")))
		})

		It("UT-REG-EAIGW-016 [AC-3]: rejects a commit after a same-UID registry update increments revision", func() {
			first := newBackendUnstructured("prod-east", "https://old.example")
			second := newBackendUnstructured("prod-east", "https://new.example")
			w.onAdd(first)
			w.ready = true
			snapshot, found := w.Snapshot("prod-east")
			Expect(found).To(BeTrue())
			w.onUpdate(first, second)

			Expect(w.WithCurrent(context.Background(), "prod-east", snapshot.Info.RegistrationUID, snapshot.Revision, func(ClusterInfo) error {
				return nil
			})).To(MatchError(ContainSubstring("registration changed")))
		})
	})

	Describe("onAdd", func() {
		It("UT-REG-EAIGW-003: should add cluster to registry and emit Added event", func() {
			u := newBackendUnstructured("prod-east",
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
			u1 := newBackendUnstructured("staging",
				"https://old-endpoint.com/staging")
			w.onAdd(u1)
			// Drain the add event
			Eventually(w.WatchClusters()).Should(Receive())

			u2 := newBackendUnstructured("staging",
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
		It("UT-REG-EAIGW-017 [AC-3]: resolves the current cluster map through the lock-scoped owner", func() {
			deleted := newBackendUnstructured("current-map", "https://mcp.example/current-map")
			staleClusters := map[string]ClusterInfo{
				"current-map": {ID: "current-map", RegistrationUID: string(deleted.GetUID())},
			}
			currentClusters := map[string]ClusterInfo{
				"current-map": {ID: "current-map", RegistrationUID: string(deleted.GetUID())},
			}
			w.clusters = currentClusters

			handleDeletedCluster(deleted, &w.mu, func() (map[string]ClusterInfo, map[string]uint64) {
				return currentClusters, nil
			}, &w.revision, w.metrics, w.logger, func(ClusterEvent) {})

			Expect(currentClusters).NotTo(HaveKey("current-map"))
			Expect(staleClusters).To(HaveKey("current-map"))
		})

		It("UT-REG-EAIGW-006: should remove cluster from registry and emit Deleted event", func() {
			u := newBackendUnstructured("dev-west",
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
			u := newBackendUnstructured("tombstone-cluster",
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
			u := newBackendUnstructured("unknown", "")
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
