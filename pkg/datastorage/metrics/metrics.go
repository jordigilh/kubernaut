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

// Package metrics provides Prometheus metrics for the Data Storage service.
//
// Business Requirement: BR-STORAGE-019 (Logging and metrics for all operations)
//
// This package defines 10+ Prometheus metrics to monitor:
// - Write operation performance and success rates
// - Dual-write coordination (PostgreSQL + Vector DB)
// - Embedding generation and caching
// - Query operation performance
// - Validation failures
//
// All metrics are automatically registered with Prometheus using promauto.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Write operation metrics
// BR-STORAGE-001, BR-STORAGE-002, BR-STORAGE-014

var (
	// WriteTotal tracks the total number of write operations by table and status.
	//
	// Labels:
	//   - table: Table name (remediation_audit, aianalysis_audit, etc.)
	//   - status: Operation status (success, failure)
	//
	// Example Prometheus query:
	//   rate(datastorage_write_total{status="success"}[5m])
	WriteTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_write_total",
			Help: "Total number of write operations by table and status",
		},
		[]string{"table", "status"},
	)

	// WriteDuration tracks the duration of write operations in seconds.
	//
	// Labels:
	//   - table: Table name
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_write_duration_seconds_bucket[5m]))
	WriteDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "datastorage_write_duration_seconds",
			Help: "Duration of write operations in seconds",
			// Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table"},
	)
)

// Dual-write coordination metrics
// BR-STORAGE-002, BR-STORAGE-014, BR-STORAGE-015

var (
	// DualWriteSuccess tracks successful dual-write operations (PostgreSQL + Vector DB).
	//
	// Example Prometheus query:
	//   rate(datastorage_dualwrite_success_total[5m])
	DualWriteSuccess = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "datastorage_dualwrite_success_total",
			Help: "Total number of successful dual-write operations",
		},
	)

	// DualWriteFailure tracks failed dual-write operations with failure reason.
	//
	// Labels:
	//   - reason: Failure reason (postgresql_failure, vectordb_failure, validation_failure)
	//
	// Example Prometheus query:
	//   rate(datastorage_dualwrite_failure_total[5m]) by (reason)
	DualWriteFailure = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_dualwrite_failure_total",
			Help: "Total number of failed dual-write operations by reason",
		},
		[]string{"reason"},
	)

	// FallbackModeTotal tracks PostgreSQL-only fallback operations.
	//
	// BR-STORAGE-015: Graceful degradation when Vector DB is unavailable
	//
	// Example Prometheus query:
	//   rate(datastorage_fallback_mode_total[5m])
	FallbackModeTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "datastorage_fallback_mode_total",
			Help: "Total number of operations using PostgreSQL-only fallback mode",
		},
	)
)

// Audit write API metrics (GAP-10)
// BR-STORAGE-001 to BR-STORAGE-020: Audit trail metrics

var (
	// AuditTracesTotal tracks total audit traces written by service and status.
	//
	// Labels:
	//   - service: Service type (notification, gateway, remediation, etc.)
	//   - status: Operation status (success, failure)
	//
	// Example Prometheus query:
	//   rate(datastorage_audit_traces_total{status="success"}[5m])
	AuditTracesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_audit_traces_total",
			Help: "Total number of audit traces written by service and status",
		},
		[]string{"service", "status"},
	)

	// AuditLagSeconds tracks time lag between event occurrence and audit write.
	//
	// Labels:
	//   - service: Service type (notification, gateway, remediation, etc.)
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_audit_lag_seconds_bucket[5m]))
	AuditLagSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "datastorage_audit_lag_seconds",
			Help:    "Time lag between event occurrence and audit write in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service"},
	)
)

// Embedding generation and caching metrics
// BR-STORAGE-008, BR-STORAGE-009

