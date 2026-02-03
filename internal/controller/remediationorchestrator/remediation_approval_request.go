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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

// RARReconciler reconciles RemediationApprovalRequest objects for audit purposes only.
// This controller does NOT manage RAR lifecycle - it only emits audit events.
type RARReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	auditStore   audit.AuditStore
	auditManager *roaudit.Manager
}

// NewRARReconciler creates a new RAR reconciler for audit event emission.
func NewRARReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	auditStore audit.AuditStore,
	auditManager *roaudit.Manager,
) *RARReconciler {
	return &RARReconciler{
		client:       client,
		scheme:       scheme,
		auditStore:   auditStore,
		auditManager: auditManager,
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

	// TDD GREEN: Only emit audit event if decision is made
	// Idempotency: Decision is immutable once set (per ADR-040)
	if rar.Status.Decision == "" {
		// No decision yet - nothing to do
		return ctrl.Result{}, nil
	}

	// Check if we already emitted audit event
	// TDD GREEN: Use simple annotation for now (REFACTOR later with proper state tracking)
	if rar.Annotations != nil && rar.Annotations["kubernaut.ai/audit-emitted"] == "true" {
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
	
	logger.Info("Emitting approval decision audit event",
		"rar", rar.Name,
		"decision", decision,
		"decidedBy", decidedBy,
		"correlationID", parentRRName)

	event, err := r.auditManager.BuildApprovalDecisionEvent(
		parentRRName,                    // correlation_id = parent RR name
		rar.Namespace,                   // namespace
		parentRRName,                    // rr_name
		rar.Name,                        // rar_name
		decision,                        // decision (Approved/Rejected/Expired)
		decidedBy,                       // decided_by (from AuthWebhook)
		rar.Status.DecisionMessage,     // decision_message
		rar.Spec.Confidence,             // confidence
		fmt.Sprintf("%.2f", rar.Spec.Confidence), // confidence_str
		rar.Spec.RecommendedWorkflow.WorkflowID,  // workflow_id
	)

	if err != nil {
		logger.Error(err, "Failed to build approval audit event", "rar", rar.Name)
		// Fire-and-forget: Don't fail reconciliation on audit errors
		return ctrl.Result{}, nil
	}

	// Store audit event (fire-and-forget)
	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store approval audit event", "rar", rar.Name)
		// Fire-and-forget: Don't fail reconciliation on audit errors
		return ctrl.Result{}, nil
	}

	logger.Info("Successfully emitted approval audit event",
		"rar", rar.Name,
		"eventType", event.EventType,
		"correlationID", parentRRName)

	// Mark as audited (TDD GREEN: simple annotation, REFACTOR later)
	if rar.Annotations == nil {
		rar.Annotations = make(map[string]string)
	}
	rar.Annotations["kubernaut.ai/audit-emitted"] = "true"
	
	if err := r.client.Update(ctx, rar); err != nil {
		logger.Error(err, "Failed to mark RAR as audited", "rar", rar.Name)
		// Don't fail reconciliation - we'll retry next time
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
