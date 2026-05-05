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

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// Registry holds Prometheus metrics for the API Frontend.
// All collectors are created here and injected into components that need them,
// avoiding package-level Prometheus vars that silently use the default registry.
//
// Metric names follow the catalog in ARCHITECTURE.md §7 (af_* prefix).
type Registry struct {
	registry *prometheus.Registry

	HTTPRequestsTotal      *prometheus.CounterVec
	HTTPRequestDuration    *prometheus.HistogramVec
	ToolCallsTotal         *prometheus.CounterVec
	ToolCallDuration       *prometheus.HistogramVec
	SessionsActive         *prometheus.GaugeVec
	LLMTokensTotal         *prometheus.CounterVec
	RateLimitDenied        *prometheus.CounterVec
	CircuitBreakerState    *prometheus.GaugeVec
	AuthDuration           *prometheus.HistogramVec
	AuditEventsTotal       *prometheus.CounterVec
	SessionTTLActionsTotal *prometheus.CounterVec
}

// NewRegistry creates and registers all AF Prometheus metrics.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	r := &Registry{
		registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, path, and status.",
		}, []string{"method", "path", "status"}),
		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency distribution by method, path, and status.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "tool_calls_total",
			Help:      "Total tool invocations by tool name and result.",
		}, []string{"tool", "result"}),
		ToolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "tool_call_duration_seconds",
			Help:      "Tool execution latency distribution by tool name and type.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tool", "type"}),
		SessionsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "af",
			Name:      "sessions_active",
			Help:      "Number of currently active InvestigationSessions by phase.",
		}, []string{"phase"}),
		LLMTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "llm_tokens_total",
			Help:      "Total LLM tokens consumed by direction (input/output).",
		}, []string{"direction", "model"}),
		RateLimitDenied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "rate_limit_rejections_total",
			Help:      "Total rate limit rejections by tier and reason.",
		}, []string{"tier", "reason"}),
		CircuitBreakerState: auth.NewCircuitBreakerStateGauge(),
		AuthDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "auth_duration_seconds",
			Help:      "Authentication latency distribution by result.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"result"}),
		AuditEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "audit_events_total",
			Help:      "Total audit trail events by type.",
		}, []string{"type"}),
		SessionTTLActionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "session_ttl_actions_total",
			Help:      "Total TTL-triggered session lifecycle actions by action type.",
		}, []string{"action"}),
	}

	reg.MustRegister(r.HTTPRequestsTotal)
	reg.MustRegister(r.HTTPRequestDuration)
	reg.MustRegister(r.ToolCallsTotal)
	reg.MustRegister(r.ToolCallDuration)
	reg.MustRegister(r.SessionsActive)
	reg.MustRegister(r.LLMTokensTotal)
	reg.MustRegister(r.RateLimitDenied)
	reg.MustRegister(r.CircuitBreakerState)
	reg.MustRegister(r.AuthDuration)
	reg.MustRegister(r.AuditEventsTotal)
	reg.MustRegister(r.SessionTTLActionsTotal)

	return r
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
