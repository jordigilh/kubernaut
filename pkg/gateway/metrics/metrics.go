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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Gateway Service Prometheus Metrics
//
// Day 9 Phase 6B Option C1: Centralized metrics with custom registry support
// This package defines all metrics for the Gateway service, following the
// specifications in docs/services/stateless/gateway-service/metrics-slos.md
//
// Metrics are organized into categories:
// 1. Signal ingestion (received, deduplicated, storms) - multi-source (Prometheus, K8s Events, etc.)
// 2. CRD creation (success, failures)
// 3. Performance (HTTP request duration, Redis operation duration)
// 4. Deduplication (cache hits/misses, deduplication rate)
// 5. HTTP observability (in-flight requests, request counts)
// 6. Redis health (availability, outages)
//
// Metrics struct provides custom registry support for test isolation.

// Metrics holds all Gateway service Prometheus metrics
// Day 9 Phase 6B Option C1: Centralized metrics structure
type Metrics struct {
	// Signal Ingestion Metrics (BR-GATEWAY-001: Multi-source signal processing)
	// Note: Field names kept as "Alerts*" for backward compatibility, but metric names use "signals_"
	AlertsReceivedTotal      *prometheus.CounterVec // gateway_signals_received_total
	AlertsDeduplicatedTotal  *prometheus.CounterVec // gateway_signals_deduplicated_total
	AlertStormsDetectedTotal *prometheus.CounterVec // gateway_signal_storms_detected_total

	// CRD Creation Metrics
	CRDsCreatedTotal  *prometheus.CounterVec
	CRDCreationErrors *prometheus.CounterVec

	// K8s API Retry Metrics (BR-GATEWAY-114: Retry observability)
	RetryAttemptsTotal   *prometheus.CounterVec   // Total retry attempts by error type
	RetryDuration        *prometheus.HistogramVec // Retry duration by error type
	RetryExhaustedTotal  *prometheus.CounterVec   // Retries exhausted by error type
	RetrySuccessTotal    *prometheus.CounterVec   // Successful retries by error type and attempt number

	// Internal: Registry for custom metrics exposure
	registry prometheus.Gatherer // Used by /metrics endpoint to expose custom registry metrics

	// Performance Metrics
	HTTPRequestDuration    *prometheus.HistogramVec
	RedisOperationDuration *prometheus.HistogramVec

	// Deduplication Metrics
	DeduplicationCacheHitsTotal   prometheus.Counter
	DeduplicationCacheMissesTotal prometheus.Counter
	DeduplicationRate             prometheus.Gauge
	DeduplicationPoolSize         prometheus.Gauge
	DeduplicationPoolMaxSize      prometheus.Gauge

	// HTTP Observability Metrics (Day 9 Phase 4)
	HTTPRequestsInFlight prometheus.Gauge
	HTTPRequestsTotal    *prometheus.CounterVec

	// Redis Health Metrics (Day 9 Phase 4)
	RedisAvailable      prometheus.Gauge
	RedisOutageDuration prometheus.Counter
	RedisOutageCount    prometheus.Counter

	// Redis Pool Metrics (Day 9 Phase 4)
	RedisPoolHits     prometheus.Gauge
	RedisPoolMisses   prometheus.Gauge
	RedisPoolTimeouts prometheus.Gauge
	RedisPoolTotal    prometheus.Gauge
	RedisPoolIdle     prometheus.Gauge
	RedisPoolStale    prometheus.Gauge

	// Redis Pool Metrics - Test-Compatible Names (Pre-Day 10 Unit Test Validation)
	RedisPoolConnectionsTotal  prometheus.Gauge
	RedisPoolConnectionsIdle   prometheus.Gauge
	RedisPoolConnectionsActive prometheus.Gauge
	RedisPoolHitsTotal         prometheus.Counter
	RedisPoolMissesTotal       prometheus.Counter
	RedisPoolTimeoutsTotal     prometheus.Counter

	// Authentication and Rate Limiting Metrics
	// DD-GATEWAY-004: Authentication metrics removed (network-level security)
	// Kept for backward compatibility but not actively used
	AuthenticationFailuresTotal     *prometheus.CounterVec
	AuthenticationDurationSeconds   prometheus.Histogram
	RateLimitExceededTotal          *prometheus.CounterVec
	RateLimitingDroppedSignalsTotal *prometheus.CounterVec // Alias for consistency

	// Additional Handler Metrics
	SignalsReceived             *prometheus.CounterVec
	SignalsFailed               *prometheus.CounterVec
	SignalsProcessed            *prometheus.CounterVec
	RedisOperationErrors        *prometheus.CounterVec
	RequestsRejectedTotal       *prometheus.CounterVec
	DuplicateCRDsPreventedTotal prometheus.Counter
	DuplicatePreventionActive   prometheus.Gauge
	DuplicateSignals            *prometheus.CounterVec
	Consecutive503Responses     *prometheus.GaugeVec
	StormProtectionActive       prometheus.Gauge
	CRDsCreated                 *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with the default global Prometheus registry
// Day 9 Phase 6B Option C1: Factory function for default registry
func NewMetrics() *Metrics {
	return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates a new Metrics instance with a custom Prometheus registry
// Day 9 Phase 6B Option C1: Factory function for custom registry (test isolation)
//
// This allows integration tests to use isolated registries, preventing metric collisions
// when running multiple Gateway instances in parallel tests.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	factory := promauto.With(registry)

	// Store registry as Gatherer for /metrics endpoint exposure
	var gatherer prometheus.Gatherer
	if reg, ok := registry.(prometheus.Gatherer); ok {
		gatherer = reg
	} else {
		gatherer = prometheus.DefaultGatherer
	}

	// Create rate limit metric first (used as alias)
	rateLimitExceeded := factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_exceeded_total",
			Help: "Total rate limit violations by source IP",
		},
		[]string{"source_ip"},
	)

	return &Metrics{
		// Store registry for /metrics endpoint
		registry: gatherer,
		// Signal Ingestion Metrics (BR-GATEWAY-001: Multi-source signal processing)
		AlertsReceivedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signals_received_total",
				Help: "Total signals received by source, severity, and environment (Prometheus alerts, K8s events, etc.)",
			},
			[]string{"source", "severity", "environment"},
		),
		AlertsDeduplicatedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signals_deduplicated_total",
				Help: "Total signals deduplicated (duplicate fingerprint detected)",
			},
			[]string{"signal_name", "environment"},
		),
		AlertStormsDetectedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signal_storms_detected_total",
				Help: "Total signal storms detected by type (rate-based or pattern-based)",
			},
			[]string{"storm_type", "signal_name"},
		),

		// CRD Creation Metrics
		CRDsCreatedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_crds_created_total",
				Help: "Total RemediationRequest CRDs created by environment and priority",
			},
			[]string{"environment", "priority"},
		),
		CRDCreationErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_crd_creation_errors_total",
				Help: "Total CRD creation errors by error type",
			},
			[]string{"error_type"},
		),

		// K8s API Retry Metrics (BR-GATEWAY-114: Retry observability)
		RetryAttemptsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_retry_attempts_total",
				Help: "Total K8s API retry attempts by error type and HTTP status code",
			},
			[]string{"error_type", "status_code"},
		),
		RetryDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_retry_duration_seconds",
				Help:    "Duration of retry attempts (including backoff) by error type",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 8), // 100ms to ~25s
			},
			[]string{"error_type"},
		),
		RetryExhaustedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_retry_exhausted_total",
				Help: "Total retries exhausted (max attempts reached) by error type and HTTP status code",
			},
			[]string{"error_type", "status_code"},
		),
		RetrySuccessTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_retry_success_total",
				Help: "Total successful retries by error type and attempt number",
			},
			[]string{"error_type", "attempt"},
		),

		// Performance Metrics
		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds (includes full pipeline)",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
			},
			[]string{"endpoint", "method", "status"},
		),
		RedisOperationDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gateway_redis_operation_duration_seconds",
				Help:    "Redis operation duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
			},
			[]string{"operation"},
		),

		// Deduplication Metrics
		DeduplicationCacheHitsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_deduplication_cache_hits_total",
				Help: "Total deduplication cache hits (duplicate fingerprint found in Redis)",
			},
		),
		DeduplicationCacheMissesTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_deduplication_cache_misses_total",
				Help: "Total deduplication cache misses (new fingerprint, not in Redis)",
			},
		),
		DeduplicationRate: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_deduplication_rate",
				Help: "Current deduplication rate (percentage of alerts deduplicated)",
			},
		),
		DeduplicationPoolSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_deduplication_pool_size",
				Help: "Current Redis connection pool size for deduplication",
			},
		),
		DeduplicationPoolMaxSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_deduplication_pool_max_size",
				Help: "Maximum Redis connection pool size for deduplication",
			},
		),

		// HTTP Observability Metrics (Day 9 Phase 4)
		HTTPRequestsInFlight: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_http_requests_in_flight",
				Help: "Current number of HTTP requests being processed",
			},
		),
		HTTPRequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_http_requests_total",
				Help: "Total HTTP requests by method, path, and status code",
			},
			[]string{"method", "path", "status"},
		),

		// Redis Health Metrics (Day 9 Phase 4)
		RedisAvailable: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_available",
				Help: "Redis availability status (1 = available, 0 = unavailable)",
			},
		),
		RedisOutageDuration: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_redis_outage_duration_seconds_total",
				Help: "Total cumulative Redis outage duration in seconds",
			},
		),
		RedisOutageCount: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_redis_outage_count_total",
				Help: "Total number of Redis outage events",
			},
		),

		// Redis Pool Metrics (Day 9 Phase 4)
		RedisPoolHits: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_hits",
				Help: "Number of times a free connection was found in the pool",
			},
		),
		RedisPoolMisses: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_misses",
				Help: "Number of times a free connection was NOT found in the pool",
			},
		),
		RedisPoolTimeouts: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_timeouts",
				Help: "Number of times a wait timeout occurred when getting a connection",
			},
		),
		RedisPoolTotal: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_total_connections",
				Help: "Total number of connections in the pool",
			},
		),
		RedisPoolIdle: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_idle_connections",
				Help: "Number of idle connections in the pool",
			},
		),
		RedisPoolStale: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_stale_connections",
				Help: "Number of stale connections removed from the pool",
			},
		),

		// Redis Pool Metrics - Test-Compatible Names (Pre-Day 10 Unit Test Validation)
		RedisPoolConnectionsTotal: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_connections_total",
				Help: "Total number of connections in the Redis pool (test-compatible name)",
			},
		),
		RedisPoolConnectionsIdle: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_connections_idle",
				Help: "Number of idle connections in the Redis pool (test-compatible name)",
			},
		),
		RedisPoolConnectionsActive: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_redis_pool_connections_active",
				Help: "Number of active connections in the Redis pool (test-compatible name)",
			},
		),
		RedisPoolHitsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_redis_pool_hits_total",
				Help: "Total number of times a free connection was found in the pool (test-compatible name)",
			},
		),
		RedisPoolMissesTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_redis_pool_misses_total",
				Help: "Total number of times a free connection was NOT found in the pool (test-compatible name)",
			},
		),
		RedisPoolTimeoutsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_redis_pool_timeouts_total",
				Help: "Total number of times a wait timeout occurred when getting a connection (test-compatible name)",
			},
		),

		// Authentication and Rate Limiting Metrics
		// DD-GATEWAY-004: Authentication metrics kept for backward compatibility
		AuthenticationFailuresTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_authentication_failures_total",
				Help: "Total authentication failures by reason (DD-GATEWAY-004: deprecated, kept for compatibility)",
			},
			[]string{"reason"},
		),
		AuthenticationDurationSeconds: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "gateway_authentication_duration_seconds",
				Help:    "TokenReview API call duration in seconds (DD-GATEWAY-004: deprecated, kept for compatibility)",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
			},
		),
		RateLimitExceededTotal:          rateLimitExceeded,
		RateLimitingDroppedSignalsTotal: rateLimitExceeded, // Alias for consistency

		// Additional Handler Metrics
		SignalsReceived: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signals_received_by_adapter_total",
				Help: "Total signals received by adapter type (prometheus, kubernetes-event, etc.)",
			},
			[]string{"adapter"},
		),
		SignalsFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signals_failed_total",
				Help: "Total signals that failed processing",
			},
			[]string{"reason"},
		),
		RedisOperationErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_redis_operation_errors_total",
				Help: "Total Redis operation errors",
			},
			[]string{"operation"},
		),
		RequestsRejectedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_requests_rejected_total",
				Help: "Total requests rejected",
			},
			[]string{"reason"},
		),
		DuplicateCRDsPreventedTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "gateway_duplicate_crds_prevented_total",
				Help: "Total duplicate CRDs prevented by deduplication",
			},
		),
		DuplicatePreventionActive: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_duplicate_prevention_active",
				Help: "Whether duplicate prevention is active (1 = active, 0 = inactive)",
			},
		),
		DuplicateSignals: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_duplicate_signals_total",
				Help: "Total duplicate signals detected",
			},
			[]string{"namespace"},
		),
		Consecutive503Responses: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gateway_consecutive_503_responses",
				Help: "Consecutive 503 responses per namespace (Redis outage tracking)",
			},
			[]string{"namespace"},
		),
		SignalsProcessed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_signals_processed_total",
				Help: "Total signals successfully processed",
			},
			[]string{"environment"},
		),
		StormProtectionActive: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "gateway_storm_protection_active",
				Help: "Whether storm protection is active (1 = active, 0 = inactive)",
			},
		),
		CRDsCreated: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gateway_crds_created_by_type_total",
				Help: "Total CRDs created by type",
			},
			[]string{"type"},
		),
	}
}

// Registry returns the Prometheus Gatherer for this metrics instance
// This is used by the /metrics endpoint to expose metrics from custom registries
func (m *Metrics) Registry() prometheus.Gatherer {
	if m.registry != nil {
		return m.registry
	}
	return prometheus.DefaultGatherer
}
