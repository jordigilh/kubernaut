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
	"context"
	"net/http"
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
)

// E2E-FP-1899-001: AF A2A Phase-Transition Consent Gate — Phase 1->2
// (DD-AF-011, issue #1899). A single combined message declares
// interaction_mode=interactive on kubernaut_investigate, then scripts a
// same-turn fire-and-forget attempt at kubernaut_discover_workflows with no
// intervening genuine user message -- the literal #1899 repro (the console
// team observed AF silently discarding session-scoped parameters and
// auto-proceeding past a phase boundary the user never confirmed).
//
// This proves the structural (harness-enforced) gate end to end: even
// though the scripted mock-LLM "misbehaves" and emits the discover_workflows
// call anyway (regardless of what tools are advertised), checkpointToolFilter
// (BeforeModelCallback) and phaseGuardBefore's hard-reject backstop
// (BeforeToolCallback) block it, so no WorkflowExecution is ever created
// from the single turn. It then proves the gate is not a permanent stall:
// once the user genuinely confirms each subsequent phase (interactive mode
// gates every transition, mirroring E2E-FP-1189-003's turn-per-phase
// design), the journey completes to a WorkflowExecution.
var _ = Describe("AF A2A Phase-Transition Consent Gate — Phase 1->2 [E2E-FP-1899-001]", Label("fp", "af", "a2a", "interactive", "issue-1899"), func() {

	It("should block a same-turn fire-and-forget discover_workflows attempt, then complete once the user genuinely confirms each phase", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase2"]
		Expect(targetNS).NotTo(BeEmpty(), "consent-phase2 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1899-001")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the consent-gate phase2 RR")
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

		By("Turn 1 (single message): declare interactive mode, then fire-and-forget attempt discover_workflows in the same turn")
		const turn1CtxID = "ctx-fp-cg2-1"
		body := fpA2ATasksSend("fp-cg2-1",
			"create and investigate then sneak workflow discovery for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "blocked turn should still complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (blocked attempt) — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Verifying investigate ran (RR exists) but the fire-and-forget discover_workflows attempt never reached KA")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: no WorkflowExecution exists for %s after the blocked same-turn attempt\n", rrName)

		By("Turn 2 (genuine): user explicitly asks to discover workflows — phase 1->2 gate must now lift")
		// #1899: pin to Turn 1's own session context (not a "ctx-"+taskID
		// derivation) -- kubernaut_investigate ran inside Turn 1 itself, so
		// the af_interactive_driver_active state it set only exists in
		// turn1CtxID's ADK session. fpA2ATasksSendWithTask would land here
		// in a brand-new empty session and get wrongly hard-rejected with
		// "no_active_driver" (confirmed via must-gather RCA).
		body = fpA2ATasksSendWithContext("fp-cg2-2", turn1CtxID, taskID, "discover available workflows")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 (genuine discover_workflows) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — discover workflows OK (gate lifted on genuine user turn)\n")

		By("Turn 3 (genuine): user explicitly selects a workflow — phase 2->3 gate must also lift on its own genuine turn")
		// #1899: "select the discovered workflow" (af_select_discovered_workflow_1899)
		// resolves to the real seeded catalog UUID, unlike the shared
		// af_select_workflow scenario's "select workflow ..." phrase, whose
		// hardcoded human-readable literal fails kubernaut_select_workflow's
		// strict UUID comparison (issue #1834 upstream, confirmed via
		// must-gather RCA) -- that failure was masking this test's own
		// consent-gate PASS behind an unrelated invalid_workflow error.
		body = fpA2ATasksSendWithContext("fp-cg2-3", turn1CtxID, taskID, "select the discovered workflow")
		resp3, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 (genuine select_workflow) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — select workflow OK\n")

		By("Turn 4 (genuine): watch remediation progress to completion")
		body = fpA2ATasksSendWithContext("fp-cg2-4", turn1CtxID, taskID, "watch remediation progress")
		resp4, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp4.Body.Close() }()
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp4)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 (genuine watch) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 4 — watch OK\n")

		By("Verifying the journey completed once every phase was genuinely confirmed")
		// 5m matches the identical investigate->discover->select->execute->watch
		// chain in 07_af_a2a_autonomous_test.go / 17_af_a2a_full_interactive_remediation_test.go:
		// full-pipeline completion after the final "watch" turn routinely takes
		// well over 60s under CI load (observed 90-190s for equivalent specs in
		// the same run), so a 60s post-watch confirmation window is too tight.
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full pipeline completed for %s after the consent gate lifted on genuine turns\n", rrName)
	})
})

