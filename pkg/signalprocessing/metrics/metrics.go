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

// Package metrics provides Prometheus metrics for the SignalProcessing controller.
//
// Per IMPLEMENTATION_PLAN_V1.21.md Day 1 and Day 3 specification:
//
// Metrics exposed:
//   - signalprocessing_processing_total: Counter for processing operations (labels: phase, result)
//   - signalprocessing_processing_duration_seconds: Histogram for operation duration (labels: phase)
//   - signalprocessing_enrichment_total: Counter for enrichment results (labels: result)
//   - signalprocessing_enrichment_duration_seconds: Histogram for K8s API enrichment latency (labels: resource_kind)
//   - signalprocessing_enrichment_errors_total: Counter for enrichment errors (labels: error_type)
//
// Usage:
//
//	registry := prometheus.NewRegistry()
//	m := metrics.NewMetrics(registry)
//	m.IncrementProcessingTotal("enriching", "success")
//	m.ObserveProcessingDuration("enriching", 0.5)
//	m.EnrichmentTotal.WithLabelValues("success").Inc()
//	m.EnrichmentDuration.WithLabelValues("k8s_context").Observe(0.5)
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)


// ========================================
// DD-005 V3.0: Metric Name Constants (Pattern B)
// ========================================
//
// Per DD-005 V3.0 mandate, all metric names MUST be defined as constants
// to prevent typos and ensure test/production parity.
//
// Pattern B: Full metric names (no Namespace/Subsystem in prometheus.Opts)
// Reference: pkg/workflowexecution/metrics/metrics.go
// ========================================

const (
	// Core business metrics
	MetricNameProcessingTotal    = "signalprocessing_processing_total"
	MetricNameProcessingDuration = "signalprocessing_processing_duration_seconds"

	// SLO metrics (Day 3 Enricher)
	MetricNameEnrichmentTotal    = "signalprocessing_enrichment_total"
	MetricNameEnrichmentDuration = "signalprocessing_enrichment_duration_seconds"

	// Operational metrics
	MetricNameEnrichmentErrors = "signalprocessing_enrichment_errors_total"
)

// Metrics holds Prometheus metrics for the SignalProcessing controller.
// Per IMPLEMENTATION_PLAN_V1.21.md metrics specification.
type Metrics struct {
	// === CORE BUSINESS METRICS ===
	ProcessingTotal    *prometheus.CounterVec
	ProcessingDuration *prometheus.HistogramVec

	// === SLO METRICS (Day 3 Enricher) ===

	// EnrichmentTotal counts K8s enrichment operations.
	// Labels: result (success, failure)
	EnrichmentTotal *prometheus.CounterVec

	// EnrichmentDuration measures K8s API enrichment latency.
	// Labels: resource_kind (k8s_context, pod, deployment, etc.)
	// SLO: <2 seconds P95 (BR-SP-001)
	EnrichmentDuration *prometheus.HistogramVec

	// === OPERATIONAL METRICS ===
	EnrichmentErrors *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with the global controller-runtime registry.
// Per AIAnalysis pattern: Uses ctrlmetrics.Registry (controller-runtime global registry)
// which enables ALL parallel processes to query metrics from Process 1's controller.
// For test isolation, use NewMetricsWithRegistry() with custom registry.
func NewMetrics() *Metrics {
	// Use controller-runtime global registry for production and integration tests
	// This registry is truly global and shared across all parallel Ginkgo processes (--procs=4)
	// Tests query ctrlmetrics.Registry.Gather() to access metrics from Process 1's controller
	return newMetricsInternal(ctrlmetrics.Registry)
}

// NewMetricsWithRegistry creates a new Metrics instance with the provided registry.
// This allows test isolation by providing a custom registry instead of the global one.
// Use this for unit tests or when you need strict test isolation.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	return newMetricsInternal(registry)
}

func newMetricsInternal(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		// Core business metrics
		ProcessingTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameProcessingTotal, // DD-005 V3.0: Pattern B,
				Help: "Total number of signal processing operations",
			},
			[]string{"phase", "result"},
		),
		ProcessingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameProcessingDuration, // DD-005 V3.0: Pattern B,
				Help:    "Duration of signal processing operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"phase"},
		),

		// SLO metrics (Day 3 Enricher)
		EnrichmentTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameEnrichmentTotal, // DD-005 V3.0: Pattern B,
				Help: "Total number of K8s enrichment operations",
			},
			[]string{"result"},
		),
		EnrichmentDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameEnrichmentDuration, // DD-005 V3.0: Pattern B,
				Help:    "Duration of K8s API enrichment operations",
				Buckets: prometheus.DefBuckets, // Will use SLO-specific buckets in production
			},
			[]string{"resource_kind"},
		),

		// Operational metrics
		EnrichmentErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameEnrichmentErrors, // DD-005 V3.0: Pattern B,
				Help: "Total number of enrichment errors",
			},
			[]string{"error_type"},
		),
	}

	registry.MustRegister(m.ProcessingTotal)
	registry.MustRegister(m.ProcessingDuration)
	registry.MustRegister(m.EnrichmentTotal)
	registry.MustRegister(m.EnrichmentDuration)
	registry.MustRegister(m.EnrichmentErrors)

	return m
}

// IncrementProcessingTotal increments the processing total counter.
func (m *Metrics) IncrementProcessingTotal(phase, result string) {
	m.ProcessingTotal.WithLabelValues(phase, result).Inc()
}

// RecordEnrichmentError records an enrichment error.
func (m *Metrics) RecordEnrichmentError(errorType string) {
	m.EnrichmentErrors.WithLabelValues(errorType).Inc()
}

// ObserveProcessingDuration records processing duration.
func (m *Metrics) ObserveProcessingDuration(phase string, durationSeconds float64) {
	m.ProcessingDuration.WithLabelValues(phase).Observe(durationSeconds)
}
