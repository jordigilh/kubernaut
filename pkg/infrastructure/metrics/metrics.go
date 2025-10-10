<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
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

	// AI Service Metrics - Following kubernaut naming convention
	// BR-AI-005: Metrics collection for AI service monitoring

	// AIRequestsTotal tracks total AI analysis requests processed
	AIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kubernaut_ai_requests_total",
		Help: "Total AI analysis requests processed",
	}, []string{"service", "endpoint", "status"})

	// AIAnalysisDuration tracks duration of AI analysis requests (proper histogram)
	AIAnalysisDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kubernaut_ai_analysis_duration_seconds",
		Help:    "Duration of AI analysis requests",
		Buckets: []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0}, // AI analysis buckets
	}, []string{"service", "model"})

	// AILLMRequestsTotal tracks LLM requests made by AI service
	AILLMRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kubernaut_ai_llm_requests_total",
		Help: "Total LLM requests made by AI service",
	}, []string{"service", "provider", "model", "status"})

	// AIFallbackUsageTotal tracks fallback client usage
	AIFallbackUsageTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kubernaut_ai_fallback_usage_total",
		Help: "Total fallback client usage by AI service",
	}, []string{"service", "reason"})

	// AIErrorsTotal tracks errors encountered by AI service
	AIErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kubernaut_ai_errors_total",
		Help: "Total errors encountered by AI service",
	}, []string{"service", "error_type", "endpoint"})

	// AIServiceUp tracks AI service availability (standard service metric)
	AIServiceUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kubernaut_ai_service_up",
		Help: "AI service availability status",
	}, []string{"service"})
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

// AI Service Helper Functions - Following kubernaut patterns
// BR-AI-005: Standardized AI service metrics recording

// RecordAIRequest records an AI analysis request
func RecordAIRequest(service, endpoint, status string) {
	AIRequestsTotal.WithLabelValues(service, endpoint, status).Inc()
}

// RecordAIAnalysis records AI analysis timing
func RecordAIAnalysis(service, model string, duration time.Duration) {
	AIAnalysisDuration.WithLabelValues(service, model).Observe(duration.Seconds())
}

// RecordAILLMRequest records an LLM request made by AI service
func RecordAILLMRequest(service, provider, model, status string) {
	AILLMRequestsTotal.WithLabelValues(service, provider, model, status).Inc()
}

// RecordAIFallbackUsage records fallback client usage
func RecordAIFallbackUsage(service, reason string) {
	AIFallbackUsageTotal.WithLabelValues(service, reason).Inc()
}

// RecordAIError records an AI service error
func RecordAIError(service, errorType, endpoint string) {
	AIErrorsTotal.WithLabelValues(service, errorType, endpoint).Inc()
}

// SetAIServiceUp sets AI service availability status
func SetAIServiceUp(service string, up bool) {
	var value float64
	if up {
		value = 1
	}
	AIServiceUp.WithLabelValues(service).Set(value)
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

// Client provides a simple metrics client interface
type Client struct {
	// This is a minimal client that can be extended as needed
}

// RecordCounter records a counter metric
func (c *Client) RecordCounter(name string, value float64, labels map[string]string) error {
	// For now, this is a no-op, but could be implemented to record actual metrics
	return nil
}
