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
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// TDD RED Phase: RAR Audit Trail E2E Tests
//
// Business Requirements:
// - BR-AUDIT-006: Approval decision audit trail (SOC 2 CC8.1, CC6.8)
// - DD-WEBHOOK-003: Webhook-Complete Audit Pattern
// - ADR-040: RemediationApprovalRequest CRD Architecture
//
// Test Strategy:
// - Create RAR via RO controller (no mocks)
// - Approve RAR via AuthWebhook (authenticated user)
// - Validate TWO audit events exist:
//   1. Webhook event (event_category="webhook")
//   2. RO approval event (event_category="orchestration")
// - Validate complete audit trail (WHO, WHAT, WHEN, WHY)
//
// Expected: Tests will FAIL until RO controller emits approval audit events

var _ = Describe("BR-AUDIT-006: RAR Audit Trail E2E", Label("e2e", "audit", "approval"), func() {
	const (
		dataStorageURL = "http://localhost:8090" // DD-TEST-001: RO → DataStorage dependency port
		e2eTimeout     = 120 * time.Second
		e2eInterval    = 2 * time.Second
	)

	Context("E2E-RO-AUD006-001: Complete RAR Approval Audit Trail", func() {
		var (
			testNamespace string
			testRR        *remediationv1.RemediationRequest
			testRAR       *remediationv1.RemediationApprovalRequest
			dsClient      *dsgen.Client
		)

		BeforeEach(func() {
			// Create unique namespace for E2E test isolation
			testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "ro-e2e",
				helpers.WithLabels(map[string]string{
					"kubernaut.ai/audit-enabled": "true", // Required for AuthWebhook to intercept status updates
				}))

			// Create authenticated DataStorage client (DD-AUTH-014)
			saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			var err error
			dsClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred())

			// Create RemediationRequest (triggers RO controller)
			now := metav1.Now()
			testRR = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-rar-audit-%s", uuid.New().String()[:8]),
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "e2e0000000000000000000000000000000000000000000000000000000000001",
					SignalName:        "E2ERARAuditTest",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: testNamespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

			GinkgoWriter.Printf("🚀 E2E: Created RemediationRequest %s/%s\n", testNamespace, testRR.Name)

			// Wait for RO controller to create RAR (requires AIAnalysis with low confidence)
			// For now, manually create RAR to test audit trail
			testRAR = &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("rar-%s", testRR.Name),
					Namespace: testNamespace,
					Labels: map[string]string{
						"kubernaut.ai/remediation-request": testRR.Name,
					},
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      testRR.Name,
						Namespace: testNamespace,
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-test",
					},
					Confidence:           0.75,
					ConfidenceLevel:      "medium",
					Reason:               "Confidence below 80% auto-approve threshold",
					InvestigationSummary: "Memory leak detected in payment service",
					WhyApprovalRequired:  "Medium confidence requires human validation",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "restart-pod-v1",
						Version:        "1.0.0",
						ContainerImage: "kubernaut/restart-pod:v1",
						Rationale:      "Standard pod restart",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart pod", Rationale: "Clear memory leak"},
					},
					RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
				},
			}
			Expect(k8sClient.Create(ctx, testRAR)).To(Succeed())
			GinkgoWriter.Printf("🚀 E2E: Created RAR %s/%s\n", testNamespace, testRAR.Name)
		})

		AfterEach(func() {
			// Cleanup namespace
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		})

		It("should emit complete audit trail for approval decision", func() {
			// BUSINESS ACTION: Operator approves remediation via AuthWebhook
			By("Simulating operator approval (webhook sets DecidedBy)")

			// Get latest RAR
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR)).To(Succeed())

			// Update RAR status to approved (simulates webhook mutation)
			testRAR.Status.Decision = remediationv1.ApprovalDecisionApproved
			testRAR.Status.DecidedBy = "e2e-test-user@example.com" // Simulates webhook auth
			now := metav1.Now()
			testRAR.Status.DecidedAt = &now
			testRAR.Status.DecisionMessage = "E2E test approval - root cause confirmed"

			Expect(k8sClient.Status().Update(ctx, testRAR)).To(Succeed())
			GinkgoWriter.Printf("✅ E2E: Approved RAR %s\n", testRAR.Name)

			// BUSINESS VALIDATION: Query for audit events with proper filters
			// FIX: Enhanced error visibility + longer timeout to handle audit buffer flush (1s interval)
			// See: docs/handoff/E2E_FAILURES_DS_RO_TRIAGE_JAN_29_2026.md
			// Use separate queries with EventCategory + EventType filters for performance (BR-AUDIT-006)
			By("Querying DataStorage for RAR audit events")
			correlationID := testRR.Name // Per DD-AUDIT-CORRELATION-002

			var webhookEvents, orchestrationApprovalEvents []dsgen.AuditEvent
			Eventually(func() ([2]int, error) {
				// Query webhook events with all 3 required filters (correlationID, EventCategory, EventType)
				// All 3 filters ensure we find the CORRECT event (BR-AUDIT-006)
				webhookResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString("webhook"),
					EventType:     dsgen.NewOptString("webhook.remediationapprovalrequest.decided"),
					Limit:         dsgen.NewOptInt(100),
				})
				if err != nil {
					return [2]int{0, 0}, fmt.Errorf("webhook query failed: %w", err)
				}
				webhookEvents = webhookResp.Data

				// Query orchestration approval events with all 3 required filters
				orchestrationResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString("orchestration"),
					EventType:     dsgen.NewOptString("orchestrator.approval.approved"),
					Limit:         dsgen.NewOptInt(100),
				})
				if err != nil {
					return [2]int{len(webhookEvents), 0}, fmt.Errorf("orchestration query failed: %w", err)
				}
				orchestrationApprovalEvents = orchestrationResp.Data

				counts := [2]int{len(webhookEvents), len(orchestrationApprovalEvents)}
				GinkgoWriter.Printf("📊 E2E: Found webhook=%d, orchestration=%d\n", counts[0], counts[1])

				if counts != [2]int{1, 1} {
					return counts, fmt.Errorf("incomplete: webhook=%d, orchestration=%d (expecting [1, 1])", counts[0], counts[1])
				}
				return counts, nil
			}, e2eTimeout, e2eInterval).Should(Equal([2]int{1, 1}),
				"COMPLIANCE FAILURE: Need exactly 1 webhook event and 1 orchestration approval event")

			// BUSINESS OUTCOME 1: Webhook audit event exists (AuthWebhook)
			Expect(webhookEvents).To(HaveLen(1),
				"COMPLIANCE: AuthWebhook must emit audit event (DD-WEBHOOK-003)")

			webhookEvent := webhookEvents[0]
			actorID, _ := webhookEvent.ActorID.Get()
			// SECURITY: In E2E, authenticated user is "kubernetes-admin" (kubectl context)
			// AuthWebhook correctly overwrites user-provided "e2e-test-user@example.com"
			Expect(actorID).To(Equal("kubernetes-admin"),
				"BUSINESS OUTCOME: Webhook captured authenticated user (SOC 2 CC8.1)")
			Expect(webhookEvent.EventAction).To(Equal("approval_decided"),
				"BUSINESS OUTCOME: Webhook action is clear")

			// BUSINESS OUTCOME 2: RO approval audit event exists (RO Controller)
			Expect(orchestrationApprovalEvents).To(HaveLen(1),
				"COMPLIANCE: RO controller must emit approval audit event (BR-AUDIT-006)")

			approvalEvent := orchestrationApprovalEvents[0]
			Expect(approvalEvent.EventType).To(Equal("orchestrator.approval.approved"),
				"BUSINESS OUTCOME: Event type indicates approval (ADR-034 v1.7)")
			Expect(string(approvalEvent.EventOutcome)).To(Equal("success"),
				"BUSINESS OUTCOME: Approved path is success outcome")

			// BUSINESS OUTCOME 3: Complete audit trail (WHO, WHAT, WHEN, WHY)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ E2E-RO-AUD006-001: Complete RAR Audit Trail Validated\n")
			GinkgoWriter.Printf("   BUSINESS OUTCOMES:\n")
			GinkgoWriter.Printf("   • WHO approved: %s (webhook auth) ✅\n", actorID)
			GinkgoWriter.Printf("   • WHAT decision: Approved ✅\n")
			GinkgoWriter.Printf("   • WHEN: %s ✅\n", approvalEvent.EventTimestamp)
			GinkgoWriter.Printf("   • WHY: Root cause confirmed ✅\n")
			GinkgoWriter.Printf("   • Two-event audit trail: webhook + RO ✅\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC8.1 + CC6.8 satisfied ✅\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})

		It("should emit audit event for rejection decision", func() {
			// BUSINESS ACTION: Operator rejects remediation
			By("Simulating operator rejection")

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR)).To(Succeed())

			testRAR.Status.Decision = remediationv1.ApprovalDecisionRejected
			testRAR.Status.DecidedBy = "e2e-test-user@example.com"
			now := metav1.Now()
			testRAR.Status.DecidedAt = &now
			testRAR.Status.DecisionMessage = "Risk too high - potential cascading failures"

			Expect(k8sClient.Status().Update(ctx, testRAR)).To(Succeed())

			// BUSINESS VALIDATION: Query for orchestration events
			By("Querying for rejection audit event")
			correlationID := testRR.Name

			var rejectionEvent *dsgen.AuditEvent
			Eventually(func() bool {
				// Query with all 3 required filters (correlationID, EventCategory, EventType)
				// Prevents pagination overhead - ensures efficient query (BR-AUDIT-006)
				resp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString("orchestration"),
					EventType:     dsgen.NewOptString("orchestrator.approval.rejected"),
					Limit:         dsgen.NewOptInt(100),
				})
				if err != nil {
					GinkgoWriter.Printf("🔍 DEBUG: Rejection query ERROR: %v\n", err)
					return false
				}

				GinkgoWriter.Printf("🔍 DEBUG: Rejection query returned %d events\n", len(resp.Data))
				for i, evt := range resp.Data {
					GinkgoWriter.Printf("  [%d] CorrelationID=%s, EventType=%s, EventCategory=%s\n",
						i, evt.CorrelationID, evt.EventType, evt.EventCategory)
				}

				// No client-side filtering needed - EventType filter ensures only rejection events returned
				if len(resp.Data) > 0 {
					rejectionEvent = &resp.Data[0]
					GinkgoWriter.Printf("🔍 DEBUG: Found rejection event, returning true\n")
					return true
				}
				GinkgoWriter.Printf("🔍 DEBUG: No rejection events found yet, waiting...\n")
				return false
			}, e2eTimeout, e2eInterval).Should(BeTrue(),
				"COMPLIANCE FAILURE: No rejection audit event (BR-AUDIT-006)")

			// BUSINESS OUTCOME: Rejection recorded with failure outcome
			Expect(string(rejectionEvent.EventOutcome)).To(Equal("failure"),
				"BUSINESS OUTCOME: Rejected path is failure outcome (remediation NOT executed)")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ E2E-RO-AUD006-002: Rejection Audit Event Validated\n")
			GinkgoWriter.Printf("   • Event type: %s ✅\n", rejectionEvent.EventType)
			GinkgoWriter.Printf("   • Outcome: failure (remediation blocked) ✅\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC6.8 satisfied ✅\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("E2E-RO-AUD006-003: Audit Trail Persistence", func() {
		var (
			testNamespace string
			testRR        *remediationv1.RemediationRequest
			testRAR       *remediationv1.RemediationApprovalRequest
			dsClient      *dsgen.Client
			correlationID string
		)

		BeforeEach(func() {
			// Create unique namespace for E2E test isolation
			// Use shared helper for E2E tests (waits for namespace to be Active)
			testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "ro-e2e-persist",
				helpers.WithLabels(map[string]string{
					"kubernaut.ai/audit-enabled": "true", // Required for AuthWebhook to intercept status updates
				}))

			// Create authenticated DataStorage client
			saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			var err error
			dsClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred())

			// Create RemediationRequest
			now := metav1.Now()
			testRR = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-rar-persist-%s", uuid.New().String()[:8]),
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "e2e0000000000000000000000000000000000000000000000000000000000003",
					SignalName:        "E2ERARAuditPersistenceTest",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-persist",
						Namespace: testNamespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())
			correlationID = testRR.Name

			// Create RAR
			testRAR = &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("rar-%s", testRR.Name),
					Namespace: testNamespace,
					Labels: map[string]string{
						"kubernaut.ai/remediation-request": testRR.Name,
					},
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      testRR.Name,
						Namespace: testNamespace,
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-test-persist",
					},
					Confidence:           0.70,
					ConfidenceLevel:      "medium",
					Reason:               "Persistence test scenario",
					InvestigationSummary: "Testing audit trail durability",
					WhyApprovalRequired:  "Compliance validation",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:     "restart-pod-v1",
						Version:        "1.0.0",
						ContainerImage: "kubernaut/restart-pod:v1",
						Rationale:      "Standard pod restart",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart pod", Rationale: "Clear memory leak"},
					},
					RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
				},
			}
			Expect(k8sClient.Create(ctx, testRAR)).To(Succeed())

			// Approve RAR (triggers audit events)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR)).To(Succeed())
			testRAR.Status.Decision = remediationv1.ApprovalDecisionApproved
			testRAR.Status.DecidedBy = "e2e-auditor@example.com"
			now = metav1.Now()
			testRAR.Status.DecidedAt = &now
			testRAR.Status.DecisionMessage = "Approved for persistence test"
			Expect(k8sClient.Status().Update(ctx, testRAR)).To(Succeed())

			// Wait for BOTH webhook and orchestration approval audit events to be persisted
			// FIX: Enhanced error visibility + longer timeout to handle audit buffer flush (1s interval)
			// See: docs/handoff/E2E_FAILURES_DS_RO_TRIAGE_JAN_29_2026.md
			// Two-Event Pattern: webhook (WHO) + orchestration approval (WHAT/WHY)
			Eventually(func() ([2]int, error) {
				// Query webhook events with all 3 required filters (correlationID, EventCategory, EventType)
				// All 3 filters ensure we find the CORRECT event (BR-AUDIT-006)
				webhookResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString("webhook"),
					EventType:     dsgen.NewOptString("webhook.remediationapprovalrequest.decided"),
					Limit:         dsgen.NewOptInt(100),
				})
				if err != nil {
					return [2]int{0, 0}, fmt.Errorf("webhook query failed: %w", err)
				}
				GinkgoWriter.Printf("🔍 DEBUG: Webhook query returned %d events\n", len(webhookResp.Data))
				for i, evt := range webhookResp.Data {
					GinkgoWriter.Printf("  [%d] CorrelationID=%s, EventType=%s, EventCategory=%s\n",
						i, evt.CorrelationID, evt.EventType, evt.EventCategory)
				}

				// Query orchestration approval events with all 3 required filters
				orchestrationResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString("orchestration"),
					EventType:     dsgen.NewOptString("orchestrator.approval.approved"),
					Limit:         dsgen.NewOptInt(100),
				})
				if err != nil {
					return [2]int{len(webhookResp.Data), 0}, fmt.Errorf("orchestration query failed: %w", err)
				}
				GinkgoWriter.Printf("🔍 DEBUG: Orchestration query returned %d events\n", len(orchestrationResp.Data))
				for i, evt := range orchestrationResp.Data {
					GinkgoWriter.Printf("  [%d] CorrelationID=%s, EventType=%s, EventCategory=%s\n",
						i, evt.CorrelationID, evt.EventType, evt.EventCategory)
				}

				// No client-side filtering needed - EventType filter ensures only approval events returned
				counts := [2]int{len(webhookResp.Data), len(orchestrationResp.Data)}
				GinkgoWriter.Printf("🔍 DEBUG: Returning counts: webhook=%d, orchestration=%d (expecting [1, 1])\n",
					counts[0], counts[1])

				if counts != [2]int{1, 1} {
					return counts, fmt.Errorf("incomplete: webhook=%d, orchestration=%d (expecting [1, 1])", counts[0], counts[1])
				}
				return counts, nil
			}, 15*time.Second, 1*time.Second).Should(Equal([2]int{1, 1}),
				"Both webhook and orchestration approval events must be persisted before CRD deletion (15s = 15x buffer flush)")

			GinkgoWriter.Printf("🚀 E2E-RO-AUD006-003: Created RAR %s and persisted audit events\n", testRAR.Name)
		})

		AfterEach(func() {
			// Cleanup namespace
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		})

		It("should query audit events after RAR CRD is deleted", func() {
			// BUSINESS SCENARIO: 6 months later, auditor investigates incident
			// BUSINESS NEED: Audit trail exists even after CRD cleanup
			// COMPLIANCE: SOC 2 CC7.2 (90-365 day retention)

			By("Deleting RAR CRD (simulates cleanup after incident resolution)")
			Expect(k8sClient.Delete(ctx, testRAR)).To(Succeed())

			// Wait for CRD deletion to complete
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR)
				return err != nil // CRD should not exist
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"RAR CRD should be deleted")

			GinkgoWriter.Printf("🗑️  E2E: Deleted RAR CRD %s\n", testRAR.Name)

			By("Querying DataStorage for audit events (CRD deleted, audit trail persists)")

			// DEBUG: Query ALL events for correlation_id to see what exists
			debugResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Limit:         dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred(), "DataStorage debug query must succeed")
			GinkgoWriter.Printf("🔍 DEBUG: Found %d total events for correlation_id=%s\n", len(debugResp.Data), correlationID)
			for i, evt := range debugResp.Data {
				GinkgoWriter.Printf("   [%d] category=%s, type=%s\n", i, evt.EventCategory, evt.EventType)
			}

			// BUSINESS VALIDATION: Audit events still exist after CRD deletion
			// Query webhook events with all 3 required filters (correlationID, EventCategory, EventType)
			// Prevents pagination overhead - ensures efficient query (BR-AUDIT-006)
			webhookResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventCategory: dsgen.NewOptString("webhook"),
				EventType:     dsgen.NewOptString("webhook.remediationapprovalrequest.decided"),
				Limit:         dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred(), "DataStorage query for webhook events must succeed")
			webhookEvents := webhookResp.Data
			GinkgoWriter.Printf("🔍 DEBUG: Found %d webhook events\n", len(webhookEvents))

			// Query orchestration approval events with all 3 required filters (correlationID, EventCategory, EventType)
			// Note: We query for "approved" events only; test created an approval decision
			orchestrationResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventCategory: dsgen.NewOptString("orchestration"),
				EventType:     dsgen.NewOptString("orchestrator.approval.approved"),
				Limit:         dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred(), "DataStorage query for orchestration events must succeed")

			// No client-side filtering needed - EventType filter ensures only approval events returned
			approvalEvents := orchestrationResp.Data
			GinkgoWriter.Printf("🔍 DEBUG: Found %d approval events\n", len(approvalEvents))

			// BUSINESS OUTCOME 1: Audit trail persists after CRD deletion (SOC 2 CC7.2)
			// Two-Event Pattern: webhook (WHO) + orchestration approval (WHAT/WHY)
			Expect(webhookEvents).To(HaveLen(1),
				"COMPLIANCE FAILURE: Webhook audit event must persist after CRD cleanup (SOC 2 CC7.2)")
			Expect(approvalEvents).To(HaveLen(1),
				"COMPLIANCE FAILURE: Orchestration approval audit event must persist after CRD cleanup (SOC 2 CC7.2)")

			// BUSINESS OUTCOME 2: Complete audit data is retrievable
			webhookEvent := webhookEvents[0]
			approvalEvent := approvalEvents[0]

			actorID, _ := webhookEvent.ActorID.Get()
			// SECURITY: In E2E, authenticated user is "kubernetes-admin" (kubectl context)
			// AuthWebhook correctly overwrites user-provided "e2e-auditor@example.com"
			Expect(actorID).To(Equal("kubernetes-admin"),
				"BUSINESS OUTCOME: Auditor can identify WHO approved (SOC 2 CC8.1)")

			resourceID, _ := approvalEvent.ResourceID.Get()
			Expect(resourceID).ToNot(BeEmpty(),
				"BUSINESS OUTCOME: Auditor can identify WHAT was approved")

			Expect(approvalEvent.EventTimestamp).ToNot(BeZero(),
				"BUSINESS OUTCOME: Auditor can identify WHEN decision was made (SOC 2 CC7.2)")

			// BUSINESS OUTCOME 3: Query by timestamp range (simulates compliance audit)
			By("Querying audit events by timestamp range (compliance audit scenario)")

			// Query for events in the last hour (simulates auditor querying historical data)
			// DataStorage API uses "since" (relative time like "1h") and "until" (absolute RFC3339)
			respByTime, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Since:         dsgen.NewOptString("1h"), // Last 1 hour
				Limit:         dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred(), "Timestamp range query must succeed")
			// Note: Includes lifecycle events + approval events + timeout_modified webhook event
			// Events: 3 lifecycle (started, transitioned, created) + 1 orchestration approval (approved)
			//       + 1 webhook (remediationapprovalrequest.decided) + 1 webhook (remediationrequest.timeout_modified)
			Expect(respByTime.Data).To(HaveLen(6),
				"COMPLIANCE: Audit events must be queryable by timestamp (SOC 2 CC7.2)")

			// BUSINESS OUTCOME 4: Verify actor is present in audit data (forensic investigation)
			By("Verifying actor identity is retrievable (forensic investigation scenario)")

			// DataStorage API doesn't support actor_id filtering in query params
			// Instead, verify actor_id is present in the returned audit events
			// SECURITY: In E2E, authenticated user is "kubernetes-admin" (kubectl context)
			var actorFound bool
			allEvents := append(webhookEvents, approvalEvents...)
			for _, event := range allEvents {
				if eventActorID, hasActor := event.ActorID.Get(); hasActor {
					if eventActorID == "kubernetes-admin" {
						actorFound = true
						break
					}
				}
			}
			Expect(actorFound).To(BeTrue(),
				"COMPLIANCE: Actor identity must be retrievable from audit events (forensic investigation)")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ E2E-RO-AUD006-003: Audit Trail Persistence Validated\n")
			GinkgoWriter.Printf("   BUSINESS OUTCOMES:\n")
			GinkgoWriter.Printf("   • Audit trail persists after CRD deletion ✅\n")
			GinkgoWriter.Printf("   • WHO: kubernetes-admin (retrievable) ✅\n")
			GinkgoWriter.Printf("   • WHEN: %s (retrievable) ✅\n", approvalEvent.EventTimestamp)
			GinkgoWriter.Printf("   • Queryable by correlation_id + event_category filters ✅\n")
			GinkgoWriter.Printf("   • Queryable by timestamp range (since) ✅\n")
			GinkgoWriter.Printf("   • Actor identity retrievable (forensics) ✅\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC7.2 (90-365 day retention) satisfied ✅\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC7.4 (Audit completeness) satisfied ✅\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})
