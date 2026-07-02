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

package shared

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention
)

// DynamicRegistration proves FMC's cluster registry reacts to a real
// gateway-specific discovery CRD add/remove event through its live
// Kubernetes watch -- something no lower tier proves. UT
// (pkg/fleet/registry/*_registry_lifecycle_test.go) only exercises
// onAdd/onDelete against synthetic informer callback invocations, never a
// real informer watching a real API server. This scenario creates and
// deletes a genuine 4th cluster resource (v.NewDynamicClusterResource) and
// confirms FMC's own /api/v1/clusters endpoint reflects the change live,
// without an FMC restart.
//
// {ScenarioPrefix}-013, e.g. E2E-FMC-054-013 (Kuadrant, MCPServerRegistration) /
// E2E-FMC-EAIGW-054-013 (EAIGW, Backend -- no MCPRoute/broker indirection).
//
// Authority: Issue #54, ADR-068 (SI-4 dynamic reconfiguration, CM-6 configuration change).
func DynamicRegistration(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf("%s-013: FMC's cluster registry reacts to real %s changes", v.ScenarioPrefix(), v.DynamicResourceKind()), Ordered, func() {
		It("adds a newly-registered cluster to /api/v1/clusters without an FMC restart", func() {
			clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
			reg := v.NewDynamicClusterResource(h.Namespace, clusterID)
			Expect(h.K8sClient.Create(h.Ctx, reg)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, reg)
			})

			Eventually(func(g Gomega) {
				g.Expect(ClusterIDs(g, h)).To(ContainElement(clusterID),
					fmt.Sprintf("a newly-created %s must appear in FMC's live cluster list", v.DynamicResourceKind()))
			}, Timeout, Interval).Should(Succeed())
		})

		It("removes a deregistered cluster from /api/v1/clusters without an FMC restart", func() {
			clusterID := fmt.Sprintf("prod-central-%d", time.Now().UnixNano())
			reg := v.NewDynamicClusterResource(h.Namespace, clusterID)
			Expect(h.K8sClient.Create(h.Ctx, reg)).To(Succeed())

			By("Confirming the cluster is picked up first")
			Eventually(func(g Gomega) {
				g.Expect(ClusterIDs(g, h)).To(ContainElement(clusterID))
			}, Timeout, Interval).Should(Succeed())

			By(fmt.Sprintf("Deleting the %s", v.DynamicResourceKind()))
			Expect(h.K8sClient.Delete(h.Ctx, reg)).To(Succeed())

			By("Confirming FMC's live cluster list drops it")
			Eventually(func(g Gomega) {
				g.Expect(ClusterIDs(g, h)).ToNot(ContainElement(clusterID),
					fmt.Sprintf("a deleted %s must disappear from FMC's live cluster list", v.DynamicResourceKind()))
			}, Timeout, Interval).Should(Succeed())

			By("Confirming the original 3 fixed clusters are unaffected")
			Eventually(func(g Gomega) {
				g.Expect(ClusterIDs(g, h)).To(ContainElements("loopback-cluster", "prod-east", "prod-west"),
					"deregistering a dynamic cluster must not disturb the fixed cluster set")
			}, Timeout, Interval).Should(Succeed())
		})
	})
}
