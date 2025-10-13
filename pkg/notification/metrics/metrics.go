package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// NotificationRequestsTotal tracks total notification requests by type, priority, and phase
// Satisfies BR-NOT-054: Observability
var NotificationRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_requests_total",
		Help: "Total number of notification requests",
	},
	[]string{"type", "priority", "phase"},
)

// DeliveryAttemptsTotal tracks delivery attempts by channel and status
var DeliveryAttemptsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_delivery_attempts_total",
		Help: "Total number of delivery attempts",
	},
	[]string{"channel", "status"},
)

// DeliveryDuration tracks delivery duration in seconds
var DeliveryDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "notification_delivery_duration_seconds",
		Help:    "Delivery duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"channel"},
)

// RetryCount tracks retry attempts by channel
var RetryCount = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_retry_count_total",
		Help: "Total number of retry attempts",
	},
	[]string{"channel", "reason"},
)

// CircuitBreakerState tracks circuit breaker state (0=closed, 1=open, 2=half-open)
var CircuitBreakerState = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
	},
	[]string{"channel"},
)

// ReconciliationDuration tracks reconciliation loop duration
var ReconciliationDuration = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "notification_reconciliation_duration_seconds",
		Help:    "Reconciliation duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
)

// ReconciliationErrors tracks reconciliation errors
var ReconciliationErrors = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_reconciliation_errors_total",
		Help: "Total number of reconciliation errors",
	},
	[]string{"error_type"},
)

// ActiveNotifications tracks currently active notifications by phase
var ActiveNotifications = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_active_total",
		Help: "Number of active notifications by phase",
	},
	[]string{"phase"},
)

// SanitizationRedactions tracks sensitive data redactions
var SanitizationRedactions = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "notification_sanitization_redactions_total",
		Help: "Total number of sensitive data redactions",
	},
	[]string{"pattern_type"},
)

// ChannelHealthScore tracks per-channel health (0-100)
var ChannelHealthScore = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "notification_channel_health_score",
		Help: "Channel health score (0-100, 100=healthy)",
	},
	[]string{"channel"},
)

func init() {
	// Register all metrics with controller-runtime
	metrics.Registry.MustRegister(
		NotificationRequestsTotal,
		DeliveryAttemptsTotal,
		DeliveryDuration,
		RetryCount,
		CircuitBreakerState,
		ReconciliationDuration,
		ReconciliationErrors,
		ActiveNotifications,
		SanitizationRedactions,
		ChannelHealthScore,
	)
}

