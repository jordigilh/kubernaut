// Package audit provides shared audit event types and utilities for all Kubernaut services.
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
// Related: ADR-034 (Unified Audit Table Design), ADR-038 (Async Buffered Audit Ingestion)
//
// This package implements the system-wide audit event structure that all services MUST use
// for consistent audit trace generation across the platform.
package audit

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditEvent represents a single audit event that will be written to the unified audit_events table.
//
// Authority: ADR-034 (Unified Audit Table Design)
//
// This structure aligns with the audit_events table schema and supports:
// - Compliance requirements (SOC 2, ISO 27001, GDPR)
// - Debugging and troubleshooting across service boundaries
// - Analytics and reporting (signal volume, success rates, performance metrics)
// - Correlation tracking (trace signal flow from ingestion to remediation)
// - Replay capabilities for testing and recovery
//
// Business Requirements:
// - BR-STORAGE-001: Complete audit trail with no data loss
// - DD-009: Dead Letter Queue pattern for error recovery
type AuditEvent struct {
	// ========================================
	// EVENT IDENTITY
	// ========================================

	// EventID is the unique identifier for this audit event (auto-generated if not provided)
	EventID uuid.UUID `json:"event_id"`

	// EventVersion is the schema version for this event (default: "1.0")
	EventVersion string `json:"version"`

	// ========================================
	// TEMPORAL INFORMATION
	// ========================================

	// EventTimestamp is when the event occurred (default: time.Now())
	EventTimestamp time.Time `json:"event_timestamp"`

	// ========================================
	// EVENT CLASSIFICATION
	// ========================================

	// EventType is the specific event type (e.g., "gateway.signal.received", "ai.analysis.completed")
	// REQUIRED: Must follow pattern: <service>.<category>.<action>
	EventType string `json:"event_type"`

	// EventCategory is the high-level category (e.g., "signal", "remediation", "workflow")
	// REQUIRED
	EventCategory string `json:"event_category"`

	// EventAction is the specific action taken (e.g., "received", "processed", "executed")
	// REQUIRED
	EventAction string `json:"event_action"`

	// EventOutcome is the result of the action (e.g., "success", "failure", "pending")
	// REQUIRED
	EventOutcome string `json:"event_outcome"`

	// ========================================
	// ACTOR INFORMATION (Who)
	// ========================================

	// ActorType is the type of actor (e.g., "service", "external", "user")
	// REQUIRED
	ActorType string `json:"actor_type"`

	// ActorID is the identifier of the actor (e.g., "gateway-service", "aws-cloudwatch", "user@example.com")
	// REQUIRED
	ActorID string `json:"actor_id"`

	// ActorIP is the IP address of the actor (optional)
	ActorIP *string `json:"actor_ip,omitempty"`

	// ========================================
	// RESOURCE INFORMATION (What)
	// ========================================

	// ResourceType is the type of resource affected (e.g., "Signal", "RemediationRequest", "Workflow")
	// REQUIRED
	ResourceType string `json:"resource_type"`

	// ResourceID is the identifier of the resource (e.g., "fp-abc123", "rr-2025-001")
	// REQUIRED
	ResourceID string `json:"resource_id"`

	// ResourceName is the human-readable name of the resource (optional)
	ResourceName *string `json:"resource_name,omitempty"`

	// ========================================
	// CONTEXT INFORMATION (Where/Why)
	// ========================================

	// CorrelationID groups related events together (e.g., remediation_id)
	// REQUIRED: Used for tracing signal flow across services
	CorrelationID string `json:"correlation_id"`

	// ParentEventID links to the parent event in a causal chain (optional)
	ParentEventID *uuid.UUID `json:"parent_event_id,omitempty"`

	// TraceID is the OpenTelemetry trace ID for distributed tracing (optional)
	TraceID *string `json:"trace_id,omitempty"`

	// SpanID is the OpenTelemetry span ID for distributed tracing (optional)
	SpanID *string `json:"span_id,omitempty"`

	// ========================================
	// KUBERNETES CONTEXT
	// ========================================

	// Namespace is the Kubernetes namespace (optional)
	Namespace *string `json:"namespace,omitempty"`

	// ClusterName is the Kubernetes cluster name (optional)
	ClusterName *string `json:"cluster_name,omitempty"`

	// ========================================
	// EVENT PAYLOAD (JSONB - flexible, queryable)
	// ========================================

	// EventData contains the event-specific payload as JSON bytes
	// REQUIRED: Use CommonEnvelope helpers from event_data.go
	// This is stored as JSONB in PostgreSQL for efficient querying
	EventData []byte `json:"event_data"`

	// EventMetadata contains additional metadata as JSON bytes (optional)
	// This is stored as JSONB in PostgreSQL
	EventMetadata []byte `json:"event_metadata,omitempty"`

	// ========================================
	// AUDIT METADATA
	// ========================================

	// Severity is the event severity (e.g., "info", "warning", "error", "critical")
	// Optional, defaults to "info"
	Severity *string `json:"severity,omitempty"`

	// DurationMs is the operation duration in milliseconds (optional)
	DurationMs *int `json:"duration_ms,omitempty"`

	// ErrorCode is the error code if the event represents a failure (optional)
	ErrorCode *string `json:"error_code,omitempty"`

	// ErrorMessage is the error message if the event represents a failure (optional)
	ErrorMessage *string `json:"error_message,omitempty"`

	// ========================================
	// COMPLIANCE
	// ========================================

	// RetentionDays is the number of days to retain this event (default: 2555 = 7 years)
	// SOC 2 / ISO 27001 compliance requirement
	RetentionDays int `json:"retention_days"`

	// IsSensitive indicates if this event contains sensitive data (default: false)
	// Sensitive events may require additional encryption or access controls
	IsSensitive bool `json:"is_sensitive"`
}

