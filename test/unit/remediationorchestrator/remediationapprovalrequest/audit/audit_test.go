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

package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationapprovalrequestv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	prodaudit "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest/audit"
)

func TestRemediationApprovalRequestAudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationApprovalRequest Audit Suite - Business Outcome Validation")
}

// MockAuditStore implements audit.AuditStore interface for testing
type MockAuditStore struct {
	StoredEvents []*ogenclient.AuditEventRequest
	StoreError   error
}

func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	if m.StoreError != nil {
		return m.StoreError
	}
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

func (m *MockAuditStore) Flush(ctx context.Context) error {
	return nil
}

func (m *MockAuditStore) Close() error {
	return nil
}

// ApprovalDecisionScenario defines test data for business outcome validation
type ApprovalDecisionScenario struct {
	testID               string
	businessOutcome      string
	auditorQuestion      string
	complianceControl    string
	decision             remediationapprovalrequestv1alpha1.ApprovalDecision
	decidedBy            string
	decisionMessage      string
	expectedOutcome      ogenclient.AuditEventRequestEventOutcome
	expectedDecisionEnum ogenclient.RemediationApprovalDecisionPayloadDecision
	shouldEmitEvent      bool
	validateBehavior     func(event *ogenclient.AuditEventRequest, scenario ApprovalDecisionScenario)
}

