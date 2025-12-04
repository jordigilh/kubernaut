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

// Package metrics provides Prometheus metrics for Signal Processing controller.
// DD-005 compliant metrics implementation.
// Metrics triaged for business value - see implementation plan v1.4.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "kubernaut"
	subsystem = "signalprocessing"
)

// Metrics holds all Prometheus metrics for Signal Processing.
// Triaged for business value:
// - Core business metrics: ReconciliationTotal, ReconciliationDuration, CategorizationConfidence
// - SLO metrics: EnrichmentDuration (<2s P95), RegoEvaluationDuration (<100ms P95)
// - Operational metrics: RegoHotReloadTotal
type Metrics struct {
	// === CORE BUSINESS METRICS ===

	// ReconciliationTotal tracks all reconciliation operations.
	// Labels: phase (enriching, classifying, detecting, complete, failed), status (success, failure)
	// Business value: Core throughput and success rate metric.
	ReconciliationTotal *prometheus.CounterVec

	// ReconciliationDuration measures end-to-end processing time.
	// Labels: phase
	// Business value: End-to-end latency for SLO tracking.
	ReconciliationDuration *prometheus.HistogramVec

	// CategorizationConfidence tracks confidence scores for all classifications.
	// Labels: classifier (environment, priority, business)
	// Business value: Are classifications reliable? Low confidence = review Rego policies.
	CategorizationConfidence *prometheus.HistogramVec

	// === SLO METRICS ===

	// EnrichmentDuration measures K8s API enrichment latency.
	// Labels: resource_kind (Pod, Deployment, Node, etc.)
	// SLO: <2 seconds P95 (BR-SP-001)
	EnrichmentDuration *prometheus.HistogramVec

	// RegoEvaluationDuration measures Rego policy evaluation time.
	// Labels: policy (environment, priority, business)
	// SLO: <100ms P95
	RegoEvaluationDuration *prometheus.HistogramVec

	// === OPERATIONAL METRICS ===

	// RegoHotReloadTotal tracks Rego policy hot-reload events.
	// Labels: status (success, failure)
	// Operational: Did policy updates succeed?
	RegoHotReloadTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
	return &Metrics{
		// Core business metrics
		ReconciliationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_total",
				Help:      "Total number of reconciliation operations by phase and status",
			},
			[]string{"phase", "status"},
		),

		ReconciliationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_duration_seconds",
				Help:      "Duration of reconciliation operations by phase",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"phase"},
		),

		CategorizationConfidence: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "categorization_confidence",
				Help:      "Confidence scores for categorization by classifier type",
				Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"classifier"},
		),

		// SLO metrics
		EnrichmentDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "enrichment_duration_seconds",
				Help:      "Duration of K8s context enrichment by resource kind. SLO: <2s P95",
				Buckets:   []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
			[]string{"resource_kind"},
		),

		RegoEvaluationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rego_evaluation_duration_seconds",
				Help:      "Duration of Rego policy evaluation by policy type. SLO: <100ms P95",
				Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"policy"},
		),

		// Operational metrics
		RegoHotReloadTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rego_hot_reload_total",
				Help:      "Total number of Rego policy hot-reload events by status",
			},
			[]string{"status"},
		),
	}
}

// NewMetricsWithRegistry creates metrics registered with a custom registry.
// Used in tests to avoid "already registered" errors with the global registry.
func NewMetricsWithRegistry(reg *prometheus.Registry) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		ReconciliationTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_total",
				Help:      "Total number of reconciliation operations by phase and status",
			},
			[]string{"phase", "status"},
		),

		ReconciliationDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_duration_seconds",
				Help:      "Duration of reconciliation operations by phase",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"phase"},
		),

		CategorizationConfidence: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "categorization_confidence",
				Help:      "Confidence scores for categorization by classifier type",
				Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"classifier"},
		),

		EnrichmentDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "enrichment_duration_seconds",
				Help:      "Duration of K8s context enrichment by resource kind. SLO: <2s P95",
				Buckets:   []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
			[]string{"resource_kind"},
		),

		RegoEvaluationDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rego_evaluation_duration_seconds",
				Help:      "Duration of Rego policy evaluation by policy type. SLO: <100ms P95",
				Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"policy"},
		),

		RegoHotReloadTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "rego_hot_reload_total",
				Help:      "Total number of Rego policy hot-reload events by status",
			},
			[]string{"status"},
		),
	}
}

