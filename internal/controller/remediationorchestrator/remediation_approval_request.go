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

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	raraudit "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// RARReconciler reconciles RemediationApprovalRequest objects for audit purposes only.
// This controller does NOT manage RAR lifecycle - it only emits audit events.
//
// REFACTOR Phase: Enhanced with metrics integration and structured logging.
type RARReconciler struct {
	client      client.Client
	scheme      *runtime.Scheme
	auditClient *raraudit.AuditClient
	metrics     *rometrics.Metrics // DD-METRICS-001: Metrics for observability
}

// NewRARReconciler creates a new RAR reconciler for audit event emission.
//
// Parameters:
//   - client: Kubernetes client for CRD operations
//   - scheme: Kubernetes scheme for type registration
//   - auditStore: Buffered audit store for event emission
//   - metrics: Metrics instance for observability (DD-METRICS-001)
//
// Returns configured RARReconciler ready for controller-runtime registration.
func NewRARReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	auditStore audit.AuditStore,
	metrics *rometrics.Metrics,
) *RARReconciler {
	logger := ctrl.Log.WithName("rar-audit")
	return &RARReconciler{
		client:      client,
		scheme:      scheme,
		auditClient: raraudit.NewAuditClient(auditStore, logger), // Use RAR-specific audit client (category="approval")
		metrics:     metrics,                                     // REFACTOR: Metrics integration
	}
}

// Reconcile handles RemediationApprovalRequest status changes for audit.
//
// TDD GREEN: Minimal implementation - only emit audit events on decision.
// REFACTOR: Will enhance with sophisticated logic later.
func (r *RARReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch RAR
	rar := &remediationv1.RemediationApprovalRequest{}
	if err := r.client.Get(ctx, req.NamespacedName, rar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Idempotency: Only emit audit if decision is made AND not yet audited
	// Per ADR-040: Decision is immutable once set (natural idempotency for decision itself)
	if rar.Status.Decision == "" {
		// No decision yet - nothing to do
		return ctrl.Result{}, nil
	}

	// Check if we already emitted audit event (per WorkflowExecution pattern)
	// Use AuditRecorded condition for idempotency (secure, tamper-proof)
	auditCondition := meta.FindStatusCondition(rar.Status.Conditions, rarconditions.ConditionAuditRecorded)
	if auditCondition != nil && auditCondition.Status == "True" {
		// Already emitted - skip
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

	// Emit approval audit event (category="approval", event_type="approval.decision.approved/rejected")
	// This uses the RAR-specific audit client which correctly sets category="approval"
	// Fire-and-forget: Errors are logged but don't block reconciliation
	r.auditClient.RecordApprovalDecision(ctx, rar)

	// Set AuditRecorded condition (RecordApprovalDecision is fire-and-forget, doesn't update RAR)
	// We assume success since RecordApprovalDecision uses fire-and-forget pattern
	rarconditions.SetAuditRecorded(rar, true,
		rarconditions.ReasonAuditSucceeded,
		"Approval decision audit event emitted",
		r.metrics)

	logger.Info("Successfully emitted approval audit event",
		"rar", rar.Name,
		"category", "approval",
		"decision", decision,
		"decidedBy", decidedBy,
		"correlationID", parentRRName,
		"namespace", rar.Namespace)

	// REFACTOR: Record audit success for compliance monitoring
	// Business Value: Track audit trail completeness (SOC 2 CC7.2)
	if r.metrics != nil {
		r.metrics.RecordAuditEventSuccess("RAR", "approval_decision", rar.Namespace)
	}

	// Update RAR status with AuditRecorded condition
	if err := r.client.Status().Update(ctx, rar); err != nil {
		logger.Error(err, "Failed to update RAR status with AuditRecorded condition", "rar", rar.Name)
		// Fire-and-forget: Don't fail reconciliation
		// Will retry on next reconcile (idempotency via condition check above)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RARReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationApprovalRequest{}).
		Complete(r)
}
