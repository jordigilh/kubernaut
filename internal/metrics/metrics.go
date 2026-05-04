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
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

// Registry holds Prometheus metrics for the API Frontend.
// All collectors are created here and injected into components that need them,
// avoiding package-level Prometheus vars that silently use the default registry.
type Registry struct {
	registry *prometheus.Registry

	RequestsTotal      *prometheus.CounterVec
	RequestDuration    *prometheus.HistogramVec
	ToolCallsTotal     *prometheus.CounterVec
	ActiveSessions     prometheus.Gauge
	LLMTokensTotal     *prometheus.CounterVec
	RateLimitDenied    *prometheus.CounterVec
	CircuitBreakerState *prometheus.GaugeVec
}

// NewRegistry creates and registers all AF Prometheus metrics.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	r := &Registry{
		registry: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "requests_total",
			Help:      "Total number of requests by protocol and status.",
		}, []string{"protocol", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "af",
			Name:      "request_duration_seconds",
			Help:      "Request latency distribution by protocol.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"protocol"}),
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "tool_calls_total",
			Help:      "Total tool invocations by tool name and outcome.",
		}, []string{"tool", "outcome"}),
		ActiveSessions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "af",
			Name:      "active_sessions",
			Help:      "Number of currently active InvestigationSessions.",
		}),
		LLMTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "af",
			Name:      "llm_tokens_total",
			Help:      "Total LLM tokens consumed by direction (input/output).",
		}, []string{"direction", "model"}),
		RateLimitDenied:     ratelimit.NewRateLimitDeniedTotal(),
		CircuitBreakerState: auth.NewCircuitBreakerStateGauge(),
	}

	reg.MustRegister(r.RequestsTotal)
	reg.MustRegister(r.RequestDuration)
	reg.MustRegister(r.ToolCallsTotal)
	reg.MustRegister(r.ActiveSessions)
	reg.MustRegister(r.LLMTokensTotal)
	reg.MustRegister(r.RateLimitDenied)
	reg.MustRegister(r.CircuitBreakerState)

	return r
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
