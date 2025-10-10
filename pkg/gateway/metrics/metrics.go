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
// This package defines all metrics for the Gateway service, following the
// specifications in docs/services/stateless/gateway-service/metrics-slos.md
//
// Metrics are organized into categories:
// 1. Alert ingestion (received, deduplicated, storms)
// 2. CRD creation (success, failures)
// 3. Performance (HTTP request duration, Redis operation duration)
// 4. Deduplication (cache hits/misses, deduplication rate)
//
// All metrics are automatically registered with Prometheus at package initialization
// using promauto, which ensures they're available on the /metrics endpoint.

// Alert Ingestion Metrics

var (
	// AlertsReceivedTotal tracks total alerts received by source, severity, and environment
	//
	// Labels:
	// - source: Adapter name ("prometheus", "kubernetes-event", etc.)
	// - severity: Alert severity ("critical", "warning", "info")
	// - environment: Target environment ("prod", "staging", "dev")
	//
	// Use cases:
	// - Monitor alert volume by source
	// - Identify high-severity alert spikes
	// - Track environment-specific alert rates
	//
	// Example query: rate(gateway_alerts_received_total{severity="critical"}[5m])
	AlertsReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_alerts_received_total",
			Help: "Total alerts received by source, severity, and environment",
		},
		[]string{"source", "severity", "environment"},
	)

	// AlertsDeduplicatedTotal tracks alerts that were deduplicated (existing fingerprint)
	//
	// Labels:
	// - alertname: Alert name from signal (e.g., "HighMemoryUsage")
	// - environment: Target environment ("prod", "staging", "dev")
	//
	// Use cases:
	// - Calculate deduplication rate (target: 40-60%)
	// - Identify frequently repeating alerts
	// - Monitor deduplication effectiveness
	//
	// Example query: rate(gateway_alerts_deduplicated_total[5m])
	AlertsDeduplicatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_alerts_deduplicated_total",
			Help: "Total alerts deduplicated (duplicate fingerprint detected)",
		},
		[]string{"alertname", "environment"},
	)

	// AlertStormsDetectedTotal tracks alert storms by type
	//
	// Labels:
	// - storm_type: Storm detection method ("rate" or "pattern")
	// - alertname: Alert name from signal
	//
	// Use cases:
	// - Monitor storm detection effectiveness
	// - Identify problematic alert patterns
	// - Track rate-based vs pattern-based storm frequency
	//
	// Storm thresholds:
	// - Rate-based: >10 alerts/minute for same alertname
	// - Pattern-based: >5 similar alerts across different resources in 2 minutes
	//
	// Example query: sum(rate(gateway_alert_storms_detected_total[5m])) by (storm_type)
	AlertStormsDetectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_alert_storms_detected_total",
			Help: "Total alert storms detected by type (rate or pattern)",
		},
		[]string{"storm_type", "alertname"},
	)
)

// CRD Creation Metrics

var (
	// RemediationRequestCreatedTotal tracks successful RemediationRequest CRD creations
	//
	// Labels:
	// - environment: Target environment ("prod", "staging", "dev")
	// - priority: Assigned priority ("P0", "P1", "P2")
	//
	// Use cases:
	// - Monitor CRD creation rate
	// - Track priority distribution
	// - Measure downstream system load (each CRD triggers remediation workflow)
	//
	// Example query: sum(rate(gateway_remediationrequest_created_total{priority="P0"}[5m]))
	RemediationRequestCreatedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_remediationrequest_created_total",
			Help: "Total RemediationRequest CRDs created successfully",
		},
		[]string{"environment", "priority"},
	)

	// RemediationRequestCreationFailuresTotal tracks CRD creation failures
	//
	// Labels:
	// - error_type: Failure reason ("k8s_api_error", "validation_error", "timeout", etc.)
	//
	// Use cases:
	// - Monitor CRD creation reliability
	// - Alert on elevated failure rates
	// - Identify specific failure causes (K8s API issues, validation errors)
	//
	// SLO: <1% failure rate
	//
	// Example query: rate(gateway_remediationrequest_creation_failures_total[5m])
	RemediationRequestCreationFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_remediationrequest_creation_failures_total",
			Help: "Total RemediationRequest CRD creation failures by error type",
		},
		[]string{"error_type"},
	)
)

// Performance Metrics

var (
	// HTTPRequestDuration tracks HTTP request latency distribution
	//
	// Labels:
	// - endpoint: HTTP route ("/api/v1/signals/prometheus", "/health", etc.)
	// - method: HTTP method ("POST", "GET")
	// - status: HTTP status code ("200", "202", "400", "500", etc.)
	//
	// Buckets: Exponential buckets from 1ms to ~1s
	// - 0.001, 0.002, 0.004, 0.008, 0.016, 0.032, 0.064, 0.128, 0.256, 0.512, 1.024
	//
	// Use cases:
	// - Monitor p95/p99 latency (target: p95 < 50ms, p99 < 100ms)
	// - Identify slow endpoints
	// - Track status code distribution
	//
	// SLO: p95 < 50ms, p99 < 100ms
	//
	// Example query: histogram_quantile(0.95, rate(gateway_http_request_duration_seconds_bucket[5m]))
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds (includes full pipeline)",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
		[]string{"endpoint", "method", "status"},
	)

	// RedisOperationDuration tracks Redis operation latency distribution
	//
	// Labels:
	// - operation: Redis operation type ("deduplication_check", "deduplication_store",
	//              "storm_detection_rate", "storm_detection_pattern")
	//
	// Buckets: Exponential buckets from 0.1ms to ~100ms
	// - 0.0001, 0.0002, 0.0004, 0.0008, 0.0016, 0.0032, 0.0064, 0.0128, 0.0256, 0.0512, 0.1024
	//
	// Use cases:
	// - Monitor Redis performance (target: p95 < 5ms, p99 < 10ms)
	// - Identify slow Redis operations
	// - Detect Redis latency degradation
	//
	// SLO: p95 < 5ms, p99 < 10ms
	//
	// Example query: histogram_quantile(0.95, rate(gateway_redis_operation_duration_seconds_bucket[5m]))
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_redis_operation_duration_seconds",
			Help:    "Redis operation duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
		},
		[]string{"operation"},
	)
)

