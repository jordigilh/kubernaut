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

// ========================================
// DD-INTERACTIVE-002: Interactive Timeout Extension Integration Tests
//
// These tests validate that the RO controller correctly interacts with
// real AIAnalysis CRD objects in envtest to determine timeout behavior
// during interactive sessions.
//
// Pattern: Let the controller flow naturally through Pending → Processing →
// Analyzing (using createRemediationRequest + updateSPStatus), then set up
// the AIAnalysis InteractiveSession and override AnalyzingStartTime to
// simulate elapsed time. This avoids fighting the controller's phase
// transitions.
// ========================================

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("DD-INTERACTIVE-002: Interactive Timeout Extension (Integration)", func() {

	var (
		namespace string
	)

	BeforeEach(func() {
		namespace = createTestNamespace(ctx, "ro-interactive")
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	// advanceToAnalyzing creates an RR, completes the SP, waits for the controller
	// to create the AI and transition to Analyzing, then returns the RR and AI names.
	advanceToAnalyzing := func(rrName string) (aiName, spName string) {
		By("Creating a RemediationRequest via the established helper")
		_ = createRemediationRequest(namespace, rrName)

		By("Waiting for SignalProcessing to be created")
		spName = fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())

		By("Completing SignalProcessing to advance RR to Processing")
		Expect(updateSPStatus(spName)).To(Succeed())

		By("Waiting for AIAnalysis to be created by the controller")
		aiName = fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())

		By("Waiting for RR to reach Analyzing phase naturally")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: rrName, Namespace: ROControllerNamespace,
			}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Analyzing"))

		return aiName, spName
	}

	// IT-RO-703-001: RO does NOT timeout Analyzing phase when AA has active InteractiveSession
	// BR: DD-INTERACTIVE-002 (dynamic timeout extension)
	Context("IT-RO-703-001: Active InteractiveSession prevents Analyzing timeout", func() {
		It("should keep RR in Analyzing when AA has an active interactive session", func() {
			rrName := fmt.Sprintf("it-703-001-%s", uuid.New().String()[:8])
			aiName, _ := advanceToAnalyzing(rrName)

			By("Setting AIAnalysis to Investigating with an active InteractiveSession")
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name: aiName, Namespace: ROControllerNamespace,
				}, ai); err != nil {
					return err
				}
				startedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				ai.Status.Phase = "Investigating"
				ai.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{
					SessionID:  "sess-integration-001",
					ActingUser: "test-user@corp",
					StartedAt:  &startedAt,
				}
				return k8sClient.Status().Update(ctx, ai)
			}, timeout, interval).Should(Succeed())

			By("Backdating AnalyzingStartTime to > 10m ago (past default timeout)")
			rr := &remediationv1.RemediationRequest{}
			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			Eventually(func() error {
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, rr); err != nil {
					return err
				}
				rr.Status.AnalyzingStartTime = &analyzingStart
				return k8sClient.Status().Update(ctx, rr)
			}, timeout, interval).Should(Succeed())

			By("Verifying controller does NOT timeout the RR (active session extends timeout)")
			Consistently(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, 10*time.Second, interval).Should(Equal(remediationv1.PhaseAnalyzing),
				"RR should remain in Analyzing (not TimedOut) when interactive session is active")
		})
	})

	// IT-RO-703-002: RO resumes normal timeout when InteractiveSession.CompletedAt is set
	// BR: DD-INTERACTIVE-002 (timeout returns to default after disconnect)
	Context("IT-RO-703-002: Completed InteractiveSession allows normal timeout", func() {
		It("should timeout RR when interactive session is completed and time exceeded", func() {
			rrName := fmt.Sprintf("it-703-002-%s", uuid.New().String()[:8])
			aiName, _ := advanceToAnalyzing(rrName)

			By("Setting AIAnalysis to Investigating with a completed InteractiveSession")
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name: aiName, Namespace: ROControllerNamespace,
				}, ai); err != nil {
					return err
				}
				startedAt := metav1.NewTime(time.Now().Add(-20 * time.Minute))
				completedAt := metav1.NewTime(time.Now().Add(-15 * time.Minute))
				ai.Status.Phase = "Investigating"
				ai.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{
					SessionID:   "sess-integration-002",
					ActingUser:  "test-user@corp",
					StartedAt:   &startedAt,
					CompletedAt: &completedAt,
				}
				return k8sClient.Status().Update(ctx, ai)
			}, timeout, interval).Should(Succeed())

			By("Backdating AnalyzingStartTime to > 10m ago (past default timeout)")
			rr := &remediationv1.RemediationRequest{}
			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			Eventually(func() error {
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, rr); err != nil {
					return err
				}
				rr.Status.AnalyzingStartTime = &analyzingStart
				return k8sClient.Status().Update(ctx, rr)
			}, timeout, interval).Should(Succeed())

			By("Waiting for controller to reconcile and timeout the RR")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseTimedOut),
				"RR should transition to TimedOut when interactive session is completed")
		})
	})

	// IT-RO-703-003: RO gracefully handles missing AA (AIAnalysisRef points to deleted AA)
	// BR: BR-ORCH-028 (graceful degradation)
	Context("IT-RO-703-003: Missing AA -- graceful fallback to default timeout", func() {
		It("should timeout at default when AIAnalysisRef points to non-existent AA", func() {
			rrName := fmt.Sprintf("it-703-003-%s", uuid.New().String()[:8])
			aiName, _ := advanceToAnalyzing(rrName)

			By("Deleting the AIAnalysis to simulate a missing AA")
			ai := &aianalysisv1.AIAnalysis{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ai)).To(Succeed())

			By("Backdating AnalyzingStartTime to > 10m ago (past default timeout)")
			rr := &remediationv1.RemediationRequest{}
			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			Eventually(func() error {
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, rr); err != nil {
					return err
				}
				rr.Status.AnalyzingStartTime = &analyzingStart
				rr.Status.AIAnalysisRef = &corev1.ObjectReference{
					Name:      "ai-deleted-nonexistent",
					Namespace: ROControllerNamespace,
				}
				return k8sClient.Status().Update(ctx, rr)
			}, timeout, interval).Should(Succeed())

			By("Waiting for controller to reconcile and timeout normally")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: rrName, Namespace: ROControllerNamespace,
				}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseTimedOut),
				"Missing AA should fall back to default timeout behavior")
		})
	})
})
