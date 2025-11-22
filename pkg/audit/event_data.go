package audit

import (
	"encoding/json"
	"fmt"
)

// CommonEnvelope is the standard event_data format for audit events.
//
// Authority: ADR-034 (Unified Audit Table Design)
//
// This structure provides a consistent format for the event_data JSONB field
// across all services, making it easier to query and analyze audit events.
//
// The envelope contains:
// - Version: Schema version for the envelope itself
// - Service: Which service generated this event
// - Operation: What operation was performed
// - Status: The status of the operation
// - Payload: Service-specific event data
// - SourcePayload: Original external payload (e.g., from webhook)
type CommonEnvelope struct {
	// Version is the schema version for this envelope (default: "1.0")
	Version string `json:"version"`

	// Service is the name of the service that generated this event
	// (e.g., "gateway", "context-api", "ai-analysis")
	Service string `json:"service"`

	// Operation is the specific operation performed
	// (e.g., "signal_received", "query_processed", "analysis_completed")
	Operation string `json:"operation"`

	// Status is the status of the operation
	// (e.g., "success", "failure", "pending", "in_progress")
	Status string `json:"status"`

	// Payload contains service-specific event data
	// This is a flexible map that can contain any service-specific fields
	Payload map[string]interface{} `json:"payload"`

	// SourcePayload contains the original external payload (optional)
	// This is useful for preserving the original webhook payload, API request, etc.
	// for debugging and audit purposes
	SourcePayload map[string]interface{} `json:"source_payload,omitempty"`
}

// NewEventData creates a new common envelope with the specified fields.
//
// Parameters:
// - service: Name of the service generating the event
// - operation: Specific operation being performed
// - status: Status of the operation
// - payload: Service-specific event data
//
// The Version field is automatically set to "1.0".
func NewEventData(service, operation, status string, payload map[string]interface{}) *CommonEnvelope {
	return &CommonEnvelope{
		Version:   "1.0",
		Service:   service,
		Operation: operation,
		Status:    status,
		Payload:   payload,
	}
}

// WithSourcePayload adds the original external payload to the envelope.
//
// This is useful for preserving the original webhook payload, API request, etc.
// for debugging and audit purposes.
//
// Returns the envelope for method chaining.
func (e *CommonEnvelope) WithSourcePayload(sourcePayload map[string]interface{}) *CommonEnvelope {
	e.SourcePayload = sourcePayload
	return e
}

// ToJSON converts the envelope to JSON bytes for storage in the event_data field.
//
// Returns an error if the envelope cannot be marshaled to JSON.
func (e *CommonEnvelope) ToJSON() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}
	return data, nil
}

// FromJSON parses JSON bytes into a CommonEnvelope.
//
// This is useful for reading event_data from the database and parsing it
// back into a structured format.
//
// Returns an error if the JSON cannot be unmarshaled.
func FromJSON(data []byte) (*CommonEnvelope, error) {
	var envelope CommonEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
	}
	return &envelope, nil
}

// Validate validates the common envelope for required fields.
//
// Returns an error if any required field is missing.
func (e *CommonEnvelope) Validate() error {
	if e.Version == "" {
		return fmt.Errorf("version is required")
	}
	if e.Service == "" {
		return fmt.Errorf("service is required")
	}
	if e.Operation == "" {
		return fmt.Errorf("operation is required")
	}
	if e.Status == "" {
		return fmt.Errorf("status is required")
	}
	if e.Payload == nil {
		return fmt.Errorf("payload is required")
	}
	return nil
}
