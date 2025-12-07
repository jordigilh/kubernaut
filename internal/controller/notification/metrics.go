package notification

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ==============================================
// v3.1 Enhancement: Prometheus Metrics for Runbook Automation
// DD-005 Compliant: Format {service}_{component}_{metric_name}_{unit}
// ==============================================

var (
	// notificationDeliveryFailureRatio tracks the current notification failure rate (percentage)
	// Used by Runbook 1: High Notification Failure Rate (>10%)
	// DD-005: notification_delivery_failure_ratio (was: notification_failure_rate)
	notificationDeliveryFailureRatio = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_delivery_failure_ratio",
			Help: "Current notification delivery failure rate (0-1 ratio) by namespace",
		},
		[]string{"namespace"},
	)

	// notificationDeliveryStuckDuration tracks time notifications spend in Delivering phase
	// Used by Runbook 2: Stuck Notifications (>10min)
	// DD-005: notification_delivery_stuck_duration_seconds (was: notification_stuck_duration_seconds)
	notificationDeliveryStuckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_delivery_stuck_duration_seconds",
			Help:    "Time notifications spend in Delivering phase",
			Buckets: []float64{60, 300, 600, 900, 1800}, // 1m, 5m, 10m, 15m, 30m
		},
		[]string{"namespace"},
	)

	// notificationDeliveryRequestsTotal counts total delivery attempts by status
	// DD-005: notification_delivery_requests_total (was: notification_deliveries_total)
	notificationDeliveryRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notification_delivery_requests_total",
			Help: "Total number of notification delivery requests",
		},
		[]string{"namespace", "status", "channel"},
	)

	// notificationDeliveryDuration tracks end-to-end delivery latency
	// DD-005: Already compliant (notification_delivery_duration_seconds)
	notificationDeliveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_delivery_duration_seconds",
			Help:    "Duration of notification delivery from creation to completion",
			Buckets: []float64{1, 2, 5, 10, 30, 60, 120, 300}, // 1s to 5m
		},
		[]string{"namespace", "channel"},
	)

	// notificationReconcilerPhase tracks current phase distribution
	// DD-005: notification_reconciler_phase (was: notification_phase)
	notificationReconcilerPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_reconciler_phase",
			Help: "Current notification phase (0=Pending, 1=Sending, 2=Sent, 3=Failed)",
		},
		[]string{"namespace", "phase"},
	)

	// notificationDeliveryRetries tracks retry attempts
	// DD-005: notification_delivery_retries (was: notification_retry_count)
	notificationDeliveryRetries = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_delivery_retries",
			Help:    "Number of delivery retry attempts per notification",
			Buckets: []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		[]string{"namespace"},
	)

	// v3.1 Category B: Slack API retry metrics
	// DD-005: notification_slack_retries_total (was: notification_slack_retry_count)
	notificationSlackRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notification_slack_retries_total",
			Help: "Total number of Slack API retry attempts",
		},
		[]string{"namespace", "reason"},
	)

	// v3.1 Category B: Slack API backoff duration metrics
	// DD-005: Already compliant (notification_slack_backoff_duration_seconds)
	notificationSlackBackoffDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_slack_backoff_duration_seconds",
			Help:    "Backoff duration for Slack API retries (with jitter)",
			Buckets: []float64{30, 60, 120, 240, 480}, // 30s, 1m, 2m, 4m, 8m
		},
		[]string{"namespace"},
	)
)

func init() {
	// Register metrics with controller-runtime's global registry
	// DD-005 Compliant naming: {service}_{component}_{metric_name}_{unit}
	metrics.Registry.MustRegister(
		notificationDeliveryFailureRatio,
		notificationDeliveryStuckDuration,
		notificationDeliveryRequestsTotal,
		notificationDeliveryDuration,
		notificationReconcilerPhase,
		notificationDeliveryRetries,
		notificationSlackRetriesTotal,
		notificationSlackBackoffDuration,
	)

	// NOTE: NOT initializing with zero values in init()
	// Let metrics appear naturally when first recorded by controller
	// This matches standard Prometheus behavior
}

// Metrics helper functions for use in controller
// DD-005 Compliant: All helper functions use DD-005 compliant metric names

// RecordDeliveryAttempt records a delivery attempt (success or failure)
func RecordDeliveryAttempt(namespace, channel, status string) {
	notificationDeliveryRequestsTotal.WithLabelValues(namespace, status, channel).Inc()
}

// RecordDeliveryDuration records the time taken for a delivery
func RecordDeliveryDuration(namespace, channel string, durationSeconds float64) {
	notificationDeliveryDuration.WithLabelValues(namespace, channel).Observe(durationSeconds)
}

// UpdateFailureRatio updates the failure ratio metric for a namespace (0-1 scale)
func UpdateFailureRatio(namespace string, ratio float64) {
	notificationDeliveryFailureRatio.WithLabelValues(namespace).Set(ratio)
}

// RecordStuckDuration records time spent in Delivering phase
func RecordStuckDuration(namespace string, durationSeconds float64) {
	notificationDeliveryStuckDuration.WithLabelValues(namespace).Observe(durationSeconds)
}

// UpdatePhaseCount updates the count of notifications in a specific phase
func UpdatePhaseCount(namespace, phase string, count float64) {
	notificationReconcilerPhase.WithLabelValues(namespace, phase).Set(count)
}

// RecordDeliveryRetries records the number of retries for a notification
func RecordDeliveryRetries(namespace string, retries float64) {
	notificationDeliveryRetries.WithLabelValues(namespace).Observe(retries)
}

// v3.1 Category B: Slack API retry metrics helpers

// RecordSlackRetry records a Slack API retry attempt
func RecordSlackRetry(namespace, reason string) {
	notificationSlackRetriesTotal.WithLabelValues(namespace, reason).Inc()
}

// RecordSlackBackoff records the backoff duration for a Slack API retry
func RecordSlackBackoff(namespace string, durationSeconds float64) {
	notificationSlackBackoffDuration.WithLabelValues(namespace).Observe(durationSeconds)
}
