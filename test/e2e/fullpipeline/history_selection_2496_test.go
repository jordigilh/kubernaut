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
	"crypto/sha256"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/validators"
)

// BR-KA-016 / DD-KA-2496. The first history entry is produced by the actual
// RR -> SP -> AA -> KA -> WFE Job -> EM lifecycle. Only the external provider
// and external monitoring inputs are scripted for the producer journeys. The
// isolation case lives in the registered hub/spoke fleet suite; no CR status
// or confidence/review/approval/RO safety control is changed. Live result
// assertions read AF's SSE artifacts, never DataStorage as a transport.
var _ = Describe("History-aware selection journeys [BR-KA-016]", Label("e2e", "fullpipeline", "history-2496"), Serial, func() {
	DescribeTable("current selection after a genuine completed prior lifecycle",
		func(currentSignal string, successfulPrior, approvalRequired bool) {
			environment := "staging"
			if approvalRequired {
				environment = "production"
			}
			targetNS := fmt.Sprintf("history-2496-%d", time.Now().UnixNano())
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: targetNS, Labels: map[string]string{
				"kubernaut.ai/managed": "true", "kubernaut.ai/environment": environment,
			}}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), ns)).To(Succeed()) })
			// Not an event-exporter fp-* namespace: direct RR creation avoids a
			// competing organic OOM RR while retaining every real controller.
			Expect(infrastructure.DeployMemoryEaterWithLimits(ctx, targetNS, kubeconfigPath, "48Mi", "18Mi", GinkgoWriter)).To(Succeed())
			// Start above both the original 48Mi and insufficient 55Mi limits.
			// The canonical fixture's 40Mi warm-up can briefly look healthy
			// during a restart even though its eventual 60Mi demand still OOMs.
			// A stable real fault is required for this zero-effectiveness
			// producer; no health/metrics scorer or audit record is overridden.
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "memory-eater"}, dep)).To(Succeed())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			beforeArgs := dep.DeepCopy()
			dep.Spec.Template.Spec.Containers[0].Args = []string{"64Mi", "1", "64Mi", "1", "999999"}
			// Without a readiness probe the container briefly becomes Ready before
			// its real 64Mi allocation OOMs. EM can audit that transient health
			// even if a later retry leaves the final CRD score at zero. Require
			// actual capacity: 48Mi/55Mi remains unready; a real 512Mi repair can
			// become Ready normally. No scorer or audit event is overridden.
			dep.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{
					"sh", "-c", `limit=$(cat /sys/fs/cgroup/memory.max) && { test "$limit" = max || test "$limit" -ge 67108864; }`,
				}}},
				InitialDelaySeconds: 3, PeriodSeconds: 1, FailureThreshold: 1,
			}
			// Patch only the intended spec field. Deployment-controller status
			// writes must not race a whole-object fixture update.
			Expect(k8sClient.Patch(ctx, dep, client.MergeFrom(beforeArgs))).To(Succeed())
			fpWaitForPodCrashInNS("memory-eater", targetNS, 2*time.Minute)
			firstSignal := "historyfirst2496"
			if successfulPrior {
				firstSignal = "historyrepeataccepted2496" // 512Mi really resolves the OOM.
			} else {
				// Persistent external telemetry complements the real unresolved
				// OOM/CrashLoop and firing AlertManager signal. EM computes its own
				// component and aggregate scores; we never assign a desired score.
				fpNonImprovingTelemetry2496(targetNS)
				fpHistoryAlert2496(targetNS, firstSignal, false)
				DeferCleanup(func() { fpHistoryAlert2496(targetNS, firstSignal, true) })
			}
			By("Producing the prior attempt through real controllers and the immutable Job bundle")
			first := fpHistoryRR2496(targetNS, firstSignal)
			firstAA := fpHistoryAA2496(first.Name)
			Expect(firstAA.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
			firstSelection := firstAA.Status.GetRCAResult().SelectedWorkflow
			Expect(firstSelection).NotTo(BeNil())
			Expect(firstSelection.WorkflowName).To(Equal("oomkill-increase-memory-v1"))
			Expect(firstSelection.RepeatJustificationRequired).To(HaveValue(BeFalse()))
			if approvalRequired {
				fpHistoryApprove2496(first.Name, firstAA)
			}
			fpWaitForWEComplete(first.Name, 4*time.Minute)
			fpAssertSelectionAudit2496(first.Name, firstSelection)
			priorEA := fpHistoryEA2496(first.Name)
			Expect(priorEA.Status.AssessmentReason).To(BeElementOf("Full", "Partial"), "invalid/no-execution/pending history is not this fixture")
			if successfulPrior {
				Expect(priorEA.Status.Components.HealthScore).To(HaveValue(BeNumerically(">", 0)))
				Expect(priorEA.Status.Components.AlertScore).To(HaveValue(Equal(1.)))
			} else {
				Expect(priorEA.Status.Components.HealthScore).To(HaveValue(BeZero()), "the insufficient 55Mi limit must remain ineffective")
				Expect(priorEA.Status.Components.AlertScore).To(HaveValue(BeZero()), "the scoped signal must remain firing")
				Expect(priorEA.Status.Components.MetricsScore).To(HaveValue(BeZero()), "the external resource-pressure series must not improve")
				fpWaitForPodCrashInNS("memory-eater", targetNS, time.Minute)
			}
			fpHistoryTerminalRR2496(first.Name, remediationv1.PhaseCompleted)
			By("Reading producer-generated history with independent selection/effectiveness confidence")
			fpAssertPriorHistory2496(first, priorEA, firstSelection, successfulPrior)
			if successfulPrior {
				By("Reintroducing the real fault only after EM finalized the successful assessment")
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "memory-eater"}, dep)).To(Succeed())
				beforeLimit := dep.DeepCopy()
				dep.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] = resource.MustParse("48Mi")
				Expect(k8sClient.Patch(ctx, dep, client.MergeFrom(beforeLimit))).To(Succeed())
				fpWaitForPodCrashInNS("memory-eater", targetNS, 2*time.Minute)
			}
			By("Starting a fresh RR whose trusted target history is consumed by the current provider request")
			current := fpHistoryRR2496(targetNS, currentSignal)
			currentAA := fpHistoryAA2496(current.Name)
			result := fpHistoryAgentResult2496(currentAA.Name)
			selection := currentAA.Status.GetRCAResult().SelectedWorkflow
			switch currentSignal {
			case "historyrepeataccepted2496", "historyrepeatapproval2496":
				Expect(currentAA.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
				confidence := .84
				if approvalRequired {
					confidence = .75
				}
				fpAssertRepeat2496(selection, first.Name, confidence)
				if approvalRequired {
					By("Proving complete repeat reasoning does not bypass the deployed Rego approval policy")
					fpHistoryApprove2496(current.Name, currentAA)
				}
				// A structural acceptance never overrides RO cooldown/chain
				// safety. A real safety block is asserted, not worked around.
				fpHistoryExecutionOrSafety2496(current.Name, selection)
			case "historydifferent2496":
				Expect(currentAA.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
				Expect(selection).NotTo(BeNil())
				Expect(selection.WorkflowID).NotTo(Equal(firstSelection.WorkflowID))
				Expect(selection.ActionType).To(Equal(firstSelection.ActionType))
				Expect(selection.RepeatJustificationRequired).To(HaveValue(BeFalse()))
				Expect(selection.RepeatJustification).To(BeNil())
				Expect(selection.RepeatValidationOutcome).To(Equal("not_required"))
				// Different IDs remain eligible but do not bypass the target's
				// normal 10-minute WFE cooldown. Wait for its real expiry and
				// the resulting Job instead of shortening or clearing the lock.
				fpWaitForWEComplete(current.Name, 12*time.Minute)
				fpAssertSelectionAudit2496(current.Name, selection)
			case "historyrepeatlow2496":
				Expect(currentAA.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
				Expect(currentAA.Status.GetReview().HumanReviewReason).To(Equal("low_confidence"))
				fpAssertRepeat2496(selection, first.Name, .42)
				fpAssertNoWEForRR(current.Name)
			case "historyrepeatrejected2496":
				Expect(currentAA.Status.Phase).To(Equal(aianalysisv1.PhaseFailed))
				Expect(selection).To(BeNil())
				assessment := currentAA.Status.GetRCAResult().SelectionAssessment
				Expect(assessment).NotTo(BeNil())
				Expect(assessment.ProposedWorkflowID).To(Equal(firstSelection.WorkflowID))
				Expect(assessment.RepeatValidationOutcome).To(Equal("rejected"))
				Expect(assessment.RepeatJustificationRequired).To(HaveValue(BeTrue()))
				Expect(assessment.PriorRemediationUIDs).To(Equal([]string{first.Name}))
				Expect(result.ValidationAttemptsHistory).NotTo(BeEmpty(), "the existing bounded correction path must run")
				fpAssertNoWEForRR(current.Name)
			case "historynoworkflow2496":
				// An explicit no-workflow result completes the investigation
				// successfully while requiring non-executable human review.
				Expect(currentAA.Status.Phase).To(Equal(aianalysisv1.PhaseCompleted))
				Expect(currentAA.Status.GetReview().HumanReviewReason).To(Equal("no_matching_workflows"))
				Expect(currentAA.Status.GetReview().NeedsHumanReview).To(BeTrue())
				Expect(selection).To(BeNil())
				Expect(currentAA.Status.GetRCAResult().SelectionAssessment).To(BeNil())
				fpAssertNoWEForRR(current.Name)
			}
			if currentAA.Status.GetReview().NeedsHumanReview {
				Expect(currentAA.Status.GetReview().NeedsHumanReview).To(BeTrue())
				// BR-ORCH-036 (#550) routes manual review by selected workflow
				// presence, independently of AA phase. Rejected repeat proposals
				// remain non-executable assessments, so RR completes with review;
				// low confidence with an admitted selection still fails.
				expectedPhase := remediationv1.PhaseCompleted
				if selection != nil {
					expectedPhase = remediationv1.PhaseFailed
				}
				terminal := fpHistoryTerminalRR2496(current.Name, expectedPhase)
				Expect(terminal.Status.EnsureCompletionStatus().Outcome).To(Equal("ManualReviewRequired"))
				Expect(terminal.Status.EnsureCompletionStatus().RequiresManualReview).To(BeTrue())
				fpAssertNoWEForRR(current.Name)
				fpHistoryReviewNotification2496(current.Name)
			}
			By("Proving catalog availability and durable KA/AA reconstruction without filtering a failed strategy")
			fpAssertHistoryAudit2496(current.Name, firstSelection.WorkflowID, result)
			By("Reading the current selection/assessment through authenticated AF A2A SSE")
			fpAssertHistoryLive2496(current.Name, result)
		},
		Entry("E2E-FP-2496-001: justified same-ID repeat after zero effectiveness", "historyrepeataccepted2496", false, false),
		Entry("E2E-FP-2496-002: different ID in the same action family remains eligible", "historydifferent2496", false, false),
		Entry("E2E-FP-2496-003: rejected same-ID repeat exhausts into non-executable review", "historyrepeatrejected2496", false, false),
		Entry("E2E-FP-2496-004: unsuitable but available catalog is explicitly declined", "historynoworkflow2496", false, false),
		Entry("E2E-FP-2496-006: complete explanation cannot replace current confidence", "historyrepeatlow2496", false, false),
		Entry("E2E-FP-2496-006a: complete repeat reasoning cannot bypass production Rego approval", "historyrepeatapproval2496", false, true),
		Entry("E2E-FP-2496-007: a previously successful same-ID recurrence also requires explanation", "historyrepeataccepted2496", true, false),
	)

	// E2E-FP-2496-005 moved, with all isolation assertions, to
	// test/e2e/fleet/history_selection_2496_test.go (registered hub/spoke origins).
})