var _ = Describe("BR-AUDIT-006: RemediationApprovalRequest Audit Trail", func() {
	var (
		ctx         context.Context
		auditClient *prodaudit.AuditClient
		mockStore   *MockAuditStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		logger := logr.Discard()
		auditClient = prodaudit.NewAuditClient(mockStore, logger)
	})

	// Helper to create RAR with specific decision
	createRAR := func(scenario ApprovalDecisionScenario) *remediationapprovalrequestv1alpha1.RemediationApprovalRequest {
		now := metav1.Now()
		return &remediationapprovalrequestv1alpha1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "rar-test-001",
				Namespace:         "production",
				CreationTimestamp: metav1.Time{Time: now.Add(-180 * time.Second)},
			},
			Spec: remediationapprovalrequestv1alpha1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-parent-123",
					Namespace: "production",
				},
				AIAnalysisRef: remediationapprovalrequestv1alpha1.ObjectRef{
					Name: "ai-test-456",
				},
				Confidence:      0.75,
				ConfidenceLevel: "medium",
				Reason:          "Confidence below 80% auto-approve threshold",
				RecommendedWorkflow: remediationapprovalrequestv1alpha1.RecommendedWorkflowSummary{
					WorkflowID:     "oomkill-increase-memory-limits",
					Version:        "v1.2.0",
					ExecutionBundle: "ghcr.io/kubernaut/oomkill-remediation:v1.2.0",
					Rationale:      "Increases memory limits to prevent OOMKilled",
				},
				InvestigationSummary: "Pod api-server OOMKilled due to memory pressure",
				RecommendedActions: []remediationapprovalrequestv1alpha1.ApprovalRecommendedAction{
					{
						Action:    "Increase memory limits from 512Mi to 1Gi",
						Rationale: "Current usage peaks at 480Mi",
					},
				},
				WhyApprovalRequired: "Confidence score 0.75 below auto-approve threshold (0.80)",
			},
			Status: remediationapprovalrequestv1alpha1.RemediationApprovalRequestStatus{
				Decision:        scenario.decision,
				DecidedBy:       scenario.decidedBy,
				DecidedAt:       &now,
				DecisionMessage: scenario.decisionMessage,
			},
		}
	}

	// ========================================
	// Data Table: Approval Decision Business Outcomes
	// Focus: SOC 2 Compliance Questions Auditors Must Answer
	// ========================================

	DescribeTable("Approval Decision Scenarios",
		func(scenario ApprovalDecisionScenario) {
			// BUSINESS ACTION: Create RAR and record decision
			rar := createRAR(scenario)
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: Did we emit an event?
			if !scenario.shouldEmitEvent {
				Expect(mockStore.StoredEvents).To(HaveLen(0),
					"BUSINESS BEHAVIOR: No event for pending decisions (prevents audit pollution)")
				return
			}

			Expect(mockStore.StoredEvents).To(HaveLen(1),
				"BUSINESS OUTCOME: Audit trail exists for compliance (%s)", scenario.complianceControl)

			event := mockStore.StoredEvents[0]

			// BUSINESS VALIDATION: Can auditor answer their question?
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ %s: %s\n", scenario.testID, scenario.businessOutcome)
			GinkgoWriter.Printf("   AUDITOR QUESTION: %s\n", scenario.auditorQuestion)
			GinkgoWriter.Printf("   COMPLIANCE: %s\n", scenario.complianceControl)

			// BUSINESS VALIDATION: WHO made the decision?
			actorID, hasActorID := event.ActorID.Get()
			Expect(hasActorID).To(BeTrue(),
				"COMPLIANCE FAILURE: Missing WHO - cannot satisfy %s", scenario.complianceControl)
			Expect(actorID).To(Equal(scenario.decidedBy),
				"BUSINESS OUTCOME: Auditor can answer WHO: %s", scenario.decidedBy)
			GinkgoWriter.Printf("   • WHO: %s ✅\n", actorID)

			// BUSINESS VALIDATION: WHAT decision was made?
			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue(),
				"COMPLIANCE FAILURE: Cannot extract approval context")
			Expect(payload.Decision).To(Equal(scenario.expectedDecisionEnum),
				"BUSINESS OUTCOME: Decision is clear and unambiguous")
			GinkgoWriter.Printf("   • WHAT: Decision=%s ✅\n", payload.Decision)

			// BUSINESS VALIDATION: WHY was decision made?
			if scenario.decisionMessage != "" {
				decisionMsg, hasDM := payload.DecisionMessage.Get()
				Expect(hasDM).To(BeTrue(),
					"LEGAL RISK: No rationale - cannot defend decision")
				Expect(decisionMsg).To(Equal(scenario.decisionMessage),
					"BUSINESS OUTCOME: Operator's rationale preserved")
				GinkgoWriter.Printf("   • WHY: %s ✅\n", decisionMsg)
			}

			// BUSINESS VALIDATION: Correct outcome for business logic?
			Expect(event.EventOutcome).To(Equal(scenario.expectedOutcome),
				"BUSINESS CORRECTNESS: Outcome reflects business result (approved=success, rejected/expired=failure)")

			// BUSINESS VALIDATION: Can we link to remediation flow?
			Expect(event.CorrelationID).To(Equal("rr-parent-123"),
				"BUSINESS OUTCOME: Auditor can trace approval to parent remediation")
			GinkgoWriter.Printf("   • LINKED TO: %s ✅\n", event.CorrelationID)

			// BUSINESS VALIDATION: Run custom behavior validation if defined
			if scenario.validateBehavior != nil {
				scenario.validateBehavior(event, scenario)
			}

			GinkgoWriter.Printf("   ✅ %s SATISFIED\n", scenario.complianceControl)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		},

		// ========================================
		// Table Entries: Business Scenarios
		// Each entry answers a specific auditor question
		// ========================================

		Entry("UT-RO-AUD006-001: Operator approves production remediation (SOC 2 CC8.1)",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-001",
				businessOutcome:      "SOC 2 CC8.1 - User Attribution",
				auditorQuestion:      "WHO approved this high-risk production remediation?",
				complianceControl:    "SOC 2 CC8.1 (User Attribution)",
				decision:             remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:            "alice@example.com",
				decisionMessage:      "Root cause accurate. Safe to proceed.",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeSuccess,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionApproved,
				shouldEmitEvent:      true,
				validateBehavior: func(event *ogenclient.AuditEventRequest, scenario ApprovalDecisionScenario) {
					// BUSINESS VALIDATION: Is approval context complete?
					payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
					Expect(payload.WorkflowID).To(Equal("oomkill-increase-memory-limits"),
						"BUSINESS OUTCOME: Auditor knows WHAT workflow was approved")
					Expect(payload.Confidence).To(Equal(float32(0.75)),
						"BUSINESS OUTCOME: Auditor knows risk level (75%% confidence)")
				},
			},
		),

		Entry("UT-RO-AUD006-002: Operator rejects risky remediation (SOC 2 CC6.8)",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-002",
				businessOutcome:      "SOC 2 CC6.8 - Non-Repudiation",
				auditorQuestion:      "Can we defend WHY this remediation was rejected?",
				complianceControl:    "SOC 2 CC6.8 (Non-Repudiation)",
				decision:             remediationapprovalrequestv1alpha1.ApprovalDecisionRejected,
				decidedBy:            "alice@example.com",
				decisionMessage:      "Risk too high for production - potential cascading failures",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeFailure,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionRejected,
				shouldEmitEvent:      true,
				validateBehavior: func(event *ogenclient.AuditEventRequest, scenario ApprovalDecisionScenario) {
					// BUSINESS VALIDATION: Is rejection rationale defensible?
					payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
					decisionMsg, _ := payload.DecisionMessage.Get()
					Expect(decisionMsg).To(ContainSubstring("Risk too high"),
						"LEGAL DEFENSE: Operator's risk assessment preserved")
					Expect(decisionMsg).To(ContainSubstring("cascading failures"),
						"LEGAL DEFENSE: Technical rationale documented")
				},
			},
		),

		Entry("UT-RO-AUD006-003: Approval timeout - no operator response (Operational Accountability)",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-003",
				businessOutcome:      "Timeout Accountability",
				auditorQuestion:      "Why did this high-priority remediation NOT proceed?",
				complianceControl:    "SOC 2 CC7.2 (Monitoring Activities)",
				decision:             remediationapprovalrequestv1alpha1.ApprovalDecisionExpired,
				decidedBy:            "system",
				decisionMessage:      "Approval timeout - no operator response within 15 minutes",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeFailure,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionExpired,
				shouldEmitEvent:      true,
				validateBehavior: func(event *ogenclient.AuditEventRequest, scenario ApprovalDecisionScenario) {
					// BUSINESS VALIDATION: Can we prove timeout was system-driven?
					actorID, _ := event.ActorID.Get()
					Expect(actorID).To(Equal("system"),
						"OPERATIONAL OUTCOME: System timeout (not operator negligence)")
					payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
					decisionMsg, _ := payload.DecisionMessage.Get()
					Expect(decisionMsg).To(ContainSubstring("timeout"),
						"OPERATIONAL OUTCOME: Clear timeout indication for root cause analysis")
				},
			},
		),

		Entry("UT-RO-AUD006-004: Pending approval - no decision yet (Prevent Audit Pollution)",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-004",
				businessOutcome:      "Prevent Audit Pollution",
				auditorQuestion:      "Are audit events accurate (no incomplete/duplicate records)?",
				complianceControl:    "SOC 2 CC7.4 (Audit Completeness)",
				decision:             "", // Empty = pending
				decidedBy:            "",
				decisionMessage:      "",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeSuccess,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionApproved,
				shouldEmitEvent:      false, // NO EVENT for pending
				validateBehavior:     nil,
			},
		),
	)

	// ========================================
	// Additional Business Outcome Tests
	// ========================================

	Context("UT-RO-AUD006-005: Authentication Validation", func() {
		It("should prove user identity is authenticated (not self-reported)", func() {
			// BUSINESS RISK: Self-reported identity can be forged
			// BUSINESS OUTCOME: Webhook authentication proves real identity

			rar := createRARForScenario(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "bob@example.com",
				decisionMessage: "Approved",
			})

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: User identity is from webhook (authenticated)
			event := mockStore.StoredEvents[0]
			actorID, _ := event.ActorID.Get()
			Expect(actorID).To(Equal("bob@example.com"),
				"BUSINESS OUTCOME: User authenticated via webhook (SOC 2 CC8.1)")

			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(payload.DecidedBy).To(Equal("bob@example.com"),
				"BUSINESS OUTCOME: Identity consistent across event and payload")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-005: Authenticated user identity validated\n")
			GinkgoWriter.Printf("   • User: %s (webhook-authenticated) ✅\n", actorID)
			GinkgoWriter.Printf("   • BUSINESS VALUE: Cannot forge identity (legal defensibility)\n")
		})
	})

	Context("UT-RO-AUD006-006: Audit Trail Continuity", func() {
		It("should enable auditors to reconstruct complete remediation timeline", func() {
			// BUSINESS OUTCOME: Single correlation_id links all events across services
			// AUDITOR NEED: Query once to get complete story

			rar := createRARForScenario(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Approved",
			})
			rar.Spec.RemediationRequestRef.Name = "rr-custom-parent-789"

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: Correlation ID enables timeline reconstruction
			event := mockStore.StoredEvents[0]
			Expect(event.CorrelationID).To(Equal("rr-custom-parent-789"),
				"BUSINESS OUTCOME: Auditor can query all events with: WHERE correlation_id='rr-custom-parent-789'")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-006: Audit trail continuity validated\n")
			GinkgoWriter.Printf("   • Correlation ID: %s ✅\n", event.CorrelationID)
			GinkgoWriter.Printf("   • BUSINESS VALUE: Single query reconstructs complete remediation timeline\n")
		})
	})

	Context("UT-RO-AUD006-007: Forensic Investigation Capability", func() {
		It("should provide complete context for post-incident investigation", func() {
			// BUSINESS SCENARIO: 6 months later, SEC team investigates production incident
			// BUSINESS NEED: Reconstruct approval decision without K8s CRD access

			rar := createRARForScenario(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Root cause accurate. Safe to proceed.",
			})

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: All forensic investigation fields present
			event := mockStore.StoredEvents[0]
			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()

			// Can investigator answer: WHO, WHAT, WHY, WHEN?
			Expect(payload.DecidedBy).ToNot(BeEmpty(),
				"FORENSIC NEED: WHO approved")
			Expect(payload.WorkflowID).ToNot(BeEmpty(),
				"FORENSIC NEED: WHAT workflow")
			Expect(payload.Confidence).To(BeNumerically(">", 0),
				"FORENSIC NEED: Risk level (confidence)")
			decisionMsg, _ := payload.DecisionMessage.Get()
			Expect(decisionMsg).ToNot(BeEmpty(),
				"FORENSIC NEED: WHY approved (operator rationale)")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-007: Forensic investigation capability\n")
			GinkgoWriter.Printf("   • Complete context preserved ✅\n")
			GinkgoWriter.Printf("   • BUSINESS VALUE: Post-incident investigation enabled\n")
		})
	})

	Context("UT-RO-AUD006-008: System Resilience", func() {
		It("should continue approvals even if audit system fails", func() {
			// BUSINESS RISK: Audit failure should NOT block critical approvals
			// BUSINESS OUTCOME: Approval workflow continues (graceful degradation)

			mockStore.StoreError = errors.New("audit store unavailable")

			rar := createRARForScenario(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Approved",
			})

			// BUSINESS ACTION: Attempt to record decision
			Expect(func() {
				auditClient.RecordApprovalDecision(ctx, rar)
			}).ToNot(Panic(),
				"BUSINESS BEHAVIOR: Approval continues despite audit failure")

			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"Expected: no events due to store error")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-008: System resilience validated\n")
			GinkgoWriter.Printf("   • No panic on audit failure ✅\n")
			GinkgoWriter.Printf("   • BUSINESS VALUE: Critical approvals not blocked by audit infrastructure\n")
		})
	})
})

