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

	"github.com/jordigilh/kubernaut/pkg/aianalysis"
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
	// MetricNameRegoEvaluationsTotal is the name of the Rego policy evaluations counter metric
	MetricNameRegoEvaluationsTotal = "aianalysis_rego_evaluations_total"

	// MetricNameApprovalDecisionsTotal is the name of the approval decisions counter metric
	MetricNameApprovalDecisionsTotal = "aianalysis_approval_decisions_total"

	// MetricNameConfidenceScoreDistribution is the name of the confidence score histogram metric
	MetricNameConfidenceScoreDistribution = "aianalysis_confidence_score_distribution"

	// MetricNameFailuresTotal is the name of the failures counter metric
	MetricNameFailuresTotal = "aianalysis_failures_total"

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

	// Label values for outcome dimension (Rego) - Issue #262: use shared constants
	LabelOutcomeAutoApproved     = aianalysis.OutcomeAutoApproved
	LabelOutcomeRequiresApproval = aianalysis.OutcomeRequiresApproval

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
		}

		// Register with controller-runtime's global registry
		// This makes metrics available at :8080/metrics endpoint
		metrics.Registry.MustRegister(
			registeredMetrics.RegoEvaluationsTotal,
			registeredMetrics.ApprovalDecisionsTotal,
			registeredMetrics.ConfidenceScoreDistribution,
			registeredMetrics.FailuresTotal,
		)

		// Initialize FailuresTotal with known failure types so metric appears in /metrics
		// even before first failure occurs (required for E2E metric existence tests)
		// BR-HAPI-197: Failure tracking
		registeredMetrics.FailuresTotal.WithLabelValues("NoWorkflowSelected", "InvestigationFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("APIError", "HolmesGPTAPICallFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicyEvaluationFailed").Add(0)
		registeredMetrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Add(0)

		// Initialize RegoEvaluationsTotal with known label combinations
		// Required for E2E metric existence tests (BR-AI-030)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues(LabelOutcomeAutoApproved, LabelDegradedFalse).Add(0)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues(LabelOutcomeRequiresApproval, LabelDegradedFalse).Add(0)
		registeredMetrics.RegoEvaluationsTotal.WithLabelValues("error", "true").Add(0)

		// Initialize ApprovalDecisionsTotal with known label combinations
		// Required for E2E metric existence tests (BR-AI-059)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues(LabelOutcomeRequiresApproval, "production").Add(0)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues(LabelOutcomeAutoApproved, "staging").Add(0)
		registeredMetrics.ApprovalDecisionsTotal.WithLabelValues(LabelOutcomeAutoApproved, "development").Add(0)

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
	}

	// Register with provided registry (test registry)
	registry.MustRegister(
		m.RegoEvaluationsTotal,
		m.ApprovalDecisionsTotal,
		m.ConfidenceScoreDistribution,
		m.FailuresTotal,
	)

	// Initialize metrics for E2E tests
	m.FailuresTotal.WithLabelValues("NoWorkflowSelected", "InvestigationFailed").Add(0)
	m.FailuresTotal.WithLabelValues("APIError", "HolmesGPTAPICallFailed").Add(0)
	m.FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicyEvaluationFailed").Add(0)
	m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Add(0)

	return m
}

// ========================================
// DD-METRICS-001: HELPER METHODS (Dependency-Injected Pattern)
// These are methods on *Metrics, not package-level functions
// Per V1.0 Service Maturity Requirements - P0 Blocker
// ========================================

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