func fpHistoryRR2496(targetNS, signal string) *remediationv1.RemediationRequest {
	GinkgoHelper()
	name := fmt.Sprintf("rr-history-2496-%d", time.Now().UnixNano())
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(name))), SignalName: signal,
			SignalType: "alert", Severity: "high", TargetType: "kubernetes", FiringTime: now, ReceivedTime: now,
			TargetResource: remediationv1.ResourceIdentifier{APIVersion: "apps/v1", Kind: "Deployment", Name: "memory-eater", Namespace: targetNS},
		}}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	return rr
}

func fpHistoryAA2496(rrName string) *aianalysisv1.AIAnalysis {
	GinkgoHelper()
	return validators.WaitForHistoryAnalysis(ctx, apiReader, client.ObjectKey{Name: rrName, Namespace: namespace})
}

func fpHistoryEA2496(rrName string) *eav1.EffectivenessAssessment {
	GinkgoHelper()
	var completed *eav1.EffectivenessAssessment
	Eventually(func(g Gomega) {
		list := &eav1.EffectivenessAssessmentList{}
		g.Expect(apiReader.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		for _, ea := range list.Items {
			if ea.Spec.CorrelationID == rrName {
				g.Expect(ea.Status.Phase).To(Equal(eav1.PhaseCompleted))
				g.Expect(ea.Status.CompletedAt).NotTo(BeNil())
				completed = ea.DeepCopy()
				return
			}
		}
		g.Expect(completed).NotTo(BeNil(), "RO must create the assessment for %s", rrName)
	}, 4*time.Minute, 2*time.Second).Should(Succeed())
	return completed
}

func fpHistoryTerminalRR2496(name string, expectedPhase remediationv1.RemediationPhase) *remediationv1.RemediationRequest {
	GinkgoHelper()
	rr := &remediationv1.RemediationRequest{}
	Eventually(func(g Gomega) {
		g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, rr)).To(Succeed())
		g.Expect(rr.Status.OverallPhase).To(Equal(expectedPhase))
	}, 4*time.Minute, 2*time.Second).Should(Succeed())
	return rr
}

