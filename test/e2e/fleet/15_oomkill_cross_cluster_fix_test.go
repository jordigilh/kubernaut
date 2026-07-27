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

// oomkillCrossClusterAppName is a dedicated memory-eater instance for this
// test, distinct from the suite-level "memory-eater" fixture that
// 01_signal_ingestion_test.go and 03_ro_clusterid_routing_test.go already
// depend on existing (unmanipulated) in kubernaut-system for the duration of
// the suite. Reusing that shared name here would race those tests across
// Ginkgo's parallel processes.
const oomkillCrossClusterAppName = "memory-eater-oom-cc"

// E2E-FLEET-015: oomkill-increase-memory-v1 performs a real, verifiable
// cross-cluster fix.
// Authority: Issue #1542, Issue #54, ADR-068
// FedRAMP: AC-3, AC-4, SI-4 (cross-cluster remediation with real MCP writes)
//
// A real memory-eater Deployment is deployed on the genuinely REMOTE cluster
// (DD-TEST-013) with a memory limit below its growth target, producing a
// real OOMKill there. Since no event-exporter bridges remote K8s events to
// the hub Gateway, the signal is delivered as a synthetic Prometheus-style
// alert carrying cluster_id=remote-cluster (mirrors how a real federated
// AlertManager would report it). This proves WE dispatches the
// oomkill-increase-memory-v1 Job to the REMOTE cluster via the MCP gateway
// (passthrough auth) and that the Job genuinely patches the Deployment's
// memory limit and lets it recover there — not a no-op simulation (see
// remediate.sh, Issue #1542 follow-up).
var _ = Describe("E2E-FLEET-015 [AC-3, AC-4, SI-4]: OOMKill increase-memory fix performs a real cross-cluster fix (BR-INTEGRATION-054)", Label("fleet", "oomkill"), func() {
	It("should patch the offending Deployment's memory limit and let it recover on the remote cluster [E2E-FLEET-015]", func() {
		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"))

		By("Step 1: Deploying a dedicated memory-eater on the REMOTE cluster (real OOMKill)")
		Expect(infrastructure.DeployMemoryEaterNamed(ctx, oomkillCrossClusterAppName, namespace,
			remoteKubeconfigPath, "50Mi", "20Mi", GinkgoWriter)).To(Succeed())
		DeferCleanup(func() {
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: oomkillCrossClusterAppName, Namespace: namespace}}
			_ = remoteK8sClient.Delete(context.Background(), dep)
		})

		By("Step 1b: Waiting for the real OOMKill on the remote cluster...")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := remoteK8sClient.List(ctx, pods, client.InNamespace(namespace),
				client.MatchingLabels{"app": oomkillCrossClusterAppName}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						GinkgoWriter.Printf("  ✅ OOMKill detected on remote cluster: restarts=%d\n", cs.RestartCount)
						return true
					}
					if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
						GinkgoWriter.Println("  ✅ OOMKill terminated state detected on remote cluster")
						return true
					}
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == "CrashLoopBackOff" {
						GinkgoWriter.Printf("  ✅ CrashLoopBackOff detected on remote cluster (OOMKill): restarts=%d\n", cs.RestartCount)
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill on the remote cluster")

		By("Step 2: Sending synthetic cluster-tagged alert to Gateway (AC-4, no real event-exporter bridge)")
		// "OOMKilled" hits oomkilledScenario's high-confidence pattern match
		// directly (test/services/mock-llm/scenarios/scenario_oomkilled.go),
		// avoiding the ambiguous generic "BackOff" fallback path that could
		// also match crashloopScenario.
		payload := buildPrometheusAlertWithCluster("OOMKilled", "critical",
			oomkillCrossClusterAppName, "remote-cluster")

		gatewayURL := urlLocalhost30080
		body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))
		rrName, ok := response["remediationRequestName"].(string)
		Expect(ok).To(BeTrue(), "Response must contain remediationRequestName")

		By("Steps 3-13: Driving the shared oomkill-increase-memory-v1 assertion flow across clusters (Issue #1542 follow-up)")
		scenarios.RunOOMKillIncreaseMemoryScenario(scenarios.OOMKillIncreaseMemoryScenarioConfig{
			Ctx:                    ctx,
			CRDClient:              apiReader,
			TargetClient:           remoteK8sClient,
			JobClient:              remoteK8sClient,
			CRDNamespace:           namespace,
			TargetNamespace:        namespace,
			TargetDeploymentName:   oomkillCrossClusterAppName,
			JobNamespace:           infrastructure.ExecutionNamespace,
			RemediationRequestName: rrName,
			ExpectClusterID:        "remote-cluster",
			ExpectedWorkflowID:     workflowUUIDs["oomkill-increase-memory-v1:production"],
			ExpectedMemoryLimit:    "512Mi",
			Timeout:                timeout,
			Interval:               interval,
		})
	})
})
