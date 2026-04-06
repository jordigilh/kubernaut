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
	"fmt"
	"sort"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NotificationRequestValidator validates NotificationRequest CRDs
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// DD-NOT-005: Immutable Spec - Cancellation via DELETE Operation
// DD-WEBHOOK-003: Webhook-Complete Audit Pattern
//
// This validator implements webhook.CustomValidator interface for Kubebuilder-style webhooks.
// It intercepts NotificationRequest DELETE operations and:
// 1. Extracts authenticated user from admission request context
// 2. Writes complete deletion audit event (WHO + WHAT + ACTION)
// 3. Allows DELETE to proceed (returns nil error)
//
// Reference: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation
// Reference: https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/
type NotificationRequestValidator struct {
	authenticator *Authenticator
	auditStore    audit.AuditStore
}

// Ensure NotificationRequestValidator implements webhook.CustomValidator
var _ webhook.CustomValidator = &NotificationRequestValidator{}

// NewNotificationRequestValidator creates a new NotificationRequest validator
func NewNotificationRequestValidator(auditStore audit.AuditStore) *NotificationRequestValidator {
	return &NotificationRequestValidator{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
	}
}

// ValidateCreate implements webhook.CustomValidator
// NotificationRequest doesn't require validation on CREATE
func (v *NotificationRequestValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for CREATE operations
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator
// NotificationRequest doesn't require validation on UPDATE
func (v *NotificationRequestValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// No validation needed for UPDATE operations
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator
// Captures operator attribution for DELETE operations via audit trail
//
// This method is invoked by envtest/K8s API server for DELETE admission requests.
// Per Kubebuilder pattern, returning nil allows the DELETE to proceed.
//
// Reference: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation
func (v *NotificationRequestValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nr, ok := obj.(*notificationv1.NotificationRequest)
	if !ok {
		return nil, fmt.Errorf("expected NotificationRequest but got %T", obj)
	}


	// Extract authenticated user from admission request context
	// Note: admission.RequestFromContext requires the request to be injected by controller-runtime
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		// Allow DELETE to proceed even if we can't capture attribution
		// (audit failure should not block business operations)
		return admission.Warnings{"audit attribution unavailable"}, nil
	}

	authCtx, err := v.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		// Allow DELETE to proceed even if authentication fails
		// (audit failure should not block business operations)
		return admission.Warnings{"authentication unavailable"}, nil
	}

	// Write complete deletion audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, EventTypeNotifCancelled) // DD-WEBHOOK-001 line 349 - Must match payload EventType
	audit.SetEventCategory(auditEvent, EventCategoryNotification)
	audit.SetEventAction(auditEvent, "deleted")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "NotificationRequest", string(nr.UID))
	audit.SetCorrelationID(auditEvent, nr.Name) // Use NR name for correlation
	audit.SetNamespace(auditEvent, nr.Namespace)

	// Set event data payload
	// Per DD-WEBHOOK-003 lines 335-340: Business context ONLY (attribution in structured columns)
	// Use structured audit payload (eliminates map[string]interface{})
	// Per DD-AUDIT-004: Zero unstructured data in audit events
	payload := api.NotificationAuditPayload{
		EventType: api.NotificationAuditPayloadEventTypeWebhookNotificationCancelled,
	}
	// Business context fields (per DD-WEBHOOK-003)
	payload.NotificationName.SetTo(nr.Name)
	payload.NotificationType.SetTo(toNotificationAuditPayloadNotificationType(string(nr.Spec.Type)))
	payload.Priority.SetTo(toNotificationAuditPayloadPriority(string(nr.Spec.Priority)))
	payload.FinalStatus.SetTo(toNotificationAuditPayloadFinalStatus(string(nr.Status.Phase)))
	// Delivery channels from status.deliveryAttempts (fixes #276: spec.recipients was always null)
	channels := ExtractDeliveryChannels(nr.Status.DeliveryAttempts)
	if channels != nil {
		payload.DeliveryChannels = channels
	} else {
		payload.DeliveryChannels = []string{}
	}

	// Note: Attribution fields (WHO, WHAT, WHERE, HOW) are in structured columns:
	// - actor_id: authCtx.Username (via audit.SetActor)
	// - resource_name: nr.Name (via audit.SetResource)
	// - namespace: nr.Namespace (via audit.SetNamespace)
	// - event_action: "deleted" (via audit.SetEventAction)
	auditEvent.EventData = api.NewAuditEventRequestEventDataWebhookNotificationCancelledAuditEventRequestEventData(payload)

	// Store audit event asynchronously (buffered write)
	if err := v.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		// Log error but don't fail the webhook (audit should not block operations)
		// The audit store has retry + DLQ mechanisms
		return admission.Warnings{fmt.Sprintf("audit storage failed: %v", err)}, nil
	}

	// Allow DELETE to proceed (return nil error)
	return nil, nil
}

// ExtractDeliveryChannels returns a sorted, deduplicated list of channel names
// from the given delivery attempts. Returns nil if attempts is nil or empty.
func ExtractDeliveryChannels(attempts []notificationv1.DeliveryAttempt) []string {
	if len(attempts) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	for _, da := range attempts {
		if da.Channel != "" {
			seen[string(da.Channel)] = true
		}
	}
	channels := make([]string, 0, len(seen))
	for ch := range seen {
		channels = append(channels, ch)
	}
	sort.Strings(channels)
	return channels
}

