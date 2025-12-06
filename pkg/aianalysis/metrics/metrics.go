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
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ========================================
	// RECONCILER METRICS (aianalysis_reconciler_*)
	// BR-AI-OBSERVABILITY-001: Track reconciliation outcomes
	// ========================================

	// ReconcilerReconciliationsTotal tracks total reconciliations
	ReconcilerReconciliationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_reconciler_reconciliations_total",
			Help: "Total number of AIAnalysis reconciliations",
		},
		[]string{"phase", "result"},
	)

	// ReconcilerDurationSeconds tracks reconciliation duration
	ReconcilerDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_reconciler_duration_seconds",
			Help:    "Duration of AIAnalysis reconciliation",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"phase"},
	)

	// ReconcilerPhaseTransitionsTotal tracks phase transitions
	ReconcilerPhaseTransitionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_reconciler_phase_transitions_total",
			Help: "Total number of phase transitions",
		},
		[]string{"from_phase", "to_phase"},
	)

	// ReconcilerPhaseDurationSeconds tracks time spent in each phase
	ReconcilerPhaseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_reconciler_phase_duration_seconds",
			Help:    "Duration spent in each phase",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"phase"},
	)

	// ========================================
	// HOLMESGPT-API METRICS (aianalysis_holmesgpt_*)
	// BR-AI-006: Track API call metrics
	// ========================================

	// HolmesGPTRequestsTotal tracks API requests
	HolmesGPTRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_holmesgpt_requests_total",
			Help: "Total number of HolmesGPT-API requests",
		},
		[]string{"endpoint", "status_code"},
	)

	// HolmesGPTLatencySeconds tracks API latency
	HolmesGPTLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_holmesgpt_latency_seconds",
			Help:    "Latency of HolmesGPT-API calls",
			Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"endpoint"},
	)

	// HolmesGPTRetriesTotal tracks retry attempts
	HolmesGPTRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_holmesgpt_retries_total",
			Help: "Total number of HolmesGPT-API retry attempts",
		},
		[]string{"endpoint"},
	)

	// ValidationAttemptsTotal tracks HAPI in-session retry attempts
	// DD-HAPI-002 v1.4: HAPI retries up to 3 times with LLM self-correction
	ValidationAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_holmesgpt_validation_attempts_total",
			Help: "Total number of HAPI in-session validation attempts (max 3 per request)",
		},
		[]string{"workflow_id", "is_valid"},
	)

	// ========================================
	// REGO POLICY METRICS (aianalysis_rego_*)
	// BR-AI-030: Track policy evaluation outcomes
	// ========================================

	// RegoEvaluationsTotal tracks policy evaluations
	RegoEvaluationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_rego_evaluations_total",
			Help: "Total number of Rego policy evaluations",
		},
		[]string{"outcome", "degraded"},
	)

	// RegoLatencySeconds tracks evaluation latency
	RegoLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aianalysis_rego_latency_seconds",
			Help:    "Latency of Rego policy evaluations",
			Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.5},
		},
		[]string{},
	)

	// RegoReloadsTotal tracks policy reloads
	RegoReloadsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aianalysis_rego_reloads_total",
			Help: "Total number of Rego policy reloads",
		},
	)

	// ========================================
	// APPROVAL METRICS (aianalysis_approval_*)
	// BR-AI-059: Track approval decisions
	// ========================================

	// ApprovalDecisionsTotal tracks approval decisions
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
	// ========================================

	// ConfidenceScoreDistribution tracks confidence score distribution
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
	// ========================================

	// FailuresTotal tracks failures by reason and sub_reason
	FailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_failures_total",
			Help: "Total number of AIAnalysis failures",
		},
		[]string{"reason", "sub_reason"},
	)

	// ========================================
	// DETECTED LABELS METRICS (aianalysis_detected_labels_*)
	// DD-WORKFLOW-001: Track label detection failures
	// ========================================

	// DetectedLabelsFailuresTotal tracks detection failures
	DetectedLabelsFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aianalysis_detected_labels_failures_total",
			Help: "Total number of failed label detections",
		},
		[]string{"field_name"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ReconcilerReconciliationsTotal,
		ReconcilerDurationSeconds,
		ReconcilerPhaseTransitionsTotal,
		ReconcilerPhaseDurationSeconds,
		HolmesGPTRequestsTotal,
		HolmesGPTLatencySeconds,
		HolmesGPTRetriesTotal,
		ValidationAttemptsTotal,
		RegoEvaluationsTotal,
		RegoLatencySeconds,
		RegoReloadsTotal,
		ApprovalDecisionsTotal,
		ConfidenceScoreDistribution,
		FailuresTotal,
		DetectedLabelsFailuresTotal,
	)
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// RecordReconciliation records a reconciliation outcome
func RecordReconciliation(phase, result string) {
	ReconcilerReconciliationsTotal.WithLabelValues(phase, result).Inc()
}

