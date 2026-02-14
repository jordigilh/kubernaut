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

// Package metrics provides Prometheus metrics for the Effectiveness Monitor.
// Per DD-METRICS-001: Uses dependency injection pattern for metrics wiring.
// All metrics follow DD-005 naming convention: kubernaut_effectivenessmonitor_<metric_name>
//
// Business Requirements:
// - BR-EM-009: Operational observability metrics for EM controller
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// DD-005 V3.0: Metric Name Constants (Pattern B)
// ========================================
//
// Per DD-005 V3.0 mandate, all metric names MUST be defined as constants.
// Pattern B: Full metric names (no Namespace/Subsystem in prometheus.Opts).
// ========================================

const (
	// Core reconciliation metrics
	MetricNameReconcileTotal    = "kubernaut_effectivenessmonitor_reconcile_total"
	MetricNameReconcileDuration = "kubernaut_effectivenessmonitor_reconcile_duration_seconds"

	// Phase transition metrics
	MetricNamePhaseTransitionsTotal = "kubernaut_effectivenessmonitor_phase_transitions_total"

	// Component assessment metrics
	MetricNameComponentAssessmentsTotal    = "kubernaut_effectivenessmonitor_component_assessments_total"
	MetricNameComponentAssessmentDuration  = "kubernaut_effectivenessmonitor_component_assessment_duration_seconds"
	MetricNameComponentScores             = "kubernaut_effectivenessmonitor_component_scores"

	// Assessment outcome metrics
	MetricNameAssessmentsCompletedTotal = "kubernaut_effectivenessmonitor_assessments_completed_total"

	// External dependency metrics
	MetricNameExternalCallsTotal    = "kubernaut_effectivenessmonitor_external_calls_total"
	MetricNameExternalCallDuration  = "kubernaut_effectivenessmonitor_external_call_duration_seconds"
	MetricNameExternalCallErrors    = "kubernaut_effectivenessmonitor_external_call_errors_total"

	// Validity window metrics
	MetricNameValidityExpirationsTotal = "kubernaut_effectivenessmonitor_validity_expirations_total"
	MetricNameStabilizationWaitsTotal  = "kubernaut_effectivenessmonitor_stabilization_waits_total"

	// Audit event metrics
	MetricNameAuditEventsTotal = "kubernaut_effectivenessmonitor_audit_events_total"

	// K8s event metrics
	MetricNameK8sEventsTotal = "kubernaut_effectivenessmonitor_k8s_events_total"
)

