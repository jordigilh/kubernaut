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
)

// SyncJourney proves FMC's real sync journey end to end -- the one journey
// no unit or integration test can prove, since IT-FLEET-VALKEY-004 manually
// seeds Valkey keys instead of exercising the real Keycloak OAuth2 ->
// gateway -> kube-mcp-server (RFC 8693 token exchange) -> Valkey pipeline.
// See TokenExchange for a scenario that drives the token-exchange mechanics
// directly rather than only indirectly through this journey's success.
//
// {ScenarioPrefix}-010, e.g. E2E-FMC-054-010 (Kuadrant) /
// E2E-FMC-EAIGW-054-010 (EAIGW).
//
// Authority: Issue #54, ADR-068 (SC-7 boundary protection, AC-3 access
// enforcement), BR-INTEGRATION-065.
func SyncJourney(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf(
		"%s-010: FMC discovers managed resources via the real Keycloak+%s+kube-mcp-server pipeline",
		v.ScenarioPrefix(), v.DiscoveryLabel(),
	), Ordered, func() {
		var testNS *corev1.Namespace

		BeforeAll(func() {
			testNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-managed-%d", v.ResourcePrefix(), time.Now().UnixNano()),
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, testNS)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, testNS)
			})
		})

		It(fmt.Sprintf("lists loopback-cluster, prod-east, and prod-west via real %s discovery", v.DynamicResourceKind()), func() {
			Eventually(func(g Gomega) {
				ids := ClusterIDs(g, h)
				g.Expect(ids).To(ContainElements("loopback-cluster", "prod-east", "prod-west"))
			}, Timeout, Interval).Should(Succeed())
		})

		It("marks a kubernaut.ai/managed=true Service as managed after a real sync cycle (SC-7, AC-3)", func() {
			svcName := v.ResourcePrefix() + "-managed-svc"
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

			Eventually(func(g Gomega) {
				managed := ScopeCheck(g, h, "loopback-cluster", "", "v1", "Service", testNS.Name, svcName)
				g.Expect(managed).To(BeTrue(),
					"resource labeled kubernaut.ai/managed=true should be discovered by FMC's real sync pipeline")
			}, SyncTimeout, Interval).Should(Succeed())
		})
	})
}
