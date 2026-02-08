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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// F-3 SOC2 Fix: WorkflowExecution Block Cleared Audit Event Tests
// BR-WE-013: Audit-Tracked Execution Block Clearing
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// Validates that the WorkflowExecution webhook emits correct audit events
// with matching event_type and EventData discriminator after F-3 fix.

// buildBlockClearanceRequest creates an admission request for block clearance testing.
func buildBlockClearanceRequest(
	wfeName, namespace, uid, clearReason, username, userUID string,
) admission.Request {
	wfe := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wfeName,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name: "rr-parent-" + wfeName,
			},
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{
			BlockClearance: &workflowexecutionv1.BlockClearanceDetails{
				ClearReason: clearReason,
			},
		},
	}

	wfeJSON, _ := json.Marshal(wfe)

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID: types.UID("admission-" + wfeName),
			Kind: metav1.GroupVersionKind{
				Group:   "workflowexecution.kubernaut.ai",
				Version: "v1alpha1",
				Kind:    "WorkflowExecution",
			},
			Name:      wfeName,
			Namespace: namespace,
			Operation: admissionv1.Update,
			UserInfo: authv1.UserInfo{
				Username: username,
				UID:      userUID,
			},
			Object: runtime.RawExtension{
				Raw: wfeJSON,
			},
		},
	}
}

// buildBlockClearanceRequestWithOldObject creates an admission request with both NEW and OLD objects.
// SOC2 Round 2 H-2: Required for identity forgery prevention testing.
func buildBlockClearanceRequestWithOldObject(
	wfeName, namespace, uid, clearReason, username, userUID string,
	oldClearedBy string,
) admission.Request {
	req := buildBlockClearanceRequest(wfeName, namespace, uid, clearReason, username, userUID)

	// Build OLD object with specified ClearedBy
	oldWFE := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wfeName,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name: "rr-parent-" + wfeName,
			},
		},
		Status: workflowexecutionv1.WorkflowExecutionStatus{
			BlockClearance: &workflowexecutionv1.BlockClearanceDetails{
				ClearReason: clearReason,
				ClearedBy:   oldClearedBy,
			},
		},
	}

	oldJSON, _ := json.Marshal(oldWFE)
	req.OldObject = runtime.RawExtension{Raw: oldJSON}

	return req
}

