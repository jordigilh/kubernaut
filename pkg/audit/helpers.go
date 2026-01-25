package audit

import (
	"encoding/json"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
// CRITICAL: Timestamp MUST be UTC to match DataStorage server validation
// Issue: DataStorage validates timestamps against server time (UTC)
// Fix: Use time.Now().UTC() instead of time.Now() to avoid timezone mismatches
func NewAuditEventRequest() *ogenclient.AuditEventRequest {
	now := time.Now().UTC() // ✅ Force UTC to match DataStorage server timezone
	version := "1.0"
	return &ogenclient.AuditEventRequest{
		Version:        version,
		EventTimestamp: now,
	}
}

// SetEventType sets the event type
func SetEventType(e *ogenclient.AuditEventRequest, eventType string) {
	e.EventType = eventType
}

// SetEventCategory sets the event category
// Converts string to enum type for OpenAPI client type safety (DD-API-001)
func SetEventCategory(e *ogenclient.AuditEventRequest, category string) {
	e.EventCategory = ogenclient.AuditEventRequestEventCategory(category)
}

// SetEventAction sets the event action
func SetEventAction(e *ogenclient.AuditEventRequest, action string) {
	e.EventAction = action
}

// SetEventOutcome sets the event outcome
func SetEventOutcome(e *ogenclient.AuditEventRequest, outcome ogenclient.AuditEventRequestEventOutcome) {
	e.EventOutcome = outcome
}

// SetActor sets actor information
func SetActor(e *ogenclient.AuditEventRequest, actorType, actorID string) {
	e.ActorType.SetTo(actorType)
	e.ActorID.SetTo(actorID)
}

// SetResource sets resource information
func SetResource(e *ogenclient.AuditEventRequest, resourceType, resourceID string) {
	e.ResourceType.SetTo(resourceType)
	e.ResourceID.SetTo(resourceID)
}

// SetCorrelationID sets the correlation ID
func SetCorrelationID(e *ogenclient.AuditEventRequest, correlationID string) {
	e.CorrelationID = correlationID
}

// SetNamespace sets the namespace
func SetNamespace(e *ogenclient.AuditEventRequest, namespace string) {
	e.Namespace.SetTo(namespace)
}

// SetClusterName sets the cluster name
func SetClusterName(e *ogenclient.AuditEventRequest, clusterName string) {
	e.ClusterName.SetTo(clusterName)
}

// SetDuration sets the operation duration in milliseconds
func SetDuration(e *ogenclient.AuditEventRequest, durationMs int) {
	e.DurationMs.SetTo(durationMs)
}

// SetSeverity sets the severity level
func SetSeverity(e *ogenclient.AuditEventRequest, severity string) {
	e.Severity.SetTo(severity)
}

// SetEventData sets the event data payload from an ogen-generated union type
//
// V3.0: OGEN TYPED UNIONS - Proper type-safe discriminated unions (no interface{})
//
// ⚠️ MIGRATION IN PROGRESS: This function is being deprecated in favor of direct ogen constructor calls.
//
// OLD PATTERN (V2.0 - oapi-codegen with interface{}):
//
//	payload := WorkflowExecutionAuditPayload{...}
//	audit.SetEventData(event, payload)  // Assigned to interface{}
//
// NEW PATTERN (V3.0 - ogen with typed unions):
//
//	payload := ogenclient.WorkflowExecutionAuditPayload{...}
//	event.EventData = ogenclient.NewWorkflowExecutionAuditPayloadAuditEventRequestEventData(payload)
//
// WHY CHANGE?
// - ✅ Type safety: Compile-time checking of payload types
// - ✅ No marshaling: Direct struct assignment (performance)
// - ✅ No interface{}: Proper discriminated unions
// - ✅ Better IDE support: Autocomplete for payload fields
//
// This function remains temporarily for backward compatibility but will be removed in V4.0.
//
// DD-AUDIT-005: Ogen Migration - Eliminating json.RawMessage and interface{} conversions
func SetEventData(e *ogenclient.AuditEventRequest, data ogenclient.AuditEventRequestEventData) {
	e.EventData = data
}

// SetEventDataFromEnvelope sets the event data from a CommonEnvelope
//
// ⚠️ DEPRECATED: This function is being removed in V4.0.
// Envelopes should be converted to proper ogen union types instead.
func SetEventDataFromEnvelope(e *ogenclient.AuditEventRequest, envelope *CommonEnvelope) error {
	// TODO: Convert envelope to proper ogen union type
	// For now, this function is not used in production code
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
	OutcomeSuccess = ogenclient.AuditEventRequestEventOutcomeSuccess

	// OutcomeFailure indicates failed operation
	OutcomeFailure = ogenclient.AuditEventRequestEventOutcomeFailure

	// OutcomePending indicates operation still in progress
	OutcomePending = ogenclient.AuditEventRequestEventOutcomePending
)
