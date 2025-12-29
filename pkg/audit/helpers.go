package audit

import (
	"encoding/json"
	"time"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

// ========================================
// AUDIT HELPERS - OpenAPI Type Convenience Functions
// 📋 Design Decision: DD-AUDIT-002 V2.0 | ✅ Approved Design | Confidence: 99%
// See: docs/architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md
// ========================================
//
// Helper functions for working with OpenAPI-generated audit event types.
//
// WHY DD-AUDIT-002 V2.0?
// - ✅ Compile-time type safety: Use generated OpenAPI types directly
// - ✅ Forced integration testing: Can't mock away OpenAPI client
// - ✅ Simpler architecture: No adapter layer, no type conversion
// - ✅ Maintainability: OpenAPI spec changes propagate automatically
//
// These helpers provide convenience functions while maintaining direct OpenAPI usage.
// ========================================

// NewAuditEventRequest creates a new audit event request with default values
func NewAuditEventRequest() *dsgen.AuditEventRequest {
	now := time.Now()
	version := "1.0"
	return &dsgen.AuditEventRequest{
		Version:        version,
		EventTimestamp: now,
	}
}

// SetEventType sets the event type
func SetEventType(e *dsgen.AuditEventRequest, eventType string) {
	e.EventType = eventType
}

// SetEventCategory sets the event category
// Converts string to enum type for OpenAPI client type safety (DD-API-001)
func SetEventCategory(e *dsgen.AuditEventRequest, category string) {
	e.EventCategory = dsgen.AuditEventRequestEventCategory(category)
}

// SetEventAction sets the event action
func SetEventAction(e *dsgen.AuditEventRequest, action string) {
	e.EventAction = action
}

// SetEventOutcome sets the event outcome
func SetEventOutcome(e *dsgen.AuditEventRequest, outcome dsgen.AuditEventRequestEventOutcome) {
	e.EventOutcome = outcome
}

// SetActor sets actor information
func SetActor(e *dsgen.AuditEventRequest, actorType, actorID string) {
	e.ActorType = &actorType
	e.ActorId = &actorID
}

// SetResource sets resource information
func SetResource(e *dsgen.AuditEventRequest, resourceType, resourceID string) {
	e.ResourceType = &resourceType
	e.ResourceId = &resourceID
}

// SetCorrelationID sets the correlation ID
func SetCorrelationID(e *dsgen.AuditEventRequest, correlationID string) {
	e.CorrelationId = correlationID
}

// SetNamespace sets the namespace
func SetNamespace(e *dsgen.AuditEventRequest, namespace string) {
	e.Namespace = &namespace
}

// SetClusterName sets the cluster name
func SetClusterName(e *dsgen.AuditEventRequest, clusterName string) {
	e.ClusterName = &clusterName
}

// SetDuration sets the operation duration in milliseconds
func SetDuration(e *dsgen.AuditEventRequest, durationMs int) {
	e.DurationMs = &durationMs
}

// SetSeverity sets the severity level
func SetSeverity(e *dsgen.AuditEventRequest, severity string) {
	e.Severity = &severity
}

// SetEventData sets the event data payload from any structured type
//
// V1.0: ZERO UNSTRUCTURED DATA - Accepts structured Go types directly.
//
// Usage:
//
//	payload := MessageSentEventData{
//	    NotificationID: notification.Name,
//	    Channel:        notification.Spec.Channel,
//	}
//	audit.SetEventData(event, payload)  // ✅ Direct assignment, no conversion
//
// The OpenAPI client now uses interface{} for EventData, eliminating the need for
// map[string]interface{} conversion. JSON marshaling happens at the HTTP layer.
//
// DD-AUDIT-004: V1.0 - Zero Unstructured Data (no map[string]interface{})
func SetEventData(e *dsgen.AuditEventRequest, data interface{}) {
	e.EventData = data
}

// SetEventDataFromEnvelope sets the event data from a CommonEnvelope
func SetEventDataFromEnvelope(e *dsgen.AuditEventRequest, envelope *CommonEnvelope) error {
	data, err := EnvelopeToMap(envelope)
	if err != nil {
		return err
	}
	e.EventData = data
	return nil
}

// ========================================
// COMMON ENVELOPE HELPERS
// ========================================

// EnvelopeToMap converts a CommonEnvelope to a map for use in AuditEventRequest.EventData
func EnvelopeToMap(e *CommonEnvelope) (map[string]interface{}, error) {
	// Marshal to JSON and back to get a map
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// StructToMap converts any structured type to a map for use in AuditEventRequest.EventData
//
// ⚠️ DEPRECATED (V1.0): This function is no longer needed. Use SetEventData() directly instead.
//
// OLD PATTERN (V0.9):
//
//	eventDataMap, err := audit.StructToMap(payload)
//	audit.SetEventData(event, eventDataMap)
//
// NEW PATTERN (V1.0):
//
//	audit.SetEventData(event, payload)  // ✅ Handles conversion internally
//
// Rationale: SetEventData() now accepts interface{} and handles conversion internally,
// eliminating the unnecessary intermediate map[string]interface{} step.
//
// This function remains for backward compatibility but will be removed in V2.0.
//
// DD-AUDIT-004: Structured Types for Audit Event Payloads (V1.0 Simplification)
func StructToMap(data interface{}) (map[string]interface{}, error) {
	// Marshal to JSON and back to get a map
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// ========================================
// VALIDATION
// ========================================
//
// Note: Validation is now handled by openapi_validator.go using automatic
// OpenAPI spec validation. See ValidateAuditEventRequest() in openapi_validator.go
// for implementation that reads constraints directly from the OpenAPI spec.
//
// Authority: api/openapi/data-storage-v1.yaml (lines 832-920)
// ========================================

// ========================================
// OUTCOME CONSTANTS
// ========================================

const (
	// OutcomeSuccess indicates successful operation
	OutcomeSuccess = dsgen.AuditEventRequestEventOutcomeSuccess

	// OutcomeFailure indicates failed operation
	OutcomeFailure = dsgen.AuditEventRequestEventOutcomeFailure

	// OutcomePending indicates operation still in progress
	OutcomePending = dsgen.AuditEventRequestEventOutcomePending
)
