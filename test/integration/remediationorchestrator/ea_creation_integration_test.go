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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ns}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ns, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		By("Waiting for Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Completing AIAnalysis (high confidence, no approval needed)")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ns}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:     "wf-restart-pods",
			Version:        "v1.0.0",
			ContainerImage: "test-image:latest",
			Confidence:     0.95,
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ns}, we)
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ns}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		// Verify EA spec fields
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.TargetResource.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.TargetResource.Name).To(Equal("test-app"))
		Expect(ea.Spec.TargetResource.Namespace).To(Equal(ns))
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ns}, sp)
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ns}, ea)
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ns}, ea)
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ns}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ns, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ns}, ai)
		}, timeout, interval).Should(Succeed())
		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID: "wf-restart-pods", Version: "v1.0.0",
			ContainerImage: "test-image:latest", Confidence: 0.95,
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ns}, we)
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
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ns}, ea)
		}, 30*time.Second, interval).Should(Succeed())

		Expect(ea.OwnerReferences).To(HaveLen(1))
		ownerRef := ea.OwnerReferences[0]
		Expect(ownerRef.Name).To(Equal(rr.Name))
		Expect(ownerRef.Kind).To(Equal("RemediationRequest"))
		Expect(ownerRef.UID).To(Equal(rr.UID))
		Expect(*ownerRef.Controller).To(BeTrue(), "EA should be controller-owned by RR")

		By("Verifying cascade deletion works")
		Expect(k8sClient.Delete(ctx, rr)).To(Succeed())

		// EA should be garbage collected (cascade deletion)
		Eventually(func() bool {
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ns}, ea)
			return err != nil // true when EA is deleted
		}, 30*time.Second, interval).Should(BeTrue(), "EA should be cascade deleted when RR is deleted")
	})
})
