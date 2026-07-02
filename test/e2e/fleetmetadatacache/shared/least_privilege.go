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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LeastPrivilege proves FMC enforces least-privilege scope boundaries at two
// layers:
//  1. The scope-check API itself (default-deny for unlabeled resources and
//     unknown clusters -- SC-7 boundary protection). Gateway-agnostic.
//  2. FMC's own ServiceAccount RBAC surface (AC-6 least privilege), which
//     genuinely differs per gateway (different discovery CRD groups) --
//     asserted via v.RBACChecks() plus the universal core-resource denials
//     (Pods/Secrets/Deployments) that apply regardless of gateway.
//
// {ScenarioPrefix}-011, e.g. E2E-FMC-054-011 (Kuadrant) /
// E2E-FMC-EAIGW-054-011 (EAIGW).
//
// Authority: Issue #54, ADR-068 (AC-6 least privilege, SC-7 boundary
// protection), BR-INTEGRATION-065.
func LeastPrivilege(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf("%s-011: FMC enforces least-privilege scope boundaries", v.ScenarioPrefix()), Ordered, func() {
		var testNS *corev1.Namespace

		BeforeAll(func() {
			testNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-unmanaged-%d", v.ResourcePrefix(), time.Now().UnixNano()),
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, testNS)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, testNS)
			})
		})

		It("returns managed=false for a resource without the kubernaut.ai/managed label (default-deny)", func() {
			svcName := v.ResourcePrefix() + "-unmanaged-svc"
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNS.Name,
					// Deliberately no kubernaut.ai/managed label.
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, svc)).To(Succeed())

			Eventually(func(g Gomega) {
				managed := ScopeCheck(g, h, "loopback-cluster", "", "v1", "Service", testNS.Name, svcName)
				g.Expect(managed).To(BeFalse(),
					"unlabeled resource must never be reported as managed")
			}, Timeout, Interval).Should(Succeed())
		})

		It("returns managed=false for a resource in an unregistered cluster (SC-7 boundary enforcement)", func() {
			Eventually(func(g Gomega) {
				managed := ScopeCheck(g, h, "totally-unknown-cluster", "", "v1", "Service", testNS.Name, "anything")
				g.Expect(managed).To(BeFalse(),
					"scope check against an unregistered cluster must fail closed")
			}, Timeout, Interval).Should(Succeed())
		})

		// This proves a boundary transition no lower tier can prove: IT-FLEET-VALKEY-003
		// only proves Valkey's own TTL eviction mechanics against a manually-seeded key
		// -- it never proves that FMC's real sync pipeline actually STOPS refreshing a
		// key once the label disappears. Without this, a stale "managed=true" cache
		// entry (never refreshed, but also never actively invalidated) would be a
		// genuine SC-7 boundary leak: a resource an operator explicitly un-scoped from
		// Kubernaut would keep granting Gateway/RO access until an operator noticed and
		// manually flushed Valkey.
		It("stops reporting managed=true once the kubernaut.ai/managed label is removed (SC-7, real resync)", func() {
			svcName := v.ResourcePrefix() + "-delabeled-svc"
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNS.Name,
					Labels:    map[string]string{"kubernaut.ai/managed": "true"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, svc)).To(Succeed())

			By("Confirming FMC's real sync pipeline first marks it managed")
			Eventually(func(g Gomega) {
				managed := ScopeCheck(g, h, "loopback-cluster", "", "v1", "Service", testNS.Name, svcName)
				g.Expect(managed).To(BeTrue(), "resource must be marked managed before the label is removed")
			}, SyncTimeout, Interval).Should(Succeed())

			By("Removing the kubernaut.ai/managed label")
			Eventually(func(g Gomega) error {
				latest := &corev1.Service{}
				if err := h.K8sClient.Get(h.Ctx, client.ObjectKeyFromObject(svc), latest); err != nil {
					return err
				}
				delete(latest.Labels, "kubernaut.ai/managed")
				return h.K8sClient.Update(h.Ctx, latest)
			}, Timeout, Interval).Should(Succeed())

			By("Confirming FMC's cache entry is not refreshed and eventually expires (SC-7 boundary re-closes)")
			Eventually(func(g Gomega) {
				managed := ScopeCheck(g, h, "loopback-cluster", "", "v1", "Service", testNS.Name, svcName)
				g.Expect(managed).To(BeFalse(),
					"a de-labeled resource must stop being reported as managed once its cache key's TTL lapses")
			}, SyncTimeout, Interval).Should(Succeed())
		})

		It(fmt.Sprintf("restricts FMC's ServiceAccount to read-only %s %s resources (AC-6 least privilege)", v.DiscoveryLabel(), v.DynamicResourceKind()), func() {
			const fmcSA = "fleetmetadatacache"

			for _, chk := range v.RBACChecks() {
				Expect(CanI(h, chk.Verb, chk.Resource, fmcSA)).To(Equal(chk.Allowed), chk.Reason)
			}

			By("denying access to unrelated core resources (FMC reads remote-cluster resources via kube-mcp-server, not local RBAC)")
			Expect(CanI(h, "list", "pods", fmcSA)).To(BeFalse(),
				"FMC must not have direct local RBAC access to Pods -- it reads remote clusters via kube-mcp-server")
			Expect(CanI(h, "list", "secrets", fmcSA)).To(BeFalse(),
				"FMC must not have direct local RBAC access to Secrets")
			Expect(CanI(h, "list", "deployments.apps", fmcSA)).To(BeFalse())
		})
	})
}
