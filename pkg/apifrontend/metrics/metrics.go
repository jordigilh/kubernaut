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
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// Registry holds Prometheus metrics for the API Frontend.
// All collectors are created here and injected into components that need them,
// avoiding package-level Prometheus vars that silently use the default registry.
//
// GA metric set (13 metrics): only things that require real-time aggregation
// or threshold-based alerting. Everything extractable from logs or audit trail
// is deliberately excluded.
type Registry struct {
	registry *prometheus.Registry

	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPPanicsTotal      prometheus.Counter
	ToolCallsTotal       *prometheus.CounterVec
	ToolCallDuration     *prometheus.HistogramVec
	CircuitBreakerState  *prometheus.GaugeVec
	DownstreamDuration   *prometheus.HistogramVec
	AuthDuration         *prometheus.HistogramVec
	AuditBufferOverflow  prometheus.Counter
	RateLimitDenied      *prometheus.CounterVec
	SSEActiveConnections prometheus.Gauge
	LLMTokensTotal       *prometheus.CounterVec
	SessionsActive       *prometheus.GaugeVec
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
		HTTPPanicsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "http_panics_total",
			Help:      "Total HTTP handler panics recovered.",
		}),
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
		CircuitBreakerState: auth.NewCircuitBreakerStateGauge(),
		DownstreamDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "downstream_request_duration_seconds",
			Help:      "Downstream HTTP request duration by dependency and status class.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"dependency", "status"}),
		AuthDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "auth_duration_seconds",
			Help:      "Authentication latency distribution by result.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"result"}),
		AuditBufferOverflow: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "audit_buffer_overflow_total",
			Help:      "Total audit events dropped due to buffer overflow.",
		}),
		RateLimitDenied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "rate_limit_rejections_total",
			Help:      "Total rate limit rejections by tier and reason.",
		}, []string{"tier", "reason"}),
		SSEActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "af",
			Name:      "sse_active_connections",
			Help:      "Number of currently active SSE connections.",
		}),
		LLMTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "llm_tokens_total",
			Help:      "Total LLM tokens consumed by direction (input/output).",
		}, []string{"direction", "model"}),
		SessionsActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "af",
			Name:      "sessions_active",
			Help:      "Number of currently active InvestigationSessions by phase.",
		}, []string{"phase"}),
	}

	reg.MustRegister(r.HTTPRequestsTotal)
	reg.MustRegister(r.HTTPRequestDuration)
	reg.MustRegister(r.HTTPPanicsTotal)
	reg.MustRegister(r.ToolCallsTotal)
	reg.MustRegister(r.ToolCallDuration)
	reg.MustRegister(r.CircuitBreakerState)
	reg.MustRegister(r.DownstreamDuration)
	reg.MustRegister(r.AuthDuration)
	reg.MustRegister(r.AuditBufferOverflow)
	reg.MustRegister(r.RateLimitDenied)
	reg.MustRegister(r.SSEActiveConnections)
	reg.MustRegister(r.LLMTokensTotal)
	reg.MustRegister(r.SessionsActive)

	return r
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Gather collects all metric families from the underlying Prometheus registry.
func (r *Registry) Gather() ([]*dto.MetricFamily, error) {
	return r.registry.Gather()
}
