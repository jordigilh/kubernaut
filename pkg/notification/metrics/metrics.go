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

// Package metrics provides Prometheus metrics for the Notification controller.
// Per DD-METRICS-001: Uses dependency injection pattern for metrics wiring.
// All metrics follow DD-005 naming convention: notification_{component}_{metric_name}
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// METRIC NAME CONSTANTS (DD-005 V3.0 MANDATORY)
// ========================================
//
// Per DD-005 V3.0: Metric name constants are MANDATORY to prevent typos
// and ensure test/production parity.
// See: docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md#11-metric-name-constants-mandatory
//
// These constants ensure tests use correct metric names and prevent runtime failures.

const (
	// === RECONCILIATION METRIC NAMES ===

	// MetricNameReconcilerRequestsTotal is the name of the reconciler requests counter metric
	MetricNameReconcilerRequestsTotal = "kubernaut_notification_reconciler_requests_total"

	// MetricNameReconcilerDuration is the name of the reconciler duration histogram metric
	MetricNameReconcilerDuration = "kubernaut_notification_reconciler_duration_seconds"

	// MetricNameReconcilerErrorsTotal is the name of the reconciler errors counter metric
	MetricNameReconcilerErrorsTotal = "kubernaut_notification_reconciler_errors_total"

	// MetricNameReconcilerActive is the name of the active notifications gauge metric
	MetricNameReconcilerActive = "kubernaut_notification_reconciler_active"

	// === DELIVERY METRIC NAMES ===

	// MetricNameDeliveryAttemptsTotal is the name of the delivery attempts counter metric
	MetricNameDeliveryAttemptsTotal = "kubernaut_notification_delivery_attempts_total"

	// MetricNameDeliveryDuration is the name of the delivery duration histogram metric
	MetricNameDeliveryDuration = "kubernaut_notification_delivery_duration_seconds"

	// MetricNameDeliveryRetriesTotal is the name of the delivery retries counter metric
	MetricNameDeliveryRetriesTotal = "kubernaut_notification_delivery_retries_total"

	// === CHANNEL HEALTH METRIC NAMES ===

	// MetricNameChannelCircuitBreakerState is the name of the circuit breaker state gauge metric
	MetricNameChannelCircuitBreakerState = "kubernaut_notification_channel_circuit_breaker_state"

	// MetricNameChannelHealthScore is the name of the channel health score gauge metric
	MetricNameChannelHealthScore = "kubernaut_notification_channel_health_score"

	// === SANITIZATION METRIC NAMES ===

	// MetricNameSanitizationRedactions is the name of the sanitization redactions counter metric
	MetricNameSanitizationRedactions = "kubernaut_notification_sanitization_redactions_total"

	// === COMMON LABEL VALUES ===

	// LabelValueStatusSuccess indicates successful delivery
	LabelValueStatusSuccess = "success"

	// LabelValueStatusFailure indicates failed delivery
	LabelValueStatusFailure = "failure"

	// LabelValueStatusRetry indicates delivery will be retried
	LabelValueStatusRetry = "retry"

	// LabelValueChannelConsole indicates console channel
	LabelValueChannelConsole = "console"

	// LabelValueChannelSlack indicates Slack channel
	LabelValueChannelSlack = "slack"

	// LabelValueChannelEmail indicates email channel
	LabelValueChannelEmail = "email"

	// LabelValueChannelWebhook indicates webhook channel
	LabelValueChannelWebhook = "webhook"
)

// DD-005 COMPLIANT METRICS
// Format: kubernaut_{service}_{metric_name}_{unit}
// See: docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md

