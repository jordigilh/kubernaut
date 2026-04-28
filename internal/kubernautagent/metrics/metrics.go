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