func fpHistoryAgentResult2496(aaName string) *agentsessionv1.AgentSessionResult {
	GinkgoHelper()
	return validators.WaitForHistoryAgentResult(ctx, apiReader, client.ObjectKey{Name: aaName, Namespace: namespace})
}

func fpAssertRepeat2496(selected *aianalysisv1.SelectedWorkflow, prior string, confidence float64) {
	GinkgoHelper()
	Expect(selected).NotTo(BeNil())
	Expect(selected.WorkflowSelectionConfidence).To(HaveValue(Equal(confidence)))
	Expect(selected.Confidence).To(Equal(confidence), "current selection confidence drives existing AA gates")
	Expect(selected.RepeatJustificationRequired).To(HaveValue(BeTrue()))
	Expect(selected.PriorWorkflowMatch).To(HaveValue(BeTrue()))
	Expect(selected.RepeatValidationOutcome).To(Equal("accepted"))
	Expect(selected.RepeatJustification).NotTo(BeNil())
	Expect(selected.RepeatJustification.EvidenceBasis).NotTo(BeEmpty())
	Expect(selected.RepeatJustification.PriorOutcomeRelevance).NotTo(BeEmpty())
	Expect(selected.RepeatJustification.ExpectedMechanism).NotTo(BeEmpty())
	Expect(selected.PriorRemediationUIDs).To(Equal([]string{prior}))
	Expect(selected.HandoffDigest).To(MatchRegexp(`^sha256:[a-f0-9]{64}$`))
}

