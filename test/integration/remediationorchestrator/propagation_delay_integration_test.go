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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// RO PROPAGATION DELAY INTEGRATION TESTS (DD-EM-004 v2.0, BR-RO-103, Issue #253, #277)
// Business Requirement: RO uses config-driven propagation delays (not stabilization
// window) when computing HashCheckDelay, and sets Config.HashCheckDelay in the EA
// spec so the EM defers hash computation until creation + HashCheckDelay.
// ============================================================================
var _ = Describe("RO Propagation Delay (DD-EM-004 v2.0, BR-RO-103, Issue #253)", func() {

	// driveToCompletedWithCRDTarget drives the RO pipeline to Completed with a CRD
	// remediation target and optional GitOps labels. The AffectedResource.Kind is set
	// to "EffectivenessAssessment" which resolves to a non-built-in API group in envtest,
	// triggering CRD detection in the RO reconciler.
	driveToCompletedWithCRDTarget := func(ns, rrName string, gitOpsManaged bool) *remediationv1.RemediationRequest {
		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, rrName)

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

		By("Completing AIAnalysis with CRD remediation target")
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
			Confidence:      0.95,
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Operator CRD drift detected",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "EffectivenessAssessment", // CRD kind (kubernaut.ai) → resolved via KindsFor in envtest
				Name:      "test-ea-target",
				Namespace: ns,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		if gitOpsManaged {
			setAt := metav1.Now()
			ai.Status.PostRCAContext = &aianalysisv1.PostRCAContext{
				DetectedLabels: &sharedtypes.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
				},
				SetAt: &setAt,
			}
		}
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

		return rr
	}

	// ========================================
	// IT-RO-253-001: Config-driven operator-only delay (envtest)
	// BR: BR-RO-103.3, BR-RO-103.4
	//
	// Business outcome: When the remediation target is a CRD (operator-managed,
	// non-built-in API group) but NOT GitOps-managed, the RO must set Config.HashCheckDelay
	// using operatorReconcileDelay (not stabilization window) so the EM defers hash
	// computation until creation + HashCheckDelay.
	// ========================================
	It("IT-RO-253-001: should use config-driven operator delay for CRD target (not stabilization window)", func() {
		ns := createTestNamespace("ro-253-001")
		defer deleteTestNamespace(ns)

		rr := driveToCompletedWithCRDTarget(ns, "rr-253-001", false /* not GitOps */)

		By("Fetching the created EA")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		By("Verifying HashCheckDelay uses operator delay (not stabilization window)")
		Expect(ea.Spec.Config.HashCheckDelay).NotTo(BeNil(),
			"BR-RO-103.3: HashCheckDelay must be set for CRD target")
		// Config: operatorReconcileDelay = 30s (wired in suite), no gitOps delay since not GitOps-managed
		Expect(ea.Spec.Config.HashCheckDelay.Duration).To(Equal(30*time.Second),
			"HashCheckDelay should match operatorReconcileDelay config value (30s)")

		By("Verifying other EA spec fields are correct")
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("EffectivenessAssessment"))

		GinkgoWriter.Printf("IT-RO-253-001: EA created with HashCheckDelay=%s\n",
			ea.Spec.Config.HashCheckDelay.Duration)
	})

	// ========================================
	// IT-RO-253-002: Config-driven compounded delay for GitOps + CRD target (envtest)
	// BR: BR-RO-103.5
	//
	// Business outcome: When the remediation target is BOTH GitOps-managed AND a CRD
	// (operator-managed), the RO must compound both delays into Config.HashCheckDelay:
	// HashCheckDelay = gitOpsSyncDelay + operatorReconcileDelay
	// ========================================
	It("IT-RO-253-002: should compound gitOpsSyncDelay + operatorReconcileDelay for dual-async target", func() {
		ns := createTestNamespace("ro-253-002")
		defer deleteTestNamespace(ns)

		rr := driveToCompletedWithCRDTarget(ns, "rr-253-002", true /* GitOps-managed */)

		By("Fetching the created EA")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		By("Verifying HashCheckDelay uses compounded delay (gitOps + operator)")
		Expect(ea.Spec.Config.HashCheckDelay).NotTo(BeNil(),
			"BR-RO-103.5: HashCheckDelay must be set for dual-async target")
		// Config: gitOpsSyncDelay=2m, operatorReconcileDelay=30s → total=2m30s
		Expect(ea.Spec.Config.HashCheckDelay.Duration).To(Equal(2*time.Minute+30*time.Second),
			"HashCheckDelay should be compounded gitOpsSyncDelay(2m) + operatorReconcileDelay(30s)")

		By("Verifying other EA spec fields are correct")
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("EffectivenessAssessment"))

		GinkgoWriter.Printf("IT-RO-253-002: EA created with HashCheckDelay=%s\n",
			ea.Spec.Config.HashCheckDelay.Duration)
	})
})
