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
	"encoding/json"
	"fmt"
	"net/http"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NotificationRequestDeleteHandler handles authentication for NotificationRequest cancellation via DELETE
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// DD-NOT-005: Immutable Spec - Cancellation via DELETE Operation
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This webhook intercepts NotificationRequest DELETE operations and:
// 1. Extracts authenticated user from admission request
// 2. Writes complete deletion audit event (WHO + WHAT + ACTION)
// 3. Allows DELETE to proceed
//
// Note: Kubernetes API prevents mutating objects during DELETE, so attribution is captured
// via audit trail rather than CRD annotations/status.
type NotificationRequestDeleteHandler struct {
	authenticator *Authenticator
	auditStore    audit.AuditStore
	decoder       admission.Decoder
}

// NewNotificationRequestDeleteHandler creates a new NotificationRequest DELETE authentication handler
func NewNotificationRequestDeleteHandler(auditStore audit.AuditStore) *NotificationRequestDeleteHandler {
	return &NotificationRequestDeleteHandler{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
	}
}

// Handle processes the admission request for NotificationRequest DELETE
// Implements admission.Handler interface from controller-runtime
func (h *NotificationRequestDeleteHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	// Only handle DELETE operations
	if req.Operation != admissionv1.Delete {
		return admission.Allowed("not a DELETE operation")
	}

	nr := &notificationv1.NotificationRequest{}

	// For DELETE operations, the object to delete is in OldObject
	err := json.Unmarshal(req.OldObject.Raw, nr)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode NotificationRequest: %w", err))
	}

	// Extract authenticated user from admission request
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// Note: Kubernetes API server does NOT allow mutating objects during DELETE operations.
	// However, we CAN write audit traces to capture attribution for SOC2 compliance.


	// Write complete deletion audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, "webhook.notification.cancelled") // DD-WEBHOOK-001 line 349 - Must match payload EventType
	audit.SetEventCategory(auditEvent, "webhook") // Per ADR-034 v1.4: event_category = emitter service
	audit.SetEventAction(auditEvent, "deleted")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "NotificationRequest", string(nr.UID))
	audit.SetCorrelationID(auditEvent, nr.Name) // Use NR name for correlation
	audit.SetNamespace(auditEvent, nr.Namespace)

	// Set event data payload
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.NotificationAuditPayload{
		EventType: "webhook.notification.cancelled",
	}
	payload.NotificationID.SetTo(nr.Name)
	payload.Type.SetTo(toNotificationAuditPayloadType(string(nr.Spec.Type)))
	payload.Priority.SetTo(toNotificationAuditPayloadPriority(string(nr.Spec.Priority)))
	payload.CancelledBy.SetTo(authCtx.Username)
	payload.UserUID.SetTo(authCtx.UID)
	payload.UserGroups = authCtx.Groups // array, not optional
	payload.Action.SetTo(api.NotificationAuditPayloadActionNotificationCancelled)

	auditEvent.EventData = api.NewAuditEventRequestEventDataWebhookNotificationCancelledAuditEventRequestEventData(payload)

	// Store audit event asynchronously (buffered write)
	if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		// Log error but don't fail the webhook (audit should not block operations)
		// The audit store has retry + DLQ mechanisms
		return admission.Allowed(fmt.Sprintf("DELETE allowed with audit warning: %v", err))
	}

	// Allow DELETE to proceed
	return admission.Allowed(fmt.Sprintf("DELETE allowed, attribution recorded (user: %s)", authCtx.Username))
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *NotificationRequestDeleteHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}

