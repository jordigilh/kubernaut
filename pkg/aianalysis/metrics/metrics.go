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
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// DD-005 V3.0: Metric Name Constants (MANDATORY)
// Per DD-005 Section 1.1: All services MUST define exported constants for metric names
// to prevent typos and ensure test/production parity
// Reference: docs/handoff/DD005_V3_METRIC_CONSTANTS_MANDATE_DEC_21_2025.md
// ========================================

// Metric name constants - DRY principle for tests and production
// These constants ensure tests use correct metric names and prevent typos
const (
	// MetricNameReconcilerReconciliationsTotal is the name of the reconciliations counter metric
	MetricNameReconcilerReconciliationsTotal = "aianalysis_reconciler_reconciliations_total"

	// MetricNameReconcilerDurationSeconds is the name of the reconciliation duration histogram metric
	MetricNameReconcilerDurationSeconds = "aianalysis_reconciler_duration_seconds"

	// MetricNameRegoEvaluationsTotal is the name of the Rego policy evaluations counter metric
	MetricNameRegoEvaluationsTotal = "aianalysis_rego_evaluations_total"

	// MetricNameApprovalDecisionsTotal is the name of the approval decisions counter metric
	MetricNameApprovalDecisionsTotal = "aianalysis_approval_decisions_total"

	// MetricNameConfidenceScoreDistribution is the name of the confidence score histogram metric
	MetricNameConfidenceScoreDistribution = "aianalysis_confidence_score_distribution"

	// MetricNameFailuresTotal is the name of the failures counter metric
	MetricNameFailuresTotal = "aianalysis_failures_total"

	// MetricNameValidationAttemptsTotal is the name of the HAPI validation attempts counter metric
	MetricNameValidationAttemptsTotal = "aianalysis_audit_validation_attempts_total"

	// MetricNameDetectedLabelsFailuresTotal is the name of the label detection failures counter metric
	MetricNameDetectedLabelsFailuresTotal = "aianalysis_quality_detected_labels_failures_total"

	// MetricNameRecoveryStatusPopulatedTotal is the name of the recovery status populated counter metric
	MetricNameRecoveryStatusPopulatedTotal = "aianalysis_recovery_status_populated_total"

	// MetricNameRecoveryStatusSkippedTotal is the name of the recovery status skipped counter metric
	MetricNameRecoveryStatusSkippedTotal = "aianalysis_recovery_status_skipped_total"
)

// Label value constants for common metric dimensions
const (
	// Label values for phase dimension
	LabelPhaseOverall      = "overall"
	LabelPhasePending      = "Pending"
	LabelPhaseInvestigating = "Investigating"
	LabelPhaseAnalyzing    = "Analyzing"
	LabelPhaseCompleted    = "Completed"
	LabelPhaseFailed       = "Failed"

	// Label values for result dimension
	LabelResultSuccess = "success"
	LabelResultError   = "error"

	// Label values for outcome dimension (Rego)
	LabelOutcomeApproved          = "approved"
	LabelOutcomeRequiresApproval  = "requires_approval"

	// Label values for degraded dimension
	LabelDegradedTrue  = "true"
	LabelDegradedFalse = "false"

	// Label values for decision dimension (Approval)
	LabelDecisionAutoExecute   = "auto_execute"
	LabelDecisionRequireApproval = "require_approval"

	// Label values for boolean fields
	LabelBoolTrue  = "true"
	LabelBoolFalse = "false"
)

var (
	// registrationOnce ensures metrics are only registered once to prevent test panics
	registrationOnce sync.Once
	// registeredMetrics stores the singleton metrics instance
	registeredMetrics *Metrics
)

// ========================================
// DD-METRICS-001: Dependency-Injected Metrics Pattern
// Per V1.0 Service Maturity Requirements - P0 Blocker
// Metrics are created via NewMetrics() and injected to reconciler
// NOT global variables (anti-pattern)
// ========================================

