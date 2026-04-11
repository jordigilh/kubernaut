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

package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// CHARACTERIZATION INTEGRATION TESTS (Issue #666, Milestone 1, Group 8)
//
// These tests exercise apiReader-dependent paths that cannot be fully tested
// with fakeClient unit tests. They validate:
//   - transitionToFailed idempotency via apiReader
//   - freshRR guard in Analyzing (WorkflowExecutionRef already set)
//   - Terminal phase handling with Ready safety net
//   - Phase start time propagation across real reconcile cycles
//
// Reference: Issue #666 (Phase Handler Registry Migration)
// ============================================================================

var _ = Describe("Issue #666: Characterization Integration Tests for RO Phase Handler Migration", Label("integration", "characterization"), func() {

	var (
		namespace string
		rrName    string
	)

	BeforeEach(func() {
		namespace = createTestNamespace("ro-char")
		rrName = fmt.Sprintf("char-%s", uuid.New().String()[:13])
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	// ========================================================================
	// IT-RO-CHAR-001: transitionToFailed apiReader idempotency
	// Exercises: reconciler.go ~2305-2312 (apiReader refetch + Failed check)
	// When RR reaches Failed, subsequent reconciles should be idempotent.
	// ========================================================================
	It("IT-RO-CHAR-001: transitionToFailed is idempotent - Failed RR stays Failed with Ready condition", func() {
		By("Creating a RemediationRequest")
		rr := createRemediationRequest(namespace, rrName)

		By("Waiting for SP to be created (Pending → Processing)")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())

		By("Completing SP to drive to Analyzing")
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		By("Waiting for AI to be created (Processing → Analyzing)")
		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())

		By("Failing the AI to trigger transitionToFailed")
		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: aiName, Namespace: ROControllerNamespace,
		}, ai)).To(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseFailed
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for RR to reach Failed phase")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).Should(Equal(string(remediationv1.PhaseFailed)))

		By("Verifying Ready safety net was applied (Issue #79)")
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: rrName, Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())

		hasReady := false
		for _, c := range fetched.Status.Conditions {
			if c.Type == "Ready" {
				hasReady = true
				Expect(c.Status).To(Equal(metav1.ConditionFalse),
					"Failed RR should have Ready=False via safety net")
			}
		}
		Expect(hasReady).To(BeTrue(), "terminal Failed RR should have Ready condition set")
		Expect(fetched.Status.FailurePhase).To(HaveValue(Equal(remediationv1.FailurePhaseAIAnalysis)),
			"FailurePhase should be AIAnalysis")

		By("Verifying idempotency: second reconcile does not change Failed RR state")
		firstResourceVersion := fetched.ResourceVersion
		// ✅ APPROVED EXCEPTION: Intentional delay to allow controller reconcile loop
		// to run a second time, verifying idempotency of terminal Failed state.
		time.Sleep(2 * time.Second)

		idempotentRR := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: rrName, Namespace: ROControllerNamespace,
		}, idempotentRR)).To(Succeed())
		Expect(idempotentRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
			"phase should still be Failed after subsequent reconciles")
		Expect(idempotentRR.Status.FailurePhase).To(HaveValue(Equal(remediationv1.FailurePhaseAIAnalysis)),
			"FailurePhase should remain AIAnalysis")
		Expect(idempotentRR.ResourceVersion).To(Equal(firstResourceVersion),
			"ResourceVersion should not change (no status mutations on idempotent reconcile)")

		_ = rr
	})

	// ========================================================================
	// IT-RO-CHAR-002: phase start times across real reconcile cycles
	// Exercises: reconciler.go ~1995-2004 (ProcessingStartTime, AnalyzingStartTime)
	// Verifies that each phase transition sets the correct start time field
	// via the real apiReader-backed transitionPhase path.
	// ========================================================================
	It("IT-RO-CHAR-002: phase start times are correctly set through Pending → Processing → Analyzing", func() {
		By("Creating a RemediationRequest")
		createRemediationRequest(namespace, rrName)

		By("Waiting for SP to be created (confirms Processing phase reached)")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())

		By("Verifying ProcessingStartTime is set")
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: rrName, Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		Expect(fetched.Status.ProcessingStartTime).ToNot(BeNil(),
			"ProcessingStartTime should be set after Pending → Processing transition")

		By("Completing SP to drive to Analyzing")
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		By("Waiting for AI to be created (confirms Analyzing phase reached)")
		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())

		By("Verifying AnalyzingStartTime is set")
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: rrName, Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		Expect(fetched.Status.AnalyzingStartTime).ToNot(BeNil(),
			"AnalyzingStartTime should be set after Processing → Analyzing transition")
		Expect(fetched.Status.AnalyzingStartTime.Time).To(BeTemporally(">=", fetched.Status.ProcessingStartTime.Time),
			"AnalyzingStartTime should be >= ProcessingStartTime")
	})

	// ========================================================================
	// IT-RO-CHAR-003: freshRR idempotency guard in Analyzing
	// Exercises: reconciler.go ~1024-1039 (apiReader refetch + WorkflowExecutionRef check)
	// When AI completes and WFE is created, the freshRR guard prevents duplicate WFE creation
	// on stale-cache replays. This test drives the full path through Analyzing.
	// ========================================================================
	It("IT-RO-CHAR-003: freshRR guard in Analyzing prevents duplicate WFE creation", func() {
		By("Creating a RemediationRequest")
		createRemediationRequest(namespace, rrName)

		By("Driving to Analyzing phase")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())

		By("Completing AI with high confidence (auto-approve)")
		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: aiName, Namespace: ROControllerNamespace,
		}, ai)).To(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.ApprovalRequired = false
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-restart-pods",
			Version:         "v1.0.0",
			Confidence:      0.95,
			ExecutionBundle: "kubernaut/workflows:latest",
			Rationale:       "High confidence auto-approve (characterization test)",
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "OOM kill detected",
			Severity:   "critical",
			SignalType: "alert",
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		}
		completedAt := metav1.Now()
		ai.Status.CompletedAt = &completedAt
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for WFE to be created (Analyzing → Executing)")
		weName := fmt.Sprintf("we-%s", rrName)
		Eventually(func() error {
			we := &workflowexecutionv1.WorkflowExecution{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: weName, Namespace: ROControllerNamespace,
			}, we)
		}, timeout, interval).Should(Succeed())

		By("Verifying exactly one WFE was created (freshRR guard prevents duplicates)")
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		Expect(k8sClient.List(ctx, weList)).To(Succeed())
		weCount := 0
		for _, we := range weList.Items {
			if we.Namespace == ROControllerNamespace {
				for _, ref := range we.OwnerReferences {
					if ref.Name == rrName {
						weCount++
					}
				}
			}
		}
		Expect(weCount).To(Equal(1), "exactly one WFE should exist for this RR (freshRR guard)")

		By("Verifying ExecutingStartTime is set")
		fetched := &remediationv1.RemediationRequest{}
		Eventually(func() string {
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).Should(Equal(string(remediationv1.PhaseExecuting)))
		Expect(fetched.Status.ExecutingStartTime).ToNot(BeNil(),
			"ExecutingStartTime should be set after Analyzing → Executing transition")
	})

	// ========================================================================
	// IT-RO-CHAR-004: terminal Completed RR sets Ready=True via safety net
	// Exercises: reconciler.go ~625-644 (Ready safety net for terminal phases)
	// Validates the full lifecycle through Completed and that Ready=True is set.
	// ========================================================================
	It("IT-RO-CHAR-004: Completed RR gets Ready=True via terminal safety net", func() {
		By("Creating a RemediationRequest and driving full lifecycle")
		createRemediationRequest(namespace, rrName)

		By("Driving to Analyzing")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())

		By("Completing AI with high confidence")
		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: aiName, Namespace: ROControllerNamespace,
		}, ai)).To(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.ApprovalRequired = false
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-restart-pods",
			Version:         "v1.0.0",
			Confidence:      0.95,
			ExecutionBundle: "kubernaut/workflows:latest",
			Rationale:       "High confidence auto-approve",
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "OOM kill detected",
			Severity:   "critical",
			SignalType: "alert",
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		}
		aiNow := metav1.Now()
		ai.Status.CompletedAt = &aiNow
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for WFE creation")
		weName := fmt.Sprintf("we-%s", rrName)
		Eventually(func() error {
			we := &workflowexecutionv1.WorkflowExecution{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: weName, Namespace: ROControllerNamespace,
			}, we)
		}, timeout, interval).Should(Succeed())

		By("Completing WFE to drive Executing → Verifying")
		we := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: weName, Namespace: ROControllerNamespace,
		}, we)).To(Succeed())
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		weNow := metav1.Now()
		we.Status.CompletionTime = &weNow
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for RR to reach Verifying phase")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).Should(Equal(string(remediationv1.PhaseVerifying)))

		By("Waiting for EA to be created by RO")
		eaName := fmt.Sprintf("ea-%s", rrName)
		Eventually(func() error {
			ea := &eav1.EffectivenessAssessment{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: eaName, Namespace: ROControllerNamespace,
			}, ea)
		}, timeout, interval).Should(Succeed())

		By("Completing EA to drive Verifying → Completed")
		ea := &eav1.EffectivenessAssessment{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name: eaName, Namespace: ROControllerNamespace,
		}, ea)).To(Succeed())
		ea.Status.Phase = eav1.PhaseCompleted
		ea.Status.AssessmentReason = eav1.AssessmentReasonExpired
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		ea.Status.ValidityDeadline = &pastDeadline
		Expect(k8sClient.Status().Update(ctx, ea)).To(Succeed())

		By("Waiting for RR to reach Completed phase")
		fetched := &remediationv1.RemediationRequest{}
		Eventually(func() string {
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).Should(Equal(string(remediationv1.PhaseCompleted)))

		By("Verifying Ready=True via safety net")
		Eventually(func() bool {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, rr); err != nil {
				return false
			}
			for _, c := range rr.Status.Conditions {
				if c.Type == "Ready" && c.Status == metav1.ConditionTrue {
					return true
				}
			}
			return false
		}, 30*time.Second, interval).Should(BeTrue(), "Completed RR should have Ready=True via safety net")
	})
})