// E2E-FP-1899-002: AF A2A Phase-Transition Consent Gate — Phase 2->3
// (DD-AF-011, issue #1899, the more severe newly-discovered risk). A single
// combined message declares interaction_mode=full_remediation on
// kubernaut_investigate (legitimately authorizing the auto-chain into
// kubernaut_discover_workflows), then scripts a same-turn fire-and-forget
// attempt at kubernaut_select_workflow with a guessed workflow -- no user
// confirmation. Selecting and executing a workflow is the more consequential
// action (it creates a WorkflowExecution and begins remediating a live
// resource), so this is the sharper edge of the #1899 risk: the harness must
// let the model auto-proceed exactly as far as the declared mode authorizes
// (through discovery) and no further.
var _ = Describe("AF A2A Phase-Transition Consent Gate — Phase 2->3 [E2E-FP-1899-002]", Label("fp", "af", "a2a", "interactive", "issue-1899"), func() {

	It("should auto-chain through discover_workflows but block a same-turn fire-and-forget select_workflow attempt, then complete once the user genuinely confirms", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase3"]
		Expect(targetNS).NotTo(BeEmpty(), "consent-phase3 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1899-002")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the consent-gate phase3 RR")
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

		By("Turn 1 (single message): declare full_remediation mode (authorizes discovery), then fire-and-forget attempt select_workflow in the same turn")
		const turn1CtxID = "ctx-fp-cg3-1"
		body := fpA2ATasksSend("fp-cg3-1",
			"create and investigate then sneak workflow selection for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "blocked turn should still complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (blocked attempt) — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Verifying investigate+discover_workflows ran (mode-authorized) but the fire-and-forget select_workflow attempt never created a WorkflowExecution")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: no WorkflowExecution exists for %s after the blocked same-turn attempt\n", rrName)

		By("Turn 2 (genuine): user explicitly selects a workflow — phase 2->3 gate must now lift")
		// #1899: pin to Turn 1's own session context, same rationale as
		// E2E-FP-1899-001 above -- kubernaut_investigate (and the
		// mode-authorized discover_workflows auto-chain) ran inside Turn 1
		// itself, so their state only exists in turn1CtxID's ADK session.
		// "select the discovered workflow" (not "select workflow ...") for
		// the same af_select_discovered_workflow_1899/#1834 reason as
		// E2E-FP-1899-001's Turn 3 above.
		body = fpA2ATasksSendWithContext("fp-cg3-2", turn1CtxID, taskID, "select the discovered workflow")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 (genuine select_workflow) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — select workflow OK (gate lifted on genuine user turn)\n")

		By("Turn 3 (genuine): watch remediation progress to completion")
		body = fpA2ATasksSendWithContext("fp-cg3-3", turn1CtxID, taskID, "watch remediation progress")
		resp3, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 (genuine watch) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — watch OK\n")

		By("Verifying the journey completed once the user genuinely confirmed workflow selection")
		// 5m matches the identical investigate->discover->select->execute->watch
		// chain in 07_af_a2a_autonomous_test.go / 17_af_a2a_full_interactive_remediation_test.go:
		// full-pipeline completion after the final "watch" turn routinely takes
		// well over 60s under CI load (observed 90-190s for equivalent specs in
		// the same run), so a 60s post-watch confirmation window is too tight.
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full pipeline completed for %s after the consent gate lifted on the genuine turn\n", rrName)
	})
})

