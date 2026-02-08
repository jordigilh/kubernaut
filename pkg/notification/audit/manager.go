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

// ========================================
// AUDIT MANAGER (P3 PATTERN)
// 📋 Controller Refactoring Pattern: Audit Manager
// Reference: pkg/remediationorchestrator/audit/manager.go
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================
//
// This manager provides centralized audit event creation for the Notification service,
// following ADR-034 unified audit table format.
//
// BENEFITS:
// - ✅ Reusability: Can be used by controller, delivery services, and tests
// - ✅ Consistency: Single source of truth for audit event creation
// - ✅ Type Safety: Structured event data types (DD-AUDIT-004)
// - ✅ Testability: Easy to test audit logic in isolation
// - ✅ Maintainability: Audit changes happen in one place
//
// Business Requirements:
// - BR-NOT-062: Unified Audit Table Integration
// - BR-NOT-063: Graceful Audit Degradation
// - BR-NOT-064: Audit Event Correlation
//
// Design Decisions:
// - DD-AUDIT-002 V2.0: OpenAPI types directly
// - DD-AUDIT-004: Structured event data types
// ========================================

package audit

import (
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Event type constants for Notification audit events (from OpenAPI spec)
const (
	EventTypeMessageSent         = "notification.message.sent"
	EventTypeMessageFailed       = "notification.message.failed"
	EventTypeMessageAcknowledged = "notification.message.acknowledged"
	EventTypeMessageEscalated    = "notification.message.escalated"
)

// Event category constant (from OpenAPI spec)
const (
	EventCategoryNotification = "notification"
)

// Event action constants (L-3 SOC2 Fix: compile-time safety for event action strings)
const (
	ActionSent         = "sent"
	ActionAcknowledged = "acknowledged"
	ActionEscalated    = "escalated"
)

// Manager provides helper functions for creating notification audit events
// following ADR-034 unified audit table format.
//
// This manager is the single source of truth for audit event creation in the
// Notification service, ensuring consistency across controller, delivery services,
// and tests.
//
// Usage Example:
//
//	auditMgr := audit.NewManager("notification-controller")
//	event, err := auditMgr.CreateMessageSentEvent(notification, "slack")
//	if err != nil {
//	    return fmt.Errorf("failed to create audit event: %w", err)
//	}
//	// Send event to DataStorage...
type Manager struct {
	serviceName string
}

// NewManager creates a new audit manager instance.
//
// Parameters:
//   - serviceName: Name of the service for actor_id field (e.g., "notification-controller", "notification")
//
// The serviceName is used in the actor_id field of all audit events created by this manager.
func NewManager(serviceName string) *Manager {
	return &Manager{
		serviceName: serviceName,
	}
}

// CreateMessageSentEvent creates an audit event for successful message delivery.
//
// BR-NOT-062: Unified audit table integration
// BR-NOT-064: Audit event correlation (uses metadata["remediationRequestName"] as correlation_id)
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
//
// Event Type: notification.message.sent
// Event Outcome: success
//
// Parameters:
//   - notification: The NotificationRequest CRD
//   - channel: The delivery channel (e.g., "slack", "email")
//
// Returns:
//   - *ogenclient.AuditEventRequest: The created audit event (OpenAPI type)
//   - error: Error if event creation fails (e.g., nil notification, empty channel)
func (m *Manager) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*ogenclient.AuditEventRequest, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}
	if channel == "" {
		return nil, fmt.Errorf("channel cannot be empty")
	}

	// Extract correlation ID per DD-AUDIT-CORRELATION-002 (Universal Correlation ID Standard)
	// MANDATORY: Use RemediationRequest.Name as correlation_id (not UID)
	correlationID := ""
	if notification.Spec.RemediationRequestRef != nil && notification.Spec.RemediationRequestRef.Name != "" {
		// Primary: Use RemediationRequest.Name (DD-AUDIT-CORRELATION-002)
		correlationID = notification.Spec.RemediationRequestRef.Name
	} else {
		// Fallback: Notification UID for standalone notifications (not part of remediation workflow)
		correlationID = string(notification.UID)
	}

	// Build structured event_data payload using OpenAPI-generated type (DD-AUDIT-004 V2.0)
	// Single source of truth: api/openapi/data-storage-v1.yaml
	payload := ogenclient.NotificationMessageSentPayload{
		NotificationID: notification.Name,
		Channel:        channel,
		Subject:        notification.Spec.Subject,
		Body:           notification.Spec.Body,
		Priority:       string(notification.Spec.Priority),
		Type:           string(notification.Spec.Type),
	}
	// Set optional metadata if present
	if notification.Spec.Metadata != nil {
		payload.Metadata.SetTo(notification.Spec.Metadata)
	}

	// Create audit event following ADR-034 format (DD-AUDIT-002 V2.2: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeMessageSent)
	audit.SetEventCategory(event, EventCategoryNotification)
	audit.SetEventAction(event, ActionSent)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "NotificationRequest", notification.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, notification.Namespace)
	// V3.0: OGEN - Use constructor to create discriminated union (DD-AUDIT-004 v1.4)
	event.EventData = ogenclient.NewNotificationMessageSentPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// CreateMessageFailedEvent creates an audit event for failed message delivery.
