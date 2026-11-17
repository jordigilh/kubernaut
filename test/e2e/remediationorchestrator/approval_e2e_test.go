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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/test/infrastructure"
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
//   1. Webhook event (event_category="approval")
//   2. RO approval event (event_category="orchestration")
// - Validate complete audit trail (WHO, WHAT, WHEN, WHY)
//
// Expected: Tests will FAIL until RO controller emits approval audit events

var _ = Describe("BR-AUDIT-006: RAR Audit Trail E2E", Label("e2e", "audit", "approval"), func() {
	const (
		dataStorageURL = "https://localhost:8090" // Issue #785: DataStorage API (HTTPS)
		e2eTimeout     = 120 * time.Second
		e2eInterval    = 2 * time.Second
	)

	// E2E-RO-AUD006-001 (Complete RAR Approval Audit Trail) was removed: its
	// decision specs raced this lane's 3s awaitingApproval config (required by
	// the expiry context below) — the authwebhook correctly denied the later
	// decision updates once RequiredBy passed, and neither patching the RAR spec
	// (immutable per ADR-040) nor replacing the RAR (races RO's reconcile) is
	// sound. The behaviors are covered in better-suited tiers: webhook decision
	// handling and audit emission at UT (pkg/authwebhook/
	// remediationapprovalrequest_audit_test.go, BR-AUDIT-006), and decision +
	// DataStorage audit correlation end-to-end at E2E fullpipeline
	// (02_approval_lifecycle_test.go, Step 16 / E2E-FP-118-006). The 3s deadline
	// control itself stays exercised by the expiry context in this file.

	// E2E-RAR-163-003: RAR Expiry (Bug Fix 2-4)
	// Tests that when spec.requiredBy is in the past, RO controller sets:
	// Decision=Expired, DecidedBy=system, Expired=true, TimeRemaining=0s
	Context("E2E-RAR-163-003: RAR Expiry", func() {
		var (
			testNamespace string
			testRR        *remediationv1.RemediationRequest
			testRAR       *remediationv1.RemediationApprovalRequest
		)

		BeforeEach(func() {
			testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "ro-e2e-expiry",
				helpers.WithLabels(map[string]string{
					"kubernaut.ai/audit-enabled": "true",
				}))

			now := metav1.Now()
			testRR = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-rar-expiry-%s", uuid.New().String()[:8]),
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: func() string {
						h := sha256.Sum256([]byte("e2e-rar-expiry-" + uuid.New().String()))
						return hex.EncodeToString(h[:])
					}(),
					SignalName: "E2ERARExpiryTest",
					Severity:   signalprocessingv1.SeverityCritical,
					SignalType: "alert",
					TargetType: "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test-app-expiry",
						Namespace:  testNamespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

			// Let the RO controller drive the full lifecycle: RR → SP → AI → RAR
			sp := helpers.WaitForSPCreation(ctx, k8sClient, controllerNamespace, testRR.Name, e2eTimeout, e2eInterval)
			helpers.SimulateSPCompletion(ctx, k8sClient, sp)

			ai := helpers.WaitForAICreation(ctx, k8sClient, controllerNamespace, testRR.Name, e2eTimeout, e2eInterval)
			completionOpts := trustedWorkflowCompletionOptions("Deployment", "test-app-expiry", testNamespace)
			completionOpts.ApprovalRequired = true
			completionOpts.ApprovalReason = "Confidence below 80% auto-approve threshold"
			completionOpts.Confidence = 0.75
			helpers.SimulateAICompletedWithWorkflow(ctx, k8sClient, ai, completionOpts)

			testRAR = helpers.WaitForRARCreation(ctx, k8sClient, controllerNamespace, testRR.Name, e2eTimeout, e2eInterval)

			// RAR RequiredBy is set by the RO controller using the configured
			// awaitingApproval timeout (3s in E2E). No spec mutation needed —
			// the deadline expires naturally, and RO detects it on next reconcile.
			helpers.WaitForRRPhase(ctx, k8sClient, testRR, remediationv1.PhaseAwaitingApproval, e2eTimeout, e2eInterval)
			GinkgoWriter.Printf("  RO reached AwaitingApproval naturally\n")
		})

		AfterEach(func() {
			if testRR != nil {
				_ = k8sClient.Delete(ctx, testRR)
			}
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		})

		It("should set Decision=Expired, DecidedBy=system, Expired=true, TimeRemaining=0s when requiredBy is in the past", func() {
			By("Waiting for RO controller to detect timeout and update RAR status")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR)).To(Succeed())
				g.Expect(testRAR.Status.Decision).To(Equal(remediationv1.ApprovalDecisionExpired),
					"Decision should be Expired when requiredBy is in the past")
				g.Expect(testRAR.Status.DecidedBy).To(Equal("system"),
					"DecidedBy should be 'system' for timeout expiry")
				g.Expect(testRAR.Status.Expired).To(BeTrue(),
					"Status.Expired must be true when approval times out (Bug Fix 3)")
				g.Expect(testRAR.Status.TimeRemaining).To(Equal("0s"),
					"TimeRemaining must be 0s when expired (Bug Fix 4)")
			}, 120*time.Second, 2*time.Second).Should(Succeed())

			By("E2E-RAR-163-003: Verifying ApprovalExpired and ApprovalPending conditions")
			Expect(testRAR.Status.Conditions).To(ContainElements(
				And(HaveField("Type", rarconditions.ConditionApprovalExpired), HaveField("Status", metav1.ConditionTrue)),
				And(HaveField("Type", rarconditions.ConditionApprovalPending), HaveField("Status", metav1.ConditionFalse)),
			))

			GinkgoWriter.Printf("✅ E2E-RAR-163-003: RAR expiry validated - Decision=%s, Expired=%v, TimeRemaining=%s\n",
				testRAR.Status.Decision, testRAR.Status.Expired, testRAR.Status.TimeRemaining)
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

			// Create authenticated DataStorage client (Issue #785: TLS)
			tlsBase, err := infrastructure.NewTLSAwareTransport(kubeconfigPath)
			Expect(err).ToNot(HaveOccurred(), "TLS transport for DataStorage")
			saTransport := testauth.NewServiceAccountTransportWithBase(e2eAuthToken, tlsBase)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			dsClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred())

			// Create RemediationRequest (ADR-057: RRs live in kubernaut-system)
			now := metav1.Now()
			testRR = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-rar-persist-%s", uuid.New().String()[:8]),
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: func() string {
						h := sha256.Sum256([]byte("e2e-rar-persist-" + uuid.New().String()))
						return hex.EncodeToString(h[:])
					}(),
					SignalName: "E2ERARAuditPersistenceTest",
					Severity:   signalprocessingv1.SeverityCritical,
					SignalType: "alert",
					TargetType: "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
						Name:       "test-app-persist",
						Namespace:  testNamespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())
			correlationID = testRR.Name

			// Create RAR (RAR lives in same namespace as RR per RO controller)
			workflowSnapshot := trustedWorkflowSnapshot()
			testRAR = &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("rar-%s", testRR.Name),
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      testRR.Name,
						Namespace: controllerNamespace,
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
						WorkflowID:      trustedWorkflowID,
						Version:         "1.0.0",
						ExecutionBundle: trustedWorkflowBundle,
						Rationale:       "Standard pod restart",
					},
					Workflow:   &workflowSnapshot,
					Parameters: trustedWorkflowParameters("Deployment", "test-app-persist", testNamespace),
					TargetContext: &remediationv1.ApprovalTargetContext{
						SignalTarget: testRR.Spec.TargetResource,
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart pod", Rationale: "Clear memory leak"},
					},
					RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
				},
			}
			Expect(k8sClient.Create(ctx, testRAR)).To(Succeed())

			// Approve RAR with retry to handle concurrent RO modifications
			Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRAR), testRAR); err != nil {
					return err
				}
				testRAR.Status.Decision = remediationv1.ApprovalDecisionApproved
				testRAR.Status.DecidedBy = "e2e-auditor@example.com"
				now = metav1.Now()
				testRAR.Status.DecidedAt = &now
				testRAR.Status.DecisionMessage = "Approved for persistence test"
				return k8sClient.Status().Update(ctx, testRAR)
			})).To(Succeed())

			// Wait for BOTH webhook and orchestration approval audit events to be persisted
			// FIX: Enhanced error visibility + longer timeout to handle audit buffer flush (1s interval)
			// Two-Event Pattern: webhook (WHO) + orchestration approval (WHAT/WHY)
			Eventually(func() ([2]int, error) {
				// Query webhook events with all 3 required filters (correlationID, EventCategory, EventType)
				// All 3 filters ensure we find the CORRECT event (BR-AUDIT-006)
				webhookResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					EventCategory: dsgen.NewOptString(authwebhook.EventCategoryApproval),
					EventType:     dsgen.NewOptString(authwebhook.EventTypeRARDecided),
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
					EventCategory: dsgen.NewOptString(roaudit.EventCategoryOrchestration),
					EventType:     dsgen.NewOptString(roaudit.EventTypeApprovalApproved),
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
			if testRR != nil {
				_ = k8sClient.Delete(ctx, testRR)
			}
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
				EventCategory: dsgen.NewOptString(authwebhook.EventCategoryApproval),
				EventType:     dsgen.NewOptString(authwebhook.EventTypeRARDecided),
				Limit:         dsgen.NewOptInt(100),
			})
			Expect(err).ToNot(HaveOccurred(), "DataStorage query for webhook events must succeed")
			webhookEvents := webhookResp.Data
			GinkgoWriter.Printf("🔍 DEBUG: Found %d webhook events\n", len(webhookEvents))

			// Query orchestration approval events with all 3 required filters (correlationID, EventCategory, EventType)
			// Note: We query for "approved" events only; test created an approval decision
			orchestrationResp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventCategory: dsgen.NewOptString(roaudit.EventCategoryOrchestration),
				EventType:     dsgen.NewOptString(roaudit.EventTypeApprovalApproved),
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
			//
			// Spec-mandated audit events for this scenario (6 total):
			//   3 lifecycle: started, transitioned, created  (DD-AUDIT-003)
			//   1 webhook:   remediationapprovalrequest.decided (ADR-034 v1.7 two-event pattern: WHO)
			//   1 approval:  orchestrator.approval.approved     (ADR-034 v1.7 two-event pattern: WHAT/WHY)
			//   1 webhook:   remediationrequest.timeout_modified (BR-AUDIT-005 Gap #8)
			//
			// The timeout_modified event may still be in-flight from the AuthWebhook
			// audit buffer when earlier assertions pass, so poll until all 6 arrive.
			expectedTypes := []string{
				roaudit.EventTypeLifecycleStarted,
				roaudit.EventTypeLifecycleTransitioned,
				roaudit.EventTypeLifecycleCreated,
				authwebhook.EventTypeRARDecided,
				roaudit.EventTypeApprovalApproved,
				authwebhook.EventTypeTimeoutModified,
			}
			Eventually(func(g Gomega) {
				resp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
					CorrelationID: dsgen.NewOptString(correlationID),
					Since:         dsgen.NewOptString("1h"),
					Limit:         dsgen.NewOptInt(100),
				})
				g.Expect(err).ToNot(HaveOccurred(), "Timestamp range query must succeed")
				g.Expect(resp.Data).To(HaveLen(6),
					"COMPLIANCE: Audit events must be queryable by timestamp (SOC 2 CC7.2)")

				eventTypes := make([]string, len(resp.Data))
				for i, evt := range resp.Data {
					eventTypes[i] = evt.EventType
				}
				g.Expect(eventTypes).To(ConsistOf(expectedTypes),
					"COMPLIANCE: Each spec-mandated audit event must appear exactly once (no duplicates, no missing)")
			}, 15*time.Second, 1*time.Second).Should(Succeed())

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
