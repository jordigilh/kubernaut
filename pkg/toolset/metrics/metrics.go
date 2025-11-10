package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BR-TOOLSET-035: Prometheus metrics for Dynamic Toolset Service
//
// DD-TOOLSET-001: REST API Deprecation - Removed 11 low-value metrics (55%)
// See: docs/architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md
//
// Metrics Categories (9 metrics - 45% of original 20):
// 1. Service Discovery - Track discovered services and health checks (4 metrics)
// 2. ConfigMap - Track ConfigMap reconciliation (3 metrics)
// 3. Toolset Generation - Track toolset generation (2 metrics)
//
// Removed Metrics (11 metrics - 0% business value):
// - API Request Metrics (3) - REST API disabled
// - Content-Type Validation (1) - No POST/PUT/PATCH endpoints
// - RFC 7807 Errors (1) - No REST API errors
// - Authentication (3) - No authenticated endpoints
// - Graceful Shutdown (2) - Pod terminates before Prometheus scrapes

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
	// Service Discovery Metrics
	ServicesDiscovered.Reset()
	DiscoveryErrors.Reset()
	HealthCheckFailures.Reset()

	// ConfigMap Metrics
	ConfigMapUpdates.Reset()

	// Toolset Generation Metrics
	ToolsetGenerations.Reset()

	// Note: Histograms and Gauges cannot be reset in prometheus client
	// They will be replaced on next registration
}
