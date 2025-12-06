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

// Package metrics provides Prometheus metrics for the AIAnalysis controller.
// DD-005 Compliant: All metrics follow {service}_{component}_{metric_name}_{unit} naming convention.
//
// METRIC SELECTION CRITERIA (v1.13):
// - Only BUSINESS VALUE metrics are included (throughput, SLA, quality, failures)
// - Operational/debugging metrics removed (phase transitions, latency breakdowns)
// - Client-side HAPI metrics removed (HAPI tracks its own server-side metrics)
// - See: IMPLEMENTATION_PLAN_V1.0.md v1.13 for rationale
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ========================================
	// RECONCILER METRICS (aianalysis_reconciler_*)
	// BR-AI-OBSERVABILITY-001: Track reconciliation outcomes
	// Business Value: HIGH - Throughput and SLA tracking
	// ========================================

	// ReconcilerReconciliationsTotal tracks total reconciliations
	// Business: "How many analyses completed?" - Throughput SLA
	ReconcilerReconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_reconciler_reconciliations_total",
			Help: "Total number of AIAnalysis reconciliations",
		},
		[]string{"phase", "result"},
	)

	// ReconcilerDurationSeconds tracks reconciliation duration
	// Business: "Did we meet <60s SLA target?" - Latency SLA
	ReconcilerDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_reconciler_duration_seconds",
			Help:    "Duration of AIAnalysis reconciliation",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"phase"},
	)

	// ========================================
	// REGO POLICY METRICS (aianalysis_rego_*)
	// BR-AI-030: Track policy evaluation outcomes
	// Business Value: HIGH - Policy decision tracking
	// ========================================

	// RegoEvaluationsTotal tracks policy evaluations
	// Business: "How many policy decisions were made?"
	RegoEvaluationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_rego_evaluations_total",
			Help: "Total number of Rego policy evaluations",
		},
		[]string{"outcome", "degraded"},
	)

	// ========================================
	// APPROVAL METRICS (aianalysis_approval_*)
	// BR-AI-059: Track approval decisions
	// Business Value: HIGH - Core business outcome
	// ========================================

	// ApprovalDecisionsTotal tracks approval decisions
	// Business: "What's the approval vs auto-execute ratio?"
	ApprovalDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_approval_decisions_total",
			Help: "Total number of approval decisions",
		},
		[]string{"decision", "environment"},
	)

	// ========================================
	// CONFIDENCE METRICS (aianalysis_confidence_*)
	// BR-AI-OBSERVABILITY-004: Track AI confidence distribution
	// Business Value: HIGH - AI quality/reliability tracking
	// ========================================

	// ConfidenceScoreDistribution tracks confidence score distribution
	// Business: "How reliable is the AI model?"
	ConfidenceScoreDistribution = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_confidence_score_distribution",
			Help:    "Distribution of AI confidence scores",
			Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
		},
		[]string{"signal_type"},
	)

	// ========================================
	// FAILURE METRICS (aianalysis_failures_*)
	// BR-HAPI-197: Track workflow resolution failures
	// Business Value: HIGH - Failure mode tracking
	// ========================================

	// FailuresTotal tracks failures by reason and sub_reason
	// Business: "What failure modes are occurring?"
	FailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_failures_total",
			Help: "Total number of AIAnalysis failures",
		},
		[]string{"reason", "sub_reason"},
	)

	// ========================================
	// AUDIT METRICS (aianalysis_audit_*)
	// DD-HAPI-002 v1.4: LLM validation attempt audit trail
	// Business Value: MEDIUM - Compliance/audit requirement
	// ========================================

	// ValidationAttemptsTotal tracks HAPI in-session LLM retry attempts
	// Audit: "How many LLM self-correction attempts occurred?"
	ValidationAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_audit_validation_attempts_total",
			Help: "Total number of HAPI in-session validation attempts (max 3 per request)",
		},
		[]string{"workflow_id", "is_valid"},
	)

	// ========================================
	// DATA QUALITY METRICS (aianalysis_quality_*)
	// DD-WORKFLOW-001: Track label detection failures
	// Business Value: MEDIUM - Data quality tracking
	// ========================================

	// DetectedLabelsFailuresTotal tracks detection failures
	// Quality: "Are there enrichment/detection issues?"
	DetectedLabelsFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_quality_detected_labels_failures_total",
			Help: "Total number of failed label detections",
		},
		[]string{"field_name"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		// Business metrics (6)
		ReconcilerReconciliationsTotal,
		ReconcilerDurationSeconds,
		RegoEvaluationsTotal,
		ApprovalDecisionsTotal,
		ConfidenceScoreDistribution,
		FailuresTotal,
		// Audit/Quality metrics (2)
		ValidationAttemptsTotal,
		DetectedLabelsFailuresTotal,
	)
}

// ========================================
// HELPER FUNCTIONS
// Business-focused helpers for recording metrics
// ========================================

// RecordReconciliation records a reconciliation outcome
// Business: Throughput tracking
func RecordReconciliation(phase, result string) {
	ReconcilerReconciliationsTotal.WithLabelValues(phase, result).Inc()
}

// RecordReconcileDuration records reconciliation duration
// Business: SLA tracking (<60s target)
func RecordReconcileDuration(phase string, durationSeconds float64) {
	ReconcilerDurationSeconds.WithLabelValues(phase).Observe(durationSeconds)
}

// RecordRegoEvaluation records a Rego policy evaluation
// Business: Policy decision tracking
func RecordRegoEvaluation(outcome string, degraded bool) {
	degradedStr := "false"
	if degraded {
		degradedStr = "true"
	}
	RegoEvaluationsTotal.WithLabelValues(outcome, degradedStr).Inc()
}

// RecordApprovalDecision records an approval decision
// Business: Approval vs auto-execute ratio
func RecordApprovalDecision(decision, environment string) {
	ApprovalDecisionsTotal.WithLabelValues(decision, environment).Inc()
}

// RecordConfidenceScore records an AI confidence score
// Business: AI quality/reliability tracking
func RecordConfidenceScore(signalType string, confidence float64) {
	ConfidenceScoreDistribution.WithLabelValues(signalType).Observe(confidence)
}

// RecordFailure records an AIAnalysis failure
// Business: Failure mode tracking (BR-HAPI-197)
func RecordFailure(reason, subReason string) {
	FailuresTotal.WithLabelValues(reason, subReason).Inc()
}

// RecordValidationAttempt records a HAPI in-session validation attempt
// Audit: LLM self-correction tracking (DD-HAPI-002 v1.4)
func RecordValidationAttempt(workflowID string, isValid bool) {
	isValidStr := "false"
	if isValid {
		isValidStr = "true"
	}
	ValidationAttemptsTotal.WithLabelValues(workflowID, isValidStr).Inc()
}

// RecordDetectedLabelsFailure records a label detection failure
// Quality: Data quality tracking
func RecordDetectedLabelsFailure(fieldName string) {
	DetectedLabelsFailuresTotal.WithLabelValues(fieldName).Inc()
}