// Deduplication Metrics

var (
	// DeduplicationCacheHitsTotal tracks Redis cache hits (duplicate fingerprint found)
	//
	// Use cases:
	// - Calculate deduplication rate: hits / (hits + misses)
	// - Monitor cache effectiveness
	// - Identify patterns in alert repetition
	//
	// Target: 40-60% cache hit rate (typical production)
	//
	// Example query: rate(gateway_deduplication_cache_hits_total[5m])
	DeduplicationCacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_deduplication_cache_hits_total",
			Help: "Total deduplication cache hits (duplicate fingerprint found in Redis)",
		},
	)

	// DeduplicationCacheMissesTotal tracks Redis cache misses (new fingerprint)
	//
	// Use cases:
	// - Calculate deduplication rate
	// - Monitor new alert creation rate
	// - Track cache effectiveness
	//
	// Example query: rate(gateway_deduplication_cache_misses_total[5m])
	DeduplicationCacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_deduplication_cache_misses_total",
			Help: "Total deduplication cache misses (new fingerprint, not in Redis)",
		},
	)

	// DeduplicationRate tracks the percentage of alerts that were deduplicated
	//
	// Value: 0-100 (percentage)
	// Calculation: (hits / (hits + misses)) * 100
	//
	// Use cases:
	// - Real-time deduplication effectiveness monitoring
	// - Alerting on abnormal deduplication rates
	// - Capacity planning (high rate = fewer downstream resources needed)
	//
	// Target: 40-60% (typical production)
	//
	// Example query: gateway_deduplication_rate
	DeduplicationRate = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_deduplication_rate",
			Help: "Percentage of alerts that were deduplicated (0-100)",
		},
	)

	// RedisConnectionPoolSize tracks active Redis connections
	//
	// Use cases:
	// - Monitor connection pool utilization
	// - Identify connection pool exhaustion
	// - Capacity planning for Redis cluster
	//
	// Max: 100 connections (configured in redis client)
	//
	// Example query: gateway_redis_connection_pool_size
	RedisConnectionPoolSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_redis_connection_pool_size",
			Help: "Current number of active Redis connections in pool",
		},
	)

	// RedisConnectionPoolMaxSize tracks maximum Redis connection pool size
	//
	// Use cases:
	// - Verify configuration
	// - Calculate pool utilization percentage
	//
	// Value: 100 (configured in redis client)
	//
	// Example query: gateway_redis_connection_pool_max_size
	RedisConnectionPoolMaxSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_redis_connection_pool_max_size",
			Help: "Maximum Redis connection pool size (configured limit)",
		},
	)
)

// Authentication and Rate Limiting Metrics

var (
	// AuthenticationFailuresTotal tracks authentication failures
	//
	// Labels:
	// - reason: Failure reason ("missing_token", "invalid_token", "expired_token", "api_error")
	//
	// Use cases:
	// - Monitor authentication issues
	// - Detect potential security issues (repeated failures from same source)
	// - Alert on elevated authentication failure rates
	//
	// Example query: rate(gateway_authentication_failures_total[5m])
	AuthenticationFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_authentication_failures_total",
			Help: "Total authentication failures by reason",
		},
		[]string{"reason"},
	)

	// AuthenticationDurationSeconds tracks TokenReview API latency
	//
	// Buckets: Exponential buckets from 1ms to ~1s
	//
	// Use cases:
	// - Monitor TokenReview API performance (target: p95 < 30ms, p99 < 50ms)
	// - Detect Kubernetes API slowness
	// - Track authentication overhead
	//
	// Example query: histogram_quantile(0.95, rate(gateway_authentication_duration_seconds_bucket[5m]))
	AuthenticationDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gateway_authentication_duration_seconds",
			Help:    "TokenReview API call duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
	)

	// RateLimitExceededTotal tracks rate limit violations
	//
	// Labels:
	// - source_ip: Client IP address (for debugging)
	//
	// Use cases:
	// - Monitor rate limiting effectiveness
	// - Identify noisy clients
	// - Detect potential DoS attempts
	//
	// Limit: 100 alerts/minute per source IP (configurable)
	//
	// Example query: topk(10, sum(rate(gateway_rate_limit_exceeded_total[5m])) by (source_ip))
	RateLimitExceededTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_exceeded_total",
			Help: "Total rate limit violations by source IP",
		},
		[]string{"source_ip"},
	)

	// RateLimitingDroppedSignalsTotal is an alias for RateLimitExceededTotal
	// This maintains naming consistency with other "dropped" metrics
	RateLimitingDroppedSignalsTotal = RateLimitExceededTotal
)

// Summary:
// - Total metrics defined: 15+
// - Alert ingestion: 3 (received, deduplicated, storms)
// - CRD creation: 2 (created, failures)
// - Performance: 2 (HTTP duration, Redis duration)
// - Deduplication: 5 (hits, misses, rate, pool size, pool max)
// - Auth/Rate Limiting: 2 (auth failures, rate limit exceeded)
//
// All metrics follow Prometheus naming conventions:
// - snake_case naming
// - _total suffix for counters
// - _seconds suffix for duration histograms
// - Descriptive help text
// - Appropriate labels for dimensionality
