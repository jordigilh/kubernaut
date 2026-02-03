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

package audit_test

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
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func TestRemediationApprovalRequestAudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationApprovalRequest Audit Suite")
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

var _ = Describe("RemediationApprovalRequest Audit", func() {
	var (
		ctx         context.Context
		auditClient *audit.AuditClient
		mockStore   *MockAuditStore
		rar         *remediationapprovalrequestv1alpha1.RemediationApprovalRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		logger := logr.Discard()
		auditClient = audit.NewAuditClient(mockStore, logger)

		// Create test RAR
		now := metav1.Now()
		rar = &remediationapprovalrequestv1alpha1.RemediationApprovalRequest{
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
				Confidence: 0.75,
				ConfidenceLevel: "medium",
				Reason: "Confidence below 80% auto-approve threshold",
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
				Decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				DecidedBy:       "alice@example.com",
				DecidedAt:       &now,
				DecisionMessage: "Root cause accurate. Safe to proceed.",
			},
		}
	})

	// ========================================
	// TDD RED Phase: Cycle 1 - Basic Event Emission
	// ========================================

	Context("UT-RO-AUD006-001: Approval decision audit event", func() {
		It("should emit approval.decision event for approved decision", func() {
			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: Audit event emitted with correct data
			Expect(mockStore.StoredEvents).To(HaveLen(1),
				"Should emit exactly 1 audit event")

			event := mockStore.StoredEvents[0]

			// Verify event metadata
			Expect(event.EventType).To(Equal("approval.decision"),
				"Event type should be approval.decision")
			Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApproval),
				"Event category should be approval")
			Expect(event.EventAction).To(Equal("decision_made"),
				"Event action should be decision_made")
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess),
				"Event outcome should be success for approved")

			// Verify actor (SOC 2 CC8.1 - User Attribution)
			actorType, hasActorType := event.ActorType.Get()
			Expect(hasActorType).To(BeTrue(), "Actor type should be set")
			Expect(actorType).To(Equal("user"),
				"Actor type should be user")
			
			actorID, hasActorID := event.ActorID.Get()
			Expect(hasActorID).To(BeTrue(), "Actor ID should be set")
			Expect(actorID).To(Equal("alice@example.com"),
				"Actor ID should match authenticated user")

			// Verify correlation ID (DD-AUDIT-CORRELATION-002)
			Expect(event.CorrelationID).To(Equal("rr-parent-123"),
				"Correlation ID should match parent RR name")

			// Verify resource
			resourceType, hasRT := event.ResourceType.Get()
			Expect(hasRT).To(BeTrue(), "Resource type should be set")
			Expect(resourceType).To(Equal("RemediationApprovalRequest"),
				"Resource type should be RemediationApprovalRequest")
			
			resourceID, hasRID := event.ResourceID.Get()
			Expect(hasRID).To(BeTrue(), "Resource ID should be set")
			Expect(resourceID).To(Equal("rar-test-001"),
				"Resource ID should match RAR name")
			
			namespace, hasNS := event.Namespace.Get()
			Expect(hasNS).To(BeTrue(), "Namespace should be set")
			Expect(namespace).To(Equal("production"),
				"Namespace should match RAR namespace")

			// Verify payload structure
			Expect(event.EventData.IsRemediationApprovalDecisionPayload()).To(BeTrue(),
				"EventData should be RemediationApprovalDecisionPayload type")

			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue(),
				"Should be able to extract RemediationApprovalDecisionPayload")

			// Verify payload fields (note: ogen uses camelCase for snake_case fields)
			Expect(payload.RemediationRequestName).To(Equal("rr-parent-123"),
				"Payload should have correct RR name")
			Expect(payload.AiAnalysisName).To(Equal("ai-test-456"),
				"Payload should have correct AI analysis name")
			Expect(payload.Decision).To(Equal(ogenclient.RemediationApprovalDecisionPayloadDecisionApproved),
				"Payload should have correct decision")
			Expect(payload.DecidedBy).To(Equal("alice@example.com"),
				"Payload should have authenticated user")
			Expect(payload.Confidence).To(Equal(float32(0.75)),
				"Payload should have correct confidence")
			Expect(payload.WorkflowID).To(Equal("oomkill-increase-memory-limits"),
				"Payload should have correct workflow ID")

			// Verify optional fields
			decisionMsg, hasDM := payload.DecisionMessage.Get()
			Expect(hasDM).To(BeTrue(), "Decision message should be set")
			Expect(decisionMsg).To(Equal("Root cause accurate. Safe to proceed."),
				"Decision message should match")

			workflowVer, hasWV := payload.WorkflowVersion.Get()
			Expect(hasWV).To(BeTrue(), "Workflow version should be set")
			Expect(workflowVer).To(Equal("v1.2.0"),
				"Workflow version should match")

			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			GinkgoWriter.Printf("✅ UT-RO-AUD006-001: Approval decision audit event\n")
			GinkgoWriter.Printf("   • Event Type: %s\n", event.EventType)
			GinkgoWriter.Printf("   • Decision: %s\n", payload.Decision)
			GinkgoWriter.Printf("   • Decided By: %s\n", payload.DecidedBy)
			GinkgoWriter.Printf("   • Workflow: %s\n", payload.WorkflowID)
			GinkgoWriter.Printf("   • Correlation ID: %s\n", event.CorrelationID)
			GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		})
	})

	Context("UT-RO-AUD006-002: Rejection decision audit event", func() {
		It("should emit approval.decision event for rejected decision", func() {
			// Given: RAR with rejected decision
			rar.Status.Decision = remediationapprovalrequestv1alpha1.ApprovalDecisionRejected
			rar.Status.DecisionMessage = "Risk too high for production"

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: Audit event emitted with outcome=failure
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			Expect(event.EventType).To(Equal("approval.decision"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure),
				"Event outcome should be failure for rejected")

			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Decision).To(Equal(ogenclient.RemediationApprovalDecisionPayloadDecisionRejected))

			decisionMsg, _ := payload.DecisionMessage.Get()
			Expect(decisionMsg).To(Equal("Risk too high for production"))

			GinkgoWriter.Printf("✅ UT-RO-AUD006-002: Rejection decision audit event\n")
		})
	})

	Context("UT-RO-AUD006-004: Idempotency check", func() {
		It("should NOT emit event if decision is empty", func() {
			// Given: RAR without decision
			rar.Status.Decision = ""

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: NO audit event emitted (idempotency)
			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"Should not emit event when decision is empty")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-004: Idempotency check (no event when decision empty)\n")
		})
	})

	// ========================================
	// TDD RED Phase: Cycle 2 - Additional Scenarios
	// ========================================

	Context("UT-RO-AUD006-003: Expired decision audit event", func() {
		It("should emit approval.decision event for expired decision", func() {
			// Given: RAR with expired decision
			rar.Status.Decision = remediationapprovalrequestv1alpha1.ApprovalDecisionExpired
			rar.Status.DecidedBy = "system"
			rar.Status.DecisionMessage = "Approval timeout - no operator response"

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: Audit event emitted with outcome=failure
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			Expect(event.EventType).To(Equal("approval.decision"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure),
				"Event outcome should be failure for expired")

			// Verify actor is system (not user)
			actorType, _ := event.ActorType.Get()
			Expect(actorType).To(Equal("user"), "Actor type should still be user")
			actorID, _ := event.ActorID.Get()
			Expect(actorID).To(Equal("system"), "Actor ID should be system for timeout")

			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Decision).To(Equal(ogenclient.RemediationApprovalDecisionPayloadDecisionExpired))

			GinkgoWriter.Printf("✅ UT-RO-AUD006-003: Expired decision audit event\n")
		})
	})

	Context("UT-RO-AUD006-005: Authenticated user captured correctly", func() {
		It("should capture authenticated user from webhook", func() {
			// Given: RAR with different authenticated user
			rar.Status.DecidedBy = "bob@example.com"

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: Authenticated user captured in actor_id
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			actorID, hasActorID := event.ActorID.Get()
			Expect(hasActorID).To(BeTrue())
			Expect(actorID).To(Equal("bob@example.com"),
				"Should capture authenticated user from webhook")

			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(payload.DecidedBy).To(Equal("bob@example.com"),
				"Payload should also have authenticated user")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-005: Authenticated user captured correctly\n")
		})
	})

	Context("UT-RO-AUD006-006: Correlation ID matches parent RR", func() {
		It("should use parent RR name as correlation ID", func() {
			// Given: RAR with specific parent RR
			rar.Spec.RemediationRequestRef.Name = "rr-custom-parent-789"

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: Correlation ID matches parent RR
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			Expect(event.CorrelationID).To(Equal("rr-custom-parent-789"),
				"Correlation ID should match parent RemediationRequest name")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-006: Correlation ID matches parent RR\n")
		})
	})

	Context("UT-RO-AUD006-007: Complete approval context included", func() {
		It("should include all approval context fields in payload", func() {
			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: All required context fields present
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue())

			// Verify all required fields
			Expect(payload.RemediationRequestName).ToNot(BeEmpty(),
				"Should have RR name")
			Expect(payload.AiAnalysisName).ToNot(BeEmpty(),
				"Should have AI analysis name")
			Expect(payload.Decision).ToNot(BeEmpty(),
				"Should have decision")
			Expect(payload.DecidedBy).ToNot(BeEmpty(),
				"Should have decided_by")
			Expect(payload.Confidence).To(BeNumerically(">", 0),
				"Should have confidence score")
			Expect(payload.WorkflowID).ToNot(BeEmpty(),
				"Should have workflow ID")

			// Verify optional fields are set
			_, hasDecisionMsg := payload.DecisionMessage.Get()
			Expect(hasDecisionMsg).To(BeTrue(), "Should have decision message")

			_, hasWorkflowVer := payload.WorkflowVersion.Get()
			Expect(hasWorkflowVer).To(BeTrue(), "Should have workflow version")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-007: Complete approval context included\n")
		})
	})

	Context("UT-RO-AUD006-008: Graceful degradation", func() {
		It("should not fail on audit store error (fire-and-forget)", func() {
			// Given: Mock store returns error
			mockStore.StoreError = errors.New("audit store unavailable")

			// When: RecordApprovalDecision is called
			// Then: No panic, graceful degradation
			Expect(func() {
				auditClient.RecordApprovalDecision(ctx, rar)
			}).ToNot(Panic(), "Should not panic on audit store error")

			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"No events should be stored due to error")

			GinkgoWriter.Printf("✅ UT-RO-AUD006-008: Fire-and-forget (no panic on audit error)\n")
		})
	})
})