var _ = Describe("BR-WE-013: WorkflowExecution Block Cleared Audit Trail", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.WorkflowExecutionAuthHandler
		mockStore *MockAuditStore
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		handler = authwebhook.NewWorkflowExecutionAuthHandler(mockStore)

		scheme = runtime.NewScheme()
		_ = workflowexecutionv1.AddToScheme(scheme)
		decoder := admission.NewDecoder(scheme)
		_ = handler.InjectDecoder(decoder)
	})

	Describe("Audit Event Field Validation", func() {
		DescribeTable("should emit correct audit event fields",
			func(fieldName string, validate func(event *ogenclient.AuditEventRequest)) {
				admReq := buildBlockClearanceRequest(
					"wfe-field-test", "production", "wfe-uid-001",
					"manual investigation complete and cluster state verified to be healthy after reviewing logs",
					"sre@kubernaut.ai", "k8s-user-456",
				)

				resp := handler.Handle(ctx, admReq)

				Expect(resp.Allowed).To(BeTrue(), "Authenticated operator can clear blocks")
				Expect(resp.Patches).NotTo(BeEmpty(), "ClearedBy field must be populated")
				Expect(mockStore.StoredEvents).To(HaveLen(1), "Webhook audit event emitted")

				validate(mockStore.StoredEvents[0])
			},
			Entry("event_type = workflowexecution.block.cleared",
				"event_type",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventType).To(Equal(authwebhook.EventTypeBlockCleared))
				},
			),
			Entry("event_category = webhook (ADR-034 v1.7)",
				"event_category",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryWebhook))
				},
			),
			Entry("event_action = block_cleared",
				"event_action",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventAction).To(Equal("block_cleared"))
				},
			),
			Entry("event_outcome = success",
				"event_outcome",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
				},
			),
			Entry("actor_id captures operator identity (SOC2 CC8.1)",
				"actor_id",
				func(event *ogenclient.AuditEventRequest) {
					actorID, hasActorID := event.ActorID.Get()
					Expect(hasActorID).To(BeTrue(), "Actor ID must be set for SOC2 CC8.1")
					Expect(actorID).To(Equal("sre@kubernaut.ai"))
				},
			),
			Entry("namespace captures WHERE",
				"namespace",
				func(event *ogenclient.AuditEventRequest) {
					namespace, hasNamespace := event.Namespace.Get()
					Expect(hasNamespace).To(BeTrue())
					Expect(namespace).To(Equal("production"))
				},
			),
			Entry("resource_id captures WFE UID",
				"resource_id",
				func(event *ogenclient.AuditEventRequest) {
					resourceID, hasResourceID := event.ResourceID.Get()
					Expect(hasResourceID).To(BeTrue())
					Expect(resourceID).To(Equal("wfe-uid-001"))
				},
			),
			Entry("F-3 consistency: outer event_type matches EventData discriminator",
				"consistency",
				func(event *ogenclient.AuditEventRequest) {
					Expect(event.EventType).To(Equal(string(event.EventData.Type)),
						"F-3 SOC2 Fix: outer event_type must match EventData discriminator")
				},
			),
		)
	})

	Describe("Identity Forgery Prevention via OLD Object Comparison", func() {
		It("should skip webhook when OLD object already has ClearedBy set", func() {
			// True idempotency: OLD object has ClearedBy, NEW object has same ClearedBy
			admReq := buildBlockClearanceRequestWithOldObject(
				"wfe-idempotent", "production", "wfe-uid-010",
				"manual investigation complete and cluster state verified to be healthy after reviewing logs",
				"sre@kubernaut.ai", "k8s-user-456",
				"original-operator@kubernaut.ai", // OLD already has ClearedBy
			)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(), "Idempotent resubmission should be allowed")
			Expect(resp.Patches).To(BeEmpty(), "No patches for idempotent request")
			Expect(mockStore.StoredEvents).To(BeEmpty(), "No audit event for idempotent request")
		})

		It("should reject forged ClearedBy when OLD object has different ClearedBy", func() {
			// Forgery attempt: OLD has ClearedBy="original", NEW has ClearedBy="attacker"
			// The webhook should preserve OLD object's attribution (no patches, no audit)
			admReq := buildBlockClearanceRequestWithOldObject(
				"wfe-forgery", "production", "wfe-uid-011",
				"manual investigation complete and cluster state verified to be healthy after reviewing logs",
				"attacker@evil.com", "k8s-attacker-999",
				"original-operator@kubernaut.ai", // OLD already has ClearedBy
			)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(), "Should allow (OLD wins)")
			Expect(resp.Patches).To(BeEmpty(), "No patches - OLD object attribution preserved")
			Expect(mockStore.StoredEvents).To(BeEmpty(), "No audit event - not a new clearance")
		})

		It("should process normally when OLD object has no ClearedBy", func() {
			// Genuine new clearance: OLD has no ClearedBy
			admReq := buildBlockClearanceRequestWithOldObject(
				"wfe-new-clearance", "production", "wfe-uid-012",
				"manual investigation complete and cluster state verified to be healthy after reviewing logs",
				"sre@kubernaut.ai", "k8s-user-456",
				"", // OLD has no ClearedBy (genuinely new clearance)
			)

			resp := handler.Handle(ctx, admReq)

			Expect(resp.Allowed).To(BeTrue(), "New clearance should be allowed")
			Expect(resp.Patches).NotTo(BeEmpty(), "Patches should be applied for new clearance")
			Expect(mockStore.StoredEvents).To(HaveLen(1), "Audit event should be emitted for new clearance")
		})
	})

	Describe("Structured Payload Validation", func() {
		It("should capture workflow name, previous/new state in payload", func() {
			admReq := buildBlockClearanceRequest(
				"wfe-payload-test", "production", "wfe-uid-004",
				"verified safe to proceed after complete review of all system components and dependencies were checked thoroughly",
				"sre@kubernaut.ai", "k8s-user-456",
			)

			resp := handler.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			event := mockStore.StoredEvents[0]

			payload, ok := event.EventData.GetWorkflowExecutionWebhookAuditPayload()
			Expect(ok).To(BeTrue(), "EventData must contain WorkflowExecutionWebhookAuditPayload")

			Expect(payload.WorkflowName).To(Equal("wfe-payload-test"))
			Expect(payload.PreviousState).To(Equal(ogenclient.WorkflowExecutionWebhookAuditPayloadPreviousStateBlocked))
			Expect(payload.NewState).To(Equal(ogenclient.WorkflowExecutionWebhookAuditPayloadNewStateRunning))
			Expect(string(payload.EventType)).To(Equal(authwebhook.EventTypeBlockCleared))
		})
	})
})
