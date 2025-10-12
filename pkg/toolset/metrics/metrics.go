package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BR-TOOLSET-035: Prometheus metrics for Dynamic Toolset Service
//
// Metrics Categories:
// 1. Service Discovery - Track discovered services and health checks
// 2. API Requests - Track HTTP API usage and performance
// 3. Authentication - Track auth attempts and failures
// 4. ConfigMap - Track ConfigMap reconciliation
// 5. Toolset Generation - Track toolset generation

var (
	// Service Discovery Metrics

	// ServicesDiscovered tracks the number of services discovered by type
	ServicesDiscovered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_services_discovered_total",
			Help: "Total number of services discovered by service type",
		},
		[]string{"service_type"},
	)

	// DiscoveryDuration tracks how long discovery takes
	DiscoveryDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dynamic_toolset_discovery_duration_seconds",
			Help:    "Time taken to complete service discovery",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
	)

	// DiscoveryErrors tracks discovery errors by type
	DiscoveryErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_discovery_errors_total",
			Help: "Total number of discovery errors by error type",
		},
		[]string{"error_type"},
	)

	// HealthCheckFailures tracks health check failures
	HealthCheckFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_health_check_failures_total",
			Help: "Total number of health check failures by service and reason",
		},
		[]string{"service_type", "reason"},
	)

	// API Request Metrics

	// APIRequests tracks API requests by endpoint, method, and status code
	APIRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_api_requests_total",
			Help: "Total number of API requests by endpoint, method, and status",
		},
		[]string{"endpoint", "method", "status_code"},
	)

	// APIRequestDuration tracks API request duration
	APIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dynamic_toolset_api_request_duration_seconds",
			Help:    "API request duration by endpoint and method",
			Buckets: prometheus.DefBuckets, // 0.005s to 10s
		},
		[]string{"endpoint", "method"},
	)

	// APIErrors tracks API errors by endpoint and error type
	APIErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_api_errors_total",
			Help: "Total number of API errors by endpoint and error type",
		},
		[]string{"endpoint", "error_type"},
	)

	// Authentication Metrics

	// AuthAttempts tracks total authentication attempts
	AuthAttempts = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_auth_attempts_total",
			Help: "Total number of authentication attempts",
		},
	)

	// AuthFailures tracks authentication failures by reason
	AuthFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_auth_failures_total",
			Help: "Total number of authentication failures by reason",
		},
		[]string{"reason"},
	)

	// AuthDuration tracks authentication duration
	AuthDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dynamic_toolset_auth_duration_seconds",
			Help:    "Time taken to authenticate requests",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
	)

	// ConfigMap Metrics

	// ConfigMapUpdates tracks ConfigMap update operations
	ConfigMapUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_configmap_updates_total",
			Help: "Total number of ConfigMap updates by result",
		},
		[]string{"result"}, // success, failure
	)

	// ConfigMapReconcileDuration tracks ConfigMap reconciliation duration
	ConfigMapReconcileDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dynamic_toolset_configmap_reconcile_duration_seconds",
			Help:    "Time taken to reconcile ConfigMap",
			Buckets: prometheus.DefBuckets,
		},
	)

	// ConfigMapDriftDetected tracks when drift is detected
	ConfigMapDriftDetected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_configmap_drift_detected_total",
			Help: "Total number of times ConfigMap drift was detected",
		},
	)

	// Toolset Generation Metrics

	// ToolsetGenerations tracks toolset generation operations
	ToolsetGenerations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dynamic_toolset_generations_total",
			Help: "Total number of toolset generations by result",
		},
		[]string{"result"}, // success, failure
	)

	// ToolsInToolset tracks the number of tools in the current toolset
	ToolsInToolset = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dynamic_toolset_tools_count",
			Help: "Current number of tools in the generated toolset",
		},
	)
)

// ResetMetrics resets all metrics (for testing)
func ResetMetrics() {
	ServicesDiscovered.Reset()
	DiscoveryErrors.Reset()
	HealthCheckFailures.Reset()
	APIRequests.Reset()
	APIErrors.Reset()
	AuthFailures.Reset()
	ConfigMapUpdates.Reset()
	ToolsetGenerations.Reset()

	// Note: Histograms and regular counters cannot be reset in prometheus client
	// They will be replaced on next registration
}
