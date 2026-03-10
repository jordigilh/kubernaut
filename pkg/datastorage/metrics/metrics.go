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
// This package defines external-facing Prometheus metrics (GitHub issue #294):
// - Write operation performance (WriteDuration)
// - Audit lag observability (AuditLagSeconds)
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
	MetricNameWriteDuration = "datastorage_write_duration_seconds"

	// Audit write API metrics (GAP-10)
	MetricNameAuditLagSeconds = "datastorage_audit_lag_seconds"
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
			// Buckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
			Buckets: prometheus.DefBuckets,
		},
		[]string{"table"},
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
			Name: MetricNameAuditLagSeconds, // DD-005 V3.0: Pattern B (full name),
			Help:    "Time lag between event occurrence and audit write in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service"},
	)
)

// Metrics Summary:
//
// Total Metrics: 2 (external-facing per GitHub issue #294)
// - WriteDuration (Histogram with labels) - write operation performance
// - AuditLagSeconds (Histogram with labels) - audit lag observability
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
		m.AuditLagSeconds = AuditLagSeconds // Reference global
		m.WriteDuration = WriteDuration     // Reference global
	} else {
		// Testing: Create isolated metrics with custom registry
		// Testing: Create isolated metrics with full names (DD-005 V3.0: Pattern B)
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

		// Register ONLY for custom registries (testing)
		reg.MustRegister(
			m.AuditLagSeconds,
			m.WriteDuration,
		)
	}

	return m
}
