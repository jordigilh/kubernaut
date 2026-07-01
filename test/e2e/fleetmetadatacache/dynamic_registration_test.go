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

package fleetmetadatacache

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mcpServerRegistrationGVK matches pkg/fleet/registry.MCPServerRegistrationGVR
// (mcp.kuadrant.io/v1alpha1, Kind MCPServerRegistration). Declared locally
// (not imported from pkg/fleet/registry) since this test only needs it to
// build an unstructured.Unstructured for k8sClient CRUD -- pulling in the
// registry package for one GVK constant isn't warranted.
func mcpServerRegistrationGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "mcp.kuadrant.io", Version: "v1alpha1", Kind: "MCPServerRegistration"}
}

// newDynamicClusterRegistration builds an unstructured MCPServerRegistration
// pointing at the same kube-mcp-server-route HTTPRoute the fixed 3 clusters
// use (test/infrastructure/fleet_e2e.go DeployFleetCoreInfra Phase 3).
func newDynamicClusterRegistration(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(mcpServerRegistrationGVK())
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{"kubernaut.ai/managed": "true"})
	_ = unstructured.SetNestedField(obj.Object, fmt.Sprintf("%s_", name), "spec", "prefix")
	_ = unstructured.SetNestedMap(obj.Object, map[string]interface{}{
		"group":     "gateway.networking.k8s.io",
		"kind":      "HTTPRoute",
		"name":      "kube-mcp-server-route",
		"namespace": namespace,
	}, "spec", "targetRef")
	return obj
}

// clusterIDs extracts just the .ID field from listClusters for ContainElement/
// Not-ContainElement assertions.
func clusterIDs(g Gomega) []string {
	clusters := listClusters(g)
	ids := make([]string, 0, len(clusters))
	for _, c := range clusters {
		ids = append(ids, c.ID)
	}
	return ids
}

// E2E-FMC-054-013: Proves FMC's cluster registry reacts to real MCPServerRegistration
// CRD add/remove events through its live Kubernetes watch -- something no lower tier
// proves. UT (pkg/fleet/registry/kuadrant_registry_lifecycle_test.go UT-REG-KUA-002/003)
// only exercises KuadrantRegistry.onAdd/onDelete against synthetic informer callback
// invocations, never a real informer watching a real API server. This test creates and
// deletes a genuine 4th MCPServerRegistration and confirms FMC's own /api/v1/clusters
// endpoint reflects the change live, without an FMC restart.
//
// Authority: Issue #54, ADR-068 (SI-4 dynamic reconfiguration, CM-6 configuration change).
var _ = Describe("E2E-FMC-054-013: FMC's cluster registry reacts to real MCPServerRegistration changes", Ordered, func() {
	It("adds a newly-registered cluster to /api/v1/clusters without an FMC restart", func() {
		clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
		reg := newDynamicClusterRegistration(clusterID)
		Expect(k8sClient.Create(ctx, reg)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, reg)
		})

		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElement(clusterID),
				"a newly-created MCPServerRegistration must appear in FMC's live cluster list")
		}, timeout, interval).Should(Succeed())
	})

	It("removes a deregistered cluster from /api/v1/clusters without an FMC restart", func() {
		clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
		reg := newDynamicClusterRegistration(clusterID)
		Expect(k8sClient.Create(ctx, reg)).To(Succeed())

		By("Confirming the cluster is picked up first")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElement(clusterID))
		}, timeout, interval).Should(Succeed())

		By("Deleting the MCPServerRegistration")
		Expect(k8sClient.Delete(ctx, reg)).To(Succeed())

		By("Confirming FMC's live cluster list drops it")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).ToNot(ContainElement(clusterID),
				"a deleted MCPServerRegistration must disappear from FMC's live cluster list")
		}, timeout, interval).Should(Succeed())

		By("Confirming the original 3 fixed clusters are unaffected")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElements("loopback-cluster", "prod-east", "prod-west"),
				"deregistering a dynamic cluster must not disturb the fixed cluster set")
		}, timeout, interval).Should(Succeed())
	})
})
