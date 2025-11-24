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

package notification

import (
	"encoding/json"
	"fmt"
	"time"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// AuditHelpers provides helper functions for creating notification audit events
// following ADR-034 unified audit table format.
//
// BR-NOT-062: Unified Audit Table Integration
// BR-NOT-063: Graceful Audit Degradation
// BR-NOT-064: Audit Event Correlation
//
// See: docs/services/crd-controllers/06-notification/DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
type AuditHelpers struct {
	serviceName string
}

// NewAuditHelpers creates a new AuditHelpers instance
//
// Parameters:
//   - serviceName: Name of the service for actor_id field (e.g., "notification-controller")
func NewAuditHelpers(serviceName string) *AuditHelpers {
	return &AuditHelpers{
		serviceName: serviceName,
	}
}

// CreateMessageSentEvent creates an audit event for successful message delivery
//
// BR-NOT-062: Unified audit table integration
// BR-NOT-064: Audit event correlation (uses metadata["remediationRequestName"] as correlation_id)
//
// Event Type: notification.message.sent
// Event Outcome: success
//
// Parameters:
//   - notification: The NotificationRequest CRD
//   - channel: The delivery channel (e.g., "slack", "email")
//
// Returns:
//   - *audit.AuditEvent: The created audit event (ADR-034 format)
//   - error: Error if event creation fails
func (a *AuditHelpers) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*audit.AuditEvent, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}
	if channel == "" {
		return nil, fmt.Errorf("channel cannot be empty")
	}

	// Extract correlation ID (BR-NOT-064: Use remediation_id for tracing)
	// Use metadata["remediationRequestName"] as correlation_id
	correlationID := ""
	if notification.Spec.Metadata != nil {
		correlationID = notification.Spec.Metadata["remediationRequestName"]
	}
	if correlationID == "" {
		// Fallback to notification name if remediationRequestName not found
		correlationID = notification.Name
	}

	// Build event_data (JSONB payload) with all notification context
	eventData := map[string]interface{}{
		"notification_id": notification.Name,
		"channel":         channel,
		"subject":         notification.Spec.Subject,
		"body":            notification.Spec.Body,
		"priority":        string(notification.Spec.Priority),
		"type":            string(notification.Spec.Type),
	}

	// Include metadata if present
	if notification.Spec.Metadata != nil {
		eventData["metadata"] = notification.Spec.Metadata
	}

	// Marshal event_data to JSON
	eventDataJSON, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Create audit event following ADR-034 format
	event := &audit.AuditEvent{
		EventVersion:   "1.0",
		EventTimestamp: time.Now(),
		EventType:      "notification.message.sent",
		EventCategory:  "notification",
		EventAction:    "sent",
		EventOutcome:   "success",
		ActorType:      "service",
		ActorID:        a.serviceName,
		ResourceType:   "NotificationRequest",
		ResourceID:     notification.Name,
		ResourceName:   &notification.Spec.Subject,
		CorrelationID:  correlationID,
		Namespace:      &notification.Namespace,
		EventData:      eventDataJSON,
		RetentionDays:  2555, // 7 years for compliance (SOC 2 / ISO 27001)
		IsSensitive:    false,
	}

	return event, nil
}

// CreateMessageFailedEvent creates an audit event for failed message delivery
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
//   - *audit.AuditEvent: The created audit event with error details in event_data
//   - error: Error if event creation fails
func (a *AuditHelpers) CreateMessageFailedEvent(notification *notificationv1alpha1.NotificationRequest, channel string, err error) (*audit.AuditEvent, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}
	if channel == "" {
		return nil, fmt.Errorf("channel cannot be empty")
	}

	// Extract correlation ID
	correlationID := ""
	if notification.Spec.Metadata != nil {
		correlationID = notification.Spec.Metadata["remediationRequestName"]
	}
	if correlationID == "" {
		correlationID = notification.Name
	}

	// Build event_data with error details
	eventData := map[string]interface{}{
		"notification_id": notification.Name,
		"channel":         channel,
		"subject":         notification.Spec.Subject,
		"body":            notification.Spec.Body,
		"priority":        string(notification.Spec.Priority),
		"type":            string(notification.Spec.Type),
	}

	// Include error details in event_data
	if err != nil {
		eventData["error"] = err.Error()
	}

	// Include metadata if present
	if notification.Spec.Metadata != nil {
		eventData["metadata"] = notification.Spec.Metadata
	}

	eventDataJSON, marshalErr := json.Marshal(eventData)
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", marshalErr)
	}

	// Prepare error message for audit event
	var errorMessage *string
	if err != nil {
		errMsg := err.Error()
		errorMessage = &errMsg
	}

	// Create audit event following ADR-034 format
	event := &audit.AuditEvent{
		EventVersion:   "1.0",
		EventTimestamp: time.Now(),
		EventType:      "notification.message.failed",
		EventCategory:  "notification",
		EventAction:    "sent", // Action was "sent" (attempted), outcome is "failure"
		EventOutcome:   "failure",
		ActorType:      "service",
		ActorID:        a.serviceName,
		ResourceType:   "NotificationRequest",
		ResourceID:     notification.Name,
		ResourceName:   &notification.Spec.Subject,
		CorrelationID:  correlationID,
		Namespace:      &notification.Namespace,
		EventData:      eventDataJSON,
		ErrorMessage:   errorMessage,
		RetentionDays:  2555,
		IsSensitive:    false,
	}

	return event, nil
}