var (
	// CacheHits tracks successful embedding cache retrievals.
	//
	// Example Prometheus query:
	//   rate(datastorage_cache_hits_total[5m])
	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "datastorage_cache_hits_total",
			Help: "Total number of embedding cache hits",
		},
	)

	// CacheMisses tracks embedding cache misses requiring generation.
	//
	// Example Prometheus query:
	//   rate(datastorage_cache_misses_total[5m])
	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "datastorage_cache_misses_total",
			Help: "Total number of embedding cache misses",
		},
	)

	// EmbeddingGenerationDuration tracks the time to generate vector embeddings.
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_embedding_generation_duration_seconds_bucket[5m]))
	EmbeddingGenerationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "datastorage_embedding_generation_duration_seconds",
			Help: "Duration of embedding generation in seconds",
			// Buckets optimized for embedding generation (10ms to 5s)
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5},
		},
	)
)

// Validation metrics
// BR-STORAGE-010, BR-STORAGE-011

var (
	// ValidationFailures tracks validation failures by field and reason.
	//
	// Labels:
	//   - field: Field name that failed validation
	//   - reason: Validation failure reason (required, invalid, length_exceeded, etc.)
	//
	// Example Prometheus query:
	//   rate(datastorage_validation_failures_total[5m]) by (field)
	ValidationFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_validation_failures_total",
			Help: "Total number of validation failures by field and reason",
		},
		[]string{"field", "reason"},
	)
)

// Query operation metrics
// BR-STORAGE-007, BR-STORAGE-012, BR-STORAGE-013

var (
	// QueryDuration tracks the duration of query operations.
	//
	// Labels:
	//   - operation: Operation type (list, get, semantic_search, filter)
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))
	QueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "datastorage_query_duration_seconds",
			Help: "Duration of query operations in seconds",
			// Buckets optimized for query operations (1ms to 1s)
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"operation"},
	)

	// QueryTotal tracks the total number of query operations by operation type.
	//
	// Labels:
	//   - operation: Operation type
	//   - status: Operation status (success, failure)
	//
	// Example Prometheus query:
	//   rate(datastorage_query_total{operation="semantic_search",status="success"}[5m])
	QueryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_query_total",
			Help: "Total number of query operations by type and status",
		},
		[]string{"operation", "status"},
	)
)

// Metrics Summary:
//
// Total Metrics: 11
// - WriteTotal (Counter with labels)
// - WriteDuration (Histogram with labels)
// - DualWriteSuccess (Counter)
// - DualWriteFailure (Counter with labels)
// - FallbackModeTotal (Counter)
// - CacheHits (Counter)
// - CacheMisses (Counter)
// - EmbeddingGenerationDuration (Histogram)
// - ValidationFailures (Counter with labels)
// - QueryDuration (Histogram with labels)
// - QueryTotal (Counter with labels)
//
// Performance Target: < 5% overhead
// BR Coverage: BR-STORAGE-001, 002, 007, 008, 009, 010, 011, 012, 013, 014, 015, 019
//
// ⚠️ CRITICAL: Label Value Guidelines for Cardinality Protection
//
// To prevent high-cardinality explosions (which can cause Prometheus performance degradation):
//
// ✅ ALWAYS USE:
//   - Bounded, enum-like values from helpers.go constants
//   - Sanitization functions (SanitizeFailureReason, SanitizeValidationReason, etc.)
//   - Schema-defined field names (never user input)
//
// ❌ NEVER USE:
//   - Error messages: err.Error() // ❌ Unlimited cardinality
//   - User input: audit.Name // ❌ User-controlled cardinality
//   - Timestamps: time.Now().String() // ❌ One time series per millisecond
//   - IDs: fmt.Sprintf("%d", audit.ID) // ❌ One time series per record
//   - Dynamic strings: fmt.Sprintf("error_%d", i) // ❌ Unlimited cardinality
//
// ✅ CORRECT EXAMPLES:
//   metrics.DualWriteFailure.WithLabelValues(metrics.ReasonPostgreSQLFailure).Inc()
//   metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonRequired).Inc()
//   metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()
//
// ❌ INCORRECT EXAMPLES (DO NOT DO THIS):
//   metrics.DualWriteFailure.WithLabelValues(err.Error()).Inc() // ❌ High cardinality
//   metrics.ValidationFailures.WithLabelValues(audit.Name, "error").Inc() // ❌ User input
//   metrics.WriteTotal.WithLabelValues(tableName, time.Now().String()).Inc() // ❌ Timestamp
//
// Current Cardinality: ~78 unique label combinations (SAFE - well under 100 target)
// - Dual-write failures: 6 values
// - Validation failures: 60 combinations (10 fields × 6 reasons)
// - Write operations: 8 combinations (4 tables × 2 statuses)
// - Query operations: 4 values
//
// For more information, see helpers.go and helpers_test.go