//
// BR-NOT-062: Unified audit table integration
//
// Event Type: notification.message.failed
// Event Outcome: failure
//
// Parameters:
//   - notification: The NotificationRequest CRD
//   - channel: The delivery channel that failed
//   - err: The error that caused the failure
//
// Returns:
//   - *ogenclient.AuditEventRequest: The created audit event with error details in event_data (OpenAPI type)
//   - error: Error if event creation fails (e.g., nil notification, empty channel)
//
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) CreateMessageFailedEvent(notification *notificationv1alpha1.NotificationRequest, channel string, err error) (*ogenclient.AuditEventRequest, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}
	if channel == "" {
		return nil, fmt.Errorf("channel cannot be empty")
	}

	// Extract correlation ID per DD-AUDIT-CORRELATION-002 (Universal Correlation ID Standard)
	correlationID := ""
	if notification.Spec.RemediationRequestRef != nil && notification.Spec.RemediationRequestRef.Name != "" {
		// Primary: Use RemediationRequest.Name (DD-AUDIT-CORRELATION-002)
		correlationID = notification.Spec.RemediationRequestRef.Name
	} else {
		// Fallback: Notification UID for standalone notifications
		correlationID = string(notification.UID)
	}

	// Build structured event_data payload using OpenAPI-generated type (DD-AUDIT-004 V2.0)
	payload := ogenclient.NotificationMessageFailedPayload{
		NotificationID: notification.Name,
		Channel:        channel,
		Subject:        notification.Spec.Subject,
		Body:           notification.Spec.Body,
		Priority:       string(notification.Spec.Priority),
		ErrorType:      "transient", // Default to transient (retry possible)
	}
	// Set optional metadata if present
	if notification.Spec.Metadata != nil {
		payload.Metadata.SetTo(notification.Spec.Metadata)
	}
	// Set optional error message if present
	if err != nil {
		payload.Error.SetTo(err.Error())
	}

	// Create audit event following ADR-034 format (DD-AUDIT-002 V2.2: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeMessageFailed)
	audit.SetEventCategory(event, EventCategoryNotification)
	audit.SetEventAction(event, ActionSent) // Action was "sent" (attempted), outcome is "failure"
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "NotificationRequest", notification.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, notification.Namespace)
	// V3.0: OGEN - Use constructor to create discriminated union (DD-AUDIT-004 v1.4)
	event.EventData = ogenclient.NewNotificationMessageFailedPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// CreateMessageAcknowledgedEvent creates an audit event for acknowledged notification.
