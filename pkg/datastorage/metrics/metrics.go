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
	MetricNameWriteDuration = "datastorage_write_duration_seconds"

	// Audit write API metrics (GAP-10)
	MetricNameAuditLagSeconds = "datastorage_audit_lag_seconds"

	// #1048 Phase 4 / BR-STORAGE-019: OpenAPI + DLQ validation failure counters
	MetricNameValidationFailures    = "datastorage_validation_failures_total"
	MetricNameDLQValidationFailures = "datastorage_dlq_validation_failures_total"

	// #1048 Phase 5 / AU-11: XADD + MAXLEN~ trim observability (combined with DLQ depth gauge)
	MetricNameDLQStreamXAddTotal = "datastorage_dlq_stream_xadd_total"

	// Workflow validation phase metrics (Issue #1070)
	MetricNameWorkflowValidationDuration = "datastorage_workflow_validation_duration_seconds"

	// #1088 Phase 7: Observability & Resilience metrics
	MetricNameDLQDrainBatchTotal    = "datastorage_dlq_drain_batch_total"
	MetricNameRetentionPurgeTotal   = "datastorage_retention_purge_total"
	MetricNameDLQPelPending         = "datastorage_dlq_pel_pending"
	MetricNameShutdownDLQDrainError = "datastorage_shutdown_dlq_drain_errors_total"

	// SRE-1 / AU-11: Guards RetentionPurgeStalled alert when retention is disabled
	MetricNameRetentionEnabled = "datastorage_retention_enabled"
)

// Write operation metrics
// BR-STORAGE-001, BR-STORAGE-002, BR-STORAGE-014

var (
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
			// prometheus.DefBuckets: 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table"},
	)
)

// #1048 Phase 4 / BR-STORAGE-019: Validation failure metrics
var (
	// ValidationFailures counts OpenAPI middleware validation rejections.
	// Labels:
	//   - source: Validation source (e.g., "openapi_middleware")
	//   - reason: Failure reason (e.g., "validation_error")
	ValidationFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameValidationFailures,
			Help: "Total number of request validation failures",
		},
		[]string{"source", "reason"},
	)

	// DLQValidationFailures counts DLQ replay validation rejections.
	// Labels:
	//   - audit_type: "events" or "notifications"
	//   - reason: "field_validation", "size_or_depth", "unmarshal_error"
	DLQValidationFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDLQValidationFailures,
			Help: "Total number of DLQ message validation failures during replay",
		},
		[]string{"audit_type", "reason"},
	)

	// #1048 Phase 5 / AU-11: Counts XADD operations that may trigger MAXLEN~ trimming.
	// Combined with dlq_depth gauge, operators can detect active trimming.
	DLQStreamXAddTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDLQStreamXAddTotal,
			Help: "Total XADD operations per stream (each may trigger MAXLEN~ trim)",
		},
		[]string{"stream"},
	)
)

// #1088 Phase 7: Observability & Resilience metrics
var (
	DLQDrainBatchTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: MetricNameDLQDrainBatchTotal,
		Help: "Total DLQ drain invocations during shutdown (one per shutdown cycle, not per XRange batch)",
	})
	RetentionPurgeTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: MetricNameRetentionPurgeTotal,
		Help: "Total retention purge operations",
	})
	DLQPelPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: MetricNameDLQPelPending,
		Help: "Number of pending entries in the DLQ PEL (XPENDING count)",
	})
	ShutdownDLQDrainError = promauto.NewCounter(prometheus.CounterOpts{
		Name: MetricNameShutdownDLQDrainError,
		Help: "Total DLQ drain errors during shutdown",
	})
	RetentionEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: MetricNameRetentionEnabled,
		Help: "1 when retention worker is enabled, 0 when disabled (guards RetentionPurgeStalled alert)",
	})
)

// Workflow validation phase metrics (Issue #1070)
// BR-STORAGE-014: Workflow catalog management

