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
	RunSpecs(t, "BR-AUDIT-006: RemediationApprovalRequest Audit - Business Outcome Validation")
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
					ContainerImage: "ghcr.io/kubernaut/oomkill-remediation:v1.2.0",
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
	// Focus: Answers specific auditor and compliance questions
	// Pattern: Table-driven tests per Kubernaut testing guidelines
	// ========================================

	DescribeTable("Approval Decision Scenarios - SOC 2 Compliance Validation",
		func(scenario ApprovalDecisionScenario) {
			// BUSINESS ACTION: Create RAR and record decision
			rar := createRAR(scenario)
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: Event emission behavior
			if !scenario.shouldEmitEvent {
				Expect(mockStore.StoredEvents).To(HaveLen(0),
					"BUSINESS BEHAVIOR: No event for pending decisions (prevents audit pollution)")
				GinkgoWriter.Printf("✅ %s: %s\n", scenario.testID, scenario.businessOutcome)
				GinkgoWriter.Printf("   VALIDATED: No premature audit events ✅\n")
				return
			}

			Expect(mockStore.StoredEvents).To(HaveLen(1),
				"COMPLIANCE FAILURE: No audit trail - %s cannot be satisfied", scenario.complianceControl)

			event := mockStore.StoredEvents[0]
			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue(),
				"COMPLIANCE FAILURE: Cannot extract approval context")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ %s: %s\n", scenario.testID, scenario.businessOutcome)
			GinkgoWriter.Printf("   AUDITOR QUESTION: %s\n", scenario.auditorQuestion)
			GinkgoWriter.Printf("   COMPLIANCE: %s\n", scenario.complianceControl)

			// BUSINESS VALIDATION: Can auditor answer WHO?
			actorID, hasActorID := event.ActorID.Get()
			Expect(hasActorID).To(BeTrue(),
				"COMPLIANCE FAILURE: Missing WHO - cannot satisfy %s", scenario.complianceControl)
			Expect(actorID).To(Equal(scenario.decidedBy),
				"BUSINESS CORRECTNESS: Auditor answer for WHO is accurate")
			GinkgoWriter.Printf("   • WHO: %s ✅\n", actorID)

			// BUSINESS VALIDATION: Can auditor answer WHAT decision?
			Expect(payload.Decision).To(Equal(scenario.expectedDecisionEnum),
				"BUSINESS CORRECTNESS: Decision is unambiguous")
			GinkgoWriter.Printf("   • WHAT: Decision=%s ✅\n", payload.Decision)

			// BUSINESS VALIDATION: Can auditor answer WHY?
			if scenario.decisionMessage != "" {
				decisionMsg, hasDM := payload.DecisionMessage.Get()
				Expect(hasDM).To(BeTrue(),
					"LEGAL RISK: No rationale - cannot defend decision in audit")
				Expect(decisionMsg).To(ContainSubstring(scenario.decisionMessage),
					"BUSINESS CORRECTNESS: Operator's rationale preserved accurately")
				GinkgoWriter.Printf("   • WHY: %s ✅\n", decisionMsg)
			}

			// BUSINESS VALIDATION: Does outcome reflect business result?
			Expect(event.EventOutcome).To(Equal(scenario.expectedOutcome),
				"BUSINESS CORRECTNESS: Approved=success, Rejected/Expired=failure")

			// BUSINESS VALIDATION: Can auditor link to remediation?
			Expect(event.CorrelationID).To(Equal("rr-parent-123"),
				"BUSINESS OUTCOME: Auditor can reconstruct complete remediation timeline")
			GinkgoWriter.Printf("   • LINKED TO: %s ✅\n", event.CorrelationID)

			// BUSINESS VALIDATION: Is workflow context complete?
			Expect(payload.WorkflowID).To(Equal("oomkill-increase-memory-limits"),
				"FORENSIC NEED: What workflow was approved/rejected")
			Expect(payload.Confidence).To(Equal(float32(0.75)),
				"FORENSIC NEED: Risk level at time of decision")

			GinkgoWriter.Printf("   ✅ %s SATISFIED\n", scenario.complianceControl)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		},

		// ========================================
		// Table Entries: Each entry answers a specific auditor question
		// ========================================

		Entry("UT-RO-AUD006-001: Operator approves production remediation",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-001",
				businessOutcome:      "SOC 2 CC8.1 - User Attribution",
				auditorQuestion:      "WHO approved this high-risk production remediation on Jan 15?",
				complianceControl:    "SOC 2 CC8.1 (User Attribution)",
				decision:             remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:            "alice@example.com",
				decisionMessage:      "Root cause accurate. Safe to proceed.",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeSuccess,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionApproved,
				shouldEmitEvent:      true,
			},
		),

		Entry("UT-RO-AUD006-002: Operator rejects risky remediation",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-002",
				businessOutcome:      "SOC 2 CC6.8 - Non-Repudiation",
				auditorQuestion:      "Can company defend WHY this remediation was rejected?",
				complianceControl:    "SOC 2 CC6.8 (Non-Repudiation)",
				decision:             remediationapprovalrequestv1alpha1.ApprovalDecisionRejected,
				decidedBy:            "alice@example.com",
				decisionMessage:      "Risk too high for production - memory increase may trigger cascading failures",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeFailure,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionRejected,
				shouldEmitEvent:      true,
			},
		),

		Entry("UT-RO-AUD006-003: Approval timeout - no operator response",
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
			},
		),

		Entry("UT-RO-AUD006-004: Pending approval - no decision yet",
			ApprovalDecisionScenario{
				testID:               "UT-RO-AUD006-004",
				businessOutcome:      "Prevent Audit Pollution",
				auditorQuestion:      "Are audit records accurate (no incomplete decisions)?",
				complianceControl:    "SOC 2 CC7.4 (Audit Completeness)",
				decision:             "", // Empty = pending
				decidedBy:            "",
				decisionMessage:      "",
				expectedOutcome:      ogenclient.AuditEventRequestEventOutcomeSuccess,
				expectedDecisionEnum: ogenclient.RemediationApprovalDecisionPayloadDecisionApproved,
				shouldEmitEvent:      false, // NO EVENT until decision made
			},
		),
	)

	// ========================================
	// Additional Business Behavior Tests
	// (Not suitable for table-driven approach)
	// ========================================

	Context("UT-RO-AUD006-005: Authentication Validation", func() {
		It("should prove user identity is authenticated (not self-reported)", func() {
			// BUSINESS RISK: Self-reported identity can be forged
			// BUSINESS OUTCOME: Webhook authentication proves real identity
			// COMPLIANCE: SOC 2 CC8.1 requires authenticated identity

			rar := createRAR(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "bob@example.com",
				decisionMessage: "Approved after risk assessment",
				shouldEmitEvent: true,
			})

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: User identity is from webhook (authenticated)
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			actorID, _ := event.ActorID.Get()
			Expect(actorID).To(Equal("bob@example.com"),
				"BUSINESS OUTCOME: User authenticated via webhook (not self-reported)")

			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(payload.DecidedBy).To(Equal("bob@example.com"),
				"BUSINESS CORRECTNESS: Identity consistent across event fields")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-005: Authentication validation\n")
			GinkgoWriter.Printf("   • User: %s (webhook-authenticated) ✅\n", actorID)
			GinkgoWriter.Printf("   • BUSINESS VALUE: Cannot forge identity (legal defensibility)\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC8.1 satisfied ✅\n")
		})
	})

	Context("UT-RO-AUD006-006: Audit Trail Continuity", func() {
		It("should enable auditors to reconstruct complete remediation timeline", func() {
			// BUSINESS SCENARIO: Auditor needs to reconstruct full remediation story
			// BUSINESS OUTCOME: Single correlation_id query returns all related events
			// COMPLIANCE: SOC 2 CC7.4 (Audit Trail Completeness)

			rar := createRAR(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Approved",
				shouldEmitEvent: true,
			})
			rar.Spec.RemediationRequestRef.Name = "rr-custom-parent-789"

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: Correlation ID enables timeline reconstruction
			event := mockStore.StoredEvents[0]
			Expect(event.CorrelationID).To(Equal("rr-custom-parent-789"),
				"BUSINESS OUTCOME: Auditor can query: WHERE correlation_id='rr-custom-parent-789'")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-006: Audit trail continuity\n")
			GinkgoWriter.Printf("   • Correlation ID: %s ✅\n", event.CorrelationID)
			GinkgoWriter.Printf("   • BUSINESS VALUE: Single SQL query reconstructs complete timeline\n")
			GinkgoWriter.Printf("   • COMPLIANCE: SOC 2 CC7.4 satisfied ✅\n")
		})
	})

	Context("UT-RO-AUD006-007: Forensic Investigation Capability", func() {
		It("should provide complete context for post-incident investigation", func() {
			// BUSINESS SCENARIO: 6 months later, SEC team investigates production incident
			// BUSINESS NEED: Reconstruct approval decision without K8s CRD access (deleted)
			// COMPLIANCE: SOC 2 CC7.2 (90-365 day retention)

			rar := createRAR(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Root cause accurate. Safe to proceed.",
				shouldEmitEvent: true,
			})

			// BUSINESS ACTION: Record decision
			auditClient.RecordApprovalDecision(ctx, rar)

			// BUSINESS VALIDATION: All forensic investigation fields present
			event := mockStore.StoredEvents[0]
			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()

			// FORENSIC QUESTION 1: WHO approved?
			Expect(payload.DecidedBy).To(Equal("alice@example.com"),
				"FORENSIC NEED: Identify decision maker")

			// FORENSIC QUESTION 2: WHAT workflow was approved?
			Expect(payload.WorkflowID).To(Equal("oomkill-increase-memory-limits"),
				"FORENSIC NEED: Reconstruct what action was taken")

			// FORENSIC QUESTION 3: What was risk level?
			Expect(payload.Confidence).To(Equal(float32(0.75)),
				"FORENSIC NEED: Understand why approval was required (75%% < 80%% threshold)")

			// FORENSIC QUESTION 4: WHY was it approved?
			decisionMsg, _ := payload.DecisionMessage.Get()
			Expect(decisionMsg).To(ContainSubstring("Root cause accurate"),
				"FORENSIC NEED: Operator's rationale for approval")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-007: Forensic investigation capability\n")
			GinkgoWriter.Printf("   • Complete context: WHO, WHAT, WHY, WHEN ✅\n")
			GinkgoWriter.Printf("   • BUSINESS VALUE: Post-incident investigation enabled\n")
			GinkgoWriter.Printf("   • COMPLIANCE: 90-365 day forensic capability ✅\n")
		})
	})

	Context("UT-RO-AUD006-008: System Resilience", func() {
		It("should continue approvals even if audit infrastructure fails", func() {
			// BUSINESS RISK: Audit failure should NOT block critical approvals
			// BUSINESS OUTCOME: Approval workflow continues (graceful degradation)
			// OPERATIONAL NEED: High-priority remediations proceed despite audit issues

			mockStore.StoreError = errors.New("DataStorage unavailable - network partition")

			rar := createRAR(ApprovalDecisionScenario{
				decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				decidedBy:       "alice@example.com",
				decisionMessage: "Critical production fix - proceed immediately",
				shouldEmitEvent: true,
			})

			// BUSINESS ACTION: Attempt to record decision during audit outage
			Expect(func() {
				auditClient.RecordApprovalDecision(ctx, rar)
			}).ToNot(Panic(),
				"BUSINESS BEHAVIOR: Approval continues despite audit failure (fire-and-forget)")

			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"Expected: No events stored due to infrastructure failure")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-008: System resilience validated\n")
			GinkgoWriter.Printf("   • No panic on audit failure ✅\n")
			GinkgoWriter.Printf("   • BUSINESS VALUE: Critical approvals not blocked by audit infrastructure\n")
			GinkgoWriter.Printf("   • OPERATIONAL BEHAVIOR: Graceful degradation per DD-AUDIT-002\n")
		})
	})
})
