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

// Package controller provides the Kubernetes controller for RemediationApprovalRequest audit.
//
// TDD GREEN Phase: Minimal implementation to make E2E tests pass
//
// Business Requirements:
// - BR-AUDIT-006: Approval decision audit trail (SOC 2 CC8.1, CC6.8)
// - DD-WEBHOOK-003: Webhook-Complete Audit Pattern
// - ADR-040: RemediationApprovalRequest CRD Architecture
//
// Responsibilities:
// - Watch RemediationApprovalRequest for status.Decision changes
// - Emit approval audit events (orchestrator.approval.approved/rejected)
// - Fire-and-forget pattern (don't block on audit failures)
package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// RARReconciler reconciles RemediationApprovalRequest objects for audit purposes only.
// This controller does NOT manage RAR lifecycle - it only emits audit events.
//
// REFACTOR Phase: Enhanced with metrics integration and structured logging.
type RARReconciler struct {
	client       client.Client
	apiReader    client.Reader      // DD-STATUS-001: Cache-bypassed reads for idempotency guard
	scheme       *runtime.Scheme
	auditStore   audit.AuditStore
	auditManager *roaudit.Manager
	metrics      *rometrics.Metrics // DD-METRICS-001: Metrics for observability
}

// NewRARReconciler creates a new RAR reconciler for audit event emission.
//
// Parameters:
//   - client: Kubernetes client for CRD operations
//   - apiReader: Cache-bypassed reader for idempotency guard (DD-STATUS-001)
//   - scheme: Kubernetes scheme for type registration
//   - auditStore: Buffered audit store for event emission
//   - metrics: Metrics instance for observability (DD-METRICS-001)
//
// Returns configured RARReconciler ready for controller-runtime registration.
func NewRARReconciler(
	client client.Client,
	apiReader client.Reader,
	scheme *runtime.Scheme,
	auditStore audit.AuditStore,
	metrics *rometrics.Metrics,
) *RARReconciler {
	return &RARReconciler{
		client:       client,
		apiReader:    apiReader,
		scheme:       scheme,
		auditStore:   auditStore,
		auditManager: roaudit.NewManager(roaudit.ServiceName), // Use RO audit manager (category="orchestration" per ADR-034 v1.7)
		metrics:      metrics,                                 // REFACTOR: Metrics integration
	}
}

// Reconcile handles RemediationApprovalRequest status changes for audit.
//
// TDD GREEN: Minimal implementation - only emit audit events on decision.
// REFACTOR: Will enhance with sophisticated logic later.
func (r *RARReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch RAR from cache for initial check
	rar := &remediationv1.RemediationApprovalRequest{}
	if err := r.client.Get(ctx, req.NamespacedName, rar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Idempotency: Only emit audit if decision is made AND not yet audited
	// Per ADR-040: Decision is immutable once set (natural idempotency for decision itself)
	if rar.Status.Decision == "" {
		return ctrl.Result{}, nil
	}

	// DD-STATUS-001: Refetch via apiReader (cache-bypassed) to confirm AuditRecorded
	// condition is genuinely absent. The informer cache is eventually consistent — a
	// second reconcile may start before the first reconcile's Status().Update() for
	// AuditRecorded is reflected in the cache, causing duplicate audit emission.
	if err := r.apiReader.Get(ctx, req.NamespacedName, rar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	auditCondition := meta.FindStatusCondition(rar.Status.Conditions, rarconditions.ConditionAuditRecorded)
	if auditCondition != nil && auditCondition.Status == "True" {
		return ctrl.Result{}, nil
	}

	// Get parent RemediationRequest name for correlation ID
	parentRRName := rar.Spec.RemediationRequestRef.Name
	if parentRRName == "" {
		logger.Error(nil, "RAR missing parent RemediationRequest reference", "rar", rar.Name)
		return ctrl.Result{}, nil
	}

	// Build approval decision audit event
	decision := string(rar.Status.Decision)
	decidedBy := rar.Status.DecidedBy

	// REFACTOR: Record approval decision for business analytics
	// Business Value: Track approval/rejection rates for compliance reporting
	if r.metrics != nil {
		r.metrics.RecordApprovalDecision(string(decision), rar.Namespace)
	}

	logger.Info("Emitting approval decision audit event",
		"rar", rar.Name,
		"decision", decision,
		"decidedBy", decidedBy,
		"correlationID", parentRRName,
		"namespace", rar.Namespace)

	// Build approval audit event using RO audit manager (category="orchestration" per ADR-034 v1.7)
	event, err := r.auditManager.BuildApprovalDecisionEvent(
		parentRRName,               // correlation_id = parent RR name
		rar.Namespace,              // namespace
		parentRRName,               // rr_name
		rar.Name,                   // rar_name
		decision,                   // decision (Approved/Rejected/Expired)
		decidedBy,                  // decided_by (from AuthWebhook)
		rar.Status.DecisionMessage, // decision_message
	)

	if err != nil {
		logger.Error(err, "Failed to build approval audit event", "rar", rar.Name)
		// Fire-and-forget: Don't fail reconciliation on audit errors
		return ctrl.Result{}, nil
	}

	// Store audit event (fire-and-forget)
	auditErr := r.auditStore.StoreAudit(ctx, event)

	// Record metrics and log the audit outcome
	if auditErr != nil {
		logger.Error(auditErr, "Failed to store approval audit event",
			"rar", rar.Name,
			"decision", decision,
			"namespace", rar.Namespace)
		if r.metrics != nil {
			r.metrics.RecordAuditEventFailure("RAR", "approval_decision", rar.Namespace)
		}
	} else {
		logger.Info("Successfully emitted approval audit event",
			"rar", rar.Name,
			"category", "orchestration",
			"eventType", event.EventType,
			"decision", decision,
			"decidedBy", decidedBy,
			"correlationID", parentRRName,
			"namespace", rar.Namespace)
		if r.metrics != nil {
			r.metrics.RecordAuditEventSuccess("RAR", "approval_decision", rar.Namespace)
		}
	}

	// Persist AuditRecorded condition with conflict retry.
	// The main RO reconciler concurrently updates RAR status conditions
	// (ApprovalPending/Decided/Ready), which can cause optimistic concurrency
	// conflicts. Without retry, a lost Status().Update() leaves AuditRecorded
	// unset, causing duplicate audit emission on the next reconciliation.
	if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, req.NamespacedName, rar); err != nil {
			return err
		}
		auditCond := meta.FindStatusCondition(rar.Status.Conditions, rarconditions.ConditionAuditRecorded)
		if auditCond != nil && auditCond.Status == "True" {
			return nil
		}
		if auditErr != nil {
			rarconditions.SetAuditRecorded(rar, false,
				rarconditions.ReasonAuditFailed,
				fmt.Sprintf("Failed to record audit event: %v", auditErr),
				r.metrics)
		} else {
			rarconditions.SetAuditRecorded(rar, true,
				rarconditions.ReasonAuditSucceeded,
				fmt.Sprintf("Approval audit event %s recorded to DataStorage", event.EventType),
				r.metrics)
		}
		return r.client.Status().Update(ctx, rar)
	}); err != nil {
		logger.Error(err, "Failed to persist AuditRecorded condition", "rar", rar.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RARReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationApprovalRequest{}).
		Complete(r)
}
