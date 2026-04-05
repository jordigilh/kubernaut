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

// Package metrics provides Prometheus metrics for the Remediation Orchestrator.
// Per DD-METRICS-001: Uses dependency injection pattern for metrics wiring.
// All metrics follow DD-005 naming convention: kubernaut_remediationorchestrator_<metric_name>
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// DD-005 V3.0: Metric Name Constants (Pattern B)
// ========================================
//
// Per DD-005 V3.0 mandate, all metric names MUST be defined as constants
// to prevent typos and ensure test/production parity.
//
// Pattern B: Full metric names (no Namespace/Subsystem in prometheus.Opts)
// Reference: pkg/workflowexecution/metrics/metrics.go
// Migrated from Pattern A (Namespace/Subsystem) on Dec 22, 2025
// ========================================

const (
	// Core reconciliation metrics
	MetricNameReconcileDuration     = "kubernaut_remediationorchestrator_reconcile_duration_seconds"
	MetricNamePhaseTransitionsTotal = "kubernaut_remediationorchestrator_phase_transitions_total"

	// Child CRD orchestration metrics
	MetricNameChildCRDCreationsTotal = "kubernaut_remediationorchestrator_child_crd_creations_total"

	// Routing decision metrics
	MetricNameNoActionNeededTotal    = "kubernaut_remediationorchestrator_no_action_needed_total"
	MetricNameDuplicatesSkippedTotal = "kubernaut_remediationorchestrator_duplicates_skipped_total"
	MetricNameTimeoutsTotal          = "kubernaut_remediationorchestrator_timeouts_total"

	// Blocking metrics (BR-ORCH-042)
	MetricNameBlockedTotal   = "kubernaut_remediationorchestrator_blocked_total"
	MetricNameCurrentBlocked = "kubernaut_remediationorchestrator_current_blocked"

	// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
	MetricNameApprovalDecisionsTotal = "kubernaut_remediationorchestrator_approval_decisions_total"

	// Override metrics (#594)
	MetricNameOverrideAppliedTotal            = "kubernaut_rar_override_applied_total"
	MetricNameOverrideValidationRejectedTotal = "kubernaut_rar_override_validation_rejected_total"
)