// Also used by both canonical full lifecycle regressions. The expected values
// come from admitted AA state, not the audit mapper or a test-seeded audit row.
func fpAssertSelectionAudit2496(rrName string, selected *aianalysisv1.SelectedWorkflow) {
	GinkgoHelper()
	validators.ValidateHistorySelectionAudit(ctx, dataStorageClient, rrName, selected)
}

func fpAssertPriorHistory2496(rr *remediationv1.RemediationRequest, ea *eav1.EffectivenessAssessment, selected *aianalysisv1.SelectedWorkflow, successful bool) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		resp, err := dataStorageClient.GetRemediationHistoryContext(ctx, ogenclient.GetRemediationHistoryContextParams{
			TargetKind: "Deployment", TargetName: "memory-eater", TargetNamespace: rr.Spec.TargetResource.Namespace,
			CurrentSpecHash: ea.Status.Components.PostRemediationSpecHash,
		})
		g.Expect(err).NotTo(HaveOccurred())
		history, ok := resp.(*ogenclient.RemediationHistoryContext)
		g.Expect(ok).To(BeTrue())
		g.Expect(history.Tier1.Chain).To(HaveLen(1))
		entry := history.Tier1.Chain[0]
		g.Expect(entry.RemediationUID).To(Equal(rr.Name))
		g.Expect(entry.WorkflowId.Value).To(Equal(selected.WorkflowID))
		g.Expect(entry.WorkflowName.Value).To(Equal(selected.WorkflowName))
		g.Expect(entry.WorkflowSelectionConfidence.Value).To(Equal(selected.Confidence))
		g.Expect(entry.AssessmentReason.Value).To(BeElementOf(
			ogenclient.RemediationHistoryEntryAssessmentReasonFull, ogenclient.RemediationHistoryEntryAssessmentReasonPartial))
		g.Expect(entry.EffectivenessScore.IsSet() && !entry.EffectivenessScore.IsNull()).To(BeTrue())
		g.Expect(entry.SignalResolved.IsSet() && !entry.SignalResolved.IsNull()).To(BeTrue())
		g.Expect(entry.SignalResolved.Value).To(Equal(successful))
		if successful {
			g.Expect(entry.EffectivenessScore.Value).To(BeNumerically(">", 0))
		} else {
			g.Expect(entry.EffectivenessScore.Value).To(BeZero())
		}
	}, time.Minute, time.Second).Should(Succeed())
}