//
// BR-NOT-062: Unified audit table integration
//
// Event Type: notification.message.acknowledged
// Event Outcome: success
//
// Parameters:
//   - notification: The NotificationRequest CRD
//
// Returns:
//   - *ogenclient.AuditEventRequest: The created audit event (OpenAPI type)
//   - error: Error if event creation fails (e.g., nil notification)
//
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) CreateMessageAcknowledgedEvent(notification *notificationv1alpha1.NotificationRequest) (*ogenclient.AuditEventRequest, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}

	// Extract correlation ID per DD-AUDIT-CORRELATION-002 (Universal Correlation ID Standard)
	correlationID := ""
	if notification.Spec.RemediationRequestRef != nil && notification.Spec.RemediationRequestRef.Name != "" {
		// Primary: Use RemediationRequest.Name (DD-AUDIT-CORRELATION-002)
		correlationID = notification.Spec.RemediationRequestRef.Name
	} else {
		// Fallback: Notification UID for standalone notifications
		correlationID = string(notification.UID)
	}

	// Build structured event_data payload using OpenAPI-generated type (DD-AUDIT-004 V2.0)
	payload := ogenclient.NotificationMessageAcknowledgedPayload{
		NotificationID: notification.Name,
		Subject:        notification.Spec.Subject,
		Priority:       string(notification.Spec.Priority),
	}
	// Set optional metadata if present
	if notification.Spec.Metadata != nil {
		payload.Metadata.SetTo(notification.Spec.Metadata)
	}

	// Create audit event (DD-AUDIT-002 V2.2: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeMessageAcknowledged)
	audit.SetEventCategory(event, EventCategoryNotification)
	audit.SetEventAction(event, ActionAcknowledged)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "NotificationRequest", notification.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, notification.Namespace)
	// V3.0: OGEN - Use constructor to create discriminated union (DD-AUDIT-004 v1.4)
	event.EventData = ogenclient.NewNotificationMessageAcknowledgedPayloadAuditEventRequestEventData(payload)

	return event, nil
}

// CreateMessageEscalatedEvent creates an audit event for escalated notification.
//
// BR-NOT-062: Unified audit table integration
//
// Event Type: notification.message.escalated
// Event Outcome: success
//
// Parameters:
//   - notification: The NotificationRequest CRD
//
// Returns:
//   - *ogenclient.AuditEventRequest: The created audit event (OpenAPI type)
//   - error: Error if event creation fails (e.g., nil notification)
//
// DD-AUDIT-002 V2.0: Uses OpenAPI types directly
func (m *Manager) CreateMessageEscalatedEvent(notification *notificationv1alpha1.NotificationRequest) (*ogenclient.AuditEventRequest, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}

	// Extract correlation ID per DD-AUDIT-CORRELATION-002 (Universal Correlation ID Standard)
	correlationID := ""
	if notification.Spec.RemediationRequestRef != nil && notification.Spec.RemediationRequestRef.Name != "" {
		// Primary: Use RemediationRequest.Name (DD-AUDIT-CORRELATION-002)
		correlationID = notification.Spec.RemediationRequestRef.Name
	} else {
		// Fallback: Notification UID for standalone notifications
		correlationID = string(notification.UID)
	}

	// Build structured event_data payload using OpenAPI-generated type (DD-AUDIT-004 V2.0)
	payload := ogenclient.NotificationMessageEscalatedPayload{
		NotificationID: notification.Name,
		Subject:        notification.Spec.Subject,
		Priority:       string(notification.Spec.Priority),
		Reason:         fmt.Sprintf("Escalated due to %s priority", notification.Spec.Priority),
	}
	// Set optional metadata if present
	if notification.Spec.Metadata != nil {
		payload.Metadata.SetTo(notification.Spec.Metadata)
	}

	// Create audit event (DD-AUDIT-002 V2.2: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeMessageEscalated)
	audit.SetEventCategory(event, EventCategoryNotification)
	audit.SetEventAction(event, ActionEscalated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", m.serviceName)
	audit.SetResource(event, "NotificationRequest", notification.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, notification.Namespace)
	// V3.0: OGEN - Use constructor to create discriminated union (DD-AUDIT-004 v1.4)
	event.EventData = ogenclient.NewNotificationMessageEscalatedPayloadAuditEventRequestEventData(payload)

	return event, nil
}
