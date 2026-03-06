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

// Integration tests for #281 (NotificationRequest retry on transient failure)
//
// Business Requirements: BR-ORCH-045 (completion notification), BR-ORCH-034 (bulk duplicate)
//
// Defense-in-Depth:
// - Unit tests (UT-NT-RETRY-001/002/003): Retry logic with interceptor-injected failures
// - Integration tests (this file): End-to-end notification creation + ref tracking with real K8s API
//
// Note: Transient failure injection is not possible in envtest (real API server).
// Retry logic is fully validated in unit tests. This IT validates the happy path:
// ensureNotificationsCreated creates notifications and tracks refs through the full lifecycle.

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
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("NotificationRequest Retry Integration (#281)", Label("integration", "notification-retry"), func() {

	// IT-NT-RETRY-001: Full lifecycle validates completion notification tracked in NotificationRequestRefs
	It("IT-NT-RETRY-001: should track completion notification ref after full remediation lifecycle", func() {
		ns := createTestNamespace("ro-nt-retry")
		defer deleteTestNamespace(ns)

		rrName := "rr-nt-retry-001"

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, rrName)

		By("Waiting for Processing phase")
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

		By("Completing AIAnalysis")
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
			Summary:    "Memory leak in container",
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

		By("Waiting for Verifying phase (#280)")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseVerifying))

		By("Verifying completion notification was created (#281)")
		completionNTName := fmt.Sprintf("nr-completion-%s", rr.Name)
		nt := &notificationv1.NotificationRequest{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: completionNTName, Namespace: ROControllerNamespace}, nt)
		}, timeout, interval).Should(Succeed(), "Completion NotificationRequest should be created during Verifying phase")

		By("Verifying completion notification ref is tracked in NotificationRequestRefs (#281)")
		Eventually(func() bool {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			for _, ref := range rr.Status.NotificationRequestRefs {
				if ref.Name == completionNTName {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "NotificationRequestRefs should contain completion notification ref")

		By("Driving EA to completion for Verifying -> Completed (#280)")
		eaName := fmt.Sprintf("ea-%s", rr.Name)
		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: eaName, Namespace: ROControllerNamespace}, ea)
		}, timeout, interval).Should(Succeed())
		ea.Status.Phase = eav1.PhaseCompleted
		ea.Status.ValidityDeadline = &metav1.Time{Time: time.Now().Add(10 * time.Minute)}
		Expect(k8sClient.Status().Update(ctx, ea)).To(Succeed())

		By("Waiting for Completed phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		By("Final verification: NotificationRequestRefs still contains completion ref after Completed")
		_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
		found := false
		for _, ref := range rr.Status.NotificationRequestRefs {
			if ref.Name == completionNTName {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "NotificationRequestRefs should persist completion ref through Completed")
	})
})