var (
	// WorkflowValidationDuration tracks the duration of each validation phase
	// during workflow registration.
	//
	// Labels:
	//   - phase: Validation phase ("action_type", "bundle_exists", "dependency", "total")
	//   - result: Outcome ("ok", "error")
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_workflow_validation_duration_seconds_bucket{phase="total"}[5m]))
	WorkflowValidationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameWorkflowValidationDuration,
			Help:    "Duration of workflow validation phases in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"phase", "result"},
	)
)

// Audit write API metrics (GAP-10)
// BR-STORAGE-001 to BR-STORAGE-020: Audit trail metrics

var (
	// AuditLagSeconds tracks time lag between event occurrence and audit write.
	//
	// Labels:
	//   - service: Service type (notification, gateway, remediation, etc.)
	//
	// Example Prometheus query:
	//   histogram_quantile(0.95, rate(datastorage_audit_lag_seconds_bucket[5m]))
	AuditLagSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameAuditLagSeconds, // DD-005 V3.0: Pattern B (full name),
			Help:    "Time lag between event occurrence and audit write in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service"},
	)
)

// Metrics Summary:
//
// Total Metrics: 11 (external-facing per GitHub issue #294, #1048, #1070, #1088)
// - WriteDuration (Histogram with labels) - write operation performance
// - AuditLagSeconds (Histogram with labels) - audit lag observability
// - ValidationFailures (Counter with labels) - OpenAPI validation rejections (#1048)
// - DLQValidationFailures (Counter with labels) - DLQ replay validation failures (#1048)
// - DLQStreamXAddTotal (Counter with labels) - DLQ stream XADD / trim correlation (#1048 Phase 5)
// - WorkflowValidationDuration (Histogram with labels) - validation phase timing (Issue #1070)
// - DLQDrainBatchTotal (Counter) - shutdown drain operations (#1088 Phase 7)
// - RetentionPurgeTotal (Counter) - retention purge operations (#1088 Phase 7)
// - DLQPelPending (Gauge) - PEL backlog depth (#1088 Phase 7)
// - ShutdownDLQDrainError (Counter) - shutdown drain errors (#1088 Phase 7)
// - RetentionEnabled (Gauge) - retention worker enabled flag (SRE-1 / AU-11)
//
// Performance Target: < 5% overhead
// BR Coverage: BR-STORAGE-001, 002, 007, 008, 012, 013, 019
//
// ⚠️ CRITICAL: Label Value Guidelines for Cardinality Protection
//
// To prevent high-cardinality explosions (which can cause Prometheus performance degradation):
//
// ✅ ALWAYS USE:
//   - Bounded, enum-like values from helpers.go constants
//   - Schema-defined field names (never user input)
//
// ❌ NEVER USE:
//   - Error messages: err.Error() // ❌ Unlimited cardinality
//   - User input: audit.Name // ❌ User-controlled cardinality
//   - Timestamps: time.Now().String() // ❌ One time series per millisecond
//   - IDs: fmt.Sprintf("%d", audit.ID) // ❌ One time series per record
//
// ✅ CORRECT EXAMPLES:
//   metrics.WriteDuration.WithLabelValues("audit_events").Observe(duration)
//   metrics.AuditLagSeconds.WithLabelValues(metrics.ServiceNotification).Observe(lag)
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
// GAP-10: Audit-specific metrics (external-facing per GitHub issue #294)
type Metrics struct {
	// GAP-10: Audit-specific metrics
	AuditLagSeconds *prometheus.HistogramVec // Time lag between event and audit write

	// Write operation metrics
	WriteDuration *prometheus.HistogramVec // Write operation duration

	// #1048 Phase 4: Validation metrics
	ValidationFailures    *prometheus.CounterVec // OpenAPI middleware rejections
	DLQValidationFailures *prometheus.CounterVec // DLQ replay validation failures

	// #1048 Phase 5 / AU-11: XADD observability for MAXLEN~ trimming
	DLQStreamXAddTotal *prometheus.CounterVec // Per-stream XADD (may trigger trimming)

	// Workflow validation phase metrics (Issue #1070)
	WorkflowValidationDuration *prometheus.HistogramVec // Validation phase timing

	// #1088 Phase 7: Observability & Resilience
	DLQDrainBatchTotal    prometheus.Counter // Drain batch operations during shutdown
	RetentionPurgeTotal   prometheus.Counter // Retention purge operations
	DLQPelPending         prometheus.Gauge   // PEL pending entries
	ShutdownDLQDrainError prometheus.Counter // DLQ drain errors during shutdown

	// SRE-1 / AU-11: Guards RetentionPurgeStalled alert
	RetentionEnabled prometheus.Gauge // 1 when enabled, 0 when disabled

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
		assignGlobalMetrics(m)
	} else {
		newIsolatedMetrics(m, reg)
	}

	return m
}