// NewAuditEvent creates a new audit event with sensible defaults.
//
// Defaults:
// - EventID: auto-generated UUID
// - EventVersion: "1.0"
// - EventTimestamp: time.Now().UTC() (MUST be UTC to match DataStorage validation)
// - RetentionDays: 2555 (7 years for SOC 2 / ISO 27001 compliance)
// - IsSensitive: false
//
// All other fields must be set by the caller.
// CRITICAL: Timestamp is UTC to avoid DataStorage rejecting events as "future timestamps"
func NewAuditEvent() *AuditEvent {
	return &AuditEvent{
		EventID:        uuid.New(),
		EventVersion:   "1.0",
		EventTimestamp: time.Now().UTC(), // ✅ Force UTC to match DataStorage server timezone
		RetentionDays:  2555, // 7 years (SOC 2 / ISO 27001)
		IsSensitive:    false,
	}
}

// Validate validates the audit event for required fields and constraints.
//
// Returns an error if any required field is missing or invalid.
func (e *AuditEvent) Validate() error {
	if e.EventType == "" {
		return fmt.Errorf("event_type is required")
	}
	if e.EventCategory == "" {
		return fmt.Errorf("event_category is required")
	}
	if e.EventAction == "" {
		return fmt.Errorf("event_action is required")
	}
	if e.EventOutcome == "" {
		return fmt.Errorf("event_outcome is required")
	}
	if e.ActorType == "" {
		return fmt.Errorf("actor_type is required")
	}
	if e.ActorID == "" {
		return fmt.Errorf("actor_id is required")
	}
	if e.ResourceType == "" {
		return fmt.Errorf("resource_type is required")
	}
	if e.ResourceID == "" {
		return fmt.Errorf("resource_id is required")
	}
	if e.CorrelationID == "" {
		return fmt.Errorf("correlation_id is required")
	}
	if len(e.EventData) == 0 {
		return fmt.Errorf("event_data is required")
	}
	if e.RetentionDays <= 0 {
		return fmt.Errorf("retention_days must be positive, got %d", e.RetentionDays)
	}
	return nil
}
