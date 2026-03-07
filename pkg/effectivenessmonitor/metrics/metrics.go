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
	// Component assessment metrics
	MetricNameComponentAssessmentsTotal = "kubernaut_effectivenessmonitor_component_assessments_total"
	MetricNameComponentScores           = "kubernaut_effectivenessmonitor_component_scores"

	// Assessment outcome metrics
	MetricNameAssessmentsCompletedTotal = "kubernaut_effectivenessmonitor_assessments_completed_total"

	// External dependency metrics
	MetricNameExternalCallErrors = "kubernaut_effectivenessmonitor_external_call_errors_total"

	// Validity window metrics
	MetricNameValidityExpirationsTotal = "kubernaut_effectivenessmonitor_validity_expirations_total"
)

// Metrics holds all Prometheus metrics for the Effectiveness Monitor controller.
type Metrics struct {
	// ComponentAssessmentsTotal counts component assessment completions.
	ComponentAssessmentsTotal *prometheus.CounterVec
	// ComponentScores tracks component score distribution.
	ComponentScores *prometheus.HistogramVec

	// AssessmentsCompletedTotal counts full assessment completions by reason.
	AssessmentsCompletedTotal *prometheus.CounterVec

	// ExternalCallErrors counts external service errors.
	ExternalCallErrors *prometheus.CounterVec

	// ValidityExpirationsTotal counts assessments that expired.
	ValidityExpirationsTotal prometheus.Counter
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
		ComponentAssessmentsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameComponentAssessmentsTotal,
				Help: "Total number of component assessments completed.",
			},
			[]string{"component", "result"}, // component: health/alert/metrics/hash; result: success/error/skipped
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
	}

	// Register all metrics
	registry.MustRegister(
		m.ComponentAssessmentsTotal,
		m.ComponentScores,
		m.AssessmentsCompletedTotal,
		m.ExternalCallErrors,
		m.ValidityExpirationsTotal,
	)

	return m
}

// RecordComponentAssessment records a component assessment result.
func (m *Metrics) RecordComponentAssessment(component, result string, score *float64) {
	m.ComponentAssessmentsTotal.WithLabelValues(component, result).Inc()
	if score != nil {
		m.ComponentScores.WithLabelValues(component).Observe(*score)
	}
}

// RecordAssessmentCompleted records a full assessment completion.
func (m *Metrics) RecordAssessmentCompleted(reason string) {
	m.AssessmentsCompletedTotal.WithLabelValues(reason).Inc()
}

// RecordExternalCallError records an external service call error.
func (m *Metrics) RecordExternalCallError(service, operation, errorType string) {
	m.ExternalCallErrors.WithLabelValues(service, operation, errorType).Inc()
}

// RecordValidityExpiration records an assessment that expired.
func (m *Metrics) RecordValidityExpiration() {
	m.ValidityExpirationsTotal.Inc()
}
