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

package audit

// ========================================
// STRUCTURED AUDIT EVENT TYPES (DD-AUDIT-004)
// 📋 Design Decision: DD-AUDIT-004 | ✅ Approved Design | Confidence: 100%
// See: docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md
// ========================================
//
// These structured types provide type-safe audit event payloads for the Notification service.
// They are converted to map[string]interface{} at the API boundary using audit.StructToMap().
//
// WHY DD-AUDIT-004?
// - ✅ Type Safety: Compile-time field validation, no runtime typos
// - ✅ Maintainability: Refactor-safe, IDE autocomplete support
// - ✅ Consistency: All services use same pattern (audit.StructToMap())
// - ✅ Testing: 100% field validation through structured types
// - ✅ Coding Standards: Eliminates map[string]interface{} from business logic
//
// USAGE PATTERN:
//   payload := MessageSentEventData{...}
//   eventDataMap, err := audit.StructToMap(payload)
//   audit.SetEventData(event, eventDataMap)
//
// Authority: DS Team Response (docs/handoff/DS_NT_FOLLOWUP_RESPONSE_DEC_17_2025.md)
// ========================================

// MessageSentEventData is the structured payload for notification.message.sent events
//
// BR-NOT-062: Unified Audit Table Integration
// BR-NOT-064: Audit Event Correlation
//
// Event Type: notification.message.sent
// Event Outcome: success
//
// This event is emitted when a notification message is successfully delivered to a channel.
type MessageSentEventData struct {
	// NotificationID is the name of the NotificationRequest CRD
	NotificationID string `json:"notification_id"`

	// Channel is the delivery channel (e.g., "slack", "email", "console")
	Channel string `json:"channel"`

	// Subject is the notification subject line
	Subject string `json:"subject"`

	// Body is the notification message body
	Body string `json:"body"`

	// Priority is the notification priority level (e.g., "high", "medium", "low")
	Priority string `json:"priority"`

	// Type is the notification type (e.g., "alert", "info", "warning")
	Type string `json:"type"`

	// Metadata contains additional notification metadata (optional)
	// This field is omitted from JSON if nil
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MessageFailedEventData is the structured payload for notification.message.failed events
//
// BR-NOT-062: Unified Audit Table Integration
//
// Event Type: notification.message.failed
// Event Outcome: failure
//
// This event is emitted when a notification message fails to deliver to a channel.
type MessageFailedEventData struct {
	// NotificationID is the name of the NotificationRequest CRD
	NotificationID string `json:"notification_id"`

	// Channel is the delivery channel that failed (e.g., "slack", "email")
	Channel string `json:"channel"`

	// Subject is the notification subject line
	Subject string `json:"subject"`

	// Body is the notification message body
	// NT-E2E-001 Fix: E2E tests expect body field for validation
	Body string `json:"body"`

	// Priority is the notification priority level
	Priority string `json:"priority"`

	// ErrorType indicates the failure category (e.g., "transient", "permanent")
	ErrorType string `json:"error_type"`

	// Error is the error message describing the failure (optional)
	// This field is omitted from JSON if empty
	Error string `json:"error,omitempty"`

	// Metadata contains additional notification metadata (optional)
	// This field is omitted from JSON if nil
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MessageAcknowledgedEventData is the structured payload for notification.message.acknowledged events
//
// BR-NOT-062: Unified Audit Table Integration
//
// Event Type: notification.message.acknowledged
// Event Outcome: success
//
// This event is emitted when a notification message is acknowledged by a recipient.
type MessageAcknowledgedEventData struct {
	// NotificationID is the name of the NotificationRequest CRD
	NotificationID string `json:"notification_id"`

	// Subject is the notification subject line
	Subject string `json:"subject"`

	// Priority is the notification priority level
	Priority string `json:"priority"`

	// Metadata contains additional notification metadata (optional)
	// This field is omitted from JSON if nil
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MessageEscalatedEventData is the structured payload for notification.message.escalated events
//
// BR-NOT-062: Unified Audit Table Integration
//
// Event Type: notification.message.escalated
// Event Outcome: success
//
// This event is emitted when a notification message is escalated to a higher priority channel.
type MessageEscalatedEventData struct {
	// NotificationID is the name of the NotificationRequest CRD
	NotificationID string `json:"notification_id"`

	// Subject is the notification subject line
	Subject string `json:"subject"`

	// Priority is the notification priority level
	Priority string `json:"priority"`

	// Reason is the escalation reason
	Reason string `json:"reason"`

	// Metadata contains additional notification metadata (optional)
	// This field is omitted from JSON if nil
	Metadata map[string]string `json:"metadata,omitempty"`
}
