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
	MetricNameReconcileTotal          = "kubernaut_remediationorchestrator_reconcile_total"
	MetricNameReconcileDuration       = "kubernaut_remediationorchestrator_reconcile_duration_seconds"
	MetricNamePhaseTransitionsTotal   = "kubernaut_remediationorchestrator_phase_transitions_total"

	// Child CRD orchestration metrics
	MetricNameChildCRDCreationsTotal = "kubernaut_remediationorchestrator_child_crd_creations_total"

	// Notification metrics
	MetricNameManualReviewNotificationsTotal  = "kubernaut_remediationorchestrator_manual_review_notifications_total"
	MetricNameApprovalNotificationsTotal      = "kubernaut_remediationorchestrator_approval_notifications_total"
	MetricNameCompletionNotificationsTotal    = "kubernaut_remediationorchestrator_completion_notifications_total"

	// Routing decision metrics
	MetricNameNoActionNeededTotal    = "kubernaut_remediationorchestrator_no_action_needed_total"
	MetricNameDuplicatesSkippedTotal = "kubernaut_remediationorchestrator_duplicates_skipped_total"
	MetricNameTimeoutsTotal          = "kubernaut_remediationorchestrator_timeouts_total"

	// Blocking metrics (BR-ORCH-042)
	MetricNameBlockedTotal               = "kubernaut_remediationorchestrator_blocked_total"
	MetricNameBlockedCooldownExpired     = "kubernaut_remediationorchestrator_blocked_cooldown_expired_total"
	MetricNameCurrentBlocked             = "kubernaut_remediationorchestrator_current_blocked"

	// Notification lifecycle metrics (BR-ORCH-029/030)
	MetricNameNotificationCancellationsTotal = "kubernaut_remediationorchestrator_notification_cancellations_total"
	MetricNameNotificationStatus             = "kubernaut_remediationorchestrator_notification_status"
	MetricNameNotificationDeliveryDuration   = "kubernaut_remediationorchestrator_notification_delivery_duration_seconds"

	// Retry metrics (REFACTOR-RO-008)
	MetricNameStatusUpdateRetriesTotal   = "kubernaut_remediationorchestrator_status_update_retries_total"
	MetricNameStatusUpdateConflictsTotal = "kubernaut_remediationorchestrator_status_update_conflicts_total"

	// Condition metrics (BR-ORCH-043, DD-CRD-002)
	MetricNameConditionStatus           = "kubernaut_remediationorchestrator_condition_status"
	MetricNameConditionTransitionsTotal = "kubernaut_remediationorchestrator_condition_transitions_total"

	// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
	MetricNameApprovalDecisionsTotal = "kubernaut_remediationorchestrator_approval_decisions_total"

	// Audit event metrics (BR-AUDIT-006 - SOC 2 CC7.2 audit trail completeness)
	MetricNameAuditEventsTotal = "kubernaut_remediationorchestrator_audit_events_total"
)

