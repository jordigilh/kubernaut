package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics contains all Prometheus metrics for Context API
// BR-CONTEXT-006: Observability and monitoring
type Metrics struct {
	// Query metrics
	QueriesTotal  *prometheus.CounterVec   // Total queries by type and status
	QueryDuration *prometheus.HistogramVec // Query latency by type

	// Cache metrics
	CacheHits   *prometheus.CounterVec // Cache hits by tier (redis, lru)
	CacheMisses *prometheus.CounterVec // Cache misses by tier

	// Single-flight metrics (Day 11: Cache stampede prevention)
	SingleFlightHits   prometheus.Counter // Requests deduplicated by single-flight
	SingleFlightMisses prometheus.Counter // Requests that executed (not deduplicated)

	// Cache size metrics (Day 11: Large object OOM prevention)
	CachedObjectSize prometheus.Histogram // Size distribution of cached objects (bytes)

	// Vector search metrics
	VectorSearchResults prometheus.Histogram // Number of results from vector search

	// Database metrics
	DatabaseQueries  *prometheus.CounterVec   // Database queries by type
	DatabaseDuration *prometheus.HistogramVec // Database query duration

	// Error metrics
	ErrorsTotal *prometheus.CounterVec // Errors by category and operation

	// HTTP metrics
	HTTPRequests *prometheus.CounterVec   // HTTP requests by method, path, status
	HTTPDuration *prometheus.HistogramVec // HTTP request duration
}

// NewMetrics creates and registers all Prometheus metrics with the default registry
func NewMetrics(namespace, subsystem string) *Metrics {
	return NewMetricsWithRegistry(namespace, subsystem, prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates and registers all Prometheus metrics with a custom registry
// This is useful for testing to avoid duplicate registration panics
func NewMetricsWithRegistry(namespace, subsystem string, registerer prometheus.Registerer) *Metrics {
	factory := promauto.With(registerer)

	return &Metrics{
		QueriesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queries_total",
				Help:      "Total number of API queries by type and status",
			},
			[]string{"type", "status"},
		),

		QueryDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "query_duration_seconds",
				Help:      "Query duration in seconds by type",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"type"},
		),

		CacheHits: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "cache_hits_total",
				Help:      "Total number of cache hits by tier",
			},
			[]string{"tier"}, // redis, lru, database
		),

		CacheMisses: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "cache_misses_total",
				Help:      "Total number of cache misses by tier",
			},
			[]string{"tier"},
		),

		// Day 11: Cache stampede prevention metrics
		SingleFlightHits: factory.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "singleflight_hits_total",
				Help:      "Total number of requests deduplicated by single-flight (waited for shared result)",
			},
		),

		SingleFlightMisses: factory.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "singleflight_misses_total",
				Help:      "Total number of requests that executed database query (not deduplicated)",
			},
		),

		// Day 11: Large object OOM prevention - track cached object sizes
		CachedObjectSize: factory.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "cached_object_size_bytes",
				Help:      "Size distribution of cached objects in bytes",
				Buckets:   []float64{1024, 10240, 102400, 1048576, 5242880, 10485760, 52428800}, // 1KB, 10KB, 100KB, 1MB, 5MB, 10MB, 50MB
			},
		),

		VectorSearchResults: factory.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "vector_search_results",
				Help:      "Number of results from vector similarity search",
				Buckets:   []float64{0, 1, 5, 10, 20, 50, 100, 200},
			},
		),

		DatabaseQueries: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "database_queries_total",
				Help:      "Total number of database queries by type",
			},
			[]string{"type", "status"},
		),

		DatabaseDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "database_duration_seconds",
				Help:      "Database query duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1.0},
			},
			[]string{"type"},
		),

		ErrorsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "errors_total",
				Help:      "Total number of errors by category and operation",
			},
			[]string{"category", "operation"},
		),

		HTTPRequests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests by method, path, and status",
			},
			[]string{"method", "path", "status"},
		),

		HTTPDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1.0, 2.5, 5.0},
			},
			[]string{"method", "path"},
		),
	}
}

// RecordQuerySuccess records a successful query
func (m *Metrics) RecordQuerySuccess(queryType string, duration float64) {
	m.QueriesTotal.WithLabelValues(queryType, "success").Inc()
	m.QueryDuration.WithLabelValues(queryType).Observe(duration)
}

// RecordQueryError records a query error
func (m *Metrics) RecordQueryError(queryType string) {
	m.QueriesTotal.WithLabelValues(queryType, "error").Inc()
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(tier string) {
	m.CacheHits.WithLabelValues(tier).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(tier string) {
	m.CacheMisses.WithLabelValues(tier).Inc()
}

// RecordSingleFlightHit records a request that was deduplicated by single-flight
// Day 11: Cache stampede prevention - tracks deduplication effectiveness
func (m *Metrics) RecordSingleFlightHit() {
	m.SingleFlightHits.Inc()
}

// RecordSingleFlightMiss records a request that executed (not deduplicated)
// Day 11: Cache stampede prevention - tracks actual database executions
func (m *Metrics) RecordSingleFlightMiss() {
	m.SingleFlightMisses.Inc()
}

// RecordCachedObjectSize records the size of a cached object
// Day 11: Large object OOM prevention - tracks size distribution
func (m *Metrics) RecordCachedObjectSize(sizeBytes int) {
	m.CachedObjectSize.Observe(float64(sizeBytes))
}

// RecordDatabaseQuery records a database query
func (m *Metrics) RecordDatabaseQuery(queryType string, duration float64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	m.DatabaseQueries.WithLabelValues(queryType, status).Inc()
	m.DatabaseDuration.WithLabelValues(queryType).Observe(duration)
}

// RecordError records an error
func (m *Metrics) RecordError(category, operation string) {
	m.ErrorsTotal.WithLabelValues(category, operation).Inc()
}

// RecordHTTPRequest records an HTTP request
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration float64) {
	m.HTTPRequests.WithLabelValues(method, path, status).Inc()
	m.HTTPDuration.WithLabelValues(method, path).Observe(duration)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PROMETHEUS METRICS IMPLEMENTATION NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Business Requirements:
// - BR-CONTEXT-006: Observability (metrics, health checks)
//
// Metrics Categories:
// 1. Query Metrics: Track query performance and success rates
// 2. Cache Metrics: Monitor cache effectiveness (hit/miss rates)
// 3. Vector Search Metrics: Track semantic search result counts
// 4. Database Metrics: Monitor database query performance
// 5. Error Metrics: Track errors by category for alerting
// 6. HTTP Metrics: Monitor API request patterns and latency
//
// Performance Targets (BR-CONTEXT-010):
// - p50 latency: < 50ms
// - p95 latency: < 200ms
// - p99 latency: < 500ms
// - Cache hit rate: > 80%
//
// Histogram Buckets:
// - Query duration: 5ms to 10s (realistic range for Context API)
// - Database duration: 1ms to 1s (optimized queries)
// - HTTP duration: 5ms to 5s (end-to-end latency)
//
// Helper Methods:
// - Convenience methods for common metric operations
// - Consistent labeling across all metrics
// - Simplified recording with status tracking
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
