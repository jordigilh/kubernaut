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

// Package workflowexecution provides metrics for the WorkflowExecution controller
// TDD GREEN: Implementation driven by failing tests in metrics_test.go
package workflowexecution

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// MetricsNamespace is the Prometheus namespace for all metrics
	MetricsNamespace = "kubernaut"
	// MetricsSubsystem is the Prometheus subsystem
	MetricsSubsystem = "workflowexecution"
)

// Metrics holds all Prometheus metrics for WorkflowExecution controller
// TDD: Struct fields defined by test requirements
type Metrics struct {
	// PhaseTransitions counts phase transitions
	// Labels: namespace, workflow_id, from_phase, to_phase
	PhaseTransitions *prometheus.CounterVec

	// ExecutionDuration tracks execution duration histogram
	// Labels: namespace, workflow_id, outcome
	ExecutionDuration *prometheus.HistogramVec

	// PipelineRunCreations counts PipelineRun creation attempts
	// Labels: namespace, workflow_id, result (success/failure)
	PipelineRunCreations *prometheus.CounterVec

	// SkippedTotal counts skipped executions
	// Labels: namespace, workflow_id, reason
	SkippedTotal *prometheus.CounterVec

	// FailedTotal counts failed executions
	// Labels: namespace, workflow_id, reason
	FailedTotal *prometheus.CounterVec

	// CompletedTotal counts completed executions
	// Labels: namespace, workflow_id
	CompletedTotal *prometheus.CounterVec

	// ActiveExecutions tracks currently active executions
	ActiveExecutions prometheus.Gauge

	// ReconcileTotal counts reconcile operations
	// Labels: namespace, result (success/error)
	ReconcileTotal *prometheus.CounterVec

	// ReconcileDuration tracks reconcile duration histogram
	ReconcileDuration prometheus.Histogram

	// registry holds all metrics for cleanup
	registry *prometheus.Registry
}

// NewMetrics creates and registers all Prometheus metrics
// TDD GREEN: Constructor defined by test requirements
func NewMetrics() *Metrics {
	m := &Metrics{
		PhaseTransitions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "phase_transitions_total",
				Help:      "Total number of phase transitions",
			},
			[]string{"namespace", "workflow_id", "from_phase", "to_phase"},
		),

		ExecutionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "execution_duration_seconds",
				Help:      "Histogram of workflow execution duration in seconds",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
			},
			[]string{"namespace", "workflow_id", "outcome"},
		),

		PipelineRunCreations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "pipelinerun_creations_total",
				Help:      "Total number of PipelineRun creation attempts",
			},
			[]string{"namespace", "workflow_id", "result"},
		),

		SkippedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "skipped_total",
				Help:      "Total number of skipped executions",
			},
			[]string{"namespace", "workflow_id", "reason"},
		),

		FailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "failed_total",
				Help:      "Total number of failed executions",
			},
			[]string{"namespace", "workflow_id", "reason"},
		),

		CompletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "completed_total",
				Help:      "Total number of completed executions",
			},
			[]string{"namespace", "workflow_id"},
		),

		ActiveExecutions: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "active_executions",
				Help:      "Number of currently active workflow executions",
			},
		),

		ReconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "reconcile_total",
				Help:      "Total number of reconcile operations",
			},
			[]string{"namespace", "result"},
		),

		ReconcileDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "reconcile_duration_seconds",
				Help:      "Histogram of reconcile duration in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
		),
	}

	// Create a new registry for testing isolation
	m.registry = prometheus.NewRegistry()
	m.registry.MustRegister(
		m.PhaseTransitions,
		m.ExecutionDuration,
		m.PipelineRunCreations,
		m.SkippedTotal,
		m.FailedTotal,
		m.CompletedTotal,
		m.ActiveExecutions,
		m.ReconcileTotal,
		m.ReconcileDuration,
	)

	return m
}

// Unregister removes all metrics from registry (for test cleanup)
func (m *Metrics) Unregister() {
	if m.registry == nil {
		return
	}
	m.registry.Unregister(m.PhaseTransitions)
	m.registry.Unregister(m.ExecutionDuration)
	m.registry.Unregister(m.PipelineRunCreations)
	m.registry.Unregister(m.SkippedTotal)
	m.registry.Unregister(m.FailedTotal)
	m.registry.Unregister(m.CompletedTotal)
	m.registry.Unregister(m.ActiveExecutions)
	m.registry.Unregister(m.ReconcileTotal)
	m.registry.Unregister(m.ReconcileDuration)
}

// =============================================================================
// TDD GREEN: Recording methods defined by test requirements
// =============================================================================

// RecordPhaseTransition records a phase transition
func (m *Metrics) RecordPhaseTransition(namespace, workflowID, fromPhase, toPhase string) {
	m.PhaseTransitions.WithLabelValues(namespace, workflowID, fromPhase, toPhase).Inc()
}

// RecordExecutionDuration records the execution duration
func (m *Metrics) RecordExecutionDuration(namespace, workflowID, outcome string, duration time.Duration) {
	m.ExecutionDuration.WithLabelValues(namespace, workflowID, outcome).Observe(duration.Seconds())
}

// RecordPipelineRunCreation records a PipelineRun creation attempt
func (m *Metrics) RecordPipelineRunCreation(namespace, workflowID string, success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	m.PipelineRunCreations.WithLabelValues(namespace, workflowID, result).Inc()
}

// RecordSkipped records a skipped execution
func (m *Metrics) RecordSkipped(namespace, workflowID, reason string) {
	m.SkippedTotal.WithLabelValues(namespace, workflowID, reason).Inc()
}

// RecordFailed records a failed execution
func (m *Metrics) RecordFailed(namespace, workflowID, reason string) {
	m.FailedTotal.WithLabelValues(namespace, workflowID, reason).Inc()
}

// RecordCompleted records a completed execution
func (m *Metrics) RecordCompleted(namespace, workflowID string) {
	m.CompletedTotal.WithLabelValues(namespace, workflowID).Inc()
}

// SetActiveExecutions sets the active execution count
func (m *Metrics) SetActiveExecutions(count int) {
	m.ActiveExecutions.Set(float64(count))
}

// RecordReconcile records a reconcile operation
func (m *Metrics) RecordReconcile(namespace string, success bool) {
	result := "error"
	if success {
		result = "success"
	}
	m.ReconcileTotal.WithLabelValues(namespace, result).Inc()
}

// RecordReconcileDuration records reconcile duration
func (m *Metrics) RecordReconcileDuration(duration time.Duration) {
	m.ReconcileDuration.Observe(duration.Seconds())
}

// Register registers all metrics with the default prometheus registry
// Used in production (not in tests)
func (m *Metrics) Register() error {
	collectors := []prometheus.Collector{
		m.PhaseTransitions,
		m.ExecutionDuration,
		m.PipelineRunCreations,
		m.SkippedTotal,
		m.FailedTotal,
		m.CompletedTotal,
		m.ActiveExecutions,
		m.ReconcileTotal,
		m.ReconcileDuration,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			// Ignore already registered errors
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}
	return nil
}

