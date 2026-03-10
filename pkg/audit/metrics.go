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
var (
	// auditEventsDropped tracks the total number of audit events dropped due to buffer full.
	//
	// This counter increments when the buffer is full and a new event cannot be buffered.
	// This indicates system overload and should trigger alerts.
	//
	// Alert threshold: >1% drop rate (audit_events_dropped_total / total events)
	auditEventsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "audit_events_dropped_total",
			Help: "Total number of audit events dropped (buffer full)",
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