// Metrics holds all Prometheus metrics for the AIAnalysis controller.
// Per DD-METRICS-001: Dependency injection pattern for testability and clarity.
type Metrics struct {
	// ========================================
	// RECONCILER METRICS (aianalysis_reconciler_*)
	// BR-AI-OBSERVABILITY-001: Track reconciliation outcomes
	// Business Value: HIGH - Throughput and SLA tracking
	// ========================================

	// ReconcilerReconciliationsTotal tracks total reconciliations
	// Business: "How many analyses completed?" - Throughput SLA
	ReconcilerReconciliationsTotal *prometheus.CounterVec

	// ReconcilerDurationSeconds tracks reconciliation duration
	// Business: "Did we meet <60s SLA target?" - Latency SLA
	ReconcilerDurationSeconds *prometheus.HistogramVec

	// ========================================
	// REGO POLICY METRICS (aianalysis_rego_*)
	// BR-AI-030: Track policy evaluation outcomes
	// Business Value: HIGH - Policy decision tracking
	// ========================================

	// RegoEvaluationsTotal tracks policy evaluations
	// Business: "How many policy decisions were made?"
	RegoEvaluationsTotal *prometheus.CounterVec

	// ========================================
	// APPROVAL METRICS (aianalysis_approval_*)
	// BR-AI-059: Track approval decisions
	// Business Value: HIGH - Core business outcome
	// ========================================

	// ApprovalDecisionsTotal tracks approval decisions
	// Business: "What's the approval vs auto-execute ratio?"
	ApprovalDecisionsTotal *prometheus.CounterVec

	// ========================================
	// CONFIDENCE METRICS (aianalysis_confidence_*)
	// BR-AI-OBSERVABILITY-004: Track AI confidence distribution
	// Business Value: HIGH - AI quality/reliability tracking
	// ========================================

	// ConfidenceScoreDistribution tracks confidence score distribution
	// Business: "How reliable is the AI model?"
	ConfidenceScoreDistribution *prometheus.HistogramVec

	// ========================================
	// FAILURE METRICS (aianalysis_failures_*)
	// BR-HAPI-197: Track workflow resolution failures
	// Business Value: HIGH - Failure mode tracking
	// ========================================

	// FailuresTotal tracks failures by reason and sub_reason
	// Business: "What failure modes are occurring?"
	FailuresTotal *prometheus.CounterVec

	// ========================================
	// AUDIT METRICS (aianalysis_audit_*)
	// DD-HAPI-002 v1.4: LLM validation attempt audit trail
	// Business Value: MEDIUM - Compliance/audit requirement
	// ========================================

	// ValidationAttemptsTotal tracks HAPI in-session LLM retry attempts
	// Audit: "How many LLM self-correction attempts occurred?"
	ValidationAttemptsTotal *prometheus.CounterVec

	// ========================================
	// DATA QUALITY METRICS (aianalysis_quality_*)
	// DD-WORKFLOW-001: Track label detection failures
	// Business Value: MEDIUM - Data quality tracking
	// ========================================

	// DetectedLabelsFailuresTotal tracks detection failures
	// Quality: "Are there enrichment/detection issues?"
	DetectedLabelsFailuresTotal *prometheus.CounterVec

	// ========================================
	// RECOVERY METRICS (aianalysis_recovery_*)
	// BR-AI-082: Track RecoveryStatus population
	// Business Value: MEDIUM - Recovery observability
	// ========================================

	// RecoveryStatusPopulatedTotal tracks successful RecoveryStatus population
	// Business: "How many recovery attempts populate status?"
	RecoveryStatusPopulatedTotal *prometheus.CounterVec

	// RecoveryStatusSkippedTotal tracks when HAPI doesn't return recovery_analysis
	// Quality: "How often is HAPI not returning recovery analysis?"
	RecoveryStatusSkippedTotal prometheus.Counter
}

// ========================================
// DD-METRICS-001: NewMetrics Constructor
// Creates and registers metrics with controller-runtime
// Per V1.0 Service Maturity Requirements - P0 Blocker
// ========================================