// Metrics holds all Prometheus metrics for the Remediation Orchestrator controller.
// Per DD-METRICS-001: Dependency-injected metrics pattern for testability and clarity.
type Metrics struct {
	// === CORE RECONCILIATION METRICS ===
	ReconcileTotal           *prometheus.CounterVec
	ReconcileDurationSeconds *prometheus.HistogramVec
	PhaseTransitionsTotal    *prometheus.CounterVec

	// === CHILD CRD ORCHESTRATION METRICS ===
	ChildCRDCreationsTotal *prometheus.CounterVec

	// === NOTIFICATION METRICS ===
	ManualReviewNotificationsTotal *prometheus.CounterVec
	ApprovalNotificationsTotal     *prometheus.CounterVec
	CompletionNotificationsTotal   *prometheus.CounterVec // BR-ORCH-045: Completion notifications

	// === ROUTING DECISION METRICS ===
	NoActionNeededTotal    *prometheus.CounterVec
	DuplicatesSkippedTotal *prometheus.CounterVec
	TimeoutsTotal          *prometheus.CounterVec

	// === BLOCKING METRICS (BR-ORCH-042) ===
	BlockedTotal                *prometheus.CounterVec
	BlockedCooldownExpiredTotal prometheus.Counter
	CurrentBlockedGauge         *prometheus.GaugeVec

	// === NOTIFICATION LIFECYCLE METRICS (BR-ORCH-029/030) ===
	NotificationCancellationsTotal      *prometheus.CounterVec
	NotificationStatusGauge             *prometheus.GaugeVec
	NotificationDeliveryDurationSeconds *prometheus.HistogramVec

	// === RETRY METRICS (REFACTOR-RO-008) ===
	StatusUpdateRetriesTotal   *prometheus.CounterVec
	StatusUpdateConflictsTotal *prometheus.CounterVec

	// === CONDITION METRICS (BR-ORCH-043, DD-CRD-002) ===
	ConditionStatus           *prometheus.GaugeVec
	ConditionTransitionsTotal *prometheus.CounterVec

	// === APPROVAL DECISION METRICS (BR-AUDIT-006 - SOC 2 Compliance) ===
	// Business Value: Track approval/rejection rates for compliance reporting and operational insights
	ApprovalDecisionsTotal *prometheus.CounterVec

	// === AUDIT EVENT METRICS (BR-AUDIT-006 - SOC 2 CC7.2 Audit Trail Completeness) ===
	// Business Value: Track audit event success/failure for compliance alerting
	AuditEventsTotal *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance and registers with controller-runtime.
// Uses controller-runtime's global registry for automatic /metrics endpoint exposure.
// Per DD-METRICS-001: Dependency injection pattern for V1.0 maturity.
func NewMetrics() *Metrics {
	m := &Metrics{
		// Core reconciliation metrics
		ReconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcileTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of reconciliation attempts",
			},
			[]string{"namespace", "phase"},
		),
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

		// Notification metrics
		ManualReviewNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameManualReviewNotificationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of manual review notifications created",
			},
			[]string{"source", "reason", "sub_reason", "namespace"},
		),
		ApprovalNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalNotificationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of approval notifications created",
			},
			[]string{"namespace"},
		),
		CompletionNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameCompletionNotificationsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of completion notifications created (BR-ORCH-045)",
			},
			[]string{"namespace"},
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
		BlockedCooldownExpiredTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameBlockedCooldownExpired, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total blocked RRs that expired and transitioned to Failed",
			},
		),
		CurrentBlockedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameCurrentBlocked, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current number of blocked RRs",
			},
			[]string{"namespace"},
		),

		// Notification lifecycle metrics (BR-ORCH-029/030)
		NotificationCancellationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameNotificationCancellationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of user-initiated notification cancellations",
			},
			[]string{"namespace"},
		),
		NotificationStatusGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameNotificationStatus, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current notification status distribution",
			},
			[]string{"namespace", "status"},
		),
		NotificationDeliveryDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameNotificationDeliveryDuration, // DD-005 V3.0: Pattern B (full name),
				Help:      "Duration of notification delivery in seconds",
				Buckets:   prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~1000s
			},
			[]string{"namespace", "status"},
		),

		// Retry metrics (REFACTOR-RO-008)
		StatusUpdateRetriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameStatusUpdateRetriesTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of retry attempts for RemediationRequest status updates (REFACTOR-RO-001)",
			},
			[]string{"namespace", "outcome"},
		),
		StatusUpdateConflictsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameStatusUpdateConflictsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of optimistic concurrency conflicts during status updates (DD-GATEWAY-011)",
			},
			[]string{"namespace"},
		),

		// Condition metrics (BR-ORCH-043, DD-CRD-002)
		ConditionStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameConditionStatus, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current Kubernetes Condition status for RO-managed CRDs (1=set, 0=not set)",
			},
			[]string{"crd_type", "condition_type", "status", "namespace"},
		),
		ConditionTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameConditionTransitionsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of Kubernetes Condition status transitions",
			},
			[]string{"crd_type", "condition_type", "from_status", "to_status", "namespace"},
		),

		// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of approval decisions (approved/rejected/expired). Business Value: Track approval rates for compliance reporting and operational insights (SOC 2 CC8.1).",
			},
			[]string{"decision", "namespace"},
		),

		// Audit event metrics (BR-AUDIT-006 - SOC 2 CC7.2)
		AuditEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuditEventsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of audit event emissions (success/failure). Business Value: Track audit trail completeness for compliance alerting (SOC 2 CC7.2 - audit integrity).",
			},
			[]string{"crd_type", "event_type", "status", "namespace"},
		),
	}

	// Register all metrics with controller-runtime's global registry
	// This makes metrics available at :8080/metrics endpoint
	ctrlmetrics.Registry.MustRegister(
		m.ReconcileTotal,
		m.ReconcileDurationSeconds,
		m.PhaseTransitionsTotal,
		m.ChildCRDCreationsTotal,
		m.ManualReviewNotificationsTotal,
		m.ApprovalNotificationsTotal,
		m.CompletionNotificationsTotal,
		m.NoActionNeededTotal,
		m.DuplicatesSkippedTotal,
		m.TimeoutsTotal,
		m.BlockedTotal,
		m.BlockedCooldownExpiredTotal,
		m.CurrentBlockedGauge,
		m.NotificationCancellationsTotal,
		m.NotificationStatusGauge,
		m.NotificationDeliveryDurationSeconds,
		m.StatusUpdateRetriesTotal,
		m.StatusUpdateConflictsTotal,
		m.ConditionStatus,
		m.ConditionTransitionsTotal,
		m.ApprovalDecisionsTotal,
		m.AuditEventsTotal,
	)

	// Initialize all metrics with 0 values so they appear in /metrics endpoint
	// Per DD-METRICS-001: Metrics visibility requirement for E2E tests
	// This ensures metrics are discoverable even before first increment
	m.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", "default").Add(0)
	m.ManualReviewNotificationsTotal.WithLabelValues("reconciler", "WorkflowResolutionFailed", "WorkflowNotFound", "default").Add(0)
	m.ApprovalNotificationsTotal.WithLabelValues("default").Add(0)
	m.CompletionNotificationsTotal.WithLabelValues("default").Add(0)
	m.NoActionNeededTotal.WithLabelValues("default", "Completed").Add(0)
	m.DuplicatesSkippedTotal.WithLabelValues("default", "test_signal").Add(0)
	m.TimeoutsTotal.WithLabelValues("default", "Pending").Add(0)
	m.BlockedTotal.WithLabelValues("default", "ConsecutiveFailures").Add(0)
	m.BlockedCooldownExpiredTotal.Add(0)
	m.CurrentBlockedGauge.WithLabelValues("default").Set(0)
	m.NotificationCancellationsTotal.WithLabelValues("default").Add(0)
	m.NotificationStatusGauge.WithLabelValues("default", "pending").Set(0)
	m.NotificationDeliveryDurationSeconds.WithLabelValues("default", "delivered").Observe(0)
	m.StatusUpdateRetriesTotal.WithLabelValues("default", "success").Add(0)
	m.StatusUpdateConflictsTotal.WithLabelValues("default").Add(0)
	m.ConditionStatus.WithLabelValues("RemediationRequest", "SignalProcessingReady", "True", "default").Set(0)
	m.ConditionTransitionsTotal.WithLabelValues("RemediationRequest", "SignalProcessingReady", "", "True", "default").Add(0)

	// BR-AUDIT-006: Initialize approval decision and audit event metrics
	m.ApprovalDecisionsTotal.WithLabelValues("Approved", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Expired", "default").Add(0)
	m.AuditEventsTotal.WithLabelValues("RAR", "approval_decision", "success", "default").Add(0)
	m.AuditEventsTotal.WithLabelValues("RAR", "approval_decision", "failure", "default").Add(0)

	return m
}