// Metrics holds all Prometheus metrics for the Remediation Orchestrator controller.
// Per DD-METRICS-001: Dependency-injected metrics pattern for testability and clarity.
type Metrics struct {
	// === CORE RECONCILIATION METRICS ===
	ReconcileDurationSeconds *prometheus.HistogramVec
	PhaseTransitionsTotal    *prometheus.CounterVec

	// === CHILD CRD ORCHESTRATION METRICS ===
	ChildCRDCreationsTotal *prometheus.CounterVec

	// === ROUTING DECISION METRICS ===
	NoActionNeededTotal    *prometheus.CounterVec
	DuplicatesSkippedTotal *prometheus.CounterVec
	TimeoutsTotal          *prometheus.CounterVec

	// === BLOCKING METRICS (BR-ORCH-042) ===
	BlockedTotal        *prometheus.CounterVec
	CurrentBlockedGauge *prometheus.GaugeVec

	// === APPROVAL DECISION METRICS (BR-AUDIT-006 - SOC 2 Compliance) ===
	// Business Value: Track approval/rejection rates for compliance reporting and operational insights
	ApprovalDecisionsTotal *prometheus.CounterVec

	// === OVERRIDE METRICS (#594) ===
	OverrideAppliedTotal            *prometheus.CounterVec
	OverrideValidationRejectedTotal *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance and registers with controller-runtime.
// Uses controller-runtime's global registry for automatic /metrics endpoint exposure.
// Per DD-METRICS-001: Dependency injection pattern for V1.0 maturity.
func NewMetrics() *Metrics {
	m := &Metrics{
		// Core reconciliation metrics
		ReconcileDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameReconcileDuration, // DD-005 V3.0: Pattern B (full name),
				Help:      "Duration of reconciliation in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
			},
			[]string{"namespace", "phase"},
		),
		PhaseTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNamePhaseTransitionsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of phase transitions",
			},
			[]string{"from_phase", "to_phase", "namespace"},
		),

		// Child CRD orchestration
		ChildCRDCreationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameChildCRDCreationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of child CRD creations",
			},
			[]string{"child_type", "namespace"},
		),

		// Routing decision metrics
		NoActionNeededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameNoActionNeededTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of remediations where no action was needed (problem self-resolved)",
			},
			[]string{"reason", "namespace"},
		),
		DuplicatesSkippedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDuplicatesSkippedTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of duplicate remediations skipped",
			},
			[]string{"skip_reason", "namespace"},
		),
		TimeoutsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameTimeoutsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of remediation timeouts",
			},
			[]string{"phase", "namespace"},
		),

		// Blocking metrics (BR-ORCH-042)
		BlockedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameBlockedTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total RemediationRequests blocked due to consecutive failures",
			},
			[]string{"namespace", "reason"},
		),
		CurrentBlockedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameCurrentBlocked, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current number of blocked RRs",
			},
			[]string{"namespace"},
		),

		// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of approval decisions (approved/rejected/expired). Business Value: Track approval rates for compliance reporting and operational insights (SOC 2 CC8.1).",
			},
			[]string{"decision", "namespace"},
		),

		// Override metrics (#594)
		OverrideAppliedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameOverrideAppliedTotal,
				Help: "Total operator overrides applied via RAR approval (#594).",
			},
			[]string{"type", "namespace"},
		),
		OverrideValidationRejectedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameOverrideValidationRejectedTotal,
				Help: "Total operator overrides rejected by webhook validation (#594).",
			},
			[]string{"reason", "namespace"},
		),
	}

	// Register all metrics with controller-runtime's global registry
	// This makes metrics available at :8080/metrics endpoint
	ctrlmetrics.Registry.MustRegister(
		m.ReconcileDurationSeconds,
		m.PhaseTransitionsTotal,
		m.ChildCRDCreationsTotal,
		m.NoActionNeededTotal,
		m.DuplicatesSkippedTotal,
		m.TimeoutsTotal,
		m.BlockedTotal,
		m.CurrentBlockedGauge,
		m.ApprovalDecisionsTotal,
		m.OverrideAppliedTotal,
		m.OverrideValidationRejectedTotal,
	)

	// Initialize all metrics with 0 values so they appear in /metrics endpoint
	// Per DD-METRICS-001: Metrics visibility requirement for E2E tests
	// This ensures metrics are discoverable even before first increment
	m.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", "default").Add(0)
	m.NoActionNeededTotal.WithLabelValues("default", "Completed").Add(0)
	m.DuplicatesSkippedTotal.WithLabelValues("default", "test_signal").Add(0)
	m.TimeoutsTotal.WithLabelValues("default", "Pending").Add(0)
	m.BlockedTotal.WithLabelValues("default", "ConsecutiveFailures").Add(0)
	m.CurrentBlockedGauge.WithLabelValues("default").Set(0)

	// BR-AUDIT-006: Initialize approval decision metrics
	m.ApprovalDecisionsTotal.WithLabelValues("Approved", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Expired", "default").Add(0)

	return m
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing).
// Tests should use this to avoid polluting the global registry.
// Per DD-METRICS-001: Test isolation pattern.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		// Core reconciliation metrics
		ReconcileDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameReconcileDuration, // DD-005 V3.0: Pattern B (full name),
				Help:      "Duration of reconciliation in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10),
			},
			[]string{"namespace", "phase"},
		),
		PhaseTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNamePhaseTransitionsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of phase transitions",
			},
			[]string{"from_phase", "to_phase", "namespace"},
		),
		ChildCRDCreationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameChildCRDCreationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of child CRD creations",
			},
			[]string{"child_type", "namespace"},
		),
		NoActionNeededTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameNoActionNeededTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of remediations where no action was needed",
			},
			[]string{"reason", "namespace"},
		),
		DuplicatesSkippedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameDuplicatesSkippedTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of duplicate remediations skipped",
			},
			[]string{"skip_reason", "namespace"},
		),
		TimeoutsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameTimeoutsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of remediation timeouts",
			},
			[]string{"phase", "namespace"},
		),
		BlockedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameBlockedTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total RemediationRequests blocked due to consecutive failures",
			},
			[]string{"namespace", "reason"},
		),
		CurrentBlockedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameCurrentBlocked, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current number of blocked RRs",
			},
			[]string{"namespace"},
		),

		// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of approval decisions (approved/rejected/expired). Business Value: Track approval rates for compliance reporting and operational insights (SOC 2 CC8.1).",
			},
			[]string{"decision", "namespace"},
		),

		// Override metrics (#594)
		OverrideAppliedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameOverrideAppliedTotal,
				Help: "Total operator overrides applied via RAR approval (#594).",
			},
			[]string{"type", "namespace"},
		),
		OverrideValidationRejectedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameOverrideValidationRejectedTotal,
				Help: "Total operator overrides rejected by webhook validation (#594).",
			},
			[]string{"reason", "namespace"},
		),
	}

	// Register with provided registry (test registry)
	registry.MustRegister(
		m.ReconcileDurationSeconds,
		m.PhaseTransitionsTotal,
		m.ChildCRDCreationsTotal,
		m.NoActionNeededTotal,
		m.DuplicatesSkippedTotal,
		m.TimeoutsTotal,
		m.BlockedTotal,
		m.CurrentBlockedGauge,
		m.ApprovalDecisionsTotal,
		m.OverrideAppliedTotal,
		m.OverrideValidationRejectedTotal,
	)

	// Initialize all metrics with 0 values so they appear in /metrics endpoint
	// Per E2E test requirements: metrics should be visible even if not yet incremented
	// This prevents "metric not found" errors in E2E tests
	m.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", "default").Add(0)
	m.NoActionNeededTotal.WithLabelValues("default", "Completed").Add(0)
	m.DuplicatesSkippedTotal.WithLabelValues("default", "test_signal").Add(0)
	m.TimeoutsTotal.WithLabelValues("default", "Pending").Add(0)
	m.BlockedTotal.WithLabelValues("default", "ConsecutiveFailures").Add(0)
	m.CurrentBlockedGauge.WithLabelValues("default").Set(0)

	// BR-AUDIT-006: Initialize approval decision metrics
	m.ApprovalDecisionsTotal.WithLabelValues("Approved", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Expired", "default").Add(0)

	return m
}

// ========================================
// APPROVAL DECISION METRICS HELPERS (BR-AUDIT-006 - SOC 2 Compliance)
// ========================================

// RecordApprovalDecision records a RemediationApprovalRequest decision.
//
// Business Value:
//   - Tracks approval/rejection rates for compliance reporting (SOC 2 CC8.1)
//   - Operational insights: Identify rejection patterns, approval trends
//   - Alerting: Monitor for unexpected approval spikes or policy violations
//
// Parameters:
//   - decision: "Approved", "Rejected", or "Expired"
//   - namespace: K8s namespace
//
// Example:
//
//	m.RecordApprovalDecision("Approved", "production")
func (m *Metrics) RecordApprovalDecision(decision, namespace string) {
	m.ApprovalDecisionsTotal.WithLabelValues(decision, namespace).Inc()
}

