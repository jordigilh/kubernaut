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
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Resilience proves FMC's real resilience journey against a genuine Valkey
// dependency failure -- something no lower tier can prove:
//   - UT (handler_test.go UT-FMC-API-014b/014c) only proves ReadyzHandler's branching
//     logic against a mockPinger.
//   - IT (fmc_broker_readiness_test.go IT-FMC-BROKER-001) only proves the syncer's
//     backoff loop against a fake ReaderFactoryFunc closure that returns errors --
//     never a real Valkey process dying and a real Kubernetes Deployment healing it.
//
// 100% gateway-agnostic: this journey exercises FMC's own dependency on
// Valkey, never the MCP Gateway edge.
//
// The outage is induced by scaling the Valkey Deployment to 0 replicas (not
// `kubectl delete pod --force`): a force-deleted pod's API-level removal is not
// synchronized with the kubelet actually tearing down the container/network, so
// the old Valkey process can keep serving traffic for a window after the delete
// call returns, making a "genuinely unreachable" assertion racy. Scaling to 0
// guarantees zero backing pods -- and thus zero Service endpoints -- for a
// deterministic outage window; scaling back to 1 exercises the same real
// Deployment self-heal path.
//
// {ScenarioPrefix}-012, e.g. E2E-FMC-054-012 (Kuadrant) /
// E2E-FMC-EAIGW-054-012 (EAIGW).
//
// Serial: both lanes run with --procs>1 against one shared Kind cluster.
// Taking Valkey down would corrupt any concurrently-running spec in
// {ScenarioPrefix}-010/011 that depends on cache continuity, so this
// Describe is marked Serial to guarantee no other spec runs while Valkey is
// down.
//
// Authority: Issue #54, ADR-068 (SI-4 health detection, CP-10 auto-reconstitution).
func Resilience(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf("%s-012: FMC recovers from a real Valkey dependency failure", v.ScenarioPrefix()), Ordered, Serial, func() {
		var testNS *corev1.Namespace

		BeforeAll(func() {
			testNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-resilience-%d", v.ResourcePrefix(), time.Now().UnixNano()),
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, testNS)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, testNS)
			})
		})

		It("degrades /readyz then auto-recovers after a real Valkey outage (SI-4, CP-10)", func() {
			By("Confirming baseline: /readyz is healthy before the failure")
			Eventually(func(g Gomega) int {
				return ReadyzStatus(g, h)
			}, Timeout, Interval).Should(Equal(http.StatusOK), "baseline /readyz must be healthy")

			By("Scaling Valkey to 0 replicas to induce a genuine, deterministic outage")
			scaleDownCtx, scaleDownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer scaleDownCancel()
			scaleDownCmd := exec.CommandContext(scaleDownCtx, "kubectl", "--kubeconfig", h.KubeconfigPath,
				"-n", h.Namespace, "scale", "deployment/valkey", "--replicas=0")
			scaleDownCmd.Stdout = GinkgoWriter
			scaleDownCmd.Stderr = GinkgoWriter
			Expect(scaleDownCmd.Run()).To(Succeed(), "kubectl scale deployment/valkey --replicas=0 should succeed")

			waitDownCtx, waitDownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer waitDownCancel()
			waitDownCmd := exec.CommandContext(waitDownCtx, "kubectl", "--kubeconfig", h.KubeconfigPath,
				"-n", h.Namespace, "wait", "--for=delete", "pod", "-l", "app=valkey", "--timeout=30s")
			waitDownCmd.Stdout = GinkgoWriter
			waitDownCmd.Stderr = GinkgoWriter
			Expect(waitDownCmd.Run()).To(Succeed(), "Valkey pod must actually terminate before asserting an outage")

			By("Verifying FMC detects the failure: /readyz reports 503 (SI-4 -- no silent false-healthy)")
			Eventually(func(g Gomega) int {
				return ReadyzStatus(g, h)
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(http.StatusServiceUnavailable),
				"SI-4: /readyz must report 503 while Valkey has zero replicas")

			By("Scaling Valkey back to 1 replica to let it self-heal")
			scaleUpCtx, scaleUpCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer scaleUpCancel()
			scaleUpCmd := exec.CommandContext(scaleUpCtx, "kubectl", "--kubeconfig", h.KubeconfigPath,
				"-n", h.Namespace, "scale", "deployment/valkey", "--replicas=1")
			scaleUpCmd.Stdout = GinkgoWriter
			scaleUpCmd.Stderr = GinkgoWriter
			Expect(scaleUpCmd.Run()).To(Succeed(), "kubectl scale deployment/valkey --replicas=1 should succeed")

			waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer waitCancel()
			rolloutCmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", h.KubeconfigPath,
				"rollout", "status", "deployment/valkey", "-n", h.Namespace, "--timeout=120s")
			rolloutCmd.Stdout = GinkgoWriter
			rolloutCmd.Stderr = GinkgoWriter
			Expect(rolloutCmd.Run()).To(Succeed(), "Valkey deployment should become ready again")

			By("Verifying FMC auto-recovers without its own restart (CP-10)")
			Eventually(func(g Gomega) int {
				return ReadyzStatus(g, h)
			}, 60*time.Second, 2*time.Second).Should(Equal(http.StatusOK),
				"CP-10: FMC must auto-reconnect to Valkey without requiring its own restart")

			By("Verifying the sync pipeline actually resumes writing fresh entries, not just that the ping succeeds")
			svcName := v.ResourcePrefix() + "-resilience-svc"
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
					"post-recovery sync must resume writing fresh managed entries to Valkey")
			}, SyncTimeout, Interval).Should(Succeed())
		})
	})
}
