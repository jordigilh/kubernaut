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

// Package types provides shared types for the Effectiveness Monitor.
//
// Business Requirements:
// - BR-EM-001 through BR-EM-008: Core EM assessment types
// - BR-AUDIT-006: Audit event types for SOC 2 compliance
package types

// ComponentType identifies which assessment component produced a result.
type ComponentType string

const (
	// ComponentHealth is the K8s health check component (BR-EM-001).
	ComponentHealth ComponentType = "health"
	// ComponentAlert is the AlertManager resolution check component (BR-EM-002).
	ComponentAlert ComponentType = "alert"
	// ComponentMetrics is the Prometheus metric comparison component (BR-EM-003).
	ComponentMetrics ComponentType = "metrics"
	// ComponentHash is the spec hash comparison component (BR-EM-004).
	ComponentHash ComponentType = "hash"
)

// ComponentResult represents the outcome of a single assessment component.
type ComponentResult struct {
	// Component identifies which check produced this result.
	Component ComponentType
	// Assessed indicates whether this component was successfully evaluated.
	Assessed bool
	// Score is the component score (0.0-1.0), nil if not assessed.
	Score *float64
	// Details provides human-readable details about the assessment.
	Details string
	// Error captures any error that occurred during assessment.
	Error error
}

// AssessmentOutcome aggregates results from all components.
type AssessmentOutcome struct {
	// Components contains results for each assessment component.
	Components []ComponentResult
	// Reason describes the overall assessment outcome.
	Reason string
	// Message provides human-readable summary.
	Message string
}

// AuditEventType identifies the type of audit event emitted by EM.
// Per DD-AUDIT-CORRELATION-002: Each component emits its own audit event.
type AuditEventType string

const (
	// AuditHealthAssessed is emitted when health check completes.
	AuditHealthAssessed AuditEventType = "effectiveness.health.assessed"
	// AuditHashComputed is emitted when spec hash comparison completes.
	AuditHashComputed AuditEventType = "effectiveness.hash.computed"
	// AuditAlertAssessed is emitted when alert resolution check completes.
	AuditAlertAssessed AuditEventType = "effectiveness.alert.assessed"
	// AuditMetricsAssessed is emitted when metric comparison completes.
	AuditMetricsAssessed AuditEventType = "effectiveness.metrics.assessed"
	// AuditAssessmentCompleted is emitted when the full assessment finishes.
	AuditAssessmentCompleted AuditEventType = "effectiveness.assessment.completed"
)