// Metrics holds all Prometheus metrics for the Effectiveness Monitor controller.
type Metrics struct {
	// ReconcileTotal counts total reconciliation attempts.
	ReconcileTotal *prometheus.CounterVec
	// ReconcileDuration tracks reconciliation duration.
	ReconcileDuration *prometheus.HistogramVec

	// PhaseTransitionsTotal counts phase transitions.
	PhaseTransitionsTotal *prometheus.CounterVec

	// ComponentAssessmentsTotal counts component assessment completions.
	ComponentAssessmentsTotal *prometheus.CounterVec
	// ComponentAssessmentDuration tracks per-component assessment duration.
	ComponentAssessmentDuration *prometheus.HistogramVec
	// ComponentScores tracks component score distribution.
	ComponentScores *prometheus.HistogramVec

	// AssessmentsCompletedTotal counts full assessment completions by reason.
	AssessmentsCompletedTotal *prometheus.CounterVec

	// ExternalCallsTotal counts external service calls (Prometheus, AlertManager).
	ExternalCallsTotal *prometheus.CounterVec
	// ExternalCallDuration tracks external service call duration.
	ExternalCallDuration *prometheus.HistogramVec
	// ExternalCallErrors counts external service errors.
	ExternalCallErrors *prometheus.CounterVec

	// ValidityExpirationsTotal counts assessments that expired.
	ValidityExpirationsTotal prometheus.Counter
	// StabilizationWaitsTotal counts requeues during stabilization.
	StabilizationWaitsTotal prometheus.Counter

	// AuditEventsTotal counts audit events emitted.
	AuditEventsTotal *prometheus.CounterVec

	// K8sEventsTotal counts Kubernetes events emitted.
	K8sEventsTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all EM metrics with the controller-runtime registry.
// Used in production (cmd/effectivenessmonitor/main.go).
func NewMetrics() *Metrics {
	return NewMetricsWithRegistry(ctrlmetrics.Registry)
}

// NewMetricsWithRegistry creates and registers all EM metrics with a custom registry.
// Used in tests to avoid global registry pollution.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		ReconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcileTotal,
				Help: "Total number of reconciliation attempts.",
			},
			[]string{"result"}, // success, error, requeue
		),
		ReconcileDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameReconcileDuration,
				Help:    "Duration of reconciliation in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		),
		PhaseTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNamePhaseTransitionsTotal,
				Help: "Total number of phase transitions.",
			},
			[]string{"from", "to"},
		),
		ComponentAssessmentsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameComponentAssessmentsTotal,
				Help: "Total number of component assessments completed.",
			},
			[]string{"component", "result"}, // component: health/alert/metrics/hash; result: success/error/skipped
		),
		ComponentAssessmentDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameComponentAssessmentDuration,
				Help:    "Duration of individual component assessments in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"component"},
		),
		ComponentScores: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameComponentScores,
				Help:    "Distribution of component scores (0.0-1.0).",
				Buckets: []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"component"},
		),
		AssessmentsCompletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAssessmentsCompletedTotal,
				Help: "Total number of assessments completed by reason.",
			},
			[]string{"reason"}, // full, partial, expired, no_execution, metrics_timed_out, spec_drift
		),
		ExternalCallsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameExternalCallsTotal,
				Help: "Total number of external service calls.",
			},
			[]string{"service", "operation"}, // service: prometheus/alertmanager; operation: query/ready/alerts
		),
		ExternalCallDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameExternalCallDuration,
				Help:    "Duration of external service calls in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "operation"},
		),
		ExternalCallErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameExternalCallErrors,
				Help: "Total number of external service call errors.",
			},
			[]string{"service", "operation", "error_type"}, // error_type: timeout/connection/http_error
		),
		ValidityExpirationsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameValidityExpirationsTotal,
				Help: "Total number of assessments that expired.",
			},
		),
		StabilizationWaitsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameStabilizationWaitsTotal,
				Help: "Total number of requeues during stabilization window.",
			},
		),
		AuditEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuditEventsTotal,
				Help: "Total number of audit events emitted.",
			},
			[]string{"event_type", "result"}, // event_type: effectiveness.*; result: success/error
		),
		K8sEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameK8sEventsTotal,
				Help: "Total number of Kubernetes events emitted.",
			},
			[]string{"type", "reason"}, // type: Normal/Warning; reason: AssessmentStarted/etc.
		),
	}

	// Register all metrics
	registry.MustRegister(
		m.ReconcileTotal,
		m.ReconcileDuration,
		m.PhaseTransitionsTotal,
		m.ComponentAssessmentsTotal,
		m.ComponentAssessmentDuration,
		m.ComponentScores,
		m.AssessmentsCompletedTotal,
		m.ExternalCallsTotal,
		m.ExternalCallDuration,
		m.ExternalCallErrors,
		m.ValidityExpirationsTotal,
		m.StabilizationWaitsTotal,
		m.AuditEventsTotal,
		m.K8sEventsTotal,
	)

	return m
}

// RecordReconcile records a reconciliation attempt.
func (m *Metrics) RecordReconcile(result string, durationSeconds float64) {
	m.ReconcileTotal.WithLabelValues(result).Inc()
	m.ReconcileDuration.WithLabelValues(result).Observe(durationSeconds)
}

// RecordPhaseTransition records a phase transition.
func (m *Metrics) RecordPhaseTransition(from, to string) {
	m.PhaseTransitionsTotal.WithLabelValues(from, to).Inc()
}

// RecordComponentAssessment records a component assessment result.
func (m *Metrics) RecordComponentAssessment(component, result string, durationSeconds float64, score *float64) {
	m.ComponentAssessmentsTotal.WithLabelValues(component, result).Inc()
	m.ComponentAssessmentDuration.WithLabelValues(component).Observe(durationSeconds)
	if score != nil {
		m.ComponentScores.WithLabelValues(component).Observe(*score)
	}
}

// RecordAssessmentCompleted records a full assessment completion.
func (m *Metrics) RecordAssessmentCompleted(reason string) {
	m.AssessmentsCompletedTotal.WithLabelValues(reason).Inc()
}

// RecordExternalCall records an external service call.
func (m *Metrics) RecordExternalCall(service, operation string, durationSeconds float64) {
	m.ExternalCallsTotal.WithLabelValues(service, operation).Inc()
	m.ExternalCallDuration.WithLabelValues(service, operation).Observe(durationSeconds)
}

// RecordExternalCallError records an external service call error.
func (m *Metrics) RecordExternalCallError(service, operation, errorType string) {
	m.ExternalCallErrors.WithLabelValues(service, operation, errorType).Inc()
}

// RecordValidityExpiration records an assessment that expired.
func (m *Metrics) RecordValidityExpiration() {
	m.ValidityExpirationsTotal.Inc()
}

// RecordStabilizationWait records a requeue during stabilization.
func (m *Metrics) RecordStabilizationWait() {
	m.StabilizationWaitsTotal.Inc()
}

// RecordAuditEvent records an audit event emission.
func (m *Metrics) RecordAuditEvent(eventType, result string) {
	m.AuditEventsTotal.WithLabelValues(eventType, result).Inc()
}

// RecordK8sEvent records a Kubernetes event emission.
func (m *Metrics) RecordK8sEvent(eventType, reason string) {
	m.K8sEventsTotal.WithLabelValues(eventType, reason).Inc()
}