// CreateMessageAcknowledgedEvent creates an audit event for acknowledged notification
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
//   - *audit.AuditEvent: The created audit event
//   - error: Error if event creation fails
func (a *AuditHelpers) CreateMessageAcknowledgedEvent(notification *notificationv1alpha1.NotificationRequest) (*audit.AuditEvent, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}

	// Extract correlation ID
	correlationID := ""
	if notification.Spec.Metadata != nil {
		correlationID = notification.Spec.Metadata["remediationRequestName"]
	}
	if correlationID == "" {
		correlationID = notification.Name
	}

	// Build event_data
	eventData := map[string]interface{}{
		"notification_id": notification.Name,
		"subject":         notification.Spec.Subject,
		"priority":        string(notification.Spec.Priority),
		"type":            string(notification.Spec.Type),
	}

	if notification.Spec.Metadata != nil {
		eventData["metadata"] = notification.Spec.Metadata
	}

	eventDataJSON, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Create audit event
	event := &audit.AuditEvent{
		EventVersion:   "1.0",
		EventTimestamp: time.Now(),
		EventType:      "notification.message.acknowledged",
		EventCategory:  "notification",
		EventAction:    "acknowledged",
		EventOutcome:   "success",
		ActorType:      "service",
		ActorID:        a.serviceName,
		ResourceType:   "NotificationRequest",
		ResourceID:     notification.Name,
		ResourceName:   &notification.Spec.Subject,
		CorrelationID:  correlationID,
		Namespace:      &notification.Namespace,
		EventData:      eventDataJSON,
		RetentionDays:  2555,
		IsSensitive:    false,
	}

	return event, nil
}

// CreateMessageEscalatedEvent creates an audit event for escalated notification
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
//   - *audit.AuditEvent: The created audit event
//   - error: Error if event creation fails
func (a *AuditHelpers) CreateMessageEscalatedEvent(notification *notificationv1alpha1.NotificationRequest) (*audit.AuditEvent, error) {
	// Input validation
	if notification == nil {
		return nil, fmt.Errorf("notification cannot be nil")
	}

	// Extract correlation ID
	correlationID := ""
	if notification.Spec.Metadata != nil {
		correlationID = notification.Spec.Metadata["remediationRequestName"]
	}
	if correlationID == "" {
		correlationID = notification.Name
	}

	// Build event_data
	eventData := map[string]interface{}{
		"notification_id": notification.Name,
		"subject":         notification.Spec.Subject,
		"priority":        string(notification.Spec.Priority),
		"type":            string(notification.Spec.Type),
	}

	if notification.Spec.Metadata != nil {
		eventData["metadata"] = notification.Spec.Metadata
	}

	eventDataJSON, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// Create audit event
	event := &audit.AuditEvent{
		EventVersion:   "1.0",
		EventTimestamp: time.Now(),
		EventType:      "notification.message.escalated",
		EventCategory:  "notification",
		EventAction:    "escalated",
		EventOutcome:   "success",
		ActorType:      "service",
		ActorID:        a.serviceName,
		ResourceType:   "NotificationRequest",
		ResourceID:     notification.Name,
		ResourceName:   &notification.Spec.Subject,
		CorrelationID:  correlationID,
		Namespace:      &notification.Namespace,
		EventData:      eventDataJSON,
		RetentionDays:  2555,
		IsSensitive:    false,
	}

	return event, nil
}