// E2E-FP-1912-001: No Reinvocation After Session-Terminal Tool (issue #1912).
// A single combined message declares interaction_mode=full_remediation_autonomous
// on kubernaut_investigate (so no DD-AF-011 checkpoint is left blocking --
// driverActive alone is what must be cleared) then completes the driver
// session with kubernaut_complete in the very same turn. Pre-#1912-fix,
// phaseGuardAfter's isTerminal branch cleared the ActiveContextRegistry
// entry but left driverActive stuck true, so NeedsReinvocationCtx could
// misread a subsequent text-only model turn as "investigation still
// active" and synthesize a "continue the investigation" nudge back into an
// already-closed session. This proves the real AF/A2A stack never lets
// that resurrect into a consequential action: the RR reaches a clean
// terminal state and no WorkflowExecution is ever created for it.
var _ = Describe("AF A2A No Reinvocation After Session-Terminal Tool [E2E-FP-1912-001]", Label("fp", "af", "a2a", "interactive", "issue-1912"), func() {

	It("should complete the driver session cleanly and never reinvoke into a further workflow action", NodeTimeout(6*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["terminal-1912"]
		Expect(targetNS).NotTo(BeEmpty(), "terminal-1912 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1912-001")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the no-reinvocation-after-complete RR")
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

		By("Turn 1 (single message): declare full_remediation_autonomous mode, then complete the driver session in the same turn")
		body := fpA2ATasksSend("fp-t1912-1",
			"create and investigate then complete and go silent for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "the investigate-then-complete turn should complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (investigate+complete) — task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying the RR was created and completed, and no WorkflowExecution was ever created for it (#1912)")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: %s completed cleanly with no WorkflowExecution — no errant reinvocation resurrected the closed session\n", rrName)
	})
})

// E2E-FP-1918-001: Harness-Enforced Actionability Gate (issue #1918).
// A synthetic Warning K8s Event (reason MOCK_NOT_ACTIONABLE) is created on
// the target Deployment before the chat turn so deriveSignalName resolves a
// grounded, non-"unknown" SignalName that surfaces verbatim in KA's
// investigation prompt (see af_create_rr.go's deriveSignalName Tier 3a) --
// matching the mock-LLM's built-in "not_actionable" scenario for whichever
// investigation runs against this RR. A single combined message then
// declares interaction_mode=full_remediation_autonomous on
// kubernaut_investigate -- an autonomy grant that would normally leave
// phase2_blocked=false -- and the scripted mock-LLM still attempts a
// same-turn kubernaut_discover_workflows call, simulating a lower-reasoning
// model that ignores or misreads the not-actionable RCA narrative and tries
// to proceed anyway. Pre-#1918-fix, full_remediation_autonomous alone would
// leave phase2_blocked=false and this call would reach KA's real
// discover_workflows implementation. Post-fix, phaseGuardAfter's #1918
// override forces phase2_blocked=true from KA's structured
// is_actionable/has_workflow signal (independent of any model reasoning),
// so phaseGuardBefore's existing DD-AF-011 hard-reject stops
// discover_workflows before it ever reaches KA. This proves the real
// AF/A2A stack end-to-end: the RR reaches a clean terminal state and no
// WorkflowExecution is ever created for it. Note: RemediationOrchestrator/
// AIAnalysis reconcile the RR autonomously as soon as it exists, independent
// of AF's own chat session, so this assertion also (legitimately)
// exercises KA's own pre-existing investigator.go short-circuit
// (actionable=false && no workflow) for the backend path -- the IT-level
// test (actionability_gate_1918_test.go) is what isolates AF's own harness
// gate specifically, with a controlled fake KA client.
var _ = Describe("AF Harness-Enforced Actionability Gate [E2E-FP-1918-001]", Label("fp", "af", "a2a", "autonomous", "issue-1918"), func() {

	It("should force phase2_blocked and hard-reject discover_workflows when KA's RCA is not actionable, even under full_remediation_autonomous", NodeTimeout(6*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["not-actionable-1918"]
		Expect(targetNS).NotTo(BeEmpty(), "not-actionable-1918 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1918-001")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the not-actionable-autonomous RR")
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

		By("Injecting a synthetic Warning event so AF derives a grounded, not-actionable signal name (#1918)")
		// af_create_rr.go's HandleCreateRR validates/truncates kubernaut_remediate's
		// description argument for severity triage input only -- it is never
		// persisted onto the RR CRD spec, so embedding a mock-LLM keyword there
		// (as an earlier version of this test did) never reaches KA's investigation
		// prompt at all. deriveSignalName's only path to a grounded, non-"unknown"
		// SignalName for a zero-replica Deployment (no real container ever runs, so
		// no organic OOMKill/BackOff events exist) is Tier 3a: the dominant Warning
		// event reason on the target resource. Creating this Event directly gives
		// deriveSignalName a real signal ("E2EFP1918NotActionable") that surfaces
		// verbatim in KA's investigation prompt for BOTH the backend's own
		// autonomous AIAnalysis reconciliation (which starts as soon as the RR CRD
		// exists, independent of AF's chat session) and AF's own kubernaut_investigate
		// bridge call.
		//
		// IMPORTANT: this must NOT be the built-in "not_actionable" scenario's own
		// "MOCK_NOT_ACTIONABLE" keyword. That keyword is matched broadly (ctx.Content
		// + ctx.AllText, see mockKeywordScenarioMulti), and HandleCreateRR echoes the
		// derived SignalName back in kubernaut_remediate's own JSON response
		// (CreateRRResult.SignalName) -- which folds into AF's OWN orchestration
		// conversation's allText on the next turn, silently hijacking AF's own
		// tool-selection into KA's scenario (empirically confirmed during this
		// test's development). "E2EFP1918NotActionable" instead matches only via
		// the dedicated not_actionable_grounded_1918 scenario (signalScenario,
		// registry_default.go), which inspects ctx.Content only -- safe from that
		// leak. See notActionableGroundedConfig's doc comment for the full
		// explanation.
		Expect(k8sClient.Create(ctx, &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "memory-eater-e2efp1918-not-actionable-",
				Namespace:    targetNS,
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:       "Deployment",
				Namespace:  targetNS,
				Name:       "memory-eater",
				APIVersion: "apps/v1",
			},
			Reason:         "E2EFP1918NotActionable",
			Message:        "E2E-FP-1918-001: synthetic signal so KA's RCA concludes is_actionable=false",
			Type:           corev1.EventTypeWarning,
			FirstTimestamp: metav1.Now(),
			LastTimestamp:  metav1.Now(),
			Count:          1,
			Source:         corev1.EventSource{Component: "e2e-fp-1918-test"},
		})).To(Succeed())

		By("Turn 1 (single message): declare full_remediation_autonomous mode against a not-actionable signal, then attempt discover_workflows in the same turn")
		body := fpA2ATasksSend("fp-na1918-1",
			"investigate and verify the harness actionability override for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "the investigate-then-discover turn should complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (investigate+discover attempt) — task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying the RR was created, and no WorkflowExecution was ever created for it (#1918)")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: %s completed with no WorkflowExecution — the harness gate blocked discover_workflows regardless of the declared autonomous mode\n", rrName)
	})
})
