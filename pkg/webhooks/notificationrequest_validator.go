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
	"fmt"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
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
	authenticator *authwebhook.Authenticator
	auditStore    audit.AuditStore
}

// Ensure NotificationRequestValidator implements webhook.CustomValidator
var _ webhook.CustomValidator = &NotificationRequestValidator{}

// NewNotificationRequestValidator creates a new NotificationRequest validator
func NewNotificationRequestValidator(auditStore audit.AuditStore) *NotificationRequestValidator {
	return &NotificationRequestValidator{
		authenticator: authwebhook.NewAuthenticator(),
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

	fmt.Printf("🔍 ValidateDelete invoked: Name=%s, Namespace=%s, UID=%s\n",
		nr.Name, nr.Namespace, nr.UID)

	// Extract authenticated user from admission request context
	// Note: admission.RequestFromContext requires the request to be injected by controller-runtime
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not extract admission request from context: %v\n", err)
		// Allow DELETE to proceed even if we can't capture attribution
		// (audit failure should not block business operations)
		return admission.Warnings{"audit attribution unavailable"}, nil
	}

	authCtx, err := v.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		fmt.Printf("⚠️  Warning: Authentication failed: %v\n", err)
		// Allow DELETE to proceed even if authentication fails
		// (audit failure should not block business operations)
		return admission.Warnings{"authentication unavailable"}, nil
	}
	fmt.Printf("✅ Authenticated user: %s (UID: %s)\n", authCtx.Username, authCtx.UID)

	// Write complete deletion audit event (DD-WEBHOOK-003: Webhook-Complete Audit Pattern)
	fmt.Printf("📝 Creating audit event for DELETE operation...\n")
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, "notification.request.cancelled") // DD-WEBHOOK-001 line 349
	audit.SetEventCategory(auditEvent, "webhook") // Per ADR-034 v1.4: event_category = emitter service
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
	payload := NotificationAuditPayload{
		// Business context fields (per DD-WEBHOOK-003)
		NotificationName: nr.Name,                   // Business field
		NotificationType: string(nr.Spec.Type),      // Business field
		Priority:         string(nr.Spec.Priority),  // Business field (useful for audit completeness)
		FinalStatus:      string(nr.Status.Phase),   // Business field (per DD-WEBHOOK-003 line 338)
		Recipients:       nr.Spec.Recipients,        // Business field (per DD-WEBHOOK-003 line 339)
	}
	// Note: Attribution fields (WHO, WHAT, WHERE, HOW) are in structured columns:
	// - actor_id: authCtx.Username (via audit.SetActor)
	// - resource_name: nr.Name (via audit.SetResource)
	// - namespace: nr.Namespace (via audit.SetNamespace)
	// - event_action: "deleted" (via audit.SetEventAction)
	audit.SetEventData(auditEvent, payload)
	fmt.Printf("✅ Audit event created: type=%s, correlation_id=%s\n",
		auditEvent.EventType, auditEvent.CorrelationId)

	// Store audit event asynchronously (buffered write)
	fmt.Printf("💾 Storing audit event to Data Storage...\n")
	if err := v.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		// Log error but don't fail the webhook (audit should not block operations)
		// The audit store has retry + DLQ mechanisms
		fmt.Printf("❌ WARNING: Failed to store audit event: %v\n", err)
		return admission.Warnings{fmt.Sprintf("audit storage failed: %v", err)}, nil
	}
	fmt.Printf("✅ Audit event stored successfully\n")

	// Allow DELETE to proceed (return nil error)
	fmt.Printf("✅ Allowing DELETE operation for %s/%s\n", nr.Namespace, nr.Name)
	return nil, nil
}

