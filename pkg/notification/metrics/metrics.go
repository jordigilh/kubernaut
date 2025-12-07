package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// DD-005 COMPLIANT METRICS
// Format: {service}_{component}_{metric_name}_{unit}
// See: docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md

// ReconcilerRequestsTotal tracks total notification requests by type, priority, and phase
// Satisfies BR-NOT-054: Observability
// DD-005: notification_reconciler_requests_total (was: notification_requests_total)
var ReconcilerRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_reconciler_requests_total",
		Help: "Total number of notification reconciler requests",
	},
	[]string{"type", "priority", "phase"},
)

// DeliveryAttemptsTotal tracks delivery attempts by channel and status
// DD-005: Already compliant (notification_delivery_attempts_total)
var DeliveryAttemptsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_delivery_attempts_total",
		Help: "Total number of delivery attempts",
	},
	[]string{"channel", "status"},
)

// DeliveryDuration tracks delivery duration in seconds
// DD-005: Already compliant (notification_delivery_duration_seconds)
var DeliveryDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "notification_delivery_duration_seconds",
		Help:    "Delivery duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"channel"},
)

// DeliveryRetriesTotal tracks retry attempts by channel
// DD-005: notification_delivery_retries_total (was: notification_retry_count_total)
var DeliveryRetriesTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_delivery_retries_total",
		Help: "Total number of delivery retry attempts",
	},
	[]string{"channel", "reason"},
)

// ChannelCircuitBreakerState tracks circuit breaker state (0=closed, 1=open, 2=half-open)
// DD-005: notification_channel_circuit_breaker_state (was: notification_circuit_breaker_state)
var ChannelCircuitBreakerState = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_channel_circuit_breaker_state",
		Help: "Circuit breaker state per channel (0=closed, 1=open, 2=half-open)",
	},
	[]string{"channel"},
)

// ReconcilerDuration tracks reconciliation loop duration
// DD-005: notification_reconciler_duration_seconds (was: notification_reconciliation_duration_seconds)
var ReconcilerDuration = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "notification_reconciler_duration_seconds",
		Help:    "Reconciler loop duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
)

// ReconcilerErrorsTotal tracks reconciliation errors
// DD-005: notification_reconciler_errors_total (was: notification_reconciliation_errors_total)
var ReconcilerErrorsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_reconciler_errors_total",
		Help: "Total number of reconciler errors",
	},
	[]string{"error_type"},
)

// ReconcilerActive tracks currently active notifications by phase
// DD-005: notification_reconciler_active (Gauge - no _total suffix per DD-005 line 122)
// Fixed: was notification_reconciler_active_total (incorrect _total suffix for Gauge)
var ReconcilerActive = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_reconciler_active",
		Help: "Number of active notifications by phase",
	},
	[]string{"phase"},
)

// SanitizationRedactions tracks sensitive data redactions
// DD-005: Already compliant (notification_sanitization_redactions_total)
var SanitizationRedactions = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_sanitization_redactions_total",
		Help: "Total number of sensitive data redactions",
	},
	[]string{"pattern_type"},
)

// ChannelHealthScore tracks per-channel health (0-100)
// DD-005: Already compliant (notification_channel_health_score)
var ChannelHealthScore = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_channel_health_score",
		Help: "Channel health score (0-100, 100=healthy)",
	},
	[]string{"channel"},
)

func init() {
	// Register all metrics with controller-runtime
	// DD-005 Compliant: {service}_{component}_{metric_name}_{unit}
	metrics.Registry.MustRegister(
		ReconcilerRequestsTotal,
		DeliveryAttemptsTotal,
		DeliveryDuration,
		DeliveryRetriesTotal,
		ChannelCircuitBreakerState,
		ReconcilerDuration,
		ReconcilerErrorsTotal,
		ReconcilerActive, // DD-005: Gauge has no _total suffix
		SanitizationRedactions,
		ChannelHealthScore,
	)
}
