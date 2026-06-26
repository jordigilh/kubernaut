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

func newTestKuadrantWatcher() *KuadrantRegistry {
	logger := zap.New(zap.UseDevMode(true))
	return NewKuadrantRegistry(nil, EAIGWRegistryConfig{
		ChannelSize:  8,
		ResyncPeriod: time.Minute,
	}, nil, logger)
}

func newMCPServerRegistrationUnstructured(name, namespace, prefix string, labels map[string]interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "mcp.kuadrant.io/v1alpha1",
		"kind":       "MCPServerRegistration",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"prefix": prefix,
		},
	}
	return &unstructured.Unstructured{Object: obj}
}

// UT-REG-KUA: KuadrantRegistry lifecycle unit tests
// Authority: BR-INTEGRATION-065 (Multi-Cluster Fleet Registry)
// FedRAMP: AC-3 (Access Enforcement), SI-4 (Information System Monitoring)
var _ = Describe("UT-REG-KUA: KuadrantRegistry lifecycle", func() {
	var w *KuadrantRegistry

	BeforeEach(func() {
		w = newTestKuadrantWatcher()
	})

	AfterEach(func() {
		w.Stop()
	})

	Describe("extractKuadrantClusterInfo", func() {
		It("UT-REG-KUA-001 [AC-3]: ExtractClusterInfo reads spec.prefix as ToolPrefix from MCPServerRegistration", func() {
			u := newMCPServerRegistrationUnstructured("loopback-cluster", "kuadrant-system", "loopback_cluster_",
				map[string]interface{}{ManagedLabel: "true"})

			info, err := extractKuadrantClusterInfo(u)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.ID).To(Equal("loopback-cluster"))
			Expect(info.ToolPrefix).To(Equal("loopback_cluster_"))
			Expect(info.Name).To(Equal("loopback-cluster"))
			Expect(info.Namespace).To(Equal("kuadrant-system"))
		})
	})

	Describe("onAdd", func() {
		It("UT-REG-KUA-002 [SI-4]: Registry emits EventAdded when labeled MCPServerRegistration appears", func() {
			u := newMCPServerRegistrationUnstructured("prod-east", "kuadrant-system", "prod_east_",
				map[string]interface{}{ManagedLabel: "true"})

			w.onAdd(u)

			clusters := w.List()
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].ID).To(Equal("prod-east"))
			Expect(clusters[0].ToolPrefix).To(Equal("prod_east_"))

			Eventually(w.WatchClusters()).Should(Receive(Equal(ClusterEvent{
				Type:    EventAdded,
				Cluster: clusters[0],
			})))
		})
	})

	Describe("onDelete", func() {
		It("UT-REG-KUA-003 [SI-4]: Registry emits EventDeleted when labeled MCPServerRegistration is removed", func() {
			u := newMCPServerRegistrationUnstructured("dev-west", "kuadrant-system", "dev_west_",
				map[string]interface{}{ManagedLabel: "true"})

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
	})

	Describe("label filtering", func() {
		It("UT-REG-KUA-004 [AC-3]: Registry ignores MCPServerRegistrations without kubernaut.ai/managed=true label", func() {
			u := newMCPServerRegistrationUnstructured("unlabeled", "kuadrant-system", "unlabeled_",
				map[string]interface{}{"other-label": "value"})

			w.onAdd(u)
			Expect(w.List()).To(BeEmpty(),
				"MCPServerRegistrations without kubernaut.ai/managed=true must be ignored")
		})
	})

	Describe("spec.state filtering", func() {
		It("UT-REG-KUA-005 [AC-3]: Registry excludes MCPServerRegistrations with spec.state: Disabled", func() {
			u := newMCPServerRegistrationUnstructured("disabled-cluster", "kuadrant-system", "disabled_",
				map[string]interface{}{ManagedLabel: "true"})
			_ = unstructured.SetNestedField(u.Object, "Disabled", "spec", "state")

			w.onAdd(u)
			Expect(w.List()).To(BeEmpty(),
				"Disabled MCPServerRegistrations must be excluded from the registry")
		})

		It("UT-REG-KUA-006 [AC-3]: Registry includes MCPServerRegistrations with spec.state: Enabled (or absent)", func() {
			uEnabled := newMCPServerRegistrationUnstructured("enabled-cluster", "kuadrant-system", "enabled_",
				map[string]interface{}{ManagedLabel: "true"})
			_ = unstructured.SetNestedField(uEnabled.Object, "Enabled", "spec", "state")

			uNoState := newMCPServerRegistrationUnstructured("default-cluster", "kuadrant-system", "default_",
				map[string]interface{}{ManagedLabel: "true"})

			w.onAdd(uEnabled)
			w.onAdd(uNoState)

			Expect(w.List()).To(HaveLen(2),
				"MCPServerRegistrations with Enabled or absent state must be included")
		})
	})

	Describe("tombstone handling", func() {
		It("handles tombstone objects on delete", func() {
			u := newMCPServerRegistrationUnstructured("tombstone-cluster", "kuadrant-system", "tombstone_",
				map[string]interface{}{ManagedLabel: "true"})
			w.onAdd(u)
			Eventually(w.WatchClusters()).Should(Receive())

			tombstone := cache.DeletedFinalStateUnknown{
				Key: "kuadrant-system/tombstone-cluster",
				Obj: u,
			}
			w.onDelete(tombstone)
			Expect(w.List()).To(BeEmpty())
		})
	})
})
