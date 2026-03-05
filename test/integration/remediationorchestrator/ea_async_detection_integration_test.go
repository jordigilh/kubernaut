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
// EA ASYNC DETECTION INTEGRATION TESTS (DD-EM-004, BR-RO-103)
// Business Requirement: RO detects async-managed targets (GitOps, operator CRDs)
// and populates Config.HashCheckDelay in the EA spec so the EM defers hash computation.
// ============================================================================
var _ = Describe("EA Async Target Detection (DD-EM-004, BR-RO-103)", func() {

	// driveToCompleted is a helper that drives the full RO pipeline from Processing
	// to Completed, returning the final RR. The caller provides a function to customize
	// the AIAnalysis status before it's updated (e.g., to set GitOps labels).
	driveToCompleted := func(ns, rrName string, aiCustomizer func(ai *aianalysisv1.AIAnalysis)) *remediationv1.RemediationRequest {
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

		By("Completing AIAnalysis with customizations")
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
			Summary:    "Config drift detected",
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
		aiCustomizer(ai)
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
	// IT-RO-251-001: GitOps-managed target → Config.HashCheckDelay set
	// BR: BR-RO-103.2, BR-RO-103.3
	//
	// Business outcome: When the HAPI/RCA pipeline detects GitOps management
	// (DetectedLabels.GitOpsManaged=true in AIAnalysis), the RO sets
	// Config.HashCheckDelay in the EA spec so the EM defers hash computation
	// until after the GitOps controller (ArgoCD/FluxCD) reconciles the target.
	// ========================================
	It("IT-RO-251-001: should set Config.HashCheckDelay in EA when AIAnalysis indicates GitOps target", func() {
		ns := createTestNamespace("ro-251-001")
		defer deleteTestNamespace(ns)

		rr := driveToCompleted(ns, "rr-251-001", func(ai *aianalysisv1.AIAnalysis) {
			setAt := metav1.Now()
			ai.Status.PostRCAContext = &aianalysisv1.PostRCAContext{
				DetectedLabels: &sharedtypes.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
				},
				SetAt: &setAt,
			}
		})

		By("Fetching the created EA")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		By("Verifying Config.HashCheckDelay is set for GitOps target")
		Expect(ea.Spec.Config.HashCheckDelay).NotTo(BeNil(),
			"BR-RO-103.2: HashCheckDelay must be set when GitOps management is detected")
		// Issue #277: HashCheckDelay is a relative duration. For GitOps target with
		// built-in Deployment, RO uses gitOpsSyncDelay (2m in IT config), no operator delay.
		Expect(ea.Spec.Config.HashCheckDelay.Duration).To(Equal(2*time.Minute),
			"HashCheckDelay should match gitOpsSyncDelay config value")

		By("Verifying all other EA spec fields are still correct")
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Deployment"))
		Expect(ea.Spec.RemediationTarget.Name).To(Equal("test-app"))
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(BeNumerically(">", 0))

		GinkgoWriter.Printf("EA created with Config.HashCheckDelay=%s (GitOps: argocd)\n",
			ea.Spec.Config.HashCheckDelay.Duration)
	})

	// ========================================
	// IT-RO-251-002: Sync target (built-in Deployment) → Config.HashCheckDelay nil
	// BR: BR-RO-103.3
	//
	// Business outcome: For sync targets (built-in K8s resources without GitOps
	// management), the RO must NOT set Config.HashCheckDelay. This ensures backward
	// compatibility: the EM computes the hash immediately on first reconcile.
	// ========================================
	It("IT-RO-251-002: should NOT set Config.HashCheckDelay for sync built-in target without GitOps", func() {
		ns := createTestNamespace("ro-251-002")
		defer deleteTestNamespace(ns)

		rr := driveToCompleted(ns, "rr-251-002", func(ai *aianalysisv1.AIAnalysis) {
			// No PostRCAContext or DetectedLabels — sync target scenario
		})

		By("Fetching the created EA")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, 30*time.Second, interval).Should(Succeed(), "EA should be created after RR completion")

		By("Verifying Config.HashCheckDelay is nil for sync target")
		Expect(ea.Spec.Config.HashCheckDelay).To(BeNil(),
			"BR-RO-103.3: HashCheckDelay must be nil for sync built-in targets")

		By("Verifying EA spec is otherwise correct")
		Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Deployment"))
	})
})
