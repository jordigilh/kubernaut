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

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AuditManager is the interface for recording audit events
// This interface allows for testing without requiring a full audit store
type AuditManager interface {
	RecordEvent(ctx context.Context, event audit.AuditEvent) error
}

// NotificationRequestDeleteHandler handles authentication for NotificationRequest cancellation via DELETE
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// DD-NOT-005: Immutable Spec - Cancellation via DELETE Operation
//
// This webhook intercepts NotificationRequest DELETE operations and writes audit attribution:
// - Extracts authenticated user from admission request
// - Writes deletion audit event to database
// - Allows DELETE to proceed
//
// Note: Kubernetes API prevents mutating objects during DELETE, so attribution is captured
// via audit trail rather than CRD annotations.
type NotificationRequestDeleteHandler struct {
	authenticator *authwebhook.Authenticator
	auditManager  AuditManager
	decoder       admission.Decoder
}

// NewNotificationRequestDeleteHandler creates a new NotificationRequest DELETE authentication handler
func NewNotificationRequestDeleteHandler(auditManager AuditManager) *NotificationRequestDeleteHandler {
	return &NotificationRequestDeleteHandler{
		authenticator: authwebhook.NewAuthenticator(),
		auditManager:  auditManager,
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

	// Initialize annotations map if it doesn't exist
	if nr.Annotations == nil {
		nr.Annotations = make(map[string]string)
	}

	// Note: Kubernetes API server does NOT allow mutating objects during DELETE operations.
	// However, we CAN write audit traces to capture attribution for SOC2 compliance.
	
	// Write deletion audit event
	eventData := map[string]interface{}{
		"notification_id": nr.Name,
		"namespace":       nr.Namespace,
		"type":            string(nr.Spec.Type),
		"priority":        string(nr.Spec.Priority),
		"cancelled_by":    authCtx.Username,
		"user_uid":        authCtx.UID,
		"user_groups":     authCtx.Groups,
	}

	eventDataBytes, err := json.Marshal(eventData)
	if err != nil {
		// If marshaling fails, allow DELETE but log the issue
		return admission.Allowed(fmt.Sprintf("DELETE allowed with audit warning: failed to marshal event data: %v", err))
	}

	err = h.auditManager.RecordEvent(ctx, audit.AuditEvent{
		EventCategory:  "notification",
		EventType:      "notification.request.deleted",
		EventOutcome:   "success",
		ActorID:        authCtx.Username,
		ResourceType:   "NotificationRequest",
		ResourceID:     fmt.Sprintf("%s/%s", nr.Namespace, nr.Name),
		CorrelationID:  nr.Name,
		EventData:      eventDataBytes,
	})

	if err != nil {
		// Log error but allow DELETE to proceed (audit failure shouldn't block operations)
		// In production, this would trigger an alert for audit integrity monitoring
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

