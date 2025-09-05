package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Core Business Logic Metrics
var (
	// AlertsProcessedTotal tracks the total number of alerts processed
	AlertsProcessedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "alerts_processed_total",
		Help: "Total number of alerts processed by the system",
	})

	// ActionsExecutedTotal tracks actions executed by type
	ActionsExecutedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "actions_executed_total",
		Help: "Total number of actions executed by action type",
	}, []string{"action"})

	// ActionProcessingDuration tracks processing time per action type
	ActionProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "action_processing_duration_seconds",
		Help:    "Time taken to process each action type",
		Buckets: prometheus.DefBuckets,
	}, []string{"action"})

	// SLMAnalysisDuration tracks time for SLM to analyze alerts
	SLMAnalysisDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "slm_analysis_duration_seconds",
		Help:    "Time taken for SLM to analyze alerts and generate recommendations",
		Buckets: prometheus.DefBuckets,
	})

	// SLMContextSizeBytes tracks the context size sent to SLM in bytes
	SLMContextSizeBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "slm_context_size_bytes",
		Help:    "Size of context sent to SLM in bytes (tracks min, avg, max)",
		Buckets: []float64{1000, 2000, 4000, 8000, 16000, 32000, 65000, 128000, 256000},
	}, []string{"provider"})

	// SLMContextSizeTokens tracks the estimated context size in tokens
	SLMContextSizeTokens = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "slm_context_size_tokens",
		Help:    "Estimated size of context sent to SLM in tokens (4 chars per token)",
		Buckets: []float64{1000, 2000, 4000, 8000, 16000, 32000, 65000},
	}, []string{"provider"})
)

// Additional Valuable Metrics
var (
	// AlertsFilteredTotal tracks alerts filtered by filter rules
	AlertsFilteredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "alerts_filtered_total",
		Help: "Total number of alerts filtered by filter rules",
	}, []string{"filter"})

	// ActionExecutionErrorsTotal tracks failed action executions
	ActionExecutionErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "action_execution_errors_total",
		Help: "Total number of failed action executions",
	}, []string{"action", "error_type"})

	// SLMAPICallsTotal tracks SLM API call count
	SLMAPICallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "slm_api_calls_total",
		Help: "Total number of SLM API calls",
	}, []string{"provider"})

	// SLMAPIErrorsTotal tracks SLM API errors
	SLMAPIErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "slm_api_errors_total",
		Help: "Total number of SLM API errors",
	}, []string{"provider", "error_type"})

	// K8sAPICallsTotal tracks Kubernetes API interactions
	K8sAPICallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "k8s_api_calls_total",
		Help: "Total number of Kubernetes API calls",
	}, []string{"operation"})

	// AlertsInCooldownTotal tracks current alerts in cooldown period
	AlertsInCooldownTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "alerts_in_cooldown_total",
		Help: "Current number of alerts in cooldown period",
	})

	// ConcurrentActionsRunning tracks currently executing actions
	ConcurrentActionsRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "concurrent_actions_running",
		Help: "Current number of actions being executed",
	})

	// WebhookRequestsTotal tracks webhook endpoint metrics
	WebhookRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_requests_total",
		Help: "Total number of webhook requests",
	}, []string{"status"})
)

// Helper functions for recording metrics

// RecordAlert increments the alerts processed counter
func RecordAlert() {
	AlertsProcessedTotal.Inc()
}

// RecordAction records an action execution with timing
func RecordAction(action string, duration time.Duration) {
	ActionsExecutedTotal.WithLabelValues(action).Inc()
	ActionProcessingDuration.WithLabelValues(action).Observe(duration.Seconds())
}

// RecordSLMAnalysis records SLM analysis timing
func RecordSLMAnalysis(duration time.Duration) {
	SLMAnalysisDuration.Observe(duration.Seconds())
}

// RecordSLMContextSize records the context size sent to SLM
func RecordSLMContextSize(provider string, contextBytes int) {
	SLMContextSizeBytes.WithLabelValues(provider).Observe(float64(contextBytes))
	// Estimate tokens (roughly 4 characters per token)
	estimatedTokens := contextBytes / 4
	SLMContextSizeTokens.WithLabelValues(provider).Observe(float64(estimatedTokens))
}

// RecordFilteredAlert records a filtered alert
func RecordFilteredAlert(filter string) {
	AlertsFilteredTotal.WithLabelValues(filter).Inc()
}

// RecordActionError records an action execution error
func RecordActionError(action, errorType string) {
	ActionExecutionErrorsTotal.WithLabelValues(action, errorType).Inc()
}

// RecordSLMAPICall records an SLM API call
func RecordSLMAPICall(provider string) {
	SLMAPICallsTotal.WithLabelValues(provider).Inc()
}

// RecordSLMAPIError records an SLM API error
func RecordSLMAPIError(provider, errorType string) {
	SLMAPIErrorsTotal.WithLabelValues(provider, errorType).Inc()
}

// RecordK8sAPICall records a Kubernetes API call
func RecordK8sAPICall(operation string) {
	K8sAPICallsTotal.WithLabelValues(operation).Inc()
}

// SetAlertsInCooldown sets the current number of alerts in cooldown
func SetAlertsInCooldown(count float64) {
	AlertsInCooldownTotal.Set(count)
}

// IncrementConcurrentActions increments the concurrent actions gauge
func IncrementConcurrentActions() {
	ConcurrentActionsRunning.Inc()
}

// DecrementConcurrentActions decrements the concurrent actions gauge
func DecrementConcurrentActions() {
	ConcurrentActionsRunning.Dec()
}

// RecordWebhookRequest records a webhook request
func RecordWebhookRequest(status string) {
	WebhookRequestsTotal.WithLabelValues(status).Inc()
}

// Timer is a helper struct for measuring durations
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Elapsed returns the duration since the timer was created
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}

// RecordAction records the elapsed time for an action
func (t *Timer) RecordAction(action string) {
	RecordAction(action, t.Elapsed())
}

// RecordSLMAnalysis records the elapsed time for SLM analysis
func (t *Timer) RecordSLMAnalysis() {
	RecordSLMAnalysis(t.Elapsed())
}