// assignGlobalMetrics points m's fields at the package-level promauto-registered
// global metrics (production path: DefaultRegisterer, already registered on
// package init). Extracted from NewMetricsWithRegistry (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func assignGlobalMetrics(m *Metrics) {
	m.AuditLagSeconds = AuditLagSeconds
	m.WriteDuration = WriteDuration
	m.ValidationFailures = ValidationFailures
	m.DLQValidationFailures = DLQValidationFailures
	m.DLQStreamXAddTotal = DLQStreamXAddTotal
	m.WorkflowValidationDuration = WorkflowValidationDuration
	m.DLQDrainBatchTotal = DLQDrainBatchTotal
	m.RetentionPurgeTotal = RetentionPurgeTotal
	m.DLQPelPending = DLQPelPending
	m.ShutdownDLQDrainError = ShutdownDLQDrainError
	m.RetentionEnabled = RetentionEnabled
}

// newIsolatedMetrics creates fresh, isolated metric instances on m and
// registers them against reg (testing path: a custom registry, to avoid
// duplicate-registration panics against the global DefaultRegisterer).
// Extracted from NewMetricsWithRegistry (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func newIsolatedMetrics(m *Metrics, reg prometheus.Registerer) {
	newIsolatedHistogramMetrics(m)
	newIsolatedCounterAndGaugeMetrics(m)

	reg.MustRegister(
		m.AuditLagSeconds,
		m.WriteDuration,
		m.ValidationFailures,
		m.DLQValidationFailures,
		m.DLQStreamXAddTotal,
		m.WorkflowValidationDuration,
		m.DLQDrainBatchTotal,
		m.RetentionPurgeTotal,
		m.DLQPelPending,
		m.ShutdownDLQDrainError,
		m.RetentionEnabled,
	)
}

// newIsolatedHistogramMetrics creates the isolated-registry histogram
// metrics. Extracted from newIsolatedMetrics (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func newIsolatedHistogramMetrics(m *Metrics) {
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

	m.WorkflowValidationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameWorkflowValidationDuration,
			Help:    "Duration of workflow validation phases in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"phase", "result"},
	)
}

// newIsolatedCounterAndGaugeMetrics creates the isolated-registry counter and
// gauge metrics. Extracted from newIsolatedMetrics (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func newIsolatedCounterAndGaugeMetrics(m *Metrics) {
	m.ValidationFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameValidationFailures,
			Help: "Total number of request validation failures",
		},
		[]string{"source", "reason"},
	)

	m.DLQValidationFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDLQValidationFailures,
			Help: "Total number of DLQ message validation failures during replay",
		},
		[]string{"audit_type", "reason"},
	)

	m.DLQStreamXAddTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDLQStreamXAddTotal,
			Help: "Total XADD operations per stream (each may trigger MAXLEN~ trim)",
		},
		[]string{"stream"},
	)

	m.DLQDrainBatchTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: MetricNameDLQDrainBatchTotal,
		Help: "Total DLQ drain batch operations during shutdown",
	})
	m.RetentionPurgeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: MetricNameRetentionPurgeTotal,
		Help: "Total retention purge operations",
	})
	m.DLQPelPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: MetricNameDLQPelPending,
		Help: "Number of pending entries in the DLQ PEL",
	})
	m.ShutdownDLQDrainError = prometheus.NewCounter(prometheus.CounterOpts{
		Name: MetricNameShutdownDLQDrainError,
		Help: "Total DLQ drain errors during shutdown",
	})
	m.RetentionEnabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: MetricNameRetentionEnabled,
		Help: "1 when retention worker is enabled, 0 when disabled",
	})
}