func fpHistoryExecutionOrSafety2496(name string, selected *aianalysisv1.SelectedWorkflow) {
	GinkgoHelper()
	rr := &remediationv1.RemediationRequest{}
	Eventually(func(g Gomega) {
		g.Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, rr)).To(Succeed())
		g.Expect(rr.Status.OverallPhase).To(BeElementOf(remediationv1.PhaseCompleted, remediationv1.PhaseBlocked))
		// The genuine WFE cooldown is 600s even after structural acceptance
		// and real Rego approval. Keep the target alive through its expiry.
	}, 12*time.Minute, 2*time.Second).Should(Succeed())
	if rr.Status.OverallPhase == remediationv1.PhaseBlocked {
		Expect(rr.Status.EnsureRoutingStatus().BlockReason).To(BeElementOf(
			remediationv1.BlockReasonRecentlyRemediated, remediationv1.BlockReasonIneffectiveChain))
		fpAssertNoWEForRR(name)
		return
	}
	fpWaitForWEComplete(name, time.Minute)
	fpAssertSelectionAudit2496(name, selected)
}

func fpHistoryReviewNotification2496(rrName string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		list := &notificationv1.NotificationRequestList{}
		g.Expect(apiReader.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		var found bool
		for _, nr := range list.Items {
			if nr.Spec.RemediationRequestRef != nil && nr.Spec.RemediationRequestRef.Name == rrName {
				found = true
			}
		}
		g.Expect(found).To(BeTrue(), "the configured operator notification must be observable for %s", rrName)
	}, time.Minute, time.Second).Should(Succeed())
}

// Same operator action as the canonical approval journey. No approval control,
// timeout, confidence score, AA status or RO routing state is overridden.
func fpHistoryApprove2496(rrName string, aa *aianalysisv1.AIAnalysis) {
	GinkgoHelper()
	Expect(aa.Status.GetApproval().ApprovalRequired).To(BeTrue())
	Expect(aa.Status.GetApproval().ApprovalReason).NotTo(BeEmpty())
	Expect(aa.Status.GetInvestigationMetadata().DegradedMode).To(BeFalse(), "approval must come from successful Rego evaluation")
	var rar remediationv1.RemediationApprovalRequest
	Eventually(func(g Gomega) {
		g.Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "rar-" + rrName}, &rar)).To(Succeed())
		g.Expect(rar.Status.Decision).To(BeEmpty())
		g.Expect(rar.Spec.Confidence).To(Equal(aa.Status.GetRCAResult().SelectedWorkflow.Confidence))
		g.Expect(rar.Spec.RecommendedWorkflow.WorkflowID).To(Equal(aa.Status.GetRCAResult().SelectedWorkflow.WorkflowID))
		rr := &remediationv1.RemediationRequest{}
		g.Expect(apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: rrName}, rr)).To(Succeed())
		g.Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval))
	}, time.Minute, time.Second).Should(Succeed())
	fpAssertNoWEForRR(rrName)
	fpHistoryReviewNotification2496(rrName)
	Eventually(func(g Gomega) {
		g.Expect(apiReader.Get(ctx, client.ObjectKeyFromObject(&rar), &rar)).To(Succeed())
		rar.Status.Decision, rar.Status.DecidedBy = remediationv1.ApprovalDecisionApproved, "e2e-test-admin@kubernaut.ai"
		rar.Status.DecisionMessage = "Approved by #2496 E2E operator after checking the current repeat rationale"
		decidedAt := metav1.Now()
		rar.Status.DecidedAt = &decidedAt
		g.Expect(k8sClient.Status().Update(ctx, &rar)).To(Succeed())
	}, 30*time.Second, time.Second).Should(Succeed())
}

