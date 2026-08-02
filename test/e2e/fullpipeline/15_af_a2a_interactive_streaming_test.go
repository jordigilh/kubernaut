package fullpipeline

import (
	"context"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E-FP-1189-005: Console-equivalent interactive remediation — replays the
// same 5-turn conversation as E2E-FP-1189-003/-004, but over message/stream
// (SSE, Accept: text/event-stream) instead of message/send.
//
// Rationale (session gap analysis, issue #1189): before this test, "what an
// interactive Console user sees" (live SSE frames — reasoning/progress text,
// terminal state) and "the core of interactive remediation" (a real RR
// driven through RO→SP→AA→KA→WE to Completed) were each fully covered, but
// never in the same test:
//   - E2E-FP-1189-003/-004 drive the real pipeline to completion, but via
//     synchronous message/send — one blocking JSON response per turn, never
//     what the Console UI actually renders to the SRE mid-turn.
//   - test/e2e/apifrontend's streaming_test.go proves SSE frames carry real
//     content, but that suite's infrastructure never deploys Gateway/RO/SP/AA/WE
//     — its RRs are inert CRDs that nothing reconciles further.
//
// This test closes that gap: every turn uses the SSE transport AND the
// assertions require the full pipeline to reach a real WorkflowExecution
// Completed phase, proving the Console-facing experience and the underlying
// remediation are the same code path end-to-end.
var _ = Describe("AF A2A Interactive Streaming Full Pipeline [E2E-FP-1189-005]", Label("fp", "af", "a2a", "interactive", "streaming", "issue-1189"), func() {

	It("should stream 5 SSE turns with visible content and complete the full pipeline", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["interactive-streaming"]
		Expect(targetNS).NotTo(BeEmpty(), "interactive-streaming namespace must be set by SynchronizedBeforeSuite")
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-005")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the interactive-streaming RR")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNS,
				Labels: map[string]string{
					"kubernaut.ai/managed":     "true",
					"kubernaut.ai/environment": "staging",
				},
			},
		}
		if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", targetNS)
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns, &client.DeleteOptions{})
		})

		By("Deploying zero-replica target Deployment in isolated namespace")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "memory-eater",
				Namespace: targetNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "memory-eater"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "memory-eater"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "busybox:1.36",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		var sawVisibleContent bool
		const sharedCtxID = "ctx-fp-ints-shared"

		By("Turn 1 (SSE): create a remediation request (kubernaut_remediate — interactive RR)")
		// #1795-adjacent bug found empirically: the mock-LLM keyword matcher does a
		// case-insensitive substring Contains() (scenarios/match_helpers.go), and
		// per-namespace kubernaut_remediate scenarios are keyed on "<afRemediateNS
		// key> remediation" (test/infrastructure/shared_e2e.go). "interactive
		// remediation" is itself a substring of "interactive-streaming remediation",
		// so using the same wording as E2E-FP-1189-003/-004's Turn 1 message here
		// cross-matched the "interactive" scenario (wrong namespace) whenever both
		// specs ran in the same parallel Ginkgo process batch, routing this test's
		// RR into E2E-FP-1189-003's namespace and leaving fpWaitForRRWithTargetNS
		// waiting forever on this test's own (empty) namespace. Must say
		// "interactive-streaming remediation" verbatim to uniquely match this
		// scenario's keyword.
		body := fpA2AMessageStreamWithContext("fp-ints-1", sharedCtxID,
			"create interactive-streaming remediation for deployment memory-eater")
		resp, err = fpA2ASSEInvoke(body, 60*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"),
			"AF must respond with a live SSE stream when Accept: text/event-stream is sent")
		arts1, statuses1 := fpScanSSEFrames(resp)
		_ = resp.Body.Close()
		Expect(len(arts1)+len(statuses1)).To(BeNumerically(">", 0),
			"turn 1 must produce at least one SSE frame (artifact or status)")
		sawVisibleContent = sawVisibleContent || fpSSEHasNonEmptyText(arts1, statuses1)
		GinkgoWriter.Printf("  Turn 1 (SSE) — %d artifacts, %d statuses, terminal=%s\n",
			len(arts1), len(statuses1), fpSSETerminalState(statuses1))

		By("Turn 2 (SSE): investigate the remediation (blocks until KA investigation completes)")
		body = fpA2AMessageStreamWithContext("fp-ints-2", sharedCtxID, "investigate the remediation")
		resp2, err := fpA2ASSEInvoke(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		arts2, statuses2 := fpScanSSEFrames(resp2)
		_ = resp2.Body.Close()
		Expect(len(arts2)+len(statuses2)).To(BeNumerically(">", 0),
			"turn 2 must produce at least one SSE frame")
		sawVisibleContent = sawVisibleContent || fpSSEHasNonEmptyText(arts2, statuses2)
		GinkgoWriter.Printf("  Turn 2 (SSE) — investigate: %d artifacts, %d statuses, terminal=%s\n",
			len(arts2), len(statuses2), fpSSETerminalState(statuses2))

		By("AU-3/SI-4: verifying the RCA was actually displayed to the user during turn 2, not just a blank ack")
		// A real Console user reads the RCA (severity/confidence/root cause) before
		// deciding which workflow to run. Production emits this either as a
		// progressive early_rca status-update (emitEarlyRCA, the normal path once
		// KA's investigation completes) or, if the bridge produced no events, as a
		// fallback investigation_summary artifact (emitFallbackInvestigationArtifact)
		// — see pkg/apifrontend/tools/ka_investigate_bridge.go. Either satisfies "the
		// user waited for the RCA to render"; accepting both avoids coupling this
		// test to which of the two internal paths KA happened to take.
		//
		// #1795 (AF->KA streaming MCP dial timeout) is fixed, and #1811 (KA
		// dropped every event emitted between InteractiveHold's fast RCA
		// completion and AF's late Subscribe) is fixed too — verified via
		// sink_nil/dropped counters in CI must-gather logs showing the fix
		// correctly buffers events for a session that stays alive.
		//
		// #1818 (fixed): when AA's autonomous submit to KA omits Interactive
		// (RequestBuilder never sets it) and the investigation reaches a
		// fully terminal StatusCompleted before AF's kubernaut_investigate
		// arrives, handleStart's reattachOrCreateFallback now seeds the fresh
		// interactive session with the real RCA from
		// GetLatestRCAResultByRemediationID (mode=interactive_reattached)
		// instead of the RCA-less placeholder, so the real RCA is no longer
		// orphaned. Combined with #1811's buffering (for the still-Running
		// case), both race outcomes now render real RCA content.
		earlyRCAStatuses := fpStatusesBySchema(statuses2, "early_rca")
		rcaArtifacts := fpArtifactsBySchema(arts2, "investigation_summary")
		Expect(len(earlyRCAStatuses)+len(rcaArtifacts)).To(BeNumerically(">", 0),
			"turn 2 must render RCA content via early_rca status or investigation_summary artifact (#1818)")
		if len(earlyRCAStatuses) > 0 {
			rcaText := fpStatusText(earlyRCAStatuses[0])
			Expect(strings.TrimSpace(rcaText)).NotTo(BeEmpty(), "early_rca status must carry non-empty RCA content")
			GinkgoWriter.Printf("  Turn 2 (SSE) — RCA displayed (early_rca): %s\n", rcaText)
		} else {
			data := fpArtifactDataPart(rcaArtifacts[0])
			Expect(data).NotTo(BeNil(), "investigation_summary artifact must carry a DataPart")
			GinkgoWriter.Printf("  Turn 2 (SSE) — RCA displayed (investigation_summary fallback): %v\n", data)
		}

		By("Turn 3 (SSE): discover available workflows")
		body = fpA2AMessageStreamWithContext("fp-ints-3", sharedCtxID, "discover available workflows")
		resp3, err := fpA2ASSEInvoke(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		arts3, statuses3 := fpScanSSEFrames(resp3)
		_ = resp3.Body.Close()
		Expect(len(arts3)+len(statuses3)).To(BeNumerically(">", 0),
			"turn 3 must produce at least one SSE frame")
		sawVisibleContent = sawVisibleContent || fpSSEHasNonEmptyText(arts3, statuses3)
		GinkgoWriter.Printf("  Turn 3 (SSE) — discover workflows: %d artifacts, %d statuses\n", len(arts3), len(statuses3))

		By("Turn 4 (SSE): select workflow")
		body = fpA2AMessageStreamWithContext("fp-ints-4", sharedCtxID, "select workflow oomkill-increase-memory-v1")
		resp4, err := fpA2ASSEInvoke(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		arts4, statuses4 := fpScanSSEFrames(resp4)
		_ = resp4.Body.Close()
		Expect(len(arts4)+len(statuses4)).To(BeNumerically(">", 0),
			"turn 4 must produce at least one SSE frame")
		sawVisibleContent = sawVisibleContent || fpSSEHasNonEmptyText(arts4, statuses4)
		GinkgoWriter.Printf("  Turn 4 (SSE) — select workflow: %d artifacts, %d statuses\n", len(arts4), len(statuses4))

		By("Turn 5 (SSE): watch remediation progress (blocks until terminal phase)")
		body = fpA2AMessageStreamWithContext("fp-ints-5", sharedCtxID, "watch remediation progress")
		resp5, err := fpA2ASSEInvoke(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp5.StatusCode).To(Equal(http.StatusOK))
		arts5, statuses5 := fpScanSSEFrames(resp5)
		_ = resp5.Body.Close()
		Expect(len(arts5)+len(statuses5)).To(BeNumerically(">", 0),
			"turn 5 must produce at least one SSE frame")
		sawVisibleContent = sawVisibleContent || fpSSEHasNonEmptyText(arts5, statuses5)
		finalState := fpSSETerminalState(statuses5)
		GinkgoWriter.Printf("  Turn 5 (SSE) — watch: %d artifacts, %d statuses, terminal=%s\n",
			len(arts5), len(statuses5), finalState)

		By("AU-6/SC-4: verifying the watch stream itself progressively rendered execution to completion")
		// This is the "user watches the UI render complete" assertion: it reads
		// the live execution_progress artifacts kubernaut_watch emits per RR phase
		// change (crd_tools_watch.go's handleRREvent -> BuildProgressSnapshot), not
		// an out-of-band CRD poll. The final one must carry completed_at, proving
		// the terminal phase was rendered through the SSE stream the Console
		// actually consumes -- the fpWaitForWEComplete poll below is a secondary,
		// independent confirmation of the underlying business outcome only.
		progressArtifacts := fpArtifactsByMetaType(arts5, "execution_progress")
		Expect(progressArtifacts).NotTo(BeEmpty(),
			"turn 5 must stream at least one metadata.type=execution_progress artifact")
		var phasesSeen []string
		for _, art := range progressArtifacts {
			if data := fpArtifactDataPart(art); data != nil {
				if phase, _ := data["current_phase"].(string); phase != "" {
					phasesSeen = append(phasesSeen, phase)
				}
			}
		}
		GinkgoWriter.Printf("  Turn 5 (SSE) — execution_progress phases observed: %v\n", phasesSeen)
		lastProgressData := fpArtifactDataPart(progressArtifacts[len(progressArtifacts)-1])
		Expect(lastProgressData).NotTo(BeNil(), "final execution_progress artifact must carry a DataPart")
		completedAt, _ := lastProgressData["completed_at"].(string)
		Expect(completedAt).NotTo(BeEmpty(),
			"the final execution_progress artifact rendered to the SSE caller must carry completed_at, "+
				"proving the Console-facing stream itself (not just a background poll) observed completion")

		By("AU-6/SC-4: verifying the SSE stream actually rendered visible content to the caller")
		Expect(sawVisibleContent).To(BeTrue(),
			"at least one turn's SSE stream must carry non-empty artifact/status text — "+
				"this is what an interactive Console user would actually see render")
		Expect(finalState).To(Equal("completed"),
			"turn 5's SSE stream must reach a terminal 'completed' state (stream_closed)")

		By("Verifying full pipeline completed (secondary confirmation via CRD poll)")
		rrName := fpWaitForRRWithTargetNS(targetNS, 30*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpWaitForWEComplete(rrName, 60*time.Second)
		GinkgoWriter.Printf("  Full pipeline completed for %s\n", rrName)

		By("Verifying interactive WFE has TARGET_RESOURCE_* parameters (parity with E2E-FP-1189-004)")
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		Expect(apiReader.List(ctx, weList, client.InNamespace(namespace))).To(Succeed())
		var we *workflowexecutionv1.WorkflowExecution
		for i := range weList.Items {
			if weList.Items[i].Spec.RemediationRequestRef.Name == rrName {
				we = &weList.Items[i]
				break
			}
		}
		Expect(we).NotTo(BeNil(), "WorkflowExecution for RR %s must exist", rrName)
		params := we.Spec.Parameters
		Expect(params).ToNot(BeNil(), "interactive WFE must have parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "memory-eater"),
			"TARGET_RESOURCE_NAME must be injected into interactive WFE parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
			"TARGET_RESOURCE_KIND must be injected into interactive WFE parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", targetNS),
			"TARGET_RESOURCE_NAMESPACE must be injected into interactive WFE parameters")
		GinkgoWriter.Printf("  [E2E-FP-1189-005] WFE params: TARGET_RESOURCE_NAME=%s, KIND=%s, NAMESPACE=%s\n",
			params["TARGET_RESOURCE_NAME"], params["TARGET_RESOURCE_KIND"], params["TARGET_RESOURCE_NAMESPACE"])
	})
})
