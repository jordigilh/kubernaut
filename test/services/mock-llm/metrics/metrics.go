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
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	MetricNameRequestsTotal          = "mock_llm_requests_total"
	MetricNameResponseDuration       = "mock_llm_response_duration_seconds"
	MetricNameScenarioDetectionTotal = "mock_llm_scenario_detection_total"
	MetricNameDAGPhaseTransitions    = "mock_llm_dag_phase_transitions_total"
)

type Metrics struct {
	registry         *prometheus.Registry
	RequestsTotal    *prometheus.CounterVec
	ResponseDuration *prometheus.HistogramVec
	ScenarioDetected *prometheus.CounterVec
	DAGTransitions   *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	return NewMetricsWithRegistry(reg)
}

func NewMetricsWithRegistry(reg *prometheus.Registry) *Metrics {
	m := &Metrics{
		registry: reg,
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameRequestsTotal,
				Help: "Total number of LLM requests processed",
			},
			[]string{"endpoint", "status_code", "scenario"},
		),
		ResponseDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameResponseDuration,
				Help:    "Response duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint", "scenario"},
		),
		ScenarioDetected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameScenarioDetectionTotal,
				Help: "Total number of scenario detections by scenario name and method",
			},
			[]string{"scenario", "method"},
		),
		DAGTransitions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDAGPhaseTransitions,
				Help: "Total DAG phase transitions by from/to node",
			},
			[]string{"from_node", "to_node"},
		),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.ResponseDuration,
		m.ScenarioDetected,
		m.DAGTransitions,
	)

	return m
}

// Registry returns the underlying Prometheus registry for HTTP handler exposure.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// RecordRequest increments the request counter and observes the response duration.
func (m *Metrics) RecordRequest(endpoint string, statusCode int, scenario string, durationSec float64) {
	code := statusCodeStr(statusCode)
	m.RequestsTotal.WithLabelValues(endpoint, code, scenario).Inc()
	m.ResponseDuration.WithLabelValues(endpoint, scenario).Observe(durationSec)
}

// RecordScenarioDetection increments the scenario detection counter.
func (m *Metrics) RecordScenarioDetection(scenario, method string) {
	m.ScenarioDetected.WithLabelValues(scenario, method).Inc()
}

// RecordDAGTransition increments the DAG phase transition counter.
func (m *Metrics) RecordDAGTransition(fromNode, toNode string) {
	m.DAGTransitions.WithLabelValues(fromNode, toNode).Inc()
}

// Reset unregisters and re-registers all collectors, effectively zeroing all metrics.
func (m *Metrics) Reset() {
	m.registry.Unregister(m.RequestsTotal)
	m.registry.Unregister(m.ResponseDuration)
	m.registry.Unregister(m.ScenarioDetected)
	m.registry.Unregister(m.DAGTransitions)

	m.RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameRequestsTotal,
			Help: "Total number of LLM requests processed",
		},
		[]string{"endpoint", "status_code", "scenario"},
	)
	m.ResponseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameResponseDuration,
			Help:    "Response duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "scenario"},
	)
	m.ScenarioDetected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameScenarioDetectionTotal,
			Help: "Total number of scenario detections by scenario name and method",
		},
		[]string{"scenario", "method"},
	)
	m.DAGTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDAGPhaseTransitions,
			Help: "Total DAG phase transitions by from/to node",
		},
		[]string{"from_node", "to_node"},
	)

	m.registry.MustRegister(
		m.RequestsTotal,
		m.ResponseDuration,
		m.ScenarioDetected,
		m.DAGTransitions,
	)
}

func statusCodeStr(code int) string {
	return strconv.Itoa(code)
}