// NewMetricsWithRegistry creates metrics with custom registry (for testing).
// Tests should use this to avoid polluting the global registry.
// Per DD-METRICS-001: Test isolation pattern.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		// Core reconciliation metrics
		ReconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameReconcileTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of reconciliation attempts",
			},
			[]string{"namespace", "phase"},
		),
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
		ManualReviewNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameManualReviewNotificationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of manual review notifications created",
			},
			[]string{"source", "reason", "sub_reason", "namespace"},
		),
		ApprovalNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalNotificationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of approval notifications created",
			},
			[]string{"namespace"},
		),
		CompletionNotificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameCompletionNotificationsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of completion notifications created (BR-ORCH-045)",
			},
			[]string{"namespace"},
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
		BlockedCooldownExpiredTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameBlockedCooldownExpired, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total blocked RRs that expired and transitioned to Failed",
			},
		),
		CurrentBlockedGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameCurrentBlocked, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current number of blocked RRs",
			},
			[]string{"namespace"},
		),
		NotificationCancellationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameNotificationCancellationsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of user-initiated notification cancellations",
			},
			[]string{"namespace"},
		),
		NotificationStatusGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameNotificationStatus, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current notification status distribution",
			},
			[]string{"namespace", "status"},
		),
		NotificationDeliveryDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: MetricNameNotificationDeliveryDuration, // DD-005 V3.0: Pattern B (full name),
				Help:      "Duration of notification delivery in seconds",
				Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"namespace", "status"},
		),
		StatusUpdateRetriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameStatusUpdateRetriesTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of retry attempts for RemediationRequest status updates",
			},
			[]string{"namespace", "outcome"},
		),
		StatusUpdateConflictsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameStatusUpdateConflictsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of optimistic concurrency conflicts during status updates",
			},
			[]string{"namespace"},
		),
		ConditionStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameConditionStatus, // DD-005 V3.0: Pattern B (full name),
				Help:      "Current Kubernetes Condition status for RO-managed CRDs",
			},
			[]string{"crd_type", "condition_type", "status", "namespace"},
		),
		ConditionTransitionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameConditionTransitionsTotal, // DD-005 V3.0: Pattern B (full name),
				Help:      "Total number of Kubernetes Condition status transitions",
			},
			[]string{"crd_type", "condition_type", "from_status", "to_status", "namespace"},
		),

		// Approval decision metrics (BR-AUDIT-006 - SOC 2 compliance)
		ApprovalDecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameApprovalDecisionsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of approval decisions (approved/rejected/expired). Business Value: Track approval rates for compliance reporting and operational insights (SOC 2 CC8.1).",
			},
			[]string{"decision", "namespace"},
		),

		// Audit event metrics (BR-AUDIT-006 - SOC 2 CC7.2)
		AuditEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameAuditEventsTotal, // DD-005 V3.0: Pattern B (full name)
				Help: "Total number of audit event emissions (success/failure). Business Value: Track audit trail completeness for compliance alerting (SOC 2 CC7.2 - audit integrity).",
			},
			[]string{"crd_type", "event_type", "status", "namespace"},
		),
	}

	// Register with provided registry (test registry)
	registry.MustRegister(
		m.ReconcileTotal,
		m.ReconcileDurationSeconds,
		m.PhaseTransitionsTotal,
		m.ChildCRDCreationsTotal,
		m.ManualReviewNotificationsTotal,
		m.ApprovalNotificationsTotal,
		m.CompletionNotificationsTotal,
		m.NoActionNeededTotal,
		m.DuplicatesSkippedTotal,
		m.TimeoutsTotal,
		m.BlockedTotal,
		m.BlockedCooldownExpiredTotal,
		m.CurrentBlockedGauge,
		m.NotificationCancellationsTotal,
		m.NotificationStatusGauge,
		m.NotificationDeliveryDurationSeconds,
		m.StatusUpdateRetriesTotal,
		m.StatusUpdateConflictsTotal,
		m.ConditionStatus,
		m.ConditionTransitionsTotal,
		m.ApprovalDecisionsTotal,
		m.AuditEventsTotal,
	)

	// Initialize all metrics with 0 values so they appear in /metrics endpoint
	// Per E2E test requirements: metrics should be visible even if not yet incremented
	// This prevents "metric not found" errors in E2E tests
	m.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", "default").Add(0)
	m.ManualReviewNotificationsTotal.WithLabelValues("reconciler", "WorkflowResolutionFailed", "WorkflowNotFound", "default").Add(0)
	m.ApprovalNotificationsTotal.WithLabelValues("default").Add(0)
	m.CompletionNotificationsTotal.WithLabelValues("default").Add(0)
	m.NoActionNeededTotal.WithLabelValues("default", "Completed").Add(0)
	m.DuplicatesSkippedTotal.WithLabelValues("default", "test_signal").Add(0)
	m.TimeoutsTotal.WithLabelValues("default", "Pending").Add(0)
	m.BlockedTotal.WithLabelValues("default", "ConsecutiveFailures").Add(0)
	m.BlockedCooldownExpiredTotal.Add(0)
	m.CurrentBlockedGauge.WithLabelValues("default").Set(0)
	m.NotificationCancellationsTotal.WithLabelValues("default").Add(0)
	m.NotificationStatusGauge.WithLabelValues("default", "pending").Set(0)
	m.NotificationDeliveryDurationSeconds.WithLabelValues("default", "delivered").Observe(0)
	m.StatusUpdateRetriesTotal.WithLabelValues("default", "success").Add(0)
	m.StatusUpdateConflictsTotal.WithLabelValues("default").Add(0)
	m.ConditionStatus.WithLabelValues("RemediationRequest", "SignalProcessingReady", "True", "default").Set(0)
	m.ConditionTransitionsTotal.WithLabelValues("RemediationRequest", "SignalProcessingReady", "", "True", "default").Add(0)

	// BR-AUDIT-006: Initialize approval decision and audit event metrics
	m.ApprovalDecisionsTotal.WithLabelValues("Approved", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Rejected", "default").Add(0)
	m.ApprovalDecisionsTotal.WithLabelValues("Expired", "default").Add(0)
	m.AuditEventsTotal.WithLabelValues("RAR", "approval_decision", "success", "default").Add(0)
	m.AuditEventsTotal.WithLabelValues("RAR", "approval_decision", "failure", "default").Add(0)

	return m
}

