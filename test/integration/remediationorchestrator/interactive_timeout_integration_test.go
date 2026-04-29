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
// Pattern: Create RR + AA CRDs, let the real controller reconcile,
// assert RR does NOT transition to TimedOut when interactive session is active.
//
// NOTE: These tests do NOT wait for real timeouts to expire.
// They validate the controller's decision logic by setting AnalyzingStartTime
// in the past and checking the resulting phase after reconciliation.
// Time manipulation is limited to initial fixture setup (CreationTimestamp
// is immutable in envtest, but AnalyzingStartTime is a status field we control).
// ========================================

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("DD-INTERACTIVE-002: Interactive Timeout Extension (Integration)", func() {

	// IT-RO-703-001: RO does NOT timeout Analyzing phase when AA has active InteractiveSession
	// BR: DD-INTERACTIVE-002 (dynamic timeout extension)
	Context("IT-RO-703-001: Active InteractiveSession prevents Analyzing timeout", func() {
		It("should keep RR in Analyzing when AA has an active interactive session", func() {
			ns := ROControllerNamespace
			rrName := "it-703-001-rr"
			aiName := "it-703-001-ai"

			By("Creating an AIAnalysis with active InteractiveSession")
			startedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: aiName, Namespace: ns},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ai)).To(Succeed())

			// Set status with interactive session (status subresource)
			ai.Status.Phase = "Investigating"
			ai.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{
				SessionID:  "sess-integration-001",
				ActingUser: "test-user@corp",
				StartedAt:  &startedAt,
			}
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Creating a RemediationRequest in Analyzing phase with start time > 10m ago")
			rr := helpers.NewRemediationRequest(rrName, ns)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Set RR status to Analyzing with start time 12 minutes ago
			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
			rr.Status.AnalyzingStartTime = &analyzingStart
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{
				Name:      ai.Name,
				Namespace: ai.Namespace,
			}
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			By("Waiting for controller to reconcile and verifying NO timeout")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ns}, updated)
				if err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing),
				"RR should remain in Analyzing (not TimedOut) when interactive session is active")
		})
	})

	// IT-RO-703-002: RO resumes normal timeout when InteractiveSession.CompletedAt is set
	// BR: DD-INTERACTIVE-002 (timeout returns to default after disconnect)
	Context("IT-RO-703-002: Completed InteractiveSession allows normal timeout", func() {
		It("should timeout RR when interactive session is completed and time exceeded", func() {
			ns := ROControllerNamespace
			rrName := "it-703-002-rr"
			aiName := "it-703-002-ai"

			By("Creating an AIAnalysis with completed InteractiveSession")
			startedAt := metav1.NewTime(time.Now().Add(-20 * time.Minute))
			completedAt := metav1.NewTime(time.Now().Add(-15 * time.Minute))
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: aiName, Namespace: ns},
				Spec: aianalysisv1.AIAnalysisSpec{
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ai)).To(Succeed())

			ai.Status.Phase = "Investigating"
			ai.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{
				SessionID:   "sess-integration-002",
				ActingUser:  "test-user@corp",
				StartedAt:   &startedAt,
				CompletedAt: &completedAt,
			}
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			By("Creating RR in Analyzing with start time > 10m ago")
			rr := helpers.NewRemediationRequest(rrName, ns)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
			rr.Status.AnalyzingStartTime = &analyzingStart
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{
				Name:      ai.Name,
				Namespace: ai.Namespace,
			}
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			By("Waiting for controller to reconcile and timeout the RR")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ns}, updated)
				if err != nil {
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
			ns := ROControllerNamespace
			rrName := "it-703-003-rr"

			By("Creating RR with AIAnalysisRef pointing to non-existent AA")
			rr := helpers.NewRemediationRequest(rrName, ns)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			analyzingStart := metav1.NewTime(time.Now().Add(-12 * time.Minute))
			rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
			rr.Status.AnalyzingStartTime = &analyzingStart
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{
				Name:      "ai-deleted-703",
				Namespace: ns,
			}
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			By("Waiting for controller to reconcile and timeout normally")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ns}, updated)
				if err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseTimedOut),
				"Missing AA should fall back to default timeout behavior")
		})
	})
})
