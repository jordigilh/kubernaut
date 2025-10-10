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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Business Requirement: BR-ORCHESTRATION-006 (Observability)
// Prometheus metrics for RemediationRequest controller

var (
	// RemediationTotal tracks total number of remediations by status and environment
	RemediationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubernaut_remediation_request_total",
			Help: "Total number of RemediationRequest resources by status and environment",
		},
		[]string{"status", "environment"}, // status: completed|failed|timeout, environment: prod|staging|dev
	)

	// RemediationDuration tracks end-to-end remediation duration
	RemediationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kubernaut_remediation_request_duration_seconds",
			Help:    "End-to-end RemediationRequest duration (start to completion) in seconds",
			Buckets: prometheus.ExponentialBuckets(10, 2, 10), // 10s to 10240s (~3 hours)
		},
		[]string{"status", "environment"},
	)

	// RemediationActive tracks active remediations by phase
	RemediationActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kubernaut_remediation_request_active",
			Help: "Number of currently active RemediationRequest resources by phase",
		},
		[]string{"phase", "environment"}, // phase: pending|processing|analyzing|executing
	)

	// PhaseTransitionTotal tracks phase transitions
	PhaseTransitionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubernaut_remediation_request_phase_transitions_total",
			Help: "Total number of phase transitions",
		},
		[]string{"from_phase", "to_phase", "environment"},
	)

	// PhaseDuration tracks time spent in each phase
	PhaseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kubernaut_remediation_request_phase_duration_seconds",
			Help:    "Duration spent in each phase in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to 4096s (~1 hour)
		},
		[]string{"phase", "environment"},
	)

	// ChildCRDCreationTotal tracks child CRD creations
	ChildCRDCreationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubernaut_remediation_request_child_crd_total",
			Help: "Total number of child CRD creations",
		},
		[]string{"crd_type", "environment"}, // crd_type: RemediationProcessing|AIAnalysis|WorkflowExecution
	)

	// TimeoutTotal tracks timeouts by phase
	TimeoutTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubernaut_remediation_request_timeout_total",
			Help: "Total number of timeouts by phase",
		},
		[]string{"phase", "environment"},
	)

	// FailureTotal tracks failures by phase and reason
	FailureTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kubernaut_remediation_request_failure_total",
			Help: "Total number of failures by phase",
		},
		[]string{"phase", "child_crd_type", "environment"},
	)
)

func init() {
	// Register metrics with controller-runtime's global registry
	metrics.Registry.MustRegister(
		RemediationTotal,
		RemediationDuration,
		RemediationActive,
		PhaseTransitionTotal,
		PhaseDuration,
		ChildCRDCreationTotal,
		TimeoutTotal,
		FailureTotal,
	)
}

// MetricsRecorder provides helper methods for recording metrics
type MetricsRecorder struct {
	environment string
}

// NewMetricsRecorder creates a new metrics recorder
func NewMetricsRecorder(environment string) *MetricsRecorder {
	// Default to "unknown" if environment is empty
	if environment == "" {
		environment = "unknown"
	}
	return &MetricsRecorder{environment: environment}
}

// RecordPhaseTransition records a phase transition
func (m *MetricsRecorder) RecordPhaseTransition(fromPhase, toPhase string) {
	PhaseTransitionTotal.WithLabelValues(fromPhase, toPhase, m.environment).Inc()
}

// RecordChildCRDCreation records a child CRD creation
func (m *MetricsRecorder) RecordChildCRDCreation(crdType string) {
	ChildCRDCreationTotal.WithLabelValues(crdType, m.environment).Inc()
}

// RecordTimeout records a timeout
func (m *MetricsRecorder) RecordTimeout(phase string) {
	TimeoutTotal.WithLabelValues(phase, m.environment).Inc()
}

// RecordFailure records a failure
func (m *MetricsRecorder) RecordFailure(phase, childCRDType string) {
	FailureTotal.WithLabelValues(phase, childCRDType, m.environment).Inc()
}

// RecordCompletion records a successful completion
func (m *MetricsRecorder) RecordCompletion(durationSeconds float64) {
	RemediationTotal.WithLabelValues("completed", m.environment).Inc()
	RemediationDuration.WithLabelValues("completed", m.environment).Observe(durationSeconds)
}

// RecordTerminalFailure records a terminal failure (failed or timeout)
func (m *MetricsRecorder) RecordTerminalFailure(status string, durationSeconds float64) {
	RemediationTotal.WithLabelValues(status, m.environment).Inc()
	RemediationDuration.WithLabelValues(status, m.environment).Observe(durationSeconds)
}

// UpdateActiveGauge updates the active remediation gauge
func (m *MetricsRecorder) UpdateActiveGauge(phase string, delta float64) {
	RemediationActive.WithLabelValues(phase, m.environment).Add(delta)
}

// RecordPhaseDuration records time spent in a phase
func (m *MetricsRecorder) RecordPhaseDuration(phase string, durationSeconds float64) {
	PhaseDuration.WithLabelValues(phase, m.environment).Observe(durationSeconds)
}