// NewMetrics creates a new Metrics instance and registers with controller-runtime.
// Uses controller-runtime's metrics.Registry for automatic /metrics endpoint exposure.
// Per DD-METRICS-001: Dependency injection pattern for testability.
func NewMetrics() *Metrics {
	// P1.1 Refactoring Fix: Use sync.Once to prevent duplicate metrics registration panics in tests
	// sync.Once ensures metrics are registered only once even if NewMetrics() called multiple times
	registrationOnce.Do(func() {
		registeredMetrics = &Metrics{
		ReconcilerReconciliationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerReconciliationsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of AIAnalysis reconciliations",
			},
			[]string{"phase", "result"},
		),
		ReconcilerDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameReconcilerDurationSeconds, // DD-005 V3.0: Use constant
				Help:    "Duration of AIAnalysis reconciliation",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"phase"},
		),
		RegoEvaluationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameRegoEvaluationsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of Rego policy evaluations",
			},
			[]string{"outcome", "degraded"},
		),
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of approval decisions",
			},
			[]string{"decision", "environment"},
		),
		ConfidenceScoreDistribution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameConfidenceScoreDistribution, // DD-005 V3.0: Use constant
				Help:    "Distribution of AI confidence scores",
				Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
			},
			[]string{"signal_type"},
		),
		FailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameFailuresTotal, // DD-005 V3.0: Use constant
				Help: "Total number of AIAnalysis failures",
			},
			[]string{"reason", "sub_reason"},
		),
		ValidationAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameValidationAttemptsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of HAPI in-session validation attempts (max 3 per request)",
			},
			[]string{"workflow_id", "is_valid"},
		),
		DetectedLabelsFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDetectedLabelsFailuresTotal, // DD-005 V3.0: Use constant
				Help: "Total number of failed label detections",
			},
			[]string{"field_name"},
		),
		RecoveryStatusPopulatedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameRecoveryStatusPopulatedTotal, // DD-005 V3.0: Use constant
				Help: "Total number of times RecoveryStatus was populated from HAPI recovery_analysis",
			},
			[]string{"failure_understood", "state_changed"},
		),
		RecoveryStatusSkippedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameRecoveryStatusSkippedTotal, // DD-005 V3.0: Use constant
				Help: "Total number of times RecoveryStatus was skipped (nil recovery_analysis from HAPI)",
			},
		),
		}

		// Register with controller-runtime's global registry
		// This makes metrics available at :8080/metrics endpoint
		metrics.Registry.MustRegister(
			// Business metrics (6)
			registeredMetrics.ReconcilerReconciliationsTotal,
			registeredMetrics.ReconcilerDurationSeconds,
			registeredMetrics.RegoEvaluationsTotal,
			registeredMetrics.ApprovalDecisionsTotal,
			registeredMetrics.ConfidenceScoreDistribution,
			registeredMetrics.FailuresTotal,
			// Audit/Quality metrics (2)
			registeredMetrics.ValidationAttemptsTotal,
			registeredMetrics.DetectedLabelsFailuresTotal,
			// Recovery metrics (2)
			registeredMetrics.RecoveryStatusPopulatedTotal,
			registeredMetrics.RecoveryStatusSkippedTotal,
		)

		// Initialize FailuresTotal with known failure types so metric appears in /metrics
		// even before first failure occurs (required for E2E metric existence tests)
		// BR-HAPI-197: Failure tracking
		registeredMetrics.FailuresTotal.WithLabelValues("NoWorkflowSelected", "InvestigationFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("APIError", "HolmesGPTAPICallFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicyEvaluationFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("RecoveryWorkflowResolutionFailed", "NoRecoveryWorkflowResolved").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("RecoveryNotPossible", "NoRecoveryStrategy").Add(0)

		// Initialize RecoveryStatusPopulatedTotal with all label combinations
		// Required for E2E metric existence tests (BR-AI-082)
		registeredMetrics.RecoveryStatusPopulatedTotal.WithLabelValues("true", "true").Add(0)
		registeredMetrics.RecoveryStatusPopulatedTotal.WithLabelValues("true", "false").Add(0)
		registeredMetrics.RecoveryStatusPopulatedTotal.WithLabelValues("false", "true").Add(0)
		registeredMetrics.RecoveryStatusPopulatedTotal.WithLabelValues("false", "false").Add(0)

		// Initialize RecoveryStatusSkippedTotal
		// Required for E2E metric existence tests (BR-AI-082)
		registeredMetrics.RecoveryStatusSkippedTotal.Add(0)

		// Initialize RegoEvaluationsTotal with known label combinations
		// Required for E2E metric existence tests (BR-AI-030)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues("auto_approved", "false").Add(0)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues("requires_approval", "false").Add(0)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues("error", "true").Add(0)

		// Initialize ApprovalDecisionsTotal with known label combinations
		// Required for E2E metric existence tests (BR-AI-059)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues("requires_approval", "production").Add(0)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues("auto_approved", "staging").Add(0)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues("auto_approved", "development").Add(0)

		// Initialize ConfidenceScoreDistribution with common signal types
		// Required for E2E metric existence tests (BR-AI-OBSERVABILITY-004)
		// Histograms need at least one Observe() call to appear in metrics
		registeredMetrics.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0)
		registeredMetrics.ConfidenceScoreDistribution.WithLabelValues("CrashLoopBackOff").Observe(0)
		registeredMetrics.ConfidenceScoreDistribution.WithLabelValues("NodeNotReady").Observe(0)
	}) // End of sync.Once.Do()

	// Return the singleton instance (safe for concurrent access)
	return registeredMetrics
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing).
// Tests should use this to avoid polluting global registry.
// Per DD-METRICS-001: Test isolation via custom registry.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		ReconcilerReconciliationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerReconciliationsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of AIAnalysis reconciliations",
			},
			[]string{"phase", "result"},
		),
		ReconcilerDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameReconcilerDurationSeconds, // DD-005 V3.0: Use constant
				Help:    "Duration of AIAnalysis reconciliation",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"phase"},
		),
		RegoEvaluationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameRegoEvaluationsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of Rego policy evaluations",
			},
			[]string{"outcome", "degraded"},
		),
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of approval decisions",
			},
			[]string{"decision", "environment"},
		),
		ConfidenceScoreDistribution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameConfidenceScoreDistribution, // DD-005 V3.0: Use constant
				Help:    "Distribution of AI confidence scores",
				Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
			},
			[]string{"signal_type"},
		),
		FailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameFailuresTotal, // DD-005 V3.0: Use constant
				Help: "Total number of AIAnalysis failures",
			},
			[]string{"reason", "sub_reason"},
		),
		ValidationAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameValidationAttemptsTotal, // DD-005 V3.0: Use constant
				Help: "Total number of HAPI in-session validation attempts (max 3 per request)",
			},
			[]string{"workflow_id", "is_valid"},
		),
		DetectedLabelsFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDetectedLabelsFailuresTotal, // DD-005 V3.0: Use constant
				Help: "Total number of failed label detections",
			},
			[]string{"field_name"},
		),
		RecoveryStatusPopulatedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameRecoveryStatusPopulatedTotal, // DD-005 V3.0: Use constant
				Help: "Total number of times RecoveryStatus was populated from HAPI recovery_analysis",
			},
			[]string{"failure_understood", "state_changed"},
		),
		RecoveryStatusSkippedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameRecoveryStatusSkippedTotal, // DD-005 V3.0: Use constant
				Help: "Total number of times RecoveryStatus was skipped (nil recovery_analysis from HAPI)",
			},
		),
	}

	// Register with provided registry (test registry)
	registry.MustRegister(
		m.ReconcilerReconciliationsTotal,
		m.ReconcilerDurationSeconds,
		m.RegoEvaluationsTotal,
		m.ApprovalDecisionsTotal,
		m.ConfidenceScoreDistribution,
		m.FailuresTotal,
		m.ValidationAttemptsTotal,
		m.DetectedLabelsFailuresTotal,
		m.RecoveryStatusPopulatedTotal,
		m.RecoveryStatusSkippedTotal,
	)

	// Initialize metrics for E2E tests
	m.FailuresTotal.WithLabelValues("NoWorkflowSelected", "InvestigationFailed").Add(0)
	m.FailuresTotal.WithLabelValues("APIError", "HolmesGPTAPICallFailed").Add(0)
	m.FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicyEvaluationFailed").Add(0)
	m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Add(0)
	m.FailuresTotal.WithLabelValues("RecoveryWorkflowResolutionFailed", "NoRecoveryWorkflowResolved").Add(0)
	m.FailuresTotal.WithLabelValues("RecoveryNotPossible", "NoRecoveryStrategy").Add(0)

	m.RecoveryStatusPopulatedTotal.WithLabelValues("true", "true").Add(0)
	m.RecoveryStatusPopulatedTotal.WithLabelValues("true", "false").Add(0)
	m.RecoveryStatusPopulatedTotal.WithLabelValues("false", "true").Add(0)
	m.RecoveryStatusPopulatedTotal.WithLabelValues("false", "false").Add(0)

	m.RecoveryStatusSkippedTotal.Add(0)

	return m
}

