/*
Copyright 2025 Jordi Gil.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E Tests for BR-HAPI-197: Human Review Required Flag
//
// Business Requirement: BR-HAPI-197
// Documentation: docs/testing/BR-HAPI-197/remediationorchestrator_test_plan_v1.0.md
//
// These tests validate the complete remediation flow when HAPI returns `needs_human_review=true`,
// ensuring that automatic remediation is blocked and NotificationRequest is created instead.
//
// Test Strategy (per TESTING_GUIDELINES.md):
// - E2E tests validate complete user journeys with all real services
// - Mock LLM is configured to return specific scenarios via signal keywords
// - Focus: Routing logic from HAPI response → RO decision → CRD creation
//
// Mock LLM Scenario Triggers (test/services/mock-llm/src/server.py):
// - "mock_rca_incomplete" → needs_human_review=true, reason="rca_incomplete"
// - "mock_low_confidence" → needs_human_review=true, reason="low_confidence"
// - "oomkilled" → needs_human_review=false (normal workflow selection)
var _ = Describe("BR-HAPI-197: Human Review E2E Tests", Label("e2e", "human-review"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("ro-human-review-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================
	// E2E-RO-197-001: Complete flow with needsHumanReview=true
	// ========================================
	Describe("E2E-RO-197-001: Complete remediation flow blocked by needsHumanReview", func() {
		It("should create NotificationRequest and block WorkflowExecution when HAPI returns needs_human_review=true", func() {
			By("Creating RemediationRequest with signal that triggers needs_human_review=true")
			now := metav1.Now()
			fingerprint := "e2e-human-review-test-fingerprint-001"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-e2e-human-review-001",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "MockRCAIncomplete", // Trigger for Mock LLM
					Severity:          "critical",
					SignalType:        "mock_rca_incomplete", // Mock LLM detects this keyword
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod-rca-incomplete",
						Namespace: testNS,
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

			By("Verifying RemediationRequest was created")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
			}, timeout, interval).Should(Succeed())

			By("Waiting for RO to create SignalProcessing CRD")
			var sp *signalprocessingv1.SignalProcessing
			Eventually(func() bool {
				spList := &signalprocessingv1.SignalProcessingList{}
				_ = k8sClient.List(ctx, spList, client.InNamespace(testNS))
				if len(spList.Items) == 0 {
					return false
				}
				sp = &spList.Items[0]
				return sp.Status.Phase == signalprocessingv1.PhaseCompleted
			}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created and completed by RO")

			By("Waiting for RO to create AIAnalysis CRD")
			var analysis *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				analysisList := &aianalysisv1.AIAnalysisList{}
				_ = k8sClient.List(ctx, analysisList, client.InNamespace(testNS))
				if len(analysisList.Items) == 0 {
					return false
				}
				analysis = &analysisList.Items[0]
				// Wait for AIAnalysis to complete with needsHumanReview=true
				return analysis.Status.Phase == aianalysisv1.PhaseFailed &&
					analysis.Status.NeedsHumanReview
			}, timeout, interval).Should(BeTrue(), "AIAnalysis should be Failed with NeedsHumanReview=true")

			By("Validating AIAnalysis status fields")
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(), "NeedsHumanReview must be true")
			Expect(analysis.Status.HumanReviewReason).To(Equal("rca_incomplete"), "HumanReviewReason must match Mock LLM scenario")
			Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"), "Reason should be WorkflowResolutionFailed")
			Expect(analysis.Status.Message).To(ContainSubstring("affectedResource"), "Message should explain missing affectedResource")

			By("Waiting for RO to create NotificationRequest")
			var notification *notificationv1.NotificationRequest
			Eventually(func() bool {
				notificationList := &notificationv1.NotificationRequestList{}
				_ = k8sClient.List(ctx, notificationList, client.InNamespace(testNS))
				if len(notificationList.Items) == 0 {
					return false
				}
				notification = &notificationList.Items[0]
				return true
			}, timeout, interval).Should(BeTrue(), "NotificationRequest should be created by RO")

			By("Validating NotificationRequest content")
			Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview), "Notification type must be ManualReview")
			Expect(notification.Spec.Subject).To(ContainSubstring("Manual Review Required"), "Subject should indicate manual review")
			Expect(notification.Spec.Body).To(ContainSubstring("human review"), "Body should mention human review")
			Expect(notification.Spec.Metadata).To(HaveKeyWithValue("humanReviewReason", "rca_incomplete"), "Metadata must include humanReviewReason")
			Expect(notification.Spec.Metadata).To(HaveKey("remediationRequest"), "Metadata must reference RemediationRequest")

			By("Validating RemediationRequest status")
			updatedRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
				return updatedRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseFailed), "RR should be in Failed phase")
			Expect(updatedRR.Status.Outcome).To(Equal("ManualReviewRequired"), "Outcome should be ManualReviewRequired")
			Expect(updatedRR.Status.RequiresManualReview).To(BeTrue(), "RequiresManualReview flag must be true")

			By("Verifying NO WorkflowExecution was created (blocked by human review)")
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			Consistently(func() int {
				_ = k8sClient.List(ctx, weList, client.InNamespace(testNS))
				return len(weList.Items)
			}, 10*time.Second, interval).Should(Equal(0), "NO WorkflowExecution should exist - remediation blocked")

			By("Verifying audit trail completeness")
			// Query DataStorage for audit events (per ADR-034 v1.2: event_category is MANDATORY)
			var eventList []string
			Eventually(func() bool {
				resp, err := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(string(rr.UID)),
					EventCategory: ogenclient.NewOptString("orchestration"),
					Limit:         ogenclient.NewOptInt(100),
				})
				if err != nil || len(resp.Data) == 0 {
					return false
				}
				// Build event type list
				eventList = []string{}
				for _, event := range resp.Data {
					eventList = append(eventList, event.EventType)
				}
				// Check if we have the human_review audit event
				for _, eventType := range eventList {
					if eventType == "orchestrator.routing.human_review" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Should find orchestrator.routing.human_review audit event")
		})
	})

	// ========================================
	// E2E-RO-197-002: Normal flow with needsHumanReview=false
	// ========================================
	Describe("E2E-RO-197-002: Verify remediation proceeds when needsHumanReview=false", func() {
		It("should create WorkflowExecution and NOT create NotificationRequest when needs_human_review=false", func() {
			By("Creating RemediationRequest with signal that triggers normal workflow selection")
			now := metav1.Now()
			fingerprint := "e2e-normal-flow-test-fingerprint-002"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-e2e-normal-flow-002",
					Namespace: testNS,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "OOMKilled", // Trigger for Mock LLM normal flow
					Severity:          "critical",
					SignalType:        "oomkilled", // Mock LLM detects this keyword → needs_human_review=false
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod-oomkilled",
						Namespace: testNS,
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

			By("Verifying RemediationRequest was created")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
			}, timeout, interval).Should(Succeed())

			By("Waiting for SignalProcessing to complete")
			Eventually(func() bool {
				spList := &signalprocessingv1.SignalProcessingList{}
				_ = k8sClient.List(ctx, spList, client.InNamespace(testNS))
				if len(spList.Items) == 0 {
					return false
				}
				return spList.Items[0].Status.Phase == signalprocessingv1.PhaseCompleted
			}, timeout, interval).Should(BeTrue())

			By("Waiting for AIAnalysis to complete successfully")
			var analysis *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				analysisList := &aianalysisv1.AIAnalysisList{}
				_ = k8sClient.List(ctx, analysisList, client.InNamespace(testNS))
				if len(analysisList.Items) == 0 {
					return false
				}
				analysis = &analysisList.Items[0]
				return analysis.Status.Phase == aianalysisv1.PhaseCompleted
			}, timeout, interval).Should(BeTrue(), "AIAnalysis should complete successfully")

			By("Validating AIAnalysis status (needs_human_review=false)")
			Expect(analysis.Status.NeedsHumanReview).To(BeFalse(), "NeedsHumanReview must be false for normal flow")
			Expect(analysis.Status.HumanReviewReason).To(BeEmpty(), "HumanReviewReason should be empty")
			Expect(analysis.Status.SelectedWorkflow).ToNot(BeNil(), "SelectedWorkflow should be populated")

			By("Waiting for RO to create WorkflowExecution")
			var we *workflowexecutionv1.WorkflowExecution
			Eventually(func() bool {
				weList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, weList, client.InNamespace(testNS))
				if len(weList.Items) == 0 {
					return false
				}
				we = &weList.Items[0]
				return true
			}, timeout, interval).Should(BeTrue(), "WorkflowExecution should be created for normal flow")

			By("Validating WorkflowExecution was created")
			Expect(we.Spec.WorkflowRef.WorkflowID).ToNot(BeEmpty(), "WorkflowExecution should have workflow ID")
			Expect(we.Spec.WorkflowRef.Version).ToNot(BeEmpty(), "WorkflowExecution should have workflow version")

			By("Verifying NO NotificationRequest was created (no human review needed)")
			notificationList := &notificationv1.NotificationRequestList{}
			Consistently(func() int {
				_ = k8sClient.List(ctx, notificationList, client.InNamespace(testNS))
				return len(notificationList.Items)
			}, 10*time.Second, interval).Should(Equal(0), "NO NotificationRequest should exist - normal flow")

			By("Validating RemediationRequest status (Executing)")
			updatedRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
				return updatedRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting), "RR should be Executing")
			Expect(updatedRR.Status.RequiresManualReview).To(BeFalse(), "RequiresManualReview must be false")
		})
	})
})
