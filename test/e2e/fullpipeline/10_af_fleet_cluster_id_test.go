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
package fullpipeline

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// E2E-AF-1409-001: AU-3, CC8.1 — Fleet cluster_id survives the full AF ADK
// round-trip (LLM-supplied arg -> RR spec -> RRContext -> emitDecisionEvent)
// and is reconstructable from the SSE-visible investigation_summary artifact
// alone, closing the Console multi-cluster context-banner gap (#1409, ADR-065).
//
// Single-turn design (one message/send call): the "fleet cluster
// remediation" keyword matches a dedicated mock-LLM scenario
// (test/infrastructure/shared_e2e.go) that emits kubernaut_remediate with
// cluster_id="cluster-fleet-e2e-1409" (creates the RR), chained via
// NextToolCall into kubernaut_present_decision with NO cluster_id of its
// own. This deliberately exercises emitDecisionEvent's RRContext auto-fill
// path (part_converter.go, cycle 9) rather than LLM-supplied precedence
// (already covered by UT-AF-1409-006b) — proving cluster_id survives purely
// through AF's server-side RRContext plumbing, the same path a real
// takeover session would use.
//
// Single-turn is required here, not two separate message/send calls:
// RRContext lives on the per-request EventBridge (WithEventBridge is called
// fresh for every incoming request — see event_bridge.go/launcher.go), so a
// second, separate HTTP request to the same task would start with no
// RRContext at all and the auto-fill this test targets would never trigger.
// A two-turn variant was tried and confirmed this via must-gather (turn 2's
// investigation_summary artifact carried a nil cluster_id).
//
// The single-turn NextToolCall chain in turn was found to infinite-loop on
// first attempt: NextToolCall had no "fire once" guard, and because
// match_last_only keeps matching the same scenario, mock-llm kept
// re-emitting kubernaut_present_decision on every ADK reasoning round-trip
// until the client-side timeout (confirmed via must-gather: mock-llm log
// showed the scenario firing every ~0.5s for 60+ seconds). Fixed at the
// source (response.HasFunctionResponseNamed guard added to
// test/services/mock-llm, #1409): NextToolCall now stops firing once its
// own target tool has already responded anywhere in the conversation
// history. Verified against a live Kind cluster (test-e2e-fullpipeline
// infra) in the authoring environment after the fix.
var _ = Describe("AF Fleet cluster_id Artifact Propagation [E2E-AF-1409-001]", Label("fp", "af", "fleet", "issue-1409"), func() {

	It("should propagate LLM-supplied cluster_id through RRContext into the investigation_summary artifact", func() {
		fleetNS := fpRemediateNS["fleet"]
		Expect(fleetNS).NotTo(BeEmpty(), "fleet namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-AF-1409-001")
		}
		_ = resp.Body.Close()

		By("Sending single-turn A2A message: kubernaut_remediate(cluster_id) -> kubernaut_present_decision")
		body := fpA2ATasksSend("fp-fleet-1409-1", "fleet cluster remediation")
		resp, err = fpA2AInvokeWithTimeout(body, 60*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "A2A message/send should return 200")

		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A should not return a JSON-RPC error")

		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  E2E-AF-1409-001 task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying the RR spec carries cluster_id (AU-3, SI-4: LLM arg -> RR spec)")
		// Cluster-aware fingerprint (#1409): createOrReuseRR computes
		// rrFingerprintWithCluster(args.ClusterID, ...) — must match here, or this
		// RR (created with cluster_id="cluster-fleet-e2e-1409") is invisible to the
		// cluster-blind rrFingerprint("", ...) helper used by other FP scenarios.
		fp := gwtypes.CalculateClusterAwareFingerprint("cluster-fleet-e2e-1409", gwtypes.ResourceIdentifier{
			Namespace: fleetNS,
			Kind:      "Deployment",
			Name:      "memory-eater",
		})
		rrName := fpWaitForRRByFingerprint(fp, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())

		var rr remediationv1.RemediationRequest
		Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: rrName}, &rr)).To(Succeed())
		Expect(rr.Spec.ClusterID).To(Equal("cluster-fleet-e2e-1409"),
			"AU-3, SI-4: cluster_id must reach the RR spec for cross-cluster correlation")

		By("Verifying the investigation_summary artifact carries cluster_id (CC8.1: reconstructable from artifact alone)")
		art := fpFindArtifactBySchema(task, "investigation_summary")
		Expect(art).NotTo(BeNil(),
			"CC8.1: a kubernaut_present_decision call must produce an investigation_summary artifact on the completed Task")

		data := fpArtifactData(art)
		Expect(data).NotTo(BeNil(), "investigation_summary artifact must carry a DataPart")
		Expect(data["cluster_id"]).To(Equal("cluster-fleet-e2e-1409"),
			"CC8.1: cluster_id must be reconstructable from the investigation_summary artifact alone via RRContext auto-fill "+
				"(emitDecisionEvent, part_converter.go) — the LLM's kubernaut_present_decision call itself carried no cluster_id")
		GinkgoWriter.Printf("  E2E-AF-1409-001: investigation_summary artifact cluster_id=%v\n", data["cluster_id"])
	})
})