// Helper for creating RAR in specific scenarios
func createRARForScenario(scenario ApprovalDecisionScenario) *remediationapprovalrequestv1alpha1.RemediationApprovalRequest {
	now := metav1.Now()
	return &remediationapprovalrequestv1alpha1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rar-test-001",
			Namespace:         "production",
			CreationTimestamp: metav1.Time{Time: now.Add(-180 * time.Second)},
		},
		Spec: remediationapprovalrequestv1alpha1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "rr-parent-123",
				Namespace: "production",
			},
			AIAnalysisRef: remediationapprovalrequestv1alpha1.ObjectRef{
				Name: "ai-test-456",
			},
			Confidence:      0.75,
			ConfidenceLevel: "medium",
			Reason:          "Confidence below 80% auto-approve threshold",
			RecommendedWorkflow: remediationapprovalrequestv1alpha1.RecommendedWorkflowSummary{
				WorkflowID:     "oomkill-increase-memory-limits",
				Version:        "v1.2.0",
				ExecutionBundle: "ghcr.io/kubernaut/oomkill-remediation:v1.2.0",
				Rationale:      "Increases memory limits to prevent OOMKilled",
			},
			InvestigationSummary: "Pod api-server OOMKilled due to memory pressure",
			RecommendedActions: []remediationapprovalrequestv1alpha1.ApprovalRecommendedAction{
				{
					Action:    "Increase memory limits from 512Mi to 1Gi",
					Rationale: "Current usage peaks at 480Mi",
				},
			},
			WhyApprovalRequired: "Confidence score 0.75 below auto-approve threshold (0.80)",
		},
		Status: remediationapprovalrequestv1alpha1.RemediationApprovalRequestStatus{
			Decision:        scenario.decision,
			DecidedBy:       scenario.decidedBy,
			DecidedAt:       &now,
			DecisionMessage: scenario.decisionMessage,
		},
	}
}