func fpAssertHistoryAudit2496(rrName, priorWorkflowID string, result *agentsessionv1.AgentSessionResult) {
	GinkgoHelper()
	validators.ValidateHistoryDecisionAudit(ctx, dataStorageClient, rrName, priorWorkflowID, result)
}

func fpAssertHistoryLive2496(rrName string, result *agentsessionv1.AgentSessionResult) {
	GinkgoHelper()
	body := fpA2AMessageStreamWithContext("history-live-2496", "history-live-"+rrName, "investigate current selection 2496 for "+rrName)
	resp, err := fpA2ASSEInvoke(body, 180*time.Second)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
	validators.ValidateHistorySelectionLive(resp, result)
}

func fpHistoryAlert2496(targetNS, signal string, resolved bool) {
	GinkgoHelper()
	alert := infrastructure.TestAlert{Name: signal, StartsAt: time.Now().Add(-time.Minute),
		Labels: map[string]string{"namespace": targetNS, "route_skip_gateway": "true"},
		Status: "firing", EndsAt: time.Now().Add(20 * time.Minute)}
	if resolved {
		alert.Status, alert.EndsAt = "resolved", time.Now().Add(-time.Second)
	}
	Expect(infrastructure.InjectAlerts("http://localhost:9193", []infrastructure.TestAlert{alert})).To(Succeed())
}

// Existing Prometheus OTLP receiver: monotonic resource-pressure samples,
// not a fabricated effectiveness score or a mocked internal metrics scorer.
// The context and join ensure the external-input feeder cannot leak goroutines.
func fpNonImprovingTelemetry2496(targetNS string) {
	GinkgoHelper()
	feedCtx, cancelFeed := context.WithCancel(ctx)
	started := time.Now()
	labels := map[string]string{"namespace": targetNS, "pod": "pressure-input-2496", "container": "workload"}
	sample := func(at time.Time) []infrastructure.TestMetric {
		step := 30 + at.Sub(started).Seconds()/10
		return []infrastructure.TestMetric{
			{Name: "container_memory_working_set_bytes", Labels: labels, Value: 64*1024*1024 + step*2*1024*1024, Timestamp: at},
			{Name: "container_cpu_usage_seconds_total", Labels: labels, Value: step * step, Timestamp: at, IsCounter: true},
		}
	}
	// Fresh Prometheus heads reject samples predating their current append
	// bounds. Feed real-time observations instead of backdating a five-minute
	// history, then wait for a second accepted sample before creating the RR.
	// The real rate()/gauge queries and EM scorer remain unchanged.
	if err := infrastructure.InjectMetrics(feedCtx, "http://localhost:9190", sample(started)); err != nil {
		cancelFeed()
		Expect(err).NotTo(HaveOccurred(), "initial external pressure observation must be accepted")
	}
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		first := true
		for {
			select {
			case <-feedCtx.Done():
				done <- nil
				return
			case at := <-ticker.C:
				if err := infrastructure.InjectMetrics(feedCtx, "http://localhost:9190", sample(at)); err != nil {
					if feedCtx.Err() != nil {
						done <- nil
					} else {
						done <- err
					}
					return
				}
				if first {
					close(ready)
					first = false
				}
			}
		}
	}()
	DeferCleanup(func() {
		cancelFeed()
		Expect(<-done).To(Succeed(), "external telemetry feed must remain available")
	})
	Eventually(ready, 30*time.Second).Should(BeClosed(), "two accepted current samples must establish the external CPU-rate baseline")
}