// Metrics holds all Prometheus metrics for the Notification controller.
// Per DD-METRICS-001: Dependency-injected metrics pattern for testability and clarity.
type Metrics struct {
	// === RECONCILIATION METRICS ===
	ReconcilerRequestsTotal *prometheus.CounterVec
	ReconcilerDuration      prometheus.Histogram
	ReconcilerErrorsTotal   *prometheus.CounterVec
	ReconcilerActive        *prometheus.GaugeVec

	// === DELIVERY METRICS ===
	DeliveryAttemptsTotal *prometheus.CounterVec
	DeliveryDuration      *prometheus.HistogramVec
	DeliveryRetriesTotal  *prometheus.CounterVec

	// === CHANNEL HEALTH METRICS ===
	ChannelCircuitBreakerState *prometheus.GaugeVec
	ChannelHealthScore         *prometheus.GaugeVec

	// === SANITIZATION METRICS ===
	SanitizationRedactions *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance and registers with controller-runtime.
// Uses controller-runtime's global registry for automatic /metrics endpoint exposure.
// Per DD-METRICS-001: Dependency injection pattern for V1.0 maturity.
func NewMetrics() *Metrics {
	m := &Metrics{
		// Reconciliation metrics
		ReconcilerRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerRequestsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of notification reconciler requests",
			},
			[]string{"type", "priority", "phase"},
		),
		ReconcilerDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    MetricNameReconcilerDuration, // DD-005 V3.0: Pattern B (full name)
				Help:    "Reconciler loop duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
		),
		ReconcilerErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerErrorsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of reconciler errors",
			},
			[]string{"error_type"},
		),
		ReconcilerActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameReconcilerActive, // DD-005 V3.0: Pattern B (full name)
				Help: "Number of active notifications by phase",
			},
			[]string{"phase"},
		),

		// Delivery metrics
		DeliveryAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDeliveryAttemptsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of delivery attempts",
			},
			[]string{"channel", "status"},
		),
		DeliveryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameDeliveryDuration, // DD-005 V3.0: Pattern B (full name)
				Help:    "Delivery duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"channel"},
		),
		DeliveryRetriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDeliveryRetriesTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of delivery retry attempts",
			},
			[]string{"channel", "reason"},
		),

		// Channel health metrics
		ChannelCircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameChannelCircuitBreakerState, // DD-005 V3.0: Pattern B (full name)
				Help: "Circuit breaker state per channel (0=closed, 1=open, 2=half-open)",
			},
			[]string{"channel"},
		),
		ChannelHealthScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameChannelHealthScore, // DD-005 V3.0: Pattern B (full name)
				Help: "Channel health score (0-100, 100=healthy)",
			},
			[]string{"channel"},
		),

		// Sanitization metrics
		SanitizationRedactions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSanitizationRedactions, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of sensitive data redactions",
			},
			[]string{"pattern_type"},
		),
	}

	// Register all metrics with controller-runtime's global registry
	// This makes metrics available at /metrics endpoint
	ctrlmetrics.Registry.MustRegister(
		m.ReconcilerRequestsTotal,
		m.ReconcilerDuration,
		m.ReconcilerErrorsTotal,
		m.ReconcilerActive,
		m.DeliveryAttemptsTotal,
		m.DeliveryDuration,
		m.DeliveryRetriesTotal,
		m.ChannelCircuitBreakerState,
		m.ChannelHealthScore,
		m.SanitizationRedactions,
	)

	return m
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing).
// Tests should use this to avoid polluting the global registry.
// Per DD-METRICS-001: Test isolation pattern.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		// Reconciliation metrics
		ReconcilerRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerRequestsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of notification reconciler requests",
			},
			[]string{"type", "priority", "phase"},
		),
		ReconcilerDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    MetricNameReconcilerDuration, // DD-005 V3.0: Pattern B (full name)
				Help:    "Reconciler loop duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
		),
		ReconcilerErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcilerErrorsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of reconciler errors",
			},
			[]string{"error_type"},
		),
		ReconcilerActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameReconcilerActive, // DD-005 V3.0: Pattern B (full name)
				Help: "Number of active notifications by phase",
			},
			[]string{"phase"},
		),

		// Delivery metrics
		DeliveryAttemptsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDeliveryAttemptsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of delivery attempts",
			},
			[]string{"channel", "status"},
		),
		DeliveryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameDeliveryDuration, // DD-005 V3.0: Pattern B (full name)
				Help:    "Delivery duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"channel"},
		),
		DeliveryRetriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDeliveryRetriesTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of delivery retry attempts",
			},
			[]string{"channel", "reason"},
		),

		// Channel health metrics
		ChannelCircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameChannelCircuitBreakerState, // DD-005 V3.0: Pattern B (full name)
				Help: "Circuit breaker state per channel (0=closed, 1=open, 2=half-open)",
			},
			[]string{"channel"},
		),
		ChannelHealthScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameChannelHealthScore, // DD-005 V3.0: Pattern B (full name)
				Help: "Channel health score (0-100, 100=healthy)",
			},
			[]string{"channel"},
		),

		// Sanitization metrics
		SanitizationRedactions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSanitizationRedactions, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of sensitive data redactions",
			},
			[]string{"pattern_type"},
		),
	}

	// Register with provided registry (test registry)
	registry.MustRegister(
		m.ReconcilerRequestsTotal,
		m.ReconcilerDuration,
		m.ReconcilerErrorsTotal,
		m.ReconcilerActive,
		m.DeliveryAttemptsTotal,
		m.DeliveryDuration,
		m.DeliveryRetriesTotal,
		m.ChannelCircuitBreakerState,
		m.ChannelHealthScore,
		m.SanitizationRedactions,
	)

	return m
}
