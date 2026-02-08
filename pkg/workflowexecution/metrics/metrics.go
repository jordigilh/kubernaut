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

// Package metrics provides Prometheus metrics for workflow execution observability.
//
// This file implements BR-WE-008 (Business-Value Metrics) by exposing workflow execution
// outcomes and durations for monitoring and alerting.
//
// Metrics Provided:
//   - workflowexecution_reconciler_total{outcome}: Counter of workflow executions by outcome
//     → Outcomes: Completed, Failed
//   - workflowexecution_reconciler_duration_seconds{outcome}: Histogram of execution durations
//     → Buckets: 5s, 10s, 20s, 40s, 80s, 160s, 320s
//   - workflowexecution_reconciler_execution_creations_total: Counter of execution resource creations
//
// Use Cases:
// - SLO Monitoring: Track execution success rate
// - Performance Analysis: Identify slow workflows
// - Capacity Planning: Predict resource usage
// - Alerting: Detect elevated failure rates
//
// Per DD-METRICS-001: Dependency-Injected Metrics Pattern (V1.0 Requirement)
// See: docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// Day 7: Business-Value Metrics (BR-WE-008)
// DD-METRICS-001: Dependency-Injected Metrics Pattern
// 3 metrics per BR-WE-008:
// - workflowexecution_reconciler_total{outcome}
// - workflowexecution_reconciler_duration_seconds{outcome}
// - workflowexecution_reconciler_execution_creations_total
//
// V1.0: Skip/backoff metrics removed - RO handles routing (DD-RO-002 Phase 3)
// ========================================

// Metric name constants - DRY principle for tests and production
// These constants ensure tests use correct metric names and prevent typos.
const (
	// MetricNameExecutionTotal is the name of the execution counter metric
	MetricNameExecutionTotal = "workflowexecution_reconciler_total"

	// MetricNameExecutionDuration is the name of the execution duration histogram metric
	MetricNameExecutionDuration = "workflowexecution_reconciler_duration_seconds"

	// MetricNameExecutionCreations is the name of the execution creation counter metric
	MetricNameExecutionCreations = "workflowexecution_reconciler_execution_creations_total"

	// Label values for outcome dimension
	// LabelOutcomeCompleted indicates successful workflow completion
	LabelOutcomeCompleted = "Completed"

	// LabelOutcomeFailed indicates workflow failure
	LabelOutcomeFailed = "Failed"
)

// Metrics holds all WorkflowExecution controller metrics.
// Per DD-METRICS-001: Metrics MUST be dependency-injected, not global variables.
type Metrics struct {
	// ExecutionTotal tracks total workflow executions by outcome
	// Labels: outcome (Completed, Failed)
	// Business value: SLO success rate tracking
	ExecutionTotal *prometheus.CounterVec

	// ExecutionDuration tracks workflow execution duration
	// Labels: outcome (Completed, Failed)
	// Business value: SLO P95 latency tracking
	ExecutionDuration *prometheus.HistogramVec

	// ExecutionCreations tracks execution resource creation attempts (PipelineRun or Job)
	// Business value: Tracks execution initiation success
	ExecutionCreations prometheus.Counter
}

// NewMetrics creates and registers WorkflowExecution metrics with controller-runtime registry.
// Per DD-METRICS-001: Use this in production (main.go). Automatically registers with controller-runtime.
//
// Example usage in main.go:
//
//	weMetrics := wemetrics.NewMetrics()
//
// Then inject into controller:
//
//	&workflowexecution.WorkflowExecutionReconciler{
//	    Metrics: weMetrics,
//	    // ... other fields
//	}
func NewMetrics() *Metrics {
	m := &Metrics{
		ExecutionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameExecutionTotal,
				Help: "Total number of workflow executions by outcome (DD-005: service_component_metric pattern)",
			},
			[]string{"outcome"},
		),
		ExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameExecutionDuration,
				Help: "Workflow execution duration in seconds (DD-005: service_component_metric pattern)",
				// Buckets: 5s, 10s, 20s, 40s, 80s, 160s, 320s (exponential for workflow durations)
				Buckets: prometheus.ExponentialBuckets(5, 2, 7),
			},
			[]string{"outcome"},
		),
		ExecutionCreations: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameExecutionCreations,
				Help: "Total number of execution resource creations (DD-005: service_component_metric pattern)",
			},
		),
	}

	// Auto-register with controller-runtime's global registry
	// This makes metrics available at :8080/metrics endpoint in production
	ctrlmetrics.Registry.MustRegister(
		m.ExecutionTotal,
		m.ExecutionDuration,
		m.ExecutionCreations,
	)

	return m
}

// NewMetricsWithRegistry creates WorkflowExecution metrics with custom registry.
// Per DD-METRICS-001 Step 5: Use this in tests for isolation (avoids polluting global registry).
//
// Example usage in tests:
//
//	testRegistry := prometheus.NewRegistry()
//	testMetrics := metrics.NewMetricsWithRegistry(testRegistry)
//
//	reconciler := &workflowexecution.WorkflowExecutionReconciler{
//	    Metrics: testMetrics,
//	    // ... other test setup
//	}
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		ExecutionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameExecutionTotal,
				Help: "Total number of workflow executions by outcome (DD-005: service_component_metric pattern)",
			},
			[]string{"outcome"},
		),
		ExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameExecutionDuration,
				Help: "Workflow execution duration in seconds (DD-005: service_component_metric pattern)",
				Buckets: prometheus.ExponentialBuckets(5, 2, 7),
			},
			[]string{"outcome"},
		),
		ExecutionCreations: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameExecutionCreations,
				Help: "Total number of execution resource creations (DD-005: service_component_metric pattern)",
			},
		),
	}

	// Register with provided test registry
	registry.MustRegister(
		m.ExecutionTotal,
		m.ExecutionDuration,
		m.ExecutionCreations,
	)

	return m
}

// Register registers all metrics with the provided registry.
// This must be called from main.go before starting the controller.
// Per DD-METRICS-001: Uses MustRegister to ensure metrics are registered (panic on duplicate)
func (m *Metrics) Register(reg prometheus.Registerer) {
	reg.MustRegister(m.ExecutionTotal)
	reg.MustRegister(m.ExecutionDuration)
	reg.MustRegister(m.ExecutionCreations)
}

// ========================================
// Metric Recording Methods (DD-METRICS-001)
// Per DD-METRICS-001: Use method receivers, not global functions
// ========================================

// RecordWorkflowCompletion records a completed workflow execution.
// Per DD-METRICS-001: Called as r.Metrics.RecordWorkflowCompletion()
func (m *Metrics) RecordWorkflowCompletion(durationSeconds float64) {
	m.ExecutionTotal.WithLabelValues(LabelOutcomeCompleted).Inc()
	m.ExecutionDuration.WithLabelValues(LabelOutcomeCompleted).Observe(durationSeconds)
}

// RecordWorkflowFailure records a failed workflow execution.
// Per DD-METRICS-001: Called as r.Metrics.RecordWorkflowFailure()
func (m *Metrics) RecordWorkflowFailure(durationSeconds float64) {
	m.ExecutionTotal.WithLabelValues(LabelOutcomeFailed).Inc()
	m.ExecutionDuration.WithLabelValues(LabelOutcomeFailed).Observe(durationSeconds)
}

// RecordExecutionCreation records an execution resource creation (PipelineRun or Job).
// Per DD-METRICS-001: Called as r.Metrics.RecordExecutionCreation()
func (m *Metrics) RecordExecutionCreation() {
	m.ExecutionCreations.Inc()
}

// V1.0: Skip metric functions removed - RO handles routing (DD-RO-002 Phase 3)