// ========================================
// CONDITION METRICS HELPERS (BR-ORCH-043, DD-CRD-002)
// ========================================

// RecordConditionStatus records the current status of a Kubernetes Condition.
// This sets the gauge to 1 for the specified status and clears other statuses for the same condition.
//
// Parameters:
//   - crdType: "RemediationRequest" or "RemediationApprovalRequest"
//   - conditionType: The condition type (e.g., "SignalProcessingReady")
//   - status: "True", "False", or "Unknown"
//   - namespace: K8s namespace
//
// Example:
//
//	m.RecordConditionStatus("RemediationRequest", "SignalProcessingReady", "True", "default")
func (m *Metrics) RecordConditionStatus(crdType, conditionType, status, namespace string) {
	// Clear all statuses for this condition (gauge can only have one value)
	for _, s := range []string{"True", "False", "Unknown"} {
		if s == status {
			m.ConditionStatus.WithLabelValues(crdType, conditionType, s, namespace).Set(1)
		} else {
			m.ConditionStatus.WithLabelValues(crdType, conditionType, s, namespace).Set(0)
		}
	}
}

// RecordConditionTransition records a transition between condition statuses.
//
// Parameters:
//   - crdType: "RemediationRequest" or "RemediationApprovalRequest"
//   - conditionType: The condition type (e.g., "SignalProcessingReady")
//   - fromStatus: Previous status ("True", "False", "Unknown", or "" for initial set)
//   - toStatus: New status ("True", "False", "Unknown")
//   - namespace: K8s namespace
//
// Example:
//
//	m.RecordConditionTransition("RemediationRequest", "SignalProcessingReady", "False", "True", "default")
func (m *Metrics) RecordConditionTransition(crdType, conditionType, fromStatus, toStatus, namespace string) {
	m.ConditionTransitionsTotal.WithLabelValues(crdType, conditionType, fromStatus, toStatus, namespace).Inc()
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

// ========================================
// AUDIT EVENT METRICS HELPERS (BR-AUDIT-006 - SOC 2 CC7.2)
// ========================================

// RecordAuditEventSuccess records successful audit event emission.
//
// Business Value:
//   - Tracks audit trail completeness for SOC 2 CC7.2 compliance
//   - Ensures all approval decisions are properly audited
//   - Baseline for compliance dashboards and reports
//
// Parameters:
//   - crdType: "RAR", "RR", "WFE", etc.
//   - eventType: "approval_decision", "lifecycle_event", etc.
//   - namespace: K8s namespace
//
// Example:
//
//	m.RecordAuditEventSuccess("RAR", "approval_decision", "production")
func (m *Metrics) RecordAuditEventSuccess(crdType, eventType, namespace string) {
	m.AuditEventsTotal.WithLabelValues(crdType, eventType, "success", namespace).Inc()
}

// RecordAuditEventFailure records failed audit event emission.
//
// Business Value:
//   - CRITICAL: Audit failures indicate compliance gaps (SOC 2 CC7.2 violation)
//   - Triggers immediate alerting for audit trail integrity issues
//   - Tracks DataStorage availability and audit infrastructure health
//
// Parameters:
//   - crdType: "RAR", "RR", "WFE", etc.
//   - eventType: "approval_decision", "lifecycle_event", etc.
//   - namespace: K8s namespace
//
// Example:
//
//	m.RecordAuditEventFailure("RAR", "approval_decision", "production")
func (m *Metrics) RecordAuditEventFailure(crdType, eventType, namespace string) {
	m.AuditEventsTotal.WithLabelValues(crdType, eventType, "failure", namespace).Inc()
}
