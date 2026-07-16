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

package fleet

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/scenarios"
)

// E2E-FLEET-014: crashloop-config-fix-v1 performs a real, verifiable
// cross-cluster fix.
// Authority: Issue #1542, Issue #54, ADR-068
// FedRAMP: AC-3, AC-4, SI-4 (cross-cluster remediation with real MCP writes)
//
// A real busybox Deployment is deployed on the genuinely REMOTE cluster
// (DD-TEST-013) with a bad ConfigMap value, producing a real CrashLoopBackOff
// there. Since no event-exporter bridges remote K8s events to the hub
// Gateway, the signal is delivered as a synthetic Prometheus-style alert
// carrying cluster_id=remote-cluster (mirrors how a real federated
// AlertManager would report it). This proves WE dispatches the
// crashloop-config-fix-v1 Job to the REMOTE cluster via the MCP gateway
// (passthrough auth) and that the Job genuinely patches the ConfigMap and
// restarts the Deployment there — not a no-op simulation (see remediate.sh,
// Issue #1542).
var _ = Describe("E2E-FLEET-014 [AC-3, AC-4, SI-4]: CrashLoop config fix performs a real cross-cluster fix (BR-INTEGRATION-054)", Label("fleet", "crashloop"), func() {
	It("should patch the offending ConfigMap and restart the deployment on the remote cluster [E2E-FLEET-014]", func() {
		Expect(workflowUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))

		By("Step 1: Deploying crashloop-app on the REMOTE cluster (bad ConfigMap, real CrashLoopBackOff)")
		Expect(infrastructure.DeployCrashLoopConfigApp(ctx, namespace, remoteKubeconfigPath, GinkgoWriter)).To(Succeed())
		DeferCleanup(func() {
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: infrastructure.CrashLoopAppName, Namespace: namespace}}
			_ = remoteK8sClient.Delete(context.Background(), dep)
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: infrastructure.CrashLoopAppConfigMapName, Namespace: namespace}}
			_ = remoteK8sClient.Delete(context.Background(), cm)
		})

		By("Step 1b: Waiting for the real CrashLoopBackOff on the remote cluster...")
		// Timeout widened from 2m to 4m (CI runs 29234664178, 29458356036): must-gather
		// events on the shared single-node "fleet-e2e-remote" Kind cluster show a
		// node-wide ~90-100s gap between image-pull-complete and the kubelet's first
		// container Started event under concurrent parallel-spec load (this test races
		// E2E-FLEET-015's OOMKill deployment and the suite-wide memory-eater fixture for
		// the same node's CPU). Reaching CrashLoopBackOff needs a full start->crash->
		// backoff cycle after that delay, which left near-zero margin in a fixed 2m
		// budget. E2E-FLEET-015 tolerates the same delay because it also accepts an
		// earlier OOMKilled termination-state signal, not just CrashLoopBackOff.
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := remoteK8sClient.List(ctx, pods, client.InNamespace(namespace),
				client.MatchingLabels{"app": infrastructure.CrashLoopAppName}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == "CrashLoopBackOff" {
						GinkgoWriter.Printf("  ✅ CrashLoopBackOff detected on remote cluster: restarts=%d\n", cs.RestartCount)
						return true
					}
				}
			}
			return false
		}, 4*time.Minute, 2*time.Second).Should(BeTrue(), "crashloop-app should reach CrashLoopBackOff on the remote cluster")

		By("Step 2: Sending synthetic cluster-tagged alert to Gateway (AC-4, no real event-exporter bridge)")
		payload := buildPrometheusAlertWithCluster("KubePodCrashLooping", namespace, "high",
			"Deployment", infrastructure.CrashLoopAppName, "remote-cluster")

		gatewayURL := "http://localhost:30080"
		_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))
		rrName, ok := response["remediationRequestName"].(string)
		Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

		By("Steps 3-13: Driving the shared crashloop-config-fix-v1 assertion flow across clusters (Issue #1542)")
		scenarios.RunCrashLoopConfigFixScenario(scenarios.CrashLoopConfigFixScenarioConfig{
			Ctx:                    ctx,
			CRDClient:              apiReader,
			TargetClient:           remoteK8sClient,
			JobClient:              remoteK8sClient,
			CRDNamespace:           namespace,
			TargetNamespace:        namespace,
			JobNamespace:           infrastructure.ExecutionNamespace,
			RemediationRequestName: rrName,
			ExpectClusterID:        "remote-cluster",
			ExpectedWorkflowID:     workflowUUIDs["crashloop-config-fix-v1:production"],
			Timeout:                timeout,
			Interval:               interval,
		})
	})
})
