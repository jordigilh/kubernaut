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

package eaigw

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// backendGVK matches pkg/fleet/registry.BackendGVR (gateway.envoyproxy.io/
// v1alpha1, Kind Backend) -- EAIGWRegistry watches this CRD directly (no
// separate broker component, unlike Kuadrant's MCPServerRegistration).
// Declared locally (not imported from pkg/fleet/registry) since this test
// only needs it to build an unstructured.Unstructured for k8sClient CRUD.
func backendGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "Backend"}
}

// newDynamicBackend builds an unstructured Backend pointing at the same
// kube-mcp-server Service the fixed 3 clusters use
// (test/infrastructure/fleet_e2e.go deployEnvoyAIGatewayRegistrations).
// Note this Backend is standalone -- it is NOT wired into the shared
// MCPRoute's backendRefs, so FMC's registry (which watches Backend directly,
// not MCPRoute) still picks it up as a candidate cluster the moment it's
// created, exactly mirroring how the Kuadrant lane's dynamic
// MCPServerRegistration test creates a registration without needing broker
// reconfiguration.
func newDynamicBackend(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(backendGVK())
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{"kubernaut.ai/managed": "true"})
	_ = unstructured.SetNestedSlice(obj.Object, []interface{}{
		map[string]interface{}{
			"fqdn": map[string]interface{}{
				"hostname": fmt.Sprintf("kube-mcp-server.%s.svc.cluster.local", namespace),
				"port":     int64(8080),
			},
		},
	}, "spec", "endpoints")
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

// E2E-FMC-EAIGW-054-013: EAIGW sibling of E2E-FMC-054-013
// (test/e2e/fleetmetadatacache/dynamic_registration_test.go), proving FMC's
// cluster registry reacts to real Backend CRD add/remove events through its
// live Kubernetes watch (pkg/fleet/registry/eaigw_registry.go, BackendGVR --
// EAIGWRegistry watches Backend directly, with no MCPRoute/broker
// indirection). This test creates and deletes a genuine 4th Backend and
// confirms FMC's own /api/v1/clusters endpoint reflects the change live,
// without an FMC restart.
//
// Authority: Issue #54, ADR-068 (SI-4 dynamic reconfiguration, CM-6 configuration change).
var _ = Describe("E2E-FMC-EAIGW-054-013: FMC's cluster registry reacts to real Backend changes", Ordered, func() {
	It("adds a newly-registered cluster to /api/v1/clusters without an FMC restart", func() {
		clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
		backend := newDynamicBackend(clusterID)
		Expect(k8sClient.Create(ctx, backend)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, backend)
		})

		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElement(clusterID),
				"a newly-created Backend must appear in FMC's live cluster list")
		}, timeout, interval).Should(Succeed())
	})

	It("removes a deregistered cluster from /api/v1/clusters without an FMC restart", func() {
		clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
		backend := newDynamicBackend(clusterID)
		Expect(k8sClient.Create(ctx, backend)).To(Succeed())

		By("Confirming the cluster is picked up first")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElement(clusterID))
		}, timeout, interval).Should(Succeed())

		By("Deleting the Backend")
		Expect(k8sClient.Delete(ctx, backend)).To(Succeed())

		By("Confirming FMC's live cluster list drops it")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).ToNot(ContainElement(clusterID),
				"a deleted Backend must disappear from FMC's live cluster list")
		}, timeout, interval).Should(Succeed())

		By("Confirming the original 3 fixed clusters are unaffected")
		Eventually(func(g Gomega) {
			g.Expect(clusterIDs(g)).To(ContainElements("loopback-cluster", "prod-east", "prod-west"),
				"deregistering a dynamic cluster must not disturb the fixed cluster set")
		}, timeout, interval).Should(Succeed())
	})
})
