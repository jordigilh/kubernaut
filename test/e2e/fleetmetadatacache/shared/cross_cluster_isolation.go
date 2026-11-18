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

	"github.com/jordigilh/kubernaut/pkg/shared/scope"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CrossClusterIsolation proves "prod-east" is backed by a genuinely
// separate Kubernetes control plane (DD-TEST-013, Spike S19), not the
// loopback pattern "loopback-cluster"/"prod-west" share: a resource created
// only in the remote cluster is visible through FMC's scope check for
// "prod-east" but invisible for "loopback-cluster"/"prod-west", and a
// resource created only in the primary cluster is visible for
// "loopback-cluster"/"prod-west" but invisible for "prod-east".
//
// Before DD-TEST-013, this scenario would have failed to distinguish
// anything meaningful: all three registered clusters targeted the same
// physical API server (the "loopback pattern" documented in
// SetupFleetE2EInfrastructure), so a resource created anywhere was
// trivially visible through every cluster ID. The reverse assertion below
// (a primary-only resource must NOT be visible via prod-east) is the one
// that would have caught a regression back to that shared-backend state.
//
// {ScenarioPrefix}-015, e.g. E2E-FMC-054-015 (Kuadrant) /
// E2E-FMC-EAIGW-054-015 (EAIGW).
//
// Authority: Issue #54, DD-TEST-013, ADR-068 (AC-4 information flow
// enforcement), SOC2 CC8.1.
func CrossClusterIsolation(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf(
		"%s-015: prod-east is backed by a genuinely separate Kubernetes control plane (DD-TEST-013)",
		v.ScenarioPrefix(),
	), Ordered, func() {
		It("reports a resource created only in the remote cluster as managed via prod-east, but not via loopback-cluster/prod-west", func() {
			svcName := fmt.Sprintf("%s-remote-only-%d", v.ResourcePrefix(), time.Now().UnixNano())
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: h.Namespace,
					Labels:    map[string]string{"kubernaut.ai/managed": "true"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.RemoteK8sClient.Create(h.Ctx, svc)).To(Succeed())
			DeferCleanup(func() {
				_ = h.RemoteK8sClient.Delete(h.Ctx, svc)
			})

			Eventually(func(g Gomega) {
				g.Expect(ScopeCheck(g, h, "prod-east", "", "v1", "Service", h.Namespace, svcName)).To(BeTrue(),
					"a resource created only in the remote cluster must be reported managed via prod-east's real, separate API server")
			}, SyncTimeout, Interval).Should(Succeed())

			Consistently(func(g Gomega) {
				ScopeCheckUnavailable(g, h, scope.ResourceIdentity{ClusterID: "loopback-cluster", Version: "v1", Kind: "Service", Namespace: h.Namespace, Name: svcName})
				ScopeCheckUnavailable(g, h, scope.ResourceIdentity{ClusterID: "prod-west", Version: "v1", Kind: "Service", Namespace: h.Namespace, Name: svcName})
			}, 10*time.Second, Interval).Should(Succeed())
		})

		It("reports a resource created only in the primary cluster as managed via loopback-cluster/prod-west, but not via prod-east", func() {
			svcName := fmt.Sprintf("%s-primary-only-%d", v.ResourcePrefix(), time.Now().UnixNano())
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: h.Namespace,
					Labels:    map[string]string{"kubernaut.ai/managed": "true"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, svc)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, svc)
			})

			Eventually(func(g Gomega) {
				g.Expect(ScopeCheck(g, h, "loopback-cluster", "", "v1", "Service", h.Namespace, svcName)).To(BeTrue(),
					"a resource created only in the primary cluster must be reported managed via loopback-cluster")
				g.Expect(ScopeCheck(g, h, "prod-west", "", "v1", "Service", h.Namespace, svcName)).To(BeTrue(),
					"the other primary-backed registration must also report the present resource managed")
			}, SyncTimeout, Interval).Should(Succeed())

			// This is the regression-sensitive assertion (DD-TEST-013): before
			// the cross-cluster bridge, prod-east shared the primary cluster's
			// API server, so a primary-only resource was trivially visible
			// here too. It must now report unavailable, proving prod-east's
			// backend is genuinely a different control plane.
			Consistently(func(g Gomega) {
				ScopeCheckUnavailable(g, h, scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "Service", Namespace: h.Namespace, Name: svcName})
			}, 10*time.Second, Interval).Should(Succeed())
		})
	})
}
