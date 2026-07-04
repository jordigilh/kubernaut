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

// newCoreAndRoutingCollectors constructs the reconciliation, child-CRD, and
// routing-decision collectors (unregistered).
func newCoreAndRoutingCollectors() (reconcileDuration *prometheus.HistogramVec, phaseTransitions, childCRDCreations, noActionNeeded, duplicatesSkipped, timeouts *prometheus.CounterVec) {
	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameReconcileDuration, // DD-005 V3.0: Pattern B (full name),
			Help:    "Duration of reconciliation in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
		[]string{"namespace", "phase"},
	)
	phaseTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNamePhaseTransitionsTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of phase transitions",
		},
		[]string{"from_phase", "to_phase", "namespace"},
	)
	childCRDCreations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameChildCRDCreationsTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of child CRD creations",
		},
		[]string{"child_type", "namespace"},
	)
	noActionNeeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameNoActionNeededTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of remediations where no action was needed (problem self-resolved)",
		},
		[]string{"reason", "namespace"},
	)
	duplicatesSkipped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameDuplicatesSkippedTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of duplicate remediations skipped",
		},
		[]string{"skip_reason", "namespace"},
	)
	timeouts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameTimeoutsTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total number of remediation timeouts",
		},
		[]string{"phase", "namespace"},
	)
	return reconcileDuration, phaseTransitions, childCRDCreations, noActionNeeded, duplicatesSkipped, timeouts
}

// newBlockingApprovalOverrideCollectors constructs the blocking, approval,
// and override collectors (unregistered).
func newBlockingApprovalOverrideCollectors() (blocked *prometheus.CounterVec, currentBlocked *prometheus.GaugeVec, approvalDecisions, overrideApplied, overrideRejected *prometheus.CounterVec) {
	blocked = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameBlockedTotal, // DD-005 V3.0: Pattern B (full name),
			Help: "Total RemediationRequests blocked due to consecutive failures",
		},
		[]string{"namespace", "reason"},
	)
	currentBlocked = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNameCurrentBlocked, // DD-005 V3.0: Pattern B (full name),
			Help: "Current number of blocked RRs",
		},
		[]string{"namespace"},
	)
	approvalDecisions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Pattern B (full name)
			Help: "Total number of approval decisions (approved/rejected/expired). Business Value: Track approval rates for compliance reporting and operational insights (SOC 2 CC8.1).",
		},
		[]string{"decision", "namespace"},
	)
	overrideApplied = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameOverrideAppliedTotal,
			Help: "Total operator overrides applied via RAR approval (#594).",
		},
		[]string{"type", "namespace"},
	)
	overrideRejected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameOverrideValidationRejectedTotal,
			Help: "Total operator overrides rejected by webhook validation (#594).",
		},
		[]string{"reason", "namespace"},
	)
	return blocked, currentBlocked, approvalDecisions, overrideApplied, overrideRejected
}

// newMetricsCollectors constructs all Metrics collectors, unregistered.
// Shared by NewMetrics and NewMetricsWithRegistry to avoid duplicating
// collector definitions across the global-registry and test-registry paths.
func newMetricsCollectors() *Metrics {
	reconcileDuration, phaseTransitions, childCRDCreations, noActionNeeded, duplicatesSkipped, timeouts := newCoreAndRoutingCollectors()
	blocked, currentBlocked, approvalDecisions, overrideApplied, overrideRejected := newBlockingApprovalOverrideCollectors()

	return &Metrics{
		ReconcileDurationSeconds:        reconcileDuration,
		PhaseTransitionsTotal:           phaseTransitions,
		ChildCRDCreationsTotal:          childCRDCreations,
		NoActionNeededTotal:             noActionNeeded,
		DuplicatesSkippedTotal:          duplicatesSkipped,
		TimeoutsTotal:                   timeouts,
		BlockedTotal:                    blocked,
		CurrentBlockedGauge:             currentBlocked,
		ApprovalDecisionsTotal:          approvalDecisions,
		OverrideAppliedTotal:            overrideApplied,
		OverrideValidationRejectedTotal: overrideRejected,
	}
}

// initZeroValues initializes all pre-registration-eligible metrics with 0
// values so they appear in the /metrics endpoint before first increment.
// Per DD-METRICS-001: Metrics visibility requirement for E2E tests.
func initZeroValues(m *Metrics) {
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
}

// NewMetrics creates a new Metrics instance and registers with controller-runtime.
// Uses controller-runtime's global registry for automatic /metrics endpoint exposure.
// Per DD-METRICS-001: Dependency injection pattern for V1.0 maturity.
func NewMetrics() *Metrics {
	m := newMetricsCollectors()

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

	initZeroValues(m)

	return m
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing).
// Tests should use this to avoid polluting the global registry.
// Per DD-METRICS-001: Test isolation pattern.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := newMetricsCollectors()

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

	initZeroValues(m)

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
