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
// All metrics follow DD-005 naming convention: kubernaut_remediationorchestrator_<metric_name>
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Namespace for all RO metrics (DD-005 compliant)
	namespace = "kubernaut"
	subsystem = "remediationorchestrator"
)

var (
	// ReconcileTotal counts total reconciliation attempts
	// Reference: Standard controller metric
	// Labels: namespace (K8s namespace), phase (RR phase at reconcile time)
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconcile_total",
			Help:      "Total number of reconciliation attempts",
		},
		[]string{"namespace", "phase"},
	)

	// ManualReviewNotificationsTotal counts manual review notifications created
	// Reference: BR-ORCH-036
	ManualReviewNotificationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "manual_review_notifications_total",
			Help:      "Total number of manual review notifications created",
		},
		[]string{"source", "reason", "sub_reason", "namespace"},
	)

	// NoActionNeededTotal counts remediations where no action was required
	// Reference: BR-ORCH-037, BR-HAPI-200
	NoActionNeededTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "no_action_needed_total",
			Help:      "Total number of remediations where no action was needed (problem self-resolved)",
		},
		[]string{"reason", "namespace"},
	)

	// ApprovalNotificationsTotal counts approval notifications created
	// Reference: BR-ORCH-001
	ApprovalNotificationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "approval_notifications_total",
			Help:      "Total number of approval notifications created",
		},
		[]string{"namespace"},
	)

	// PhaseTransitionsTotal counts phase transitions
	// Reference: Standard controller metric
	PhaseTransitionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "phase_transitions_total",
			Help:      "Total number of phase transitions",
		},
		[]string{"from_phase", "to_phase", "namespace"},
	)

	// ReconcileDurationSeconds measures reconciliation duration
	// Reference: Standard controller metric
	ReconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliation in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to ~10s
		},
		[]string{"namespace", "phase"},
	)

	// ChildCRDCreationsTotal counts child CRD creations
	// Reference: BR-ORCH-025 (child CRD orchestration)
	ChildCRDCreationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "child_crd_creations_total",
			Help:      "Total number of child CRD creations",
		},
		[]string{"crd_type", "namespace"},
	)

	// DuplicatesSkippedTotal counts duplicate remediations skipped
	// Reference: BR-ORCH-032, BR-ORCH-033
	DuplicatesSkippedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "duplicates_skipped_total",
			Help:      "Total number of duplicate remediations skipped",
		},
		[]string{"skip_reason", "namespace"},
	)

	// TimeoutsTotal counts remediation timeouts
	// Reference: BR-ORCH-027, BR-ORCH-028
	TimeoutsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "timeouts_total",
			Help:      "Total number of remediation timeouts",
		},
		[]string{"phase", "namespace"},
	)

	// ========================================
	// BLOCKING METRICS (BR-ORCH-042)
	// TDD: Tests in test/unit/remediationorchestrator/metrics_test.go
	// ========================================

	// BlockedTotal counts RRs blocked due to consecutive failures
	// Reference: BR-ORCH-042
	// Labels: namespace (K8s namespace), reason (block reason)
	BlockedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "blocked_total",
			Help:      "Total RemediationRequests blocked due to consecutive failures",
		},
		[]string{"namespace", "reason"},
	)

	// BlockedCooldownExpiredTotal counts blocked RRs that expired
	// Reference: BR-ORCH-042.3
	BlockedCooldownExpiredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "blocked_cooldown_expired_total",
			Help:      "Total blocked RRs that expired and transitioned to Failed",
		},
	)

	// CurrentBlockedGauge tracks current blocked RR count
	// Reference: BR-ORCH-042
	// Labels: namespace (K8s namespace)
	CurrentBlockedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "blocked_current",
			Help:      "Current number of blocked RRs",
		},
		[]string{"namespace"},
	)
)

func init() {
	// Register all metrics with controller-runtime registry
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ManualReviewNotificationsTotal,
		NoActionNeededTotal,
		ApprovalNotificationsTotal,
		PhaseTransitionsTotal,
		ReconcileDurationSeconds,
		ChildCRDCreationsTotal,
		DuplicatesSkippedTotal,
		TimeoutsTotal,
		// BR-ORCH-042: Blocking metrics (TDD validated)
		BlockedTotal,
		BlockedCooldownExpiredTotal,
		CurrentBlockedGauge,
	)
}

// Collector holds all Prometheus metrics for the orchestrator.
// Deprecated: Use package-level metrics directly instead.
type Collector struct{}

// NewCollector creates a new metrics Collector.
// Deprecated: Use package-level metrics directly instead.
func NewCollector() *Collector {
	return &Collector{}
}

