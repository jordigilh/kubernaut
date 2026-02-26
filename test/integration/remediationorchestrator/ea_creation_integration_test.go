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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// EA CREATION INTEGRATION TESTS (ADR-EM-001, BR-EM-001)
// Business Requirement: RO creates EffectivenessAssessment CRD when RR reaches terminal phase
// Phase 1: Manual child CRD control (no child controllers running)
// ============================================================================
var _ = Describe("EA Creation on Terminal Phase (ADR-EM-001)", func() {

	// ========================================
	// IT-RO-EA-001: Completed RR creates EA with correct spec
	// ========================================
	It("IT-RO-EA-001: should create EA when RR reaches Completed phase", func() {
		ns := createTestNamespace("ro-ea-001")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-ea-001")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Completing SignalProcessing")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		By("Waiting for Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Completing AIAnalysis (high confidence, no approval needed)")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     "wf-restart-pods",
			Version:        "v1.0.0",
			ExecutionBundle: "test-image:latest",
			Confidence:     0.95,
		}
		// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "OOM kill detected",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: ns,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for Executing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

		By("Completing WorkflowExecution")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		completionTime := metav1.Now()
		we.Status.CompletionTime = &completionTime
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for Completed phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		By("Verifying EA was created with correct spec")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		// Verify EA spec fields
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("test-app"))
		Expect(ea.Spec.RemediationTarget.Namespace).To(Equal(ns))
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(BeNumerically(">", 0))

		// Verify owner reference (cascade deletion)
		Expect(ea.OwnerReferences).To(HaveLen(1))
		Expect(ea.OwnerReferences[0].Name).To(Equal(rr.Name))
		Expect(ea.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
	})

	// ========================================
	// IT-RO-EA-002: Failed RR creates EA
	// ========================================
	It("IT-RO-EA-002: should create EA when RR reaches Failed phase", func() {
		ns := createTestNamespace("ro-ea-002")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-ea-002")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Failing SignalProcessing")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())

		sp.Status.Phase = signalprocessingv1.PhaseFailed
		failedNow := metav1.Now()
		sp.Status.CompletionTime = &failedNow
		sp.Status.Error = "Simulated SP failure for EA test"
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("Waiting for Failed phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying EA was created with Failed phase in spec")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR failure")

		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Failed"))
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
	})

	// ========================================
	// IT-RO-EA-003: TimedOut RR creates EA
	// ========================================
	It("IT-RO-EA-003: should create EA when RR reaches TimedOut phase", func() {
		ns := createTestNamespace("ro-ea-003")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest with an expired global timeout")
		rr := createRemediationRequest(ns, "rr-ea-003")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Manually setting RR to TimedOut via status update (simulating global timeout)")
		// To trigger handleGlobalTimeout, we need to set the StartTime far in the past
		// and use a very short Global timeout. Instead, we manually set the status
		// since the real global timeout check is in the reconciler.
		// The most reliable approach: set RR status to TimedOut manually
		// and verify EA creation from the handleGlobalTimeout path.
		// NOTE: This tests that the controller creates EA on timeout.
		// For Phase 1 (manual control), we force the timeout by backdating StartTime.
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return err
			}
			// Backdate start time to ensure global timeout (1h default) is exceeded
			pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			rr.Status.StartTime = &pastTime
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		By("Waiting for TimedOut phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseTimedOut))

		By("Verifying EA was created with TimedOut phase in spec")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR timeout")

		Expect(ea.Spec.RemediationRequestPhase).To(Equal("TimedOut"))
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
	})

	// ========================================
	// IT-RO-EA-004: EA has correct owner reference for cascade deletion
	// ========================================
	It("IT-RO-EA-004: should set EA owner reference for cascade deletion (BR-ORCH-031)", func() {
		ns := createTestNamespace("ro-ea-004")
		defer deleteTestNamespace(ns)

		By("Creating and completing a full remediation pipeline")
		rr := createRemediationRequest(ns, "rr-ea-004")

		// Drive to Completed (same pattern as IT-RO-EA-001)
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID: "wf-restart-pods", Version: "v1.0.0",
			ExecutionBundle: "test-image:latest", Confidence: 0.95,
		}
		// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "OOM kill detected",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: ns,
			},
		}
		completedAt := metav1.Now()
		ai.Status.CompletedAt = &completedAt
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		weCompletionTime := metav1.Now()
		we.Status.CompletionTime = &weCompletionTime
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		By("Verifying EA owner reference")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed())

		Expect(ea.OwnerReferences).To(HaveLen(1))
		ownerRef := ea.OwnerReferences[0]
		Expect(ownerRef.Name).To(Equal(rr.Name))
		Expect(ownerRef.Kind).To(Equal("RemediationRequest"))
		Expect(ownerRef.UID).To(Equal(rr.UID))
		Expect(*ownerRef.Controller).To(BeTrue(), "EA should be controller-owned by RR")

		// NOTE: Cascade deletion via owner references requires the garbage collector
		// (kube-controller-manager), which is not available in envtest. The owner
		// reference verification above (lines 301-306) validates BR-ORCH-031.
		// Cascade deletion is verified in E2E tests where a full Kind cluster runs.
		// ADR-EM-001 Section 8: blockOwnerDeletion must be false so RR deletion
		// does not block on EA finalizers; GC still deletes EA when RR is removed.
		By("Verifying owner reference is set correctly for cascade deletion (BR-ORCH-031)")
		Expect(ownerRef.BlockOwnerDeletion).To(HaveValue(BeFalse()),
			"ADR-EM-001 Section 8: blockOwnerDeletion must be false to prevent RR deletion blocking on EA finalizers")
	})
})

