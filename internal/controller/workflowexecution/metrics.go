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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// Day 7: Business-Value Metrics (BR-WE-008)
// 4 metrics per BR-WE-008:
// - workflowexecution_total{outcome}
// - workflowexecution_duration_seconds{outcome}
// - workflowexecution_pipelinerun_creation_total
// - workflowexecution_skip_total{reason}
//
// Day 6 Extension (BR-WE-012, DD-WE-004):
// - workflowexecution_backoff_skip_total{reason}
// - workflowexecution_consecutive_failures{target_resource}
// ========================================

var (
	// WorkflowExecutionTotal tracks total workflow executions by outcome
	// Labels: outcome (Completed, Failed)
	// Business value: SLO success rate tracking
	WorkflowExecutionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_total",
			Help: "Total number of workflow executions by outcome",
		},
		[]string{"outcome"},
	)

	// WorkflowExecutionDuration tracks workflow execution duration
	// Labels: outcome (Completed, Failed)
	// Business value: SLO P95 latency tracking
	WorkflowExecutionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "workflowexecution_duration_seconds",
			Help: "Workflow execution duration in seconds",
			// Buckets: 5s, 10s, 20s, 40s, 80s, 160s, 320s
			Buckets: prometheus.ExponentialBuckets(5, 2, 7),
		},
		[]string{"outcome"},
	)

	// PipelineRunCreationTotal tracks PipelineRun creation attempts
	// Business value: Tracks execution initiation success
	PipelineRunCreationTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "workflowexecution_pipelinerun_creation_total",
			Help: "Total number of PipelineRun creations",
		},
	)

	// WorkflowExecutionSkipTotal tracks skipped executions by reason
	// Labels: reason (ResourceBusy, RecentlyRemediated, ExhaustedRetries, PreviousExecutionFailed)
	// Business value: DD-WE-001 resource locking visibility
	WorkflowExecutionSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_skip_total",
			Help: "Total number of skipped workflow executions by reason",
		},
		[]string{"reason"},
	)

	// ========================================
	// Day 6 Extension: Backoff Metrics (BR-WE-012, DD-WE-004)
	// ========================================

	// BackoffSkipTotal tracks workflows skipped due to exponential backoff
	// Labels: reason (ExhaustedRetries, PreviousExecutionFailed)
	// Business value: Visibility into remediation storm prevention
	BackoffSkipTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workflowexecution_backoff_skip_total",
			Help: "Total workflows skipped due to backoff (ExhaustedRetries, PreviousExecutionFailed)",
		},
		[]string{"reason"},
	)

	// ConsecutiveFailuresGauge tracks current consecutive failure count per target
	// Labels: target_resource (e.g., "default/deployment/payment-api")
	// Business value: Real-time visibility into retry state
	ConsecutiveFailuresGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workflowexecution_consecutive_failures",
			Help: "Current consecutive failure count per target resource",
		},
		[]string{"target_resource"},
	)
)

func init() {
	// Register metrics with controller-runtime metrics registry
	metrics.Registry.MustRegister(
		WorkflowExecutionTotal,
		WorkflowExecutionDuration,
		PipelineRunCreationTotal,
		WorkflowExecutionSkipTotal,
		// Day 6 Extension: Backoff metrics (BR-WE-012)
		BackoffSkipTotal,
		ConsecutiveFailuresGauge,
	)
}

// ========================================
// Metric Recording Helpers
// ========================================

// RecordWorkflowCompletion records a completed workflow execution
func RecordWorkflowCompletion(durationSeconds float64) {
	WorkflowExecutionTotal.WithLabelValues("Completed").Inc()
	WorkflowExecutionDuration.WithLabelValues("Completed").Observe(durationSeconds)
}

// RecordWorkflowFailure records a failed workflow execution
func RecordWorkflowFailure(durationSeconds float64) {
	WorkflowExecutionTotal.WithLabelValues("Failed").Inc()
	WorkflowExecutionDuration.WithLabelValues("Failed").Observe(durationSeconds)
}

// RecordPipelineRunCreation records a PipelineRun creation
func RecordPipelineRunCreation() {
	PipelineRunCreationTotal.Inc()
}

// RecordWorkflowSkip records a skipped workflow execution
// reason: "ResourceBusy", "RecentlyRemediated", "ExhaustedRetries", "PreviousExecutionFailed"
func RecordWorkflowSkip(reason string) {
	WorkflowExecutionSkipTotal.WithLabelValues(reason).Inc()
}

// ========================================
// Day 6 Extension: Backoff Metric Helpers (BR-WE-012, DD-WE-004)
// ========================================

// RecordBackoffSkip records a skip due to exponential backoff
// reason: "ExhaustedRetries" or "PreviousExecutionFailed"
func RecordBackoffSkip(reason string) {
	BackoffSkipTotal.WithLabelValues(reason).Inc()
}

// SetConsecutiveFailures updates the consecutive failure gauge for a target
func SetConsecutiveFailures(targetResource string, count int32) {
	ConsecutiveFailuresGauge.WithLabelValues(targetResource).Set(float64(count))
}

// ResetConsecutiveFailures resets the consecutive failure gauge for a target
func ResetConsecutiveFailures(targetResource string) {
	ConsecutiveFailuresGauge.WithLabelValues(targetResource).Set(0)
}