// ========================================
// DD-METRICS-001: HELPER METHODS (Dependency-Injected Pattern)
// These are methods on *Metrics, not package-level functions
// Per V1.0 Service Maturity Requirements - P0 Blocker
// ========================================

// RecordReconciliation records a reconciliation outcome
// Business: Throughput tracking
func (m *Metrics) RecordReconciliation(phase, result string) {
	m.ReconcilerReconciliationsTotal.WithLabelValues(phase, result).Inc()
}

// RecordReconcileDuration records reconciliation duration
// Business: SLA tracking (<60s target)
func (m *Metrics) RecordReconcileDuration(phase string, durationSeconds float64) {
	m.ReconcilerDurationSeconds.WithLabelValues(phase).Observe(durationSeconds)
}

// RecordRegoEvaluation records a Rego policy evaluation
// Business: Policy decision tracking
func (m *Metrics) RecordRegoEvaluation(outcome string, degraded bool) {
	degradedStr := "false"
	if degraded {
		degradedStr = "true"
	}
	m.RegoEvaluationsTotal.WithLabelValues(outcome, degradedStr).Inc()
}

// RecordApprovalDecision records an approval decision
// Business: Approval vs auto-execute ratio
func (m *Metrics) RecordApprovalDecision(decision, environment string) {
	m.ApprovalDecisionsTotal.WithLabelValues(decision, environment).Inc()
}

