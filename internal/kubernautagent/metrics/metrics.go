/*
Copyright 2026 Jordi Gil.

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

// Package metrics provides Prometheus metrics for the Kubernaut Agent.
// DD-005 Compliant: All metrics follow {service}_{metric_name}_{unit} naming.
// DD-METRICS-001 Compliant: Dependency-injected via NewMetrics / NewMetricsWithRegistry.
//
// Authority: BR-KA-OBSERVABILITY-001
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// DD-005 V3.0: Metric Name Constants (MANDATORY)
const (
	MetricNameSessionsStartedTotal       = "aiagent_sessions_started_total"
	MetricNameSessionsCompletedTotal     = "aiagent_sessions_completed_total"
	MetricNameSessionsActive             = "aiagent_sessions_active"
	MetricNameSessionDurationSeconds     = "aiagent_session_duration_seconds"
	MetricNameHTTPRateLimitedTotal       = "aiagent_http_rate_limited_total"
	MetricNameHTTPRequestDurationSeconds = "aiagent_http_request_duration_seconds"
	MetricNameHTTPRequestsInFlight       = "aiagent_http_requests_in_flight"
	MetricNameAuthzDeniedTotal           = "aiagent_authz_denied_total"
	MetricNameAuditEventsEmittedTotal    = "aiagent_audit_events_emitted_total"

	// Interactive MCP mode metrics (PR6a, BR-INTERACTIVE-001..008)
	MetricNameInteractiveSessionsActive       = "aiagent_mcp_interactive_sessions_active"
	MetricNameInteractiveCommandDuration      = "aiagent_mcp_interactive_command_duration_seconds"
	MetricNameInteractiveTakeoverTotal        = "aiagent_mcp_interactive_takeover_total"
	MetricNameInteractiveLeaseContentionTotal = "aiagent_mcp_interactive_lease_contention_total"
)

// maxSignalNameLen bounds the signal_name label to prevent Prometheus TSDB
// memory pressure from attacker-influenced input (SEC-1).
const maxSignalNameLen = 128

// Metrics holds all Prometheus metrics for the Kubernaut Agent.
// Per DD-METRICS-001: Dependency injection pattern for testability.
type Metrics struct {
	SessionsStartedTotal        *prometheus.CounterVec
	SessionsCompletedTotal      *prometheus.CounterVec
	SessionsActive              prometheus.Gauge
	SessionDurationSeconds *prometheus.HistogramVec
	HTTPRateLimitedTotal   prometheus.Counter
	HTTPRequestDurationSeconds  *prometheus.HistogramVec
	HTTPRequestsInFlight        prometheus.Gauge
	AuthzDeniedTotal            *prometheus.CounterVec
	AuditEventsEmittedTotal     *prometheus.CounterVec

	// Interactive mode metrics (PR6a)
	InteractiveSessionsActive       prometheus.Gauge
	InteractiveCommandDuration      *prometheus.HistogramVec
	InteractiveTakeoverTotal        *prometheus.CounterVec
	InteractiveLeaseContentionTotal prometheus.Counter
}

var (
	registrationOnce  sync.Once
	registeredMetrics *Metrics
)

// NewMetrics creates and registers metrics with the default global registry.
// Uses sync.Once to prevent double-registration panics in tests (OPS-2).
func NewMetrics() *Metrics {
	registrationOnce.Do(func() {
		registeredMetrics = newMetrics(prometheus.DefaultRegisterer)
	})
	return registeredMetrics
}

// NewMetricsWithRegistry creates metrics with a custom registry for test isolation.
// Per DD-METRICS-001: Tests use this to avoid polluting the global registry.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	return newMetrics(registry)
}

func newMetrics(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		SessionsStartedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSessionsStartedTotal,
				Help: "Total investigation sessions started by signal type and severity",
			},
			[]string{"signal_name", "severity"},
		),
		SessionsCompletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSessionsCompletedTotal,
				Help: "Total investigation sessions completed by outcome",
			},
			[]string{"outcome"},
		),
		SessionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: MetricNameSessionsActive,
				Help: "Number of currently active investigation sessions",
			},
		),
		SessionDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameSessionDurationSeconds,
				Help:    "Investigation session duration in seconds",
				Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 900},
			},
			[]string{"outcome"},
		),
		HTTPRateLimitedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameHTTPRateLimitedTotal,
				Help: "Total HTTP requests rejected by rate limiter",
			},
		),
		HTTPRequestDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameHTTPRequestDurationSeconds,
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
			},
			[]string{"endpoint", "method", "status"},
		),
		HTTPRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: MetricNameHTTPRequestsInFlight,
				Help: "Number of HTTP requests currently being processed",
			},
		),
		AuthzDeniedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuthzDeniedTotal,
				Help: "Total authorization denials by reason",
			},
			[]string{"reason"},
		),
		AuditEventsEmittedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuditEventsEmittedTotal,
				Help: "Total audit events emitted by event type",
			},
			[]string{"event_type"},
		),
		InteractiveSessionsActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: MetricNameInteractiveSessionsActive,
				Help: "Number of currently active MCP interactive sessions",
			},
		),
		InteractiveCommandDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameInteractiveCommandDuration,
				Help:    "MCP tool call duration in seconds by tool name and action",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"tool", "action"},
		),
		InteractiveTakeoverTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameInteractiveTakeoverTotal,
				Help: "Total interactive takeover operations by outcome",
			},
			[]string{"outcome"},
		),
		InteractiveLeaseContentionTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameInteractiveLeaseContentionTotal,
				Help: "Total Lease contention events (another driver holds the Lease)",
			},
		),
	}

	registry.MustRegister(
		m.SessionsStartedTotal,
		m.SessionsCompletedTotal,
		m.SessionsActive,
		m.SessionDurationSeconds,
		m.HTTPRateLimitedTotal,
		m.HTTPRequestDurationSeconds,
		m.HTTPRequestsInFlight,
		m.AuthzDeniedTotal,
		m.AuditEventsEmittedTotal,
		m.InteractiveSessionsActive,
		m.InteractiveCommandDuration,
		m.InteractiveTakeoverTotal,
		m.InteractiveLeaseContentionTotal,
	)

	return m
}

// RecordSessionStarted increments the sessions started counter.
// signal_name is truncated to maxSignalNameLen to bound cardinality (SEC-1).
func (m *Metrics) RecordSessionStarted(signalName, severity string) {
	if m == nil {
		return
	}
	if len(signalName) > maxSignalNameLen {
		signalName = signalName[:maxSignalNameLen]
	}
	m.SessionsStartedTotal.WithLabelValues(signalName, severity).Inc()
	m.SessionsActive.Inc()
}

// RecordSessionCompleted increments the completed counter, decrements active gauge,
// and observes session duration.
func (m *Metrics) RecordSessionCompleted(outcome string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.SessionsCompletedTotal.WithLabelValues(outcome).Inc()
	m.SessionsActive.Dec()
	m.SessionDurationSeconds.WithLabelValues(outcome).Observe(durationSeconds)
}

// RecordSessionSuspended increments the session suspended counter, distinguishing
// takeover-driven suspension from explicit operator cancellation (M7).
func (m *Metrics) RecordSessionSuspended() {
	if m == nil {
		return
	}
	m.SessionsCompletedTotal.WithLabelValues("suspended").Inc()
}

// RecordRateLimited increments the rate limited counter.
func (m *Metrics) RecordRateLimited() {
	if m == nil {
		return
	}
	m.HTTPRateLimitedTotal.Inc()
}

// RecordAuthzDenied increments the authorization denial counter.
func (m *Metrics) RecordAuthzDenied(reason string) {
	if m == nil {
		return
	}
	m.AuthzDeniedTotal.WithLabelValues(reason).Inc()
}

// RecordAuditEventEmitted increments the audit event emission counter.
func (m *Metrics) RecordAuditEventEmitted(eventType string) {
	if m == nil {
		return
	}
	m.AuditEventsEmittedTotal.WithLabelValues(eventType).Inc()
}

// RecordInteractiveSessionStarted increments the active interactive sessions gauge.
func (m *Metrics) RecordInteractiveSessionStarted() {
	if m == nil {
		return
	}
	m.InteractiveSessionsActive.Inc()
}

// RecordInteractiveSessionEnded decrements the active interactive sessions gauge.
func (m *Metrics) RecordInteractiveSessionEnded() {
	if m == nil {
		return
	}
	m.InteractiveSessionsActive.Dec()
}

// RecordInteractiveCommandDuration observes the duration of an MCP tool call.
func (m *Metrics) RecordInteractiveCommandDuration(tool, action string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.InteractiveCommandDuration.WithLabelValues(tool, action).Observe(durationSeconds)
}

// RecordInteractiveTakeover increments the takeover counter by outcome.
func (m *Metrics) RecordInteractiveTakeover(outcome string) {
	if m == nil {
		return
	}
	m.InteractiveTakeoverTotal.WithLabelValues(outcome).Inc()
}

// RecordInteractiveLeaseContention increments the Lease contention counter.
func (m *Metrics) RecordInteractiveLeaseContention() {
	if m == nil {
		return
	}
	m.InteractiveLeaseContentionTotal.Inc()
}
