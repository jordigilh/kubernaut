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

package workflowexecution

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ============================================================================
// Prometheus Metrics for WorkflowExecution Controller
// BR-WE-008: Prometheus Metrics for Execution Outcomes
// ============================================================================

var (
	// phaseTransitionsTotal counts phase transitions by namespace and phase
	phaseTransitionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_phase_transitions_total",
			Help: "Total number of WorkflowExecution phase transitions",
		},
		[]string{"namespace", "phase"},
	)

	// phaseDurationSeconds measures duration in each phase
	phaseDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "workflowexecution_phase_duration_seconds",
			Help:    "Duration of WorkflowExecution in each phase",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~68m
		},
		[]string{"namespace", "workflow_id", "outcome"},
	)

	// skipTotal counts skipped executions by namespace and reason
	skipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_skip_total",
			Help: "Total number of skipped WorkflowExecutions",
		},
		[]string{"namespace", "reason"},
	)

	// pipelineRunCreationTotal counts PipelineRun creation attempts
	pipelineRunCreationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_pipelinerun_creation_total",
			Help: "Total number of PipelineRun creation attempts",
		},
		[]string{"outcome"},
	)

	// pipelineRunCreationDurationSeconds measures time to create PipelineRun
	pipelineRunCreationDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "workflowexecution_pipelinerun_creation_duration_seconds",
			Help:    "Time to create a Tekton PipelineRun",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
	)

	// reconcileTotal counts reconciliation attempts
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_reconcile_total",
			Help: "Total number of reconciliation attempts",
		},
		[]string{"namespace", "result"},
	)

	// reconcileDurationSeconds measures reconciliation duration
	reconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "workflowexecution_reconcile_duration_seconds",
			Help:    "Duration of WorkflowExecution reconciliation",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~33s
		},
		[]string{"namespace"},
	)

	// activeExecutions tracks currently running WorkflowExecutions
	activeExecutions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workflowexecution_active_executions",
			Help: "Number of currently running WorkflowExecutions",
		},
		[]string{"namespace"},
	)

	// resourceLockCheckDurationSeconds measures lock check performance
	resourceLockCheckDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "workflowexecution_resource_lock_check_duration_seconds",
			Help:    "Duration of resource lock checks (DD-WE-001)",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
		},
	)

	// cooldownSkipsTotal counts cooldown-triggered skips
	cooldownSkipsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_cooldown_skips_total",
			Help: "Total number of executions skipped due to cooldown period",
		},
		[]string{"namespace", "workflow_id"},
	)
)

func init() {
	// Register all metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		phaseTransitionsTotal,
		phaseDurationSeconds,
		skipTotal,
		pipelineRunCreationTotal,
		pipelineRunCreationDurationSeconds,
		reconcileTotal,
		reconcileDurationSeconds,
		activeExecutions,
		resourceLockCheckDurationSeconds,
		cooldownSkipsTotal,
	)
}

// InitMetrics initializes metrics with zero values to ensure they appear in Prometheus
func InitMetrics() {
	// Initialize phase transitions
	phaseTransitionsTotal.WithLabelValues("default", "Pending").Add(0)
	phaseTransitionsTotal.WithLabelValues("default", "Running").Add(0)
	phaseTransitionsTotal.WithLabelValues("default", "Completed").Add(0)
	phaseTransitionsTotal.WithLabelValues("default", "Failed").Add(0)
	phaseTransitionsTotal.WithLabelValues("default", "Skipped").Add(0)

	// Initialize skip reasons
	skipTotal.WithLabelValues("default", "ResourceBusy").Add(0)
	skipTotal.WithLabelValues("default", "RecentlyRemediated").Add(0)

	// Initialize PipelineRun creation outcomes
	pipelineRunCreationTotal.WithLabelValues("success").Add(0)
	pipelineRunCreationTotal.WithLabelValues("failure").Add(0)

	// Initialize reconcile results
	reconcileTotal.WithLabelValues("default", "success").Add(0)
	reconcileTotal.WithLabelValues("default", "error").Add(0)
	reconcileTotal.WithLabelValues("default", "requeue").Add(0)
}

// ============================================================================
// Metric Recording Functions
// ============================================================================

// RecordPhaseTransition records a phase transition
func RecordPhaseTransition(namespace, phase string) {
	phaseTransitionsTotal.WithLabelValues(namespace, phase).Inc()
}

// RecordDuration records the duration of a completed execution
func RecordDuration(namespace, workflowID, outcome string, startTime time.Time) {
	duration := time.Since(startTime).Seconds()
	phaseDurationSeconds.WithLabelValues(namespace, workflowID, outcome).Observe(duration)
}

// RecordSkip records a skipped execution
func RecordSkip(namespace, reason string) {
	skipTotal.WithLabelValues(namespace, reason).Inc()
}

// RecordPipelineRunCreation records a PipelineRun creation attempt
func RecordPipelineRunCreation(outcome string) {
	pipelineRunCreationTotal.WithLabelValues(outcome).Inc()
}

// RecordReconcile records a reconciliation result
func RecordReconcile(namespace, result string) {
	reconcileTotal.WithLabelValues(namespace, result).Inc()
}

// RecordReconcileDuration records reconciliation duration
func RecordReconcileDuration(namespace string, duration time.Duration) {
	reconcileDurationSeconds.WithLabelValues(namespace).Observe(duration.Seconds())
}

// SetActiveExecutions sets the number of active executions in a namespace
func SetActiveExecutions(namespace string, count float64) {
	activeExecutions.WithLabelValues(namespace).Set(count)
}

// RecordResourceLockCheck records the duration of a resource lock check
func RecordResourceLockCheck(duration time.Duration) {
	resourceLockCheckDurationSeconds.Observe(duration.Seconds())
}

// RecordCooldownSkip records a cooldown-triggered skip
func RecordCooldownSkip(namespace, workflowID string) {
	cooldownSkipsTotal.WithLabelValues(namespace, workflowID).Inc()
}