// RecordConfidenceScore records an AI confidence score
// Business: AI quality/reliability tracking
func (m *Metrics) RecordConfidenceScore(signalType string, confidence float64) {
	m.ConfidenceScoreDistribution.WithLabelValues(signalType).Observe(confidence)
}

// RecordFailure records an AIAnalysis failure
// Business: Failure mode tracking (BR-HAPI-197)
func (m *Metrics) RecordFailure(reason, subReason string) {
	m.FailuresTotal.WithLabelValues(reason, subReason).Inc()
}

// RecordValidationAttempt records a HAPI in-session validation attempt
// Audit: LLM self-correction tracking (DD-HAPI-002 v1.4)
func (m *Metrics) RecordValidationAttempt(workflowID string, isValid bool) {
	isValidStr := "false"
	if isValid {
		isValidStr = "true"
	}
	m.ValidationAttemptsTotal.WithLabelValues(workflowID, isValidStr).Inc()
}

// RecordDetectedLabelsFailure records a label detection failure
// Quality: Data quality tracking
func (m *Metrics) RecordDetectedLabelsFailure(fieldName string) {
	m.DetectedLabelsFailuresTotal.WithLabelValues(fieldName).Inc()
}

// RecordRecoveryStatusPopulated records successful RecoveryStatus population
// Business: Recovery observability (BR-AI-082)
func (m *Metrics) RecordRecoveryStatusPopulated(failureUnderstood, stateChanged bool) {
	failureStr := boolToString(failureUnderstood)
	stateStr := boolToString(stateChanged)
	m.RecoveryStatusPopulatedTotal.WithLabelValues(failureStr, stateStr).Inc()
}

// RecordRecoveryStatusSkipped records when HAPI doesn't return recovery_analysis
// Quality: HAPI contract compliance tracking
func (m *Metrics) RecordRecoveryStatusSkipped() {
	m.RecoveryStatusSkippedTotal.Inc()
}

// boolToString converts bool to string for metric labels
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
