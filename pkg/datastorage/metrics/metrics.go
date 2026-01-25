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
// - Fallback mode operations
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

// ========================================
// DD-005 V3.0: Metric Name Constants (Pattern B)
// ========================================
//
// Per DD-005 V3.0 mandate, all metric names MUST be defined as constants
// to prevent typos and ensure test/production parity.
//
// Pattern B: Full metric names (no Namespace/Subsystem in prometheus.Opts)
// Reference: pkg/workflowexecution/metrics/metrics.go
// ========================================

const (
	// Write operation metrics
	MetricNameWriteTotal    = "datastorage_write_total"
	MetricNameWriteDuration = "datastorage_write_duration_seconds"

	// Fallback mode metrics
	MetricNameFallbackMode = "datastorage_fallback_mode_total"

	// Audit write API metrics (GAP-10)
	MetricNameAuditTracesTotal = "datastorage_audit_traces_total"
	MetricNameAuditLagSeconds  = "datastorage_audit_lag_seconds"

	// Embedding generation and caching metrics
	MetricNameCacheHits                     = "datastorage_cache_hits_total"
	MetricNameCacheMisses                   = "datastorage_cache_misses_total"
	MetricNameEmbeddingGenerationDuration   = "datastorage_embedding_generation_duration_seconds"

	// Validation metrics
	MetricNameValidationFailures = "datastorage_validation_failures_total"

	// Query operation metrics
	MetricNameQueryDuration = "datastorage_query_duration_seconds"
	MetricNameQueryTotal    = "datastorage_query_total"
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
			Name: MetricNameWriteTotal, // DD-005 V3.0: Pattern B (full name)
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
			Name: MetricNameWriteDuration, // DD-005 V3.0: Pattern B (full name),
			Help: "Duration of write operations in seconds",
			// Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table"},
	)
)

// Fallback mode metrics
// BR-STORAGE-015: Graceful degradation during DLQ operations

var (
	// FallbackModeTotal tracks PostgreSQL-only fallback operations.
	//
	// Example Prometheus query:
	//   rate(datastorage_fallback_mode_total[5m])
	FallbackModeTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: MetricNameFallbackMode, // DD-005 V3.0: Pattern B (full name),
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
			Name: MetricNameAuditTracesTotal, // DD-005 V3.0: Pattern B (full name),
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
			Name: MetricNameAuditLagSeconds, // DD-005 V3.0: Pattern B (full name),
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
			Name: MetricNameCacheHits, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of embedding cache hits",
		},
	)

	// CacheMisses tracks embedding cache misses requiring generation.
	//
	// Example Prometheus query:
	//   rate(datastorage_cache_misses_total[5m])
	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: MetricNameCacheMisses, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of embedding cache misses",
		},
	)

	// EmbeddingGenerationDuration tracks the time to generate vector embeddings.
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_embedding_generation_duration_seconds_bucket[5m]))
	EmbeddingGenerationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: MetricNameEmbeddingGenerationDuration, // DD-005 V3.0: Pattern B (full name),
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
			Name: MetricNameValidationFailures, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of validation failures by field and reason",
		},
		[]string{"field", "reason"},
	)
)

// Legal Hold metrics
// SOC2 Gap #8: Legal Hold & Retention
// BR-AUDIT-006: Legal hold capability for Sarbanes-Oxley and HIPAA compliance

var (
	// LegalHoldSuccesses tracks successful legal hold operations.
	//
	// Labels:
	//   - operation: Operation type (place, release, list)
	//
	// Example Prometheus query:
	//   rate(datastorage_legal_hold_successes_total{operation="place"}[5m])
	LegalHoldSuccesses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_legal_hold_successes_total",
			Help: "Total number of successful legal hold operations by type",
		},
		[]string{"operation"},
	)

	// LegalHoldFailures tracks failed legal hold operations.
	//
	// Labels:
	//   - reason: Failure reason (invalid_request, correlation_id_not_found, db_error, etc.)
	//
	// Example Prometheus query:
	//   rate(datastorage_legal_hold_failures_total[5m]) by (reason)
	LegalHoldFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "datastorage_legal_hold_failures_total",
			Help: "Total number of failed legal hold operations by reason",
		},
		[]string{"reason"},
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
			Name: MetricNameQueryDuration, // DD-005 V3.0: Pattern B (full name),
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
			Name: MetricNameQueryTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of query operations by type and status",
		},
		[]string{"operation", "status"},
	)
)