// ========================================
// TDD GREEN PHASE: Metrics Struct
// Tests: pkg/datastorage/metrics/metrics_test.go
// Authority: GAP-10, BR-STORAGE-019
// ========================================
//
// Metrics struct provides dependency injection for Prometheus metrics
// following the Context API pattern for testability.
//
// Benefits:
// - Testable: Use custom registry to avoid duplicate registration
// - Injectable: Pass metrics to handlers via Server struct
// - Isolated: Each test gets its own metrics instance

// Metrics contains all Prometheus metrics for Data Storage Service
// BR-STORAGE-019: Logging and metrics for all operations
// GAP-10: Audit-specific metrics
type Metrics struct {
	// GAP-10: Audit-specific metrics
	AuditTracesTotal *prometheus.CounterVec   // Total audit traces by service and status
	AuditLagSeconds  *prometheus.HistogramVec // Time lag between event and audit write

	// Write operation metrics
	WriteDuration *prometheus.HistogramVec // Write operation duration

	// Validation metrics
	ValidationFailures *prometheus.CounterVec // Validation failures by field and reason

	// Store registry for testing
	registry prometheus.Registerer
}

// NewMetrics creates a Metrics struct that references the global metrics
// Note: Global metrics are already auto-registered via promauto
// This function exists for backwards compatibility and dependency injection
func NewMetrics(namespace, subsystem string) *Metrics {
	return NewMetricsWithRegistry(namespace, subsystem, prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates a Metrics struct with custom registry support
// For testing: provide a custom registry to avoid global metric conflicts
// For production: uses global promauto metrics (already registered)
func NewMetricsWithRegistry(namespace, subsystem string, reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		registry: reg,
	}

	// For production (default registry): Reference existing global promauto metrics
	// For testing (custom registry): Create new isolated metrics
	if reg == prometheus.DefaultRegisterer {
		// Production: Use global promauto metrics (already registered)
		// These are referenced, not created, to avoid duplicate registration
		m.AuditTracesTotal = AuditTracesTotal     // Reference global
		m.AuditLagSeconds = AuditLagSeconds       // Reference global
		m.WriteDuration = WriteDuration           // Reference global
		m.ValidationFailures = ValidationFailures // Reference global
	} else {
		// Testing: Create isolated metrics with custom registry
		m.AuditTracesTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "audit_traces_total",
				Help:      "Total number of audit traces written by service and status",
			},
			[]string{"service", "status"},
		)

		m.AuditLagSeconds = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "audit_lag_seconds",
				Help:      "Time lag between event occurrence and audit write in seconds",
				Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"service"},
		)

		m.WriteDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "write_duration_seconds",
				Help:      "Duration of write operations in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"table"},
		)

		m.ValidationFailures = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "validation_failures_total",
				Help:      "Total number of validation failures by field and reason",
			},
			[]string{"field", "reason"},
		)

		// Register ONLY for custom registries (testing)
		reg.MustRegister(
			m.AuditTracesTotal,
			m.AuditLagSeconds,
			m.WriteDuration,
			m.ValidationFailures,
		)
	}

	return m
}
