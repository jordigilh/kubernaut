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
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds Prometheus metrics for the API Frontend.
type Registry struct {
	registry *prometheus.Registry

	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	ToolCallsTotal  *prometheus.CounterVec
	ActiveSessions  prometheus.Gauge
	LLMTokensTotal  *prometheus.CounterVec
}

// NewRegistry creates and registers all AF Prometheus metrics.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	r := &Registry{
		registry: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kubernaut_apifrontend",
			Name:      "requests_total",
			Help:      "Total number of requests by protocol and status.",
		}, []string{"protocol", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "kubernaut_apifrontend",
			Name:      "request_duration_seconds",
			Help:      "Request latency distribution by protocol.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"protocol"}),
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kubernaut_apifrontend",
			Name:      "tool_calls_total",
			Help:      "Total tool invocations by tool name and outcome.",
		}, []string{"tool", "outcome"}),
		ActiveSessions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "kubernaut_apifrontend",
			Name:      "active_sessions",
			Help:      "Number of currently active InvestigationSessions.",
		}),
		LLMTokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "kubernaut_apifrontend",
			Name:      "llm_tokens_total",
			Help:      "Total LLM tokens consumed by direction (input/output).",
		}, []string{"direction", "model"}),
	}

	reg.MustRegister(r.RequestsTotal)
	reg.MustRegister(r.RequestDuration)
	reg.MustRegister(r.ToolCallsTotal)
	reg.MustRegister(r.ActiveSessions)
	reg.MustRegister(r.LLMTokensTotal)

	return r
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