// Metrics Summary:
//
// Total Metrics: 9
// - WriteTotal (Counter with labels)
// - WriteDuration (Histogram with labels)
// - FallbackModeTotal (Counter)
// - CacheHits (Counter)
// - CacheMisses (Counter)
// - EmbeddingGenerationDuration (Histogram)
// - ValidationFailures (Counter with labels)
// - QueryDuration (Histogram with labels)
// - QueryTotal (Counter with labels)
//
// Performance Target: < 5% overhead
// BR Coverage: BR-STORAGE-001, 002, 007, 008, 009, 010, 011, 012, 013, 015, 019
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
//   metrics.ValidationFailures.WithLabelValues("name", metrics.ValidationReasonRequired).Inc()
//   metrics.WriteTotal.WithLabelValues(metrics.TableRemediationAudit, metrics.StatusSuccess).Inc()
//   metrics.FallbackModeTotal.Inc()
//
// ❌ INCORRECT EXAMPLES (DO NOT DO THIS):
//   metrics.ValidationFailures.WithLabelValues(audit.Name, "error").Inc() // ❌ User input
//   metrics.WriteTotal.WithLabelValues(tableName, time.Now().String()).Inc() // ❌ Timestamp
//
// Current Cardinality: ~72 unique label combinations (SAFE - well under 100 target)
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

	// SOC2 Gap #8: Legal Hold metrics
	LegalHoldSuccesses *prometheus.CounterVec // Successful legal hold operations
	LegalHoldFailures  *prometheus.CounterVec // Failed legal hold operations

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
		m.LegalHoldSuccesses = LegalHoldSuccesses // Reference global (SOC2 Gap #8)
		m.LegalHoldFailures = LegalHoldFailures   // Reference global (SOC2 Gap #8)
	} else {
		// Testing: Create isolated metrics with custom registry
		// Testing: Create isolated metrics with full names (DD-005 V3.0: Pattern B)
		m.AuditTracesTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuditTracesTotal, // DD-005 V3.0: Use constant
				Help: "Total number of audit traces written by service and status",
			},
			[]string{"service", "status"},
		)

		m.AuditLagSeconds = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameAuditLagSeconds, // DD-005 V3.0: Use constant
				Help:    "Time lag between event occurrence and audit write in seconds",
				Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"service"},
		)

		m.WriteDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameWriteDuration, // DD-005 V3.0: Use constant
				Help:    "Duration of write operations in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"table"},
		)

		m.ValidationFailures = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameValidationFailures, // DD-005 V3.0: Use constant
				Help: "Total number of validation failures by field and reason",
			},
			[]string{"field", "reason"},
		)

		// SOC2 Gap #8: Legal Hold metrics (for testing)
		m.LegalHoldSuccesses = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "datastorage_legal_hold_successes_total",
				Help: "Total number of successful legal hold operations by type",
			},
			[]string{"operation"},
		)

		m.LegalHoldFailures = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "datastorage_legal_hold_failures_total",
				Help: "Total number of failed legal hold operations by reason",
			},
			[]string{"reason"},
		)

		// Register ONLY for custom registries (testing)
		reg.MustRegister(
			m.AuditTracesTotal,
			m.AuditLagSeconds,
			m.WriteDuration,
			m.ValidationFailures,
			m.LegalHoldSuccesses,
			m.LegalHoldFailures,
		)
	}

	return m
}
