/*
Copyright 2026 Jordi Gil.

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

// Package audit provides audit event construction for the Effectiveness Monitor.
// Each assessment component emits a typed audit event to DataStorage via the
// buffered audit store (pkg/audit). Events follow DD-AUDIT-CORRELATION-002.
//
// Business Requirements:
// - BR-AUDIT-006: SOC 2 CC7.2 audit trail completeness
// - BR-EM-005: Component-level audit events
//
// Audit Event Types:
//   - effectiveness.assessment.scheduled
//   - effectiveness.health.assessed
//   - effectiveness.hash.computed
//   - effectiveness.alert.assessed
//   - effectiveness.metrics.assessed
//   - effectiveness.assessment.completed
package audit

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// EventData contains the common fields for all EM audit events.
// Per DD-AUDIT-CORRELATION-002: correlation_id links events to the RR.
type EventData struct {
	// CorrelationID is the name of the parent RemediationRequest.
	CorrelationID string
	// AssessmentName is the name of the EffectivenessAssessment CRD.
	AssessmentName string
	// Namespace is the namespace of the EA.
	Namespace string
	// Timestamp is when the event occurred.
	Timestamp time.Time
}

// HealthEvent contains the audit payload for a health assessment.
type HealthEvent struct {
	EventData
	// Score is the health check score (0.0-1.0).
	Score *float64
	// TotalReplicas is the total number of desired replicas.
	TotalReplicas int32
	// ReadyReplicas is the number of ready replicas.
	ReadyReplicas int32
	// RestartsSinceRemediation is the restart count since remediation.
	RestartsSinceRemediation int32
}

// HashEvent contains the audit payload for a hash computation (DD-EM-002).
type HashEvent struct {
	EventData
	// PostRemediationSpecHash is the computed hash of the target spec after remediation.
	PostRemediationSpecHash string
	// PreRemediationSpecHash is the hash from before remediation (from DS audit trail).
	// Empty string if not available.
	PreRemediationSpecHash string
	// Match indicates whether pre and post hashes are identical.
	// nil if no pre-hash was available (comparison not possible).
	// true if hashes match (no spec change detected).
	// false if hashes differ (spec changed).
	Match *bool
}

// AlertEvent contains the audit payload for an alert resolution check.
type AlertEvent struct {
	EventData
	// Score is the alert resolution score (0.0 or 1.0).
	Score *float64
	// AlertName is the name of the checked alert.
	AlertName string
	// AlertResolved indicates whether the alert has resolved.
	AlertResolved bool
}

// MetricsEvent contains the audit payload for a metric comparison.
type MetricsEvent struct {
	EventData
	// Score is the metric comparison score (0.0-1.0).
	Score *float64
	// QueriesExecuted is the number of PromQL queries run.
	QueriesExecuted int
	// Details provides human-readable summary of metric comparison.
	Details string
}

// CompletedEvent contains the audit payload for the overall assessment completion.
type CompletedEvent struct {
	EventData
	// Components contains results for each assessed component.
	Components []types.ComponentResult
	// Reason is the assessment outcome reason (full, partial, expired, etc.).
	Reason string
	// Message is the human-readable summary.
	Message string
}

// ScheduledEvent contains the audit payload for the assessment timeline computation.
// Emitted on first reconciliation when all derived timing fields are computed (BR-EM-009.4).
type ScheduledEvent struct {
	EventData
	// ValidityDeadline is the computed absolute expiry time.
	ValidityDeadline time.Time
	// PrometheusCheckAfter is the computed earliest time for Prometheus checks.
	PrometheusCheckAfter time.Time
	// AlertManagerCheckAfter is the computed earliest time for AlertManager checks.
	AlertManagerCheckAfter time.Time
	// ValidityWindow is the duration from EM config used to compute ValidityDeadline.
	ValidityWindow time.Duration
	// StabilizationWindow is the duration from EA spec used to compute check-after times.
	StabilizationWindow time.Duration
}

// Builder constructs audit events from assessment results.
// This is unit-testable pure logic (no I/O).
type Builder interface {
	// BuildHealthEvent creates an audit event for health assessment.
	BuildHealthEvent(data EventData, score *float64, totalReplicas, readyReplicas, restarts int32) HealthEvent

	// BuildHashEvent creates an audit event for hash computation (DD-EM-002).
	// Parameters: postHash is the post-remediation hash, preHash is the pre-remediation hash
	// (empty if unavailable), match indicates whether pre/post match (nil if no pre-hash).
	BuildHashEvent(data EventData, postHash, preHash string, match *bool) HashEvent

	// BuildAlertEvent creates an audit event for alert resolution.
	BuildAlertEvent(data EventData, score *float64, alertName string, resolved bool) AlertEvent

	// BuildMetricsEvent creates an audit event for metric comparison.
	BuildMetricsEvent(data EventData, score *float64, queriesExecuted int, details string) MetricsEvent

	// BuildCompletedEvent creates an audit event for overall assessment completion.
	BuildCompletedEvent(data EventData, components []types.ComponentResult, reason, message string) CompletedEvent

	// BuildScheduledEvent creates an audit event for assessment timeline computation (BR-EM-009.4).
	BuildScheduledEvent(data EventData, validityDeadline, prometheusCheckAfter, alertManagerCheckAfter time.Time, validityWindow, stabilizationWindow time.Duration) ScheduledEvent
}

// builder is the concrete implementation of Builder.
type builder struct{}

// NewBuilder creates a new audit event builder.
func NewBuilder() Builder {
	return &builder{}
}

// BuildHealthEvent creates an audit event for health assessment.
func (b *builder) BuildHealthEvent(data EventData, score *float64, totalReplicas, readyReplicas, restarts int32) HealthEvent {
	return HealthEvent{
		EventData:               data,
		Score:                   score,
		TotalReplicas:           totalReplicas,
		ReadyReplicas:           readyReplicas,
		RestartsSinceRemediation: restarts,
	}
}

// BuildHashEvent creates an audit event for hash computation (DD-EM-002).
func (b *builder) BuildHashEvent(data EventData, postHash, preHash string, match *bool) HashEvent {
	return HashEvent{
		EventData:               data,
		PostRemediationSpecHash: postHash,
		PreRemediationSpecHash:  preHash,
		Match:                   match,
	}
}

// BuildAlertEvent creates an audit event for alert resolution.
func (b *builder) BuildAlertEvent(data EventData, score *float64, alertName string, resolved bool) AlertEvent {
	return AlertEvent{
		EventData:     data,
		Score:         score,
		AlertName:     alertName,
		AlertResolved: resolved,
	}
}

// BuildMetricsEvent creates an audit event for metric comparison.
func (b *builder) BuildMetricsEvent(data EventData, score *float64, queriesExecuted int, details string) MetricsEvent {
	return MetricsEvent{
		EventData:       data,
		Score:           score,
		QueriesExecuted: queriesExecuted,
		Details:         details,
	}
}

// BuildCompletedEvent creates an audit event for overall assessment completion.
func (b *builder) BuildCompletedEvent(data EventData, components []types.ComponentResult, reason, message string) CompletedEvent {
	return CompletedEvent{
		EventData:  data,
		Components: components,
		Reason:     reason,
		Message:    message,
	}
}

// BuildScheduledEvent creates an audit event for assessment timeline computation (BR-EM-009.4).
func (b *builder) BuildScheduledEvent(data EventData, validityDeadline, prometheusCheckAfter, alertManagerCheckAfter time.Time, validityWindow, stabilizationWindow time.Duration) ScheduledEvent {
	return ScheduledEvent{
		EventData:              data,
		ValidityDeadline:       validityDeadline,
		PrometheusCheckAfter:   prometheusCheckAfter,
		AlertManagerCheckAfter: alertManagerCheckAfter,
		ValidityWindow:         validityWindow,
		StabilizationWindow:    stabilizationWindow,
	}
}
