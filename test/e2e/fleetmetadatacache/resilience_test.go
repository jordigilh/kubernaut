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
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// readyzStatus queries FMC's real /readyz endpoint (wired in cmd/fleetmetadatacache/main.go
// via fmc.ReadyzHandler backed by scopecache.ValkeyCacheReader.Ping) and returns the HTTP
// status code. No path constant is exported for /readyz (unlike ScopeCheckPath/ClustersPath)
// since it is a Kubernetes probe endpoint, not a public API contract.
func readyzStatus(g Gomega) int {
	resp, err := fmcHTTPClient.Get(fmcAPIBaseURL + "/readyz")
	g.Expect(err).ToNot(HaveOccurred(), "/readyz request failed")
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// E2E-FMC-054-012: Proves FMC's real resilience journey against a genuine Valkey
// dependency failure -- something no lower tier can prove:
//   - UT (handler_test.go UT-FMC-API-014b/014c) only proves ReadyzHandler's branching
//     logic against a mockPinger.
//   - IT (fmc_broker_readiness_test.go IT-FMC-BROKER-001) only proves the syncer's
//     backoff loop against a fake ReaderFactoryFunc closure that returns errors --
//     never a real Valkey process dying and a real Kubernetes Deployment healing it.
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
// This test confirms FMC's /readyz genuinely flips to 503 while Valkey has no
// replicas (SI-4: proactive failure detection, not a silent false-healthy), then
// confirms FMC reconnects and resumes writing fresh cache entries once Valkey is
// scaled back up, WITHOUT requiring FMC's own restart (CP-10: auto-reconstitution).
//
// Serial: this suite runs with --procs>1 (Makefile test-e2e-fleetmetadatacache-kuadrant) against
// one shared Kind cluster. Taking Valkey down would corrupt any concurrently-running
// spec in E2E-FMC-054-010/011 that depends on cache continuity, so this Describe is
// marked Serial to guarantee no other spec runs while Valkey is down.
//
// Authority: Issue #54, ADR-068 (SI-4 health detection, CP-10 auto-reconstitution).
var _ = Describe("E2E-FMC-054-012: FMC recovers from a real Valkey dependency failure", Ordered, Serial, func() {
	var testNS *corev1.Namespace

	BeforeAll(func() {
		testNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("fmc-e2e-resilience-%d", time.Now().UnixNano()),
			},
		}
		Expect(k8sClient.Create(ctx, testNS)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, testNS)
		})
	})

	It("degrades /readyz then auto-recovers after a real Valkey outage (SI-4, CP-10)", func() {
		By("Confirming baseline: /readyz is healthy before the failure")
		Eventually(func(g Gomega) int {
			return readyzStatus(g)
		}, timeout, interval).Should(Equal(http.StatusOK), "baseline /readyz must be healthy")

		By("Scaling Valkey to 0 replicas to induce a genuine, deterministic outage")
		scaleDownCtx, scaleDownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer scaleDownCancel()
		scaleDownCmd := exec.CommandContext(scaleDownCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", namespace, "scale", "deployment/valkey", "--replicas=0")
		scaleDownCmd.Stdout = GinkgoWriter
		scaleDownCmd.Stderr = GinkgoWriter
		Expect(scaleDownCmd.Run()).To(Succeed(), "kubectl scale deployment/valkey --replicas=0 should succeed")

		waitDownCtx, waitDownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer waitDownCancel()
		waitDownCmd := exec.CommandContext(waitDownCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", namespace, "wait", "--for=delete", "pod", "-l", "app=valkey", "--timeout=30s")
		waitDownCmd.Stdout = GinkgoWriter
		waitDownCmd.Stderr = GinkgoWriter
		Expect(waitDownCmd.Run()).To(Succeed(), "Valkey pod must actually terminate before asserting an outage")

		By("Verifying FMC detects the failure: /readyz reports 503 (SI-4 -- no silent false-healthy)")
		Eventually(func(g Gomega) int {
			return readyzStatus(g)
		}, 30*time.Second, 500*time.Millisecond).Should(Equal(http.StatusServiceUnavailable),
			"SI-4: /readyz must report 503 while Valkey has zero replicas")

		By("Scaling Valkey back to 1 replica to let it self-heal")
		scaleUpCtx, scaleUpCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer scaleUpCancel()
		scaleUpCmd := exec.CommandContext(scaleUpCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"-n", namespace, "scale", "deployment/valkey", "--replicas=1")
		scaleUpCmd.Stdout = GinkgoWriter
		scaleUpCmd.Stderr = GinkgoWriter
		Expect(scaleUpCmd.Run()).To(Succeed(), "kubectl scale deployment/valkey --replicas=1 should succeed")

		waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer waitCancel()
		rolloutCmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/valkey", "-n", namespace, "--timeout=120s")
		rolloutCmd.Stdout = GinkgoWriter
		rolloutCmd.Stderr = GinkgoWriter
		Expect(rolloutCmd.Run()).To(Succeed(), "Valkey deployment should become ready again")

		By("Verifying FMC auto-recovers without its own restart (CP-10)")
		Eventually(func(g Gomega) int {
			return readyzStatus(g)
		}, 60*time.Second, 2*time.Second).Should(Equal(http.StatusOK),
			"CP-10: FMC must auto-reconnect to Valkey without requiring its own restart")

		By("Verifying the sync pipeline actually resumes writing fresh entries, not just that the ping succeeds")
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fmc-e2e-resilience-svc",
				Namespace: testNS.Name,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())

		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "loopback-cluster", "", "v1", "Service", testNS.Name, "fmc-e2e-resilience-svc")
			g.Expect(managed).To(BeTrue(),
				"post-recovery sync must resume writing fresh managed entries to Valkey")
		}, fmcSyncTimeout, interval).Should(Succeed())
	})
})