// ============================================================================
// EA DUAL-TARGET RESOLUTION INTEGRATION TESTS (Issue #188, DD-EM-003)
// Business Requirement: resolveDualTargets correctly derives SignalTarget from RR
// and RemediationTarget from AA.status.rootCauseAnalysis.affectedResource.
// ============================================================================
var _ = Describe("EA Dual-Target Resolution (Issue #188, DD-EM-003)", func() {

	// ========================================
	// IT-RO-188-003: Divergent targets when AA has a different affectedResource
	// ========================================
	It("IT-RO-188-003: should create EA with divergent targets when AA identifies a different affected resource", func() {
		ns := createTestNamespace("ro-dt-003")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-dt-003")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Completing SignalProcessing")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		By("Waiting for Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Completing AIAnalysis with a DIFFERENT affectedResource than RR target")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-scale-hpa",
			Version:         "v1.0.0",
			ExecutionBundle: "test-image:latest",
			Confidence:      0.90,
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "HPA maxed out, scaling target pod autoscaler",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "HorizontalPodAutoscaler",
				Name:      "api-frontend-hpa",
				Namespace: ns,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for Executing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

		By("Completing WorkflowExecution")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		completionTime := metav1.Now()
		we.Status.CompletionTime = &completionTime
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("Waiting for Completed phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		By("Verifying EA was created with divergent targets")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		// DD-EM-003: SignalTarget = RR target (the alerting resource)
		Expect(ea.Spec.SignalTarget.Kind).To(Equal("Deployment"),
			"DD-EM-003: SignalTarget.Kind should come from RR.Spec.TargetResource")
		Expect(ea.Spec.SignalTarget.Name).To(Equal("test-app"),
			"DD-EM-003: SignalTarget.Name should come from RR.Spec.TargetResource")
		Expect(ea.Spec.SignalTarget.Namespace).To(Equal(ns))

		// DD-EM-003: RemediationTarget = AA's AffectedResource (the modified resource)
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("HorizontalPodAutoscaler"),
			"DD-EM-003: RemediationTarget.Kind should come from AA.status.rootCauseAnalysis.affectedResource")
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("api-frontend-hpa"),
			"DD-EM-003: RemediationTarget.Name should come from AA.status.rootCauseAnalysis.affectedResource")
		Expect(ea.Spec.RemediationTarget.Namespace).To(Equal(ns))
	})

	// ========================================
	// IT-RO-188-003b: Defense-in-depth when AA has empty affectedResource
	// DD-HAPI-006 v1.2 / BR-ORCH-036 v4.0: RO must fail with ManualReviewRequired
	// when AffectedResource is nil or has empty Kind/Name.
	// ========================================
	It("IT-RO-188-003b: should fail with ManualReviewRequired when AA has empty affectedResource", func() {
		ns := createTestNamespace("ro-dt-003b")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-dt-003b")

		By("Driving pipeline to Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Completing AIAnalysis with EMPTY affectedResource (no RCA resource identified)")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-restart-pods",
			Version:         "v1.0.0",
			ExecutionBundle: "test-image:latest",
			Confidence:      0.85,
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Generic OOM detected",
			Severity:   "high",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "",
				Name:      "",
				Namespace: "",
			},
		}
		aiNow := metav1.Now()
		ai.Status.CompletedAt = &aiNow
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for RR to transition to Failed (DD-HAPI-006 v1.2 defense-in-depth)")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying ManualReviewRequired is set")
		Expect(rr.Status.RequiresManualReview).To(BeTrue(),
			"BR-ORCH-036 v4.0: RR should have RequiresManualReview=true when AffectedResource is missing")
		Expect(rr.Status.Outcome).To(Equal("ManualReviewRequired"),
			"BR-ORCH-036 v4.0: RR outcome should be ManualReviewRequired")
	})

	// ========================================
	// IT-RO-188-003c: Fallback when no AIAnalysis exists (SP failure path)
	// ========================================
	It("IT-RO-188-003c: should fall back to RR target when no AIAnalysis exists (SP failure)", func() {
		ns := createTestNamespace("ro-dt-003c")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-dt-003c")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Failing SignalProcessing (no AIAnalysis will be created)")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		sp.Status.Phase = signalprocessingv1.PhaseFailed
		failedNow := metav1.Now()
		sp.Status.CompletionTime = &failedNow
		sp.Status.Error = "Simulated SP failure for dual-target fallback test"
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("Waiting for Failed phase (no AA was ever created)")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying EA targets both fall back to RR target (nil dualTarget path)")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created even on SP failure")

		// DD-EM-003 nil fallback: When no AIAnalysis exists, dualTarget is nil,
		// and CreateEffectivenessAssessment falls back to RR.Spec.TargetResource for both.
		Expect(ea.Spec.SignalTarget.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.SignalTarget.Name).To(Equal("test-app"))
		Expect(ea.Spec.SignalTarget.Namespace).To(Equal(ns))

		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Deployment"),
			"DD-EM-003: RemediationTarget should fall back to RR target when no AA exists")
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("test-app"),
			"DD-EM-003: RemediationTarget should fall back to RR target when no AA exists")
		Expect(ea.Spec.RemediationTarget.Namespace).To(Equal(ns))

		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Failed"))
	})

	// ========================================
	// IT-RO-192-001: Cluster-scoped Node target with empty namespace
	// Issue #192: EA creation fails when TargetResource.Namespace is empty
	// because the CRD schema marks it as Required. Envtest enforces this.
	// ========================================
	It("IT-RO-192-001: should create EA with empty namespace for cluster-scoped Node target", func() {
		ns := createTestNamespace("ro-192-001")
		defer deleteTestNamespace(ns)

		By("Creating a Node with kubernaut managed label")
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(ctx, node)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, node)
		}()

		By("Creating a RemediationRequest targeting a cluster-scoped Node (empty namespace)")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-node-192",
				Namespace: ns,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "NodeNotReady",
				Severity:   "critical",
				SignalType: "alert",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Node",
					Name: "worker-1",
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Failing SignalProcessing to trigger EA creation on Failed path")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		sp.Status.Phase = signalprocessingv1.PhaseFailed
		failedNow := metav1.Now()
		sp.Status.CompletionTime = &failedNow
		sp.Status.Error = "Simulated SP failure for cluster-scoped Node test"
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("Waiting for Failed phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		By("Verifying EA was created with empty namespace on both targets")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(),
			"Issue #192: EA should be created even when TargetResource has empty namespace (cluster-scoped Node)")

		Expect(ea.Spec.SignalTarget.Kind).To(Equal("Node"),
			"Issue #192: SignalTarget.Kind must be Node")
		Expect(ea.Spec.SignalTarget.Name).To(Equal("worker-1"),
			"Issue #192: SignalTarget.Name must be worker-1")
		Expect(ea.Spec.SignalTarget.Namespace).To(BeEmpty(),
			"Issue #192: SignalTarget.Namespace must be empty for cluster-scoped resources")

		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Node"),
			"Issue #192: RemediationTarget.Kind must be Node (fallback to RR)")
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("worker-1"),
			"Issue #192: RemediationTarget.Name must be worker-1 (fallback to RR)")
		Expect(ea.Spec.RemediationTarget.Namespace).To(BeEmpty(),
			"Issue #192: RemediationTarget.Namespace must be empty for cluster-scoped resources")

		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Failed"))
	})
})
