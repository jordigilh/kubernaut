package notification

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ==============================================
// v3.1 Enhancement: Prometheus Metrics for Runbook Automation
// ==============================================

var (
	// notificationFailureRate tracks the current notification failure rate (percentage)
	// Used by Runbook 1: High Notification Failure Rate (>10%)
	notificationFailureRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_failure_rate",
			Help: "Current notification failure rate (percentage) by namespace",
		},
		[]string{"namespace"},
	)

	// notificationStuckDuration tracks time notifications spend in Delivering phase
	// Used by Runbook 2: Stuck Notifications (>10min)
	notificationStuckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_stuck_duration_seconds",
			Help:    "Time notifications spend in Delivering phase",
			Buckets: []float64{60, 300, 600, 900, 1800}, // 1m, 5m, 10m, 15m, 30m
		},
		[]string{"namespace"},
	)

	// notificationDeliveriesTotal counts total delivery attempts by status
	notificationDeliveriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notification_deliveries_total",
			Help: "Total number of notification delivery attempts",
		},
		[]string{"namespace", "status", "channel"},
	)

	// notificationDeliveryDuration tracks end-to-end delivery latency
	notificationDeliveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_delivery_duration_seconds",
			Help:    "Duration of notification delivery from creation to completion",
			Buckets: []float64{1, 2, 5, 10, 30, 60, 120, 300}, // 1s to 5m
		},
		[]string{"namespace", "channel"},
	)

	// notificationPhase tracks current phase distribution
	notificationPhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_phase",
			Help: "Current notification phase (0=Pending, 1=Sending, 2=Sent, 3=Failed)",
		},
		[]string{"namespace", "phase"},
	)

	// notificationRetryCount tracks retry attempts
	notificationRetryCount = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_retry_count",
			Help:    "Number of retry attempts per notification",
			Buckets: []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		[]string{"namespace"},
	)

	// v3.1 Category B: Slack API retry metrics
	notificationSlackRetryCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notification_slack_retry_count",
			Help: "Total number of Slack API retry attempts",
		},
		[]string{"namespace", "reason"},
	)

	// v3.1 Category B: Slack API backoff duration metrics
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
	metrics.Registry.MustRegister(
		notificationFailureRate,
		notificationStuckDuration,
		notificationDeliveriesTotal,
		notificationDeliveryDuration,
		notificationPhase,
		notificationRetryCount,
		notificationSlackRetryCount,
		notificationSlackBackoffDuration,
	)
}

// Metrics helper functions for use in controller

// RecordDeliveryAttempt records a delivery attempt (success or failure)
func RecordDeliveryAttempt(namespace, channel, status string) {
	notificationDeliveriesTotal.WithLabelValues(namespace, status, channel).Inc()
}

// RecordDeliveryDuration records the time taken for a delivery
func RecordDeliveryDuration(namespace, channel string, durationSeconds float64) {
	notificationDeliveryDuration.WithLabelValues(namespace, channel).Observe(durationSeconds)
}

// UpdateFailureRate updates the failure rate metric for a namespace
func UpdateFailureRate(namespace string, rate float64) {
	notificationFailureRate.WithLabelValues(namespace).Set(rate)
}

// RecordStuckDuration records time spent in Delivering phase
func RecordStuckDuration(namespace string, durationSeconds float64) {
	notificationStuckDuration.WithLabelValues(namespace).Observe(durationSeconds)
}

// UpdatePhaseCount updates the count of notifications in a specific phase
func UpdatePhaseCount(namespace, phase string, count float64) {
	notificationPhase.WithLabelValues(namespace, phase).Set(count)
}

// RecordRetryCount records the number of retries for a notification
func RecordRetryCount(namespace string, retries float64) {
	notificationRetryCount.WithLabelValues(namespace).Observe(retries)
}

// v3.1 Category B: Slack API retry metrics helpers

// RecordSlackRetry records a Slack API retry attempt
func RecordSlackRetry(namespace, reason string) {
	notificationSlackRetryCount.WithLabelValues(namespace, reason).Inc()
}

// RecordSlackBackoff records the backoff duration for a Slack API retry
func RecordSlackBackoff(namespace string, durationSeconds float64) {
	notificationSlackBackoffDuration.WithLabelValues(namespace).Observe(durationSeconds)
}
