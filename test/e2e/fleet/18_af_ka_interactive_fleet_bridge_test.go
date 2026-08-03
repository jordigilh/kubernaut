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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// kaInteractiveFleetTargetName/Namespace mirror the mock-llm scenario's
// constants of the same name (test/services/mock-llm/scenarios/scenario_ka_interactive_fleet_bridge.go)
// -- duplicated as literals since E2E tests don't import mock-llm packages
// (same convention as kaToolE2ETargetName in 17_ka_real_fleet_investigation_test.go).
const kaInteractiveFleetTargetName = "ka-interactive-fleet-target"
const kaInteractiveFleetTargetNamespace = "kubernaut-system"

// kaInteractiveFleetEvidence mirrors the mock-llm scenario's memory-limit
// evidence constant. Deliberately distinct from #17's kaToolE2ELocal/RemoteEvidence
// (111Mi/222Mi) and #16's coredns marker so all three fleet E2E fixtures can
// coexist under parallel Ginkgo execution without cross-matching.
const kaInteractiveFleetEvidence = "247Mi"

// E2E-FLEET-018 [AC-4, AC-6, AU-3]: AF's kubernaut_message tool, sent over a
// real A2A request, drives KA's real RunInteractiveTurn through the fleet
// overlay to a genuinely remote cluster, closing issue #1768 Track 2 Gap D.
//
// Authority: issue #1768, docs/testing/1768/TEST_PLAN_TRACK2.md
// (BR-INTEGRATION-054, BR-FLEET-054).
//
// Why this test exists (what it closes that no other test does):
//   - UT-KA-FLEET-026/027 and IT-KA-FLEET-022 (internal/kubernautagent/investigator,
//     internal/kubernautagent/mcp/tools) prove RunInteractiveTurn's
//     prescopeFleetOverlay in isolation and through KA's own production
//     dispatch path, but never through a real, compiled AF binary's
//     kubernaut_message tool over a real A2A request.
//   - E2E-FLEET-016 (Gaps A+C) proves AF's OWN autonomous kubectl_get tool
//     call reaches a real remote cluster, but never touches KA or the
//     interactive bridge.
//   - E2E-FLEET-017 (issue #1729) proves KA's AUTONOMOUS investigation loop
//     calls the correct fleet-scoped tool, but only for the initial RCA
//     phase -- never for a follow-up interactive turn triggered by
//     kubernaut_message after the autonomous investigation has already
//     completed.
//   - test/e2e/fullpipeline/08_af_a2a_interactive_test.go exercises a
//     multi-turn interactive conversation (investigate -> discover_workflows
//     -> select_workflow -> watch) but never sends kubernaut_message, and
//     fullpipeline is a single-cluster suite where "remote" is a fiction.
//
// This test closes the gap by driving AF's real A2A endpoint through three
// turns -- kubernaut_remediate (creates a new RR scoped to cluster_id
// "remote-cluster"), kubernaut_investigate (blocks until KA's autonomous
// investigation establishes the interactive session), then kubernaut_message
// (continues that session) -- against a dedicated marker Deployment that
// exists ONLY on the genuinely separate remote Kind cluster (DD-TEST-013).
// Three turns, not two, mirroring test/e2e/fullpipeline/08_af_a2a_interactive_test.go's
// established remediate->investigate->message/discover_workflows/... flow:
// investigate_start.go's handleStart requires an rr_id that already
// references an existing RemediationRequest (ErrCodeRRNotFound otherwise),
// and investigate_takeover.go's handleMessage requires an active driver
// session (authorizeActiveDriver) that only a prior kubernaut_investigate
// call establishes. CI RCA for run 30828771399 (job 91740902692) proved a
// 2-turn design (kubernaut_investigate then kubernaut_message) broken:
// InvestigateOutput carries no rr_id field for kubernaut_message's
// "$from_tool:kubernaut_investigate:rr_id" to resolve, so the message turn
// silently ran with an empty rr_id and never reached RunInteractiveTurn at
// all. The evidence value asserted on is invisible to both the alert/RR
// payload and SignalProcessing's own enrichment (see
// scenario_ka_interactive_fleet_bridge.go's doc comment), so its presence
// in the kubernaut_message turn's A2A artifact text can only come from a
// genuine RunInteractiveTurn -> prescopeFleetOverlay -> remote-cluster tool
// round trip -- not a canned response and not a silent fallback to the
// hub-local registry.
var _ = Describe("E2E-FLEET-018 [AC-4, AC-6, AU-3]: AF kubernaut_message drives KA's interactive bridge to the fleet-scoped cluster (issue #1768 Track 2 Gap D)", Label("fleet", "af", "ka", "a2a", "interactive", "issue-1768"), func() {

	It("should route a kubernaut_message turn through the fleet overlay and return genuine remote-cluster evidence", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in Fleet E2E cluster — skipping E2E-FLEET-018")
		}
		_ = resp.Body.Close()

		By("Deploying the dedicated ka-interactive-fleet-target marker on the REMOTE cluster only")
		Expect(infrastructure.DeployMemoryEaterNamed(ctx, kaInteractiveFleetTargetName, kaInteractiveFleetTargetNamespace,
			remoteKubeconfigPath, kaInteractiveFleetEvidence, "20Mi", GinkgoWriter)).To(Succeed())
		DeferCleanup(func() {
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: kaInteractiveFleetTargetName, Namespace: kaInteractiveFleetTargetNamespace}}
			_ = remoteK8sClient.Delete(context.Background(), dep)
		})

		By("Waiting for the marker Deployment to become Available on the remote cluster")
		Eventually(func(g Gomega) {
			dep := &appsv1.Deployment{}
			g.Expect(remoteK8sClient.Get(ctx, client.ObjectKey{Name: kaInteractiveFleetTargetName, Namespace: kaInteractiveFleetTargetNamespace}, dep)).To(Succeed())
			g.Expect(dep.Status.AvailableReplicas).To(BeNumerically(">=", 1))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("Turn 1: kubernaut_remediate (creates RR with cluster_id=remote-cluster targeting the marker deployment)")
		body := afA2ATasksSend("fleet-018-1", "ka-interactive-fleet-bridge-start")
		resp1, err := afA2AInvokeWithTimeout(body, 60*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp1.Body.Close() }()
		Expect(resp1.StatusCode).To(Equal(http.StatusOK), "Turn 1 A2A message/send should return 200")
		rpc, parseErr := afParseRPC(resp1)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 1 should not return a JSON-RPC error")
		task, taskErr := afExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty(), "Turn 1 A2A task ID must not be empty")
		GinkgoWriter.Printf("  E2E-FLEET-018 Turn 1 — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Turn 2: kubernaut_investigate (blocks until KA's autonomous investigation establishes the interactive session)")
		body = afA2ATasksSendWithTask("fleet-018-2", taskID, "investigate the remediation")
		resp2, err := afA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK), "Turn 2 A2A message/send should return 200")
		rpc, parseErr = afParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 should not return a JSON-RPC error")
		task2, taskErr := afExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		GinkgoWriter.Printf("  E2E-FLEET-018 Turn 2 — task: %s (state: %s)\n", task2.ID, task2.Status.State)

		By("Turn 3: kubernaut_message (continues the interactive session, triggers RunInteractiveTurn's fleet overlay)")
		body = afA2ATasksSendWithTask("fleet-018-3", taskID, "ka-interactive-fleet-e2e-test")
		resp3, err := afA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK), "Turn 3 A2A message/send should return 200")
		rpc, parseErr = afParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 should not return a JSON-RPC error")
		task3, taskErr := afExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		GinkgoWriter.Printf("  E2E-FLEET-018 Turn 3 — task: %s (state: %s)\n", task3.ID, task3.Status.State)

		By("Verifying the kubernaut_message turn's artifact carries genuine remote-cluster evidence (AC-4: fleet-scoped interactive routing)")
		msgStr := afArtifactText(task3)
		Expect(msgStr).NotTo(BeEmpty(),
			"completed kubernaut_message task must carry accumulated artifact text with the echoed tool result")
		Expect(msgStr).To(ContainSubstring(kaInteractiveFleetEvidence),
			"AC-4/AC-6, AU-3: the remote cluster's live memory-limit value must surface in the kubernaut_message "+
				"turn's A2A artifact, proving RunInteractiveTurn's prescopeFleetOverlay routed this specific "+
				"interactive turn's tool call to the fleet-scoped cluster_id, not the hub-local registry and "+
				"not a canned response")
		GinkgoWriter.Printf("  E2E-FLEET-018: confirmed kubernaut_message routed through the fleet overlay to the remote cluster\n")
	})
})