// RecordReconcileDuration records reconciliation duration
func RecordReconcileDuration(phase string, durationSeconds float64) {
	ReconcilerDurationSeconds.WithLabelValues(phase).Observe(durationSeconds)
}

// RecordPhaseTransition records a phase transition
func RecordPhaseTransition(from, to string) {
	ReconcilerPhaseTransitionsTotal.WithLabelValues(from, to).Inc()
}

// RecordHolmesGPTRequest records a HolmesGPT-API request
func RecordHolmesGPTRequest(endpoint, statusCode string) {
	HolmesGPTRequestsTotal.WithLabelValues(endpoint, statusCode).Inc()
}

// RecordHolmesGPTLatency records HolmesGPT-API latency
func RecordHolmesGPTLatency(endpoint string, durationSeconds float64) {
	HolmesGPTLatencySeconds.WithLabelValues(endpoint).Observe(durationSeconds)
}

// RecordHolmesGPTRetry records a HolmesGPT-API retry attempt
func RecordHolmesGPTRetry(endpoint string) {
	HolmesGPTRetriesTotal.WithLabelValues(endpoint).Inc()
}

// RecordValidationAttempt records a HAPI in-session validation attempt
// DD-HAPI-002 v1.4: Track LLM self-correction attempts
func RecordValidationAttempt(workflowID string, isValid bool) {
	isValidStr := "false"
	if isValid {
		isValidStr = "true"
	}
	ValidationAttemptsTotal.WithLabelValues(workflowID, isValidStr).Inc()
}

// RecordRegoEvaluation records a Rego policy evaluation
func RecordRegoEvaluation(outcome string, degraded bool) {
	degradedStr := "false"
	if degraded {
		degradedStr = "true"
	}
	RegoEvaluationsTotal.WithLabelValues(outcome, degradedStr).Inc()
}

// RecordRegoLatency records Rego policy evaluation latency
func RecordRegoLatency(durationSeconds float64) {
	RegoLatencySeconds.WithLabelValues().Observe(durationSeconds)
}

// RecordApprovalDecision records an approval decision
func RecordApprovalDecision(decision, environment string) {
	ApprovalDecisionsTotal.WithLabelValues(decision, environment).Inc()
}

// RecordConfidenceScore records an AI confidence score
func RecordConfidenceScore(signalType string, confidence float64) {
	ConfidenceScoreDistribution.WithLabelValues(signalType).Observe(confidence)
}

// RecordFailure records an AIAnalysis failure
// BR-HAPI-197: Track failures with reason and sub_reason
func RecordFailure(reason, subReason string) {
	FailuresTotal.WithLabelValues(reason, subReason).Inc()
}

// RecordDetectedLabelsFailure records a label detection failure
func RecordDetectedLabelsFailure(fieldName string) {
	DetectedLabelsFailuresTotal.WithLabelValues(fieldName).Inc()
}

