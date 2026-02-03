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

package authwebhook

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApprovalDecisionScenario defines test data for business outcome validation
type ApprovalDecisionScenario struct {
	testID              string
	businessOutcome     string
	auditorQuestion     string
	complianceControl   string
	decision            remediationv1.ApprovalDecision
	decisionMessage     string
	forgedDecidedBy     string // User-provided DecidedBy (should be overwritten)
	shouldRejectRequest bool   // For invalid decision test
}

var _ = Describe("BR-AUTH-001: RemediationApprovalRequest Decision Attribution", func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	// Helper to create RAR with scenario-specific decision
	createRAR := func(scenario ApprovalDecisionScenario, testSuffix string) *remediationv1.RemediationApprovalRequest {
		return &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rar-" + strings.ToLower(testSuffix) + "-" + randomSuffix(),
				Namespace: namespace,
			},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-rr",
					Namespace: namespace,
				},
				AIAnalysisRef: remediationv1.ObjectRef{
					Name: "test-analysis",
				},
				Confidence:           0.75,
				ConfidenceLevel:      "medium",
				Reason:               "Confidence below auto-approve threshold",
				InvestigationSummary: "Memory leak detected in payment service",
				WhyApprovalRequired:  "Medium confidence requires human validation",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID:     "restart-pod-v1",
					Version:        "1.0.0",
					ContainerImage: "kubernaut/restart-pod:v1",
					Rationale:      "Standard pod restart for memory leak",
				},
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{Action: "Restart pod", Rationale: "Clear memory leak"},
				},
				RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)), // 15 minutes
			},
		}
	}

	// ========================================
	// Data Table: Approval Decision Business Outcomes
	// Focus: Answers specific auditor and compliance questions
	// Pattern: Table-driven tests per Kubernaut testing guidelines
	// ========================================

	DescribeTable("Approval Decision Scenarios - SOC 2 Compliance Validation",
		func(scenario ApprovalDecisionScenario) {
			// BUSINESS ACTION: Create RAR
			rar := createRAR(scenario, scenario.testID)
			createAndWaitForCRD(ctx, k8sClient, rar)

			// BUSINESS ACTION: Operator makes decision
			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = scenario.decision
					rar.Status.DecisionMessage = scenario.decisionMessage
					// DecidedBy intentionally NOT set - webhook will populate from K8s UserInfo
				},
				func() bool {
					return rar.Status.DecidedBy != ""
				},
			)

			// BUSINESS VALIDATION: Can auditor answer their question?
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ %s: %s\n", scenario.testID, scenario.businessOutcome)
			GinkgoWriter.Printf("   AUDITOR QUESTION: %s\n", scenario.auditorQuestion)
			GinkgoWriter.Printf("   COMPLIANCE: %s\n", scenario.complianceControl)

			// BUSINESS VALIDATION: Was authenticated user identity captured?
			Expect(rar.Status.DecidedBy).ToNot(BeEmpty(),
				"COMPLIANCE FAILURE: Missing WHO - cannot satisfy %s", scenario.complianceControl)

			// In envtest: "admin" or "system:serviceaccount:namespace:name"
			// In production: "alice@example.com" (from OAuth/OIDC)
			Expect(rar.Status.DecidedBy).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount"),
				ContainSubstring("@")),
				"BUSINESS OUTCOME: Authenticated user identity is valid K8s UserInfo")

			GinkgoWriter.Printf("   • WHO: %s (authenticated) ✅\n", rar.Status.DecidedBy)

			// BUSINESS VALIDATION: Was timestamp captured?
			Expect(rar.Status.DecidedAt).ToNot(BeNil(),
				"BUSINESS OUTCOME: WHEN decision made is captured")
			GinkgoWriter.Printf("   • WHEN: %s ✅\n", rar.Status.DecidedAt.Time)

			// BUSINESS VALIDATION: Was decision captured?
			Expect(rar.Status.Decision).To(Equal(scenario.decision),
				"BUSINESS OUTCOME: WHAT decision made is clear")
			GinkgoWriter.Printf("   • WHAT: Decision=%s ✅\n", rar.Status.Decision)

			// BUSINESS VALIDATION: Was rationale preserved?
			if scenario.decisionMessage != "" {
				Expect(rar.Status.DecisionMessage).To(Equal(scenario.decisionMessage),
					"BUSINESS OUTCOME: WHY decision made is preserved (legal defense)")
				GinkgoWriter.Printf("   • WHY: %s ✅\n", rar.Status.DecisionMessage)
			}

			GinkgoWriter.Printf("   ✅ %s SATISFIED\n", scenario.complianceControl)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		},

		// ========================================
		// Table Entries: Each entry answers a specific auditor question
		// ========================================

		Entry("INT-RAR-01: Operator approves production remediation",
			ApprovalDecisionScenario{
				testID:            "INT-RAR-01",
				businessOutcome:   "SOC 2 CC8.1 - User Attribution",
				auditorQuestion:   "WHO approved this high-risk production remediation?",
				complianceControl: "SOC 2 CC8.1 (User Attribution)",
				decision:          remediationv1.ApprovalDecisionApproved,
				decisionMessage:   "Reviewed investigation summary - memory leak confirmed, restart approved",
			},
		),

		Entry("INT-RAR-02: Operator rejects risky remediation",
			ApprovalDecisionScenario{
				testID:            "INT-RAR-02",
				businessOutcome:   "SOC 2 CC6.8 - Non-Repudiation",
				auditorQuestion:   "Can company defend WHY this remediation was rejected?",
				complianceControl: "SOC 2 CC6.8 (Non-Repudiation)",
				decision:          remediationv1.ApprovalDecisionRejected,
				decisionMessage:   "Disk expansion requires change control approval first",
			},
		),
	)

	// ========================================
	// Additional Business Behavior Tests
	// (Not suitable for table-driven approach)
	// ========================================

	Context("INT-RAR-03: Invalid Decision Validation", func() {
		It("should reject invalid decision to ensure audit trail integrity", func() {
			// BUSINESS RISK: Invalid decisions corrupt audit trail
			// BUSINESS OUTCOME: Only valid decisions (Approved/Rejected/Expired) accepted

			rar := createRAR(ApprovalDecisionScenario{
				testID:          "INT-RAR-03",
				businessOutcome: "Audit Trail Integrity",
			}, "invalid")
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator provides invalid decision")
			rar.Status.Decision = remediationv1.ApprovalDecision("Maybe") // Invalid

			// BUSINESS VALIDATION: Webhook rejects invalid decision
			err := k8sClient.Status().Update(ctx, rar)
			Expect(err).To(HaveOccurred(),
				"BUSINESS BEHAVIOR: Webhook must reject invalid decision")
			Expect(err.Error()).To(Or(
				ContainSubstring("decision"),
				ContainSubstring("Maybe"),
				ContainSubstring("enum"),
			), "Error should indicate invalid decision")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-03: Invalid Decision Rejection\n")
			GinkgoWriter.Printf("   BUSINESS OUTCOME: Audit trail integrity protected\n")
			GinkgoWriter.Printf("   • Invalid decision rejected: %s ✅\n", err.Error())
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-RAR-04: Identity Forgery Prevention", func() {
		It("should prevent user from forging DecidedBy field", func() {
			// BUSINESS RISK: Operator can frame another operator by setting DecidedBy
			// SECURITY OUTCOME: User-provided DecidedBy MUST be overwritten by webhook
			// COMPLIANCE: SOC 2 CC8.1 requires authenticated identity

			rar := createRAR(ApprovalDecisionScenario{
				testID:          "INT-RAR-04",
				businessOutcome: "Identity Forgery Prevention",
			}, "forgery")
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator attempts to forge identity")
			forgedIdentity := "malicious-user@example.com"

			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = remediationv1.ApprovalDecisionApproved
					rar.Status.DecisionMessage = "Approved"
					rar.Status.DecidedBy = forgedIdentity // USER TRIES TO SET THIS
				},
				func() bool {
					// Wait for webhook to overwrite DecidedBy
					return rar.Status.DecidedBy != "" && rar.Status.DecidedBy != forgedIdentity
				},
			)

			// SECURITY VALIDATION: Webhook OVERWRITES user-provided DecidedBy
			Expect(rar.Status.DecidedBy).ToNot(Equal(forgedIdentity),
				"SECURITY FAILURE: User was able to forge identity")

			// SECURITY VALIDATION: DecidedBy is from authenticated source (webhook)
			Expect(rar.Status.DecidedBy).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount")),
				"SECURITY OUTCOME: DecidedBy is from webhook authentication (not user-provided)")

			authenticatedUser := rar.Status.DecidedBy

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-04: Identity Forgery Prevention\n")
			GinkgoWriter.Printf("   SECURITY OUTCOME: User cannot forge identity\n")
			GinkgoWriter.Printf("   • User tried: %s ❌\n", forgedIdentity)
			GinkgoWriter.Printf("   • Webhook set: %s ✅ (authenticated)\n", authenticatedUser)
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC8.1 satisfied (tamper-proof attribution)\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-RAR-05: Webhook Audit Event Emission", func() {
		It("should emit webhook audit event for tamper-proof audit trail", func() {
			// BUSINESS OUTCOME: Webhook authentication has its own audit trail
			// COMPLIANCE: DD-WEBHOOK-003 (Webhook-Complete Audit Pattern)
			// AUDITOR NEED: Separate audit trail for authentication step

			rar := createRAR(ApprovalDecisionScenario{
				testID:          "INT-RAR-05",
				businessOutcome: "Webhook Audit Event Emission",
			}, "audit")
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator approves via webhook")
			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = remediationv1.ApprovalDecisionApproved
					rar.Status.DecisionMessage = "Approved after security review"
				},
				func() bool {
					return rar.Status.DecidedBy != ""
				},
			)

			authenticatedUser := rar.Status.DecidedBy

			By("Querying DataStorage for webhook audit event")
			// Wait for webhook audit event to appear in DataStorage
			// Correlation ID is RAR name (per remediationapprovalrequest_handler.go:107)
			Eventually(func() int {
				events, err := queryAuditEvents(dsClient, rar.Name, nil)
				if err != nil {
					GinkgoWriter.Printf("⏳ Audit query error: %v\n", err)
					return 0
				}
				return len(events)
			}, 30*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
				"COMPLIANCE FAILURE: No webhook audit event (DD-WEBHOOK-003)")

			// BUSINESS VALIDATION: Query for webhook category events
			events, err := queryAuditEvents(dsClient, rar.Name, nil)
			Expect(err).ToNot(HaveOccurred())

			// Filter for webhook events
			var webhookEvents []string
			for _, event := range events {
				if string(event.EventCategory) == "webhook" {
					webhookEvents = append(webhookEvents, event.EventType)
				}
			}

			Expect(webhookEvents).ToNot(BeEmpty(),
				"COMPLIANCE FAILURE: No webhook audit event (DD-WEBHOOK-003)")

			// BUSINESS VALIDATION: Webhook event captures authenticated user
			var webhookEvent *remediationv1.RemediationApprovalRequest
			for _, event := range events {
				if string(event.EventCategory) == "webhook" {
					// Validate event captures authenticated user
					actorID, hasActorID := event.ActorID.Get()
					Expect(hasActorID).To(BeTrue(),
						"COMPLIANCE FAILURE: Webhook audit event missing actor_id")
					Expect(actorID).To(Equal(authenticatedUser),
						"BUSINESS OUTCOME: Webhook audit event captures authenticated user")

					// Validate event action
					Expect(event.EventAction).To(Equal("approval_decided"),
						"BUSINESS OUTCOME: Webhook audit event captures decision action")

					webhookEvent = &remediationv1.RemediationApprovalRequest{}
					break
				}
			}

			Expect(webhookEvent).ToNot(BeNil(),
				"COMPLIANCE FAILURE: No webhook audit event found")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-05: Webhook Audit Event Emission\n")
			GinkgoWriter.Printf("   BUSINESS OUTCOME: Tamper-proof webhook authentication trail\n")
			GinkgoWriter.Printf("   • Webhook events found: %d ✅\n", len(webhookEvents))
			GinkgoWriter.Printf("   • Event types: %v\n", webhookEvents)
			GinkgoWriter.Printf("   • Authenticated user: %s ✅\n", authenticatedUser)
			GinkgoWriter.Printf("   • COMPLIANCE: DD-WEBHOOK-003 satisfied\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("INT-RAR-06: DecidedBy Preservation for RO Audit", func() {
		It("should preserve authenticated DecidedBy for RO controller audit event", func() {
			// BUSINESS FLOW:
			// 1. AuthWebhook sets DecidedBy (authenticated user)
			// 2. RO controller reads RAR
			// 3. RO emits approval audit event using DecidedBy
			//
			// BUSINESS OUTCOME: End-to-end audit trail from webhook to controller
			// COMPLIANCE: SOC 2 CC8.1 (User Attribution across service boundary)

			rar := createRAR(ApprovalDecisionScenario{
				testID:          "INT-RAR-06",
				businessOutcome: "DecidedBy Preservation for RO Audit",
			}, "preservation")
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Operator approves via webhook")
			updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
				func() {
					rar.Status.Decision = remediationv1.ApprovalDecisionApproved
					rar.Status.DecisionMessage = "Approved for production deployment"
				},
				func() bool {
					return rar.Status.DecidedBy != ""
				},
			)

			authenticatedUser := rar.Status.DecidedBy

			// BUSINESS VALIDATION: Authenticated user is accessible
			Expect(authenticatedUser).ToNot(BeEmpty(),
				"BUSINESS FAILURE: DecidedBy not set by webhook")

			Expect(authenticatedUser).To(Or(
				Equal("admin"),
				ContainSubstring("system:serviceaccount")),
				"BUSINESS OUTCOME: Authenticated user is valid K8s identity")

			// BUSINESS VALIDATION: RO controller can read DecidedBy field
			// (This integration test validates webhook behavior)
			// (RO controller integration tests will validate audit event emission)

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ INT-RAR-06: DecidedBy Preservation for RO Audit\n")
			GinkgoWriter.Printf("   BUSINESS OUTCOME: Authenticated user available for RO audit\n")
			GinkgoWriter.Printf("   • DecidedBy: %s ✅\n", authenticatedUser)
			GinkgoWriter.Printf("   • Field accessible: Yes ✅\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC8.1 (cross-service attribution)\n")
			GinkgoWriter.Printf("   • Next: RO controller will emit approval audit event\n")
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})
})
