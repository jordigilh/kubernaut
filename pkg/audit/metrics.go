package audit

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for audit event buffering and writing.
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
//
// These metrics are exposed by all services using the shared audit library,
// providing consistent observability across the platform.
//
// Key metrics (external-facing):
// - audit_events_dropped_total: Total events dropped due to buffer full (counter)
// - audit_events_written_total: Total events written to storage (counter)
// - audit_batches_failed_total: Total batches failed after max retries (counter)
// - audit_buffer_size: Current buffer size (gauge)
var (
	// auditEventsDropped tracks the total number of audit events dropped due to buffer full.
	//
	// This counter increments when the buffer is full and a new event cannot be buffered.
	// This indicates system overload and should trigger alerts.
	//
	// Alert threshold: >1% drop rate (audit_events_dropped_total / audit_events_written_total)
	auditEventsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "audit_events_dropped_total",
			Help: "Total number of audit events dropped (buffer full)",
		},
		[]string{"service"},
	)

	// auditEventsWritten tracks the total number of audit events written to storage.
	//
	// This counter increments after a batch is successfully written to the Data Storage Service.
	// Use to monitor audit write throughput.
	auditEventsWritten = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "audit_events_written_total",
			Help: "Total number of audit events written to storage",
		},
		[]string{"service"},
	)

	// auditBatchesFailed tracks the total number of batches failed after max retries.
	//
	// This counter increments when a batch fails to write after MaxRetries attempts.
	// Failed batches are dropped and logged for manual investigation.
	//
	// Alert threshold: >5% failure rate (audit_batches_failed_total / audit_events_written_total)
	auditBatchesFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "audit_batches_failed_total",
			Help: "Total number of audit batches failed after max retries",
		},
		[]string{"service"},
	)

	// auditBufferSize tracks the current number of events in the audit buffer.
	//
	// This gauge shows the current buffer utilization.
	// High buffer utilization indicates write lag or system overload.
	//
	// Alert threshold: >80% utilization (audit_buffer_size / buffer_capacity)
	auditBufferSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "audit_buffer_size",
			Help: "Current number of events in audit buffer",
		},
		[]string{"service"},
	)

	// auditBufferCapacity tracks the maximum buffer capacity (from config).
	//
	// DD-AUDIT-004: Buffer Sizing Strategy for Burst Traffic
	// This gauge is set once at initialization and represents the configured buffer size.
	// Use with audit_buffer_size to calculate buffer utilization percentage.
	//
	// Query: audit_buffer_size / audit_buffer_capacity
	auditBufferCapacity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "audit_buffer_capacity",
			Help: "Maximum buffer capacity (configured buffer size)",
		},
		[]string{"service"},
	)

)

// MetricsLabels contains the labels for audit metrics.
//
// This struct is used to pass service-specific labels to the metrics functions.
type MetricsLabels struct {
	// Service is the name of the service generating audit events
	// (e.g., "gateway", "context-api", "ai-analysis")
	Service string
}

// RecordDropped increments the dropped events counter.
func (m MetricsLabels) RecordDropped() {
	auditEventsDropped.WithLabelValues(m.Service).Inc()
}

// RecordWritten increments the written events counter by the specified count.
func (m MetricsLabels) RecordWritten(count int) {
	auditEventsWritten.WithLabelValues(m.Service).Add(float64(count))
}

// RecordBatchFailed increments the failed batches counter.
func (m MetricsLabels) RecordBatchFailed() {
	auditBatchesFailed.WithLabelValues(m.Service).Inc()
}

// SetBufferSize sets the current buffer size gauge.
func (m MetricsLabels) SetBufferSize(size int) {
	auditBufferSize.WithLabelValues(m.Service).Set(float64(size))
}

// SetBufferCapacity sets the maximum buffer capacity gauge (called once at initialization).
//
// DD-AUDIT-004: Buffer Sizing Strategy for Burst Traffic
func (m MetricsLabels) SetBufferCapacity(capacity int) {
	auditBufferCapacity.WithLabelValues(m.Service).Set(float64(capacity))
}

