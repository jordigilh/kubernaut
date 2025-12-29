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

import (
	"encoding/json"
	"time"
)

// EventData represents the standardized event_data JSONB structure
// for the unified audit_events table (ADR-034).
//
// All audit events follow this common envelope format:
// - version: Schema version (semver, e.g., "1.0")
// - service: Originating service name (e.g., "gateway", "aianalysis")
// - event_type: Specific event identifier (e.g., "signal.received", "analysis.completed")
// - timestamp: Event creation timestamp (RFC3339)
// - data: Service-specific event data (flexible JSONB)
//
// Business Requirement: BR-STORAGE-033-001
type EventData struct {
	Version   string                 `json:"version"`    // Schema version (e.g., "1.0")
	Service   string                 `json:"service"`    // Service name (e.g., "gateway")
	EventType string                 `json:"event_type"` // Event type (e.g., "signal.received")
	Timestamp time.Time              `json:"timestamp"`  // Event timestamp (RFC3339)
	Data      map[string]interface{} `json:"data"`       // Service-specific data
}

// BaseEventBuilder provides common event building functionality for all audit events.
//
// Usage:
//
//	builder := audit.NewEventBuilder("gateway", "signal.received").
//	    WithCustomField("signal_type", "prometheus").
//	    WithCustomField("alert_name", "HighMemoryUsage")
//
//	eventData, err := builder.Build()
//
// Business Requirement: BR-STORAGE-033-002 (Type-safe event building API)
type BaseEventBuilder struct {
	eventData EventData
}

// NewEventBuilder creates a new base event builder.
//
// Parameters:
// - service: The name of the service creating the event (e.g., "gateway", "aianalysis", "workflow")
// - eventType: The specific event type identifier (e.g., "signal.received", "analysis.completed")
//
// The builder is initialized with:
// - version: "1.0" (current schema version)
// - timestamp: current UTC time
// - data: empty map (populated via WithCustomField)
//
// Example:
//
//	builder := audit.NewEventBuilder("gateway", "signal.received")
func NewEventBuilder(service, eventType string) *BaseEventBuilder {
	return &BaseEventBuilder{
		eventData: EventData{
			Version:   "1.0",
			Service:   service,
			EventType: eventType,
			Timestamp: time.Now().UTC(),
			Data:      make(map[string]interface{}),
		},
	}
}

// WithCustomField adds a custom field to the event data.
//
// This method is chainable (fluent API) and can be called multiple times.
// If the same key is set multiple times, the last value wins.
//
// Parameters:
// - key: Field name (will appear in event_data.data JSONB)
// - value: Field value (any JSON-serializable type)
//
// Supported value types:
// - Primitives: string, int, float64, bool
// - Nil: nil (JSON null)
// - Complex: map[string]interface{}, []interface{}, structs
//
// Example:
//
//	builder.WithCustomField("signal_type", "prometheus").
//	    WithCustomField("severity", "critical").
//	    WithCustomField("labels", map[string]string{"app": "api-server"})
//
// Business Requirement: BR-STORAGE-033-002 (Type-safe event building API)
func (b *BaseEventBuilder) WithCustomField(key string, value interface{}) *BaseEventBuilder {
	b.eventData.Data[key] = value
	return b
}

// Build returns the final event_data as a JSONB-ready map.
//
// The returned map can be directly marshaled to JSON and stored in the
// audit_events.event_data JSONB column.
//
// Returns:
// - map[string]interface{}: JSONB-ready event data
// - error: JSON marshaling error (should not occur for valid inputs)
//
// Example:
//
//	eventData, err := builder.Build()
//	if err != nil {
//	    return fmt.Errorf("failed to build event data: %w", err)
//	}
//	// eventData can now be stored in PostgreSQL JSONB column
//
// Business Requirement: BR-STORAGE-033-003 (Valid JSONB output)
func (b *BaseEventBuilder) Build() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Marshal to JSON and back to ensure proper JSONB structure
	// This normalizes all types to JSON-compatible types (e.g., int -> float64)
	jsonBytes, err := json.Marshal(b.eventData)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}
