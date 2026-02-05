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

package authwebhook

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// TDD Phase: AuthWebhook RAR Audit Emission Tests
// BR-AUDIT-006: RemediationApprovalRequest Audit Trail (Webhook Component)
// ADR-034 v1.7: Two-Event Audit Trail Pattern
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// Per TESTING_GUIDELINES.md: Unit tests validate business behavior + implementation correctness
// Focus: User attribution (SOC 2 CC8.1) and webhook audit event emission (SOC 2 CC7.2)
//
// Scope: AuthWebhook's responsibility in RAR audit trail:
//   1. Extract authenticated user from admission request
//   2. Populate status.DecidedBy field (user attribution)
//   3. Emit webhook audit event (event_category = "webhook")
//   4. Prevent identity forgery (idempotency on DecidedBy)

// MockAuditStore implements audit.AuditStore for testing webhook audit emission
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

var _ = Describe("BR-AUDIT-006: RemediationApprovalRequest Webhook Audit Trail", func() {
	var (
		ctx           context.Context
		handler       *authwebhook.RemediationApprovalRequestAuthHandler
		mockStore     *MockAuditStore
		decoder       admission.Decoder
		scheme        *runtime.Scheme
		testUserID    string
		testUserEmail string
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore)

		// Setup decoder (required by admission.Handler)
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		decoder = admission.NewDecoder(scheme)
		_ = handler.InjectDecoder(decoder)

		testUserID = "k8s-user-123"
		testUserEmail = "operator@kubernaut.ai"
	})

	// ========================================
	// BUSINESS OUTCOME: User Attribution (SOC 2 CC8.1)
	// ========================================

	Describe("User Attribution - DecidedBy Population", func() {
		DescribeTable("Approval Decision Scenarios",
			func(decision remediationv1.ApprovalDecision) {
				// BUSINESS CONTEXT: Operator makes approval decision via kubectl
				rar := &remediationv1.RemediationApprovalRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rar-test-001",
						Namespace: "production",
						UID:       "rar-uid-123",
					},
					Spec: remediationv1.RemediationApprovalRequestSpec{
						RemediationRequestRef: corev1.ObjectReference{
							Name: "rr-parent-456",
						},
						AIAnalysisRef: remediationv1.ObjectRef{
							Name: "ai-analysis-789",
						},
					},
					Status: remediationv1.RemediationApprovalRequestStatus{
						Decision:        decision,
						DecisionMessage: "Operator decision via kubectl",
					},
				}

				rarJSON, _ := json.Marshal(rar)

				admReq := admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						UID: "admission-req-001",
						Kind: metav1.GroupVersionKind{
							Group:   "remediation.kubernaut.ai",
							Version: "v1alpha1",
							Kind:    "RemediationApprovalRequest",
						},
						Name:      rar.Name,
						Namespace: rar.Namespace,
						Operation: admissionv1.Update,
						UserInfo: authv1.UserInfo{
							Username: testUserEmail,
							UID:      testUserID,
						},
						Object: runtime.RawExtension{
							Raw: rarJSON,
						},
					},
				}

				// BUSINESS ACTION: Webhook intercepts RAR status update
				resp := handler.Handle(ctx, admReq)

				// BUSINESS VALIDATION: Operator identity captured for audit trail
				Expect(resp.Allowed).To(BeTrue(), "Authenticated operator can make decisions")
				Expect(resp.Patches).NotTo(BeEmpty(), "DecidedBy field must be populated (webhook mutates RAR)")

				// BUSINESS VALIDATION: Webhook audit event emitted
				Expect(mockStore.StoredEvents).To(HaveLen(1),
					"BUSINESS VALUE: Webhook audit event emitted for compliance (SOC 2 CC7.2)")

				event := mockStore.StoredEvents[0]

				// Validate audit event structure (ADR-034 v1.7 Section 1.1.1)
				// Two-Event Pattern: webhook.remediationapprovalrequest.decided (this event)
				Expect(event.EventType).To(Equal("webhook.remediationapprovalrequest.decided"),
					"Event type per ADR-034 v1.7 webhook namespace")
				Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryWebhook),
					"CRITICAL: event_category = 'webhook' (per ADR-034 v1.7 two-event pattern)")
				Expect(event.EventAction).To(Equal("approval_decided"),
					"Event action describes the operation")
				Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess),
					"Successful attribution")

				// Validate actor (WHO)
				actorID, hasActorID := event.ActorID.Get()
				Expect(hasActorID).To(BeTrue(), "Actor ID must be set for SOC 2 CC8.1")
				Expect(actorID).To(Equal(testUserEmail),
					"BUSINESS VALUE: Audit event captures WHO (operator identity)")

				// Validate resource (WHAT)
				resourceID, hasResourceID := event.ResourceID.Get()
				Expect(hasResourceID).To(BeTrue(), "Resource ID must be set")
				Expect(resourceID).To(Equal("rar-uid-123"),
					"Audit event captures WHAT (RAR UID)")

				// Validate correlation ID (PARENT RR per DD-AUDIT-CORRELATION-002)
				Expect(event.CorrelationID).To(Equal("rr-parent-456"),
					"Audit event uses parent RR name for correlation (DD-AUDIT-CORRELATION-002)")

				// Validate namespace (WHERE)
				namespace, hasNamespace := event.Namespace.Get()
				Expect(hasNamespace).To(BeTrue(), "Namespace must be set")
				Expect(namespace).To(Equal("production"),
					"Audit event captures WHERE (namespace)")
			},
			Entry("UNIT-RAR-AUDIT-AW-001: Approved decision",
				remediationv1.ApprovalDecisionApproved),
			Entry("UNIT-RAR-AUDIT-AW-002: Rejected decision",
				remediationv1.ApprovalDecisionRejected),
			Entry("UNIT-RAR-AUDIT-AW-003: Expired decision",
				remediationv1.ApprovalDecisionExpired),
		)
	})

	// ========================================
	// BUSINESS OUTCOME: Identity Forgery Prevention
	// ========================================

	Describe("Identity Forgery Prevention (Idempotency)", func() {
		It("UNIT-RAR-AUDIT-AW-004: should preserve existing DecidedBy (prevent identity forgery)", func() {
			// BUSINESS RISK: Attacker attempts to change DecidedBy to frame another operator
			// CONTROL: Webhook preserves existing DecidedBy (immutable after first decision)
			// SECURITY: Per AUTHWEBHOOK_SECURITY_FIX_SUCCESS_FEB_03_2026.md - OLD object comparison for true idempotency

			existingDecidedBy := "original-operator@kubernaut.ai"
			attackerEmail := "attacker@malicious.com"

			// OLD object: RAR with existing decision (TRUE idempotency scenario)
			oldRAR := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-forgery-test",
					Namespace: "production",
					UID:       "rar-uid-forgery",
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name: "rr-parent-456",
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-analysis-789",
					},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionApproved,
					DecidedBy:       existingDecidedBy, // Already decided by original operator
					DecidedAt:       &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
					DecisionMessage: "Original decision",
				},
			}
			oldRARJSON, _ := json.Marshal(oldRAR)

			// NEW object: Attacker attempts to modify DecidedBy (will be ignored due to OLD object check)
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-forgery-test",
					Namespace: "production",
					UID:       "rar-uid-forgery",
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name: "rr-parent-456",
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-analysis-789",
					},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionApproved,
					DecidedBy:       existingDecidedBy, // Already decided by original operator
					DecidedAt:       &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
					DecisionMessage: "Original decision",
				},
			}

			rarJSON, _ := json.Marshal(rar)

			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "admission-req-forgery",
					Kind: metav1.GroupVersionKind{
						Group:   "remediation.kubernaut.ai",
						Version: "v1alpha1",
						Kind:    "RemediationApprovalRequest",
					},
					Name:      rar.Name,
					Namespace: rar.Namespace,
					Operation: admissionv1.Update,
					UserInfo: authv1.UserInfo{
						Username: attackerEmail, // Attacker attempts to update
						UID:      "attacker-uid-123",
					},
					Object: runtime.RawExtension{
						Raw: rarJSON,
					},
					OldObject: runtime.RawExtension{
						Raw: oldRARJSON, // CRITICAL: OLD object has existing decision (true idempotency)
					},
				},
			}

			// BUSINESS ACTION: Attacker attempts to modify already-decided RAR
			resp := handler.Handle(ctx, admReq)

			// BUSINESS VALIDATION: DecidedBy preserved (identity forgery prevented)
			Expect(resp.Allowed).To(BeTrue(), "Request allowed but no changes made")
			Expect(resp.Patches).To(BeEmpty(),
				"SECURITY CONTROL: No patches applied (DecidedBy is immutable)")

			// BUSINESS VALIDATION: No duplicate audit event emitted
			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"IDEMPOTENCY: No duplicate webhook audit event (already attributed)")
		})

		It("UNIT-RAR-AUDIT-AW-005: should not emit audit event for pending decisions", func() {
			// BUSINESS BEHAVIOR: No audit event until decision is made
			// Prevents audit pollution with incomplete/pending records

			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-pending-test",
					Namespace: "production",
					UID:       "rar-uid-pending",
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name: "rr-parent-456",
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-analysis-789",
					},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision: "", // No decision yet
				},
			}

			rarJSON, _ := json.Marshal(rar)

			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "admission-req-pending",
					Kind: metav1.GroupVersionKind{
						Group:   "remediation.kubernaut.ai",
						Version: "v1alpha1",
						Kind:    "RemediationApprovalRequest",
					},
					Name:      rar.Name,
					Namespace: rar.Namespace,
					Operation: admissionv1.Update,
					UserInfo: authv1.UserInfo{
						Username: testUserEmail,
						UID:      testUserID,
					},
					Object: runtime.RawExtension{
						Raw: rarJSON,
					},
				},
			}

			// BUSINESS ACTION: Webhook intercepts RAR update (no decision yet)
			resp := handler.Handle(ctx, admReq)

			// BUSINESS VALIDATION: Request allowed without modification
			Expect(resp.Allowed).To(BeTrue(), "Pending RAR update allowed")
			Expect(resp.Patches).To(BeEmpty(), "No patches for pending decisions")

			// BUSINESS VALIDATION: No audit event emitted (prevents pollution)
			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"BUSINESS BEHAVIOR: No audit event for pending decisions (prevents audit pollution)")
		})
	})

	// ========================================
	// BUSINESS OUTCOME: Audit Event Payload Validation
	// ========================================

	Describe("Audit Event Payload Structure (DD-AUDIT-004)", func() {
		It("UNIT-RAR-AUDIT-AW-006: should emit structured audit payload (no unstructured data)", func() {
			// BUSINESS VALUE: Structured audit data for compliance querying (SOC 2 auditors)
			// DD-AUDIT-004: Zero unstructured data in audit events

			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rar-payload-test",
					Namespace: "production",
					UID:       "rar-uid-payload",
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name: "rr-parent-456",
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "ai-analysis-789",
					},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionApproved,
					DecisionMessage: "Approved for production deployment",
				},
			}

			rarJSON, _ := json.Marshal(rar)

			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: "admission-req-payload",
					Kind: metav1.GroupVersionKind{
						Group:   "remediation.kubernaut.ai",
						Version: "v1alpha1",
						Kind:    "RemediationApprovalRequest",
					},
					Name:      rar.Name,
					Namespace: rar.Namespace,
					Operation: admissionv1.Update,
					UserInfo: authv1.UserInfo{
						Username: testUserEmail,
						UID:      testUserID,
					},
					Object: runtime.RawExtension{
						Raw: rarJSON,
					},
				},
			}

			// BUSINESS ACTION: Webhook emits audit event
			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			event := mockStore.StoredEvents[0]

			// Validate structured payload (RemediationApprovalAuditPayload)
			payload, ok := event.EventData.GetRemediationApprovalAuditPayload()
			Expect(ok).To(BeTrue(),
				"BUSINESS VALUE: Structured payload for auditor queries (DD-AUDIT-004)")

			Expect(payload.RequestName).To(Equal("rar-payload-test"))
			Expect(payload.Decision).To(Equal(ogenclient.RemediationApprovalAuditPayloadDecisionApproved))
			Expect(payload.DecisionMessage).To(Equal("Approved for production deployment"))
			Expect(payload.AiAnalysisRef).To(Equal("ai-analysis-789"))
			Expect(string(payload.EventType)).To(Equal("webhook.approval.decided"))
		})
	})
})
