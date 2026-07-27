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

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// ========================================
// AUDIT EVENT EMISSION (DD-AUDIT-003)
// ========================================

// emitRemediationCreatedAudit emits an audit event for RemediationRequest creation with TimeoutConfig.
// Per BR-AUDIT-005 Gap #8: Captures initial TimeoutConfig for RR reconstruction.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per ADR-034: orchestrator.lifecycle.created event
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitRemediationCreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "orchestrator.lifecycle.created")
		// Note: In production, this never happens due to main.go:128 crash check.
		// If we reach here, it's a programming error (e.g., test misconfiguration).
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Convert TimeoutConfig for audit event (Gap #8)
	// Direct pointer assignment - roaudit.TimeoutConfig uses same *metav1.Duration type
	var auditTimeoutConfig *roaudit.TimeoutConfig
	if rr.Status.TimeoutConfig != nil {
		auditTimeoutConfig = &roaudit.TimeoutConfig{
			Global:     rr.Status.TimeoutConfig.Global,
			Processing: rr.Status.TimeoutConfig.Processing,
			Analyzing:  rr.Status.TimeoutConfig.Analyzing,
			Executing:  rr.Status.TimeoutConfig.Executing,
		}
	}

	event, err := r.auditManager.BuildRemediationCreatedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
		auditTimeoutConfig,
	)
	if err != nil {
		logger.Error(err, "Failed to build remediation created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store remediation created audit event")
	}
}

// emitWorkflowCreatedAudit emits the remediation.workflow_created audit event
// before WorkflowExecution creation. Includes the pre-remediation spec hash
// and selected workflow metadata for the audit trail.
// ADR-EM-001 Section 9.1, GAP-RO-1, DD-EM-002.
// Non-blocking — failures are logged but don't affect business logic.
//
// The preHash parameter is the hash already captured by the caller (sites 1/2)
// via CapturePreRemediationHash. This avoids a redundant uncached API read of
// the same target resource that was just hashed moments before.
func (r *Reconciler) emitWorkflowCreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, preHash string) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record workflow_created audit event - violates ADR-032 §1",
			"remediationRequest", rr.Name)
		return
	}

	correlationID := rr.Name
	// DD-EM-003: Use remediation target for audit trail consistency
	remTarget := resolveDualTargets(rr, ai).Remediation
	targetResource := fmt.Sprintf("%s/%s/%s", remTarget.Namespace, remTarget.Kind, remTarget.Name)

	// Extract workflow metadata from AIAnalysis status
	var workflowID, workflowVersion, actionType string
	if ai.Status.SelectedWorkflow != nil {
		workflowID = ai.Status.SelectedWorkflow.WorkflowID
		workflowVersion = ai.Status.SelectedWorkflow.Version
		actionType = ai.Status.SelectedWorkflow.ActionType
	}

	event, err := r.auditManager.BuildRemediationWorkflowCreatedEvent(
		correlationID, rr.Namespace, rr.Name, rr.Spec.ClusterID,
		roaudit.RemediationWorkflowCreatedData{
			PreRemediationSpecHash: preHash,
			TargetResource:         targetResource,
			WorkflowID:             workflowID,
			WorkflowVersion:        workflowVersion,
			ActionType:             actionType,
			SignalType:             rr.Spec.SignalType,
			SignalFingerprint:      rr.Spec.SignalFingerprint,
		},
	)
	if err != nil {
		logger.Error(err, "Failed to build workflow_created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store workflow_created audit event")
	}
}

// emitEACreatedAudit emits the orchestrator.ea.created audit event with propagation
// delay breakdown. Issue #277: The RO is the source of truth for these delays.
// Non-blocking — failures are logged but don't affect business logic.
func (r *Reconciler) emitEACreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, eaName string, hashComputeDelay, alertCheckDelay *metav1.Duration, isGitOpsManaged, isCRD bool) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		return
	}

	data := roaudit.EACreatedData{
		EAName:          eaName,
		IsGitOpsManaged: isGitOpsManaged,
		IsCRD:           isCRD,
	}
	if hashComputeDelay != nil {
		data.HashComputeDelay = hashComputeDelay.Duration
	}
	if alertCheckDelay != nil {
		data.AlertCheckDelay = alertCheckDelay.Duration
	}
	auditAsyncCfg := r.getAsyncPropagation()
	if isGitOpsManaged {
		data.GitOpsSyncDelay = auditAsyncCfg.GitOpsSyncDelay
	}
	if isCRD {
		data.OperatorReconcileDelay = auditAsyncCfg.OperatorReconcileDelay
	}

	event, err := r.auditManager.BuildEACreatedEvent(rr.Name, rr.Namespace, rr.Name, rr.Spec.ClusterID, data)
	if err != nil {
		logger.Error(err, "Failed to build EA created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store EA created audit event")
	}
}

// emitLifecycleStartedAudit emits an audit event for remediation lifecycle started.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitLifecycleStartedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.started")
		// Note: In production, this never happens due to main.go:128 crash check.
		// If we reach here, it's a programming error (e.g., test misconfiguration).
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildLifecycleStartedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
	)
	if err != nil {
		logger.Error(err, "Failed to build lifecycle started audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store lifecycle started audit event")
	}
}

// emitVerifyingStartedAudit emits an audit event when RR enters the Verifying phase (#280).
// Non-blocking — failures are logged but don't affect business logic.
func (r *Reconciler) emitVerifyingStartedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	event, err := r.auditManager.BuildLifecycleVerifyingStartedEvent(rr.Name, rr.Namespace, rr.Name, rr.Spec.ClusterID)
	if err != nil {
		logger.Error(err, "Failed to build verifying_started audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verifying_started audit event")
	}
}

// emitVerificationCompletedAudit emits an audit event when Verifying -> Completed (#280).
func (r *Reconciler) emitVerificationCompletedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	eaName := ""
	if rr.Status.EffectivenessAssessmentRef != nil {
		eaName = rr.Status.EffectivenessAssessmentRef.Name
	}
	durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
	event, err := r.auditManager.BuildLifecycleVerificationCompletedEvent(
		rr.Name, rr.Namespace, rr.Name, rr.Spec.ClusterID, eaName, rr.Status.Outcome, durationMs)
	if err != nil {
		logger.Error(err, "Failed to build verification_completed audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verification_completed audit event")
	}
}

// emitVerificationTimedOutAudit emits an audit event when Verifying times out (#280).
func (r *Reconciler) emitVerificationTimedOutAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	eaName := ""
	if rr.Status.EffectivenessAssessmentRef != nil {
		eaName = rr.Status.EffectivenessAssessmentRef.Name
	}
	durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
	event, err := r.auditManager.BuildLifecycleVerificationTimedOutEvent(
		rr.Name, rr.Namespace, rr.Name, rr.Spec.ClusterID, eaName, durationMs)
	if err != nil {
		logger.Error(err, "Failed to build verification_timed_out audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verification_timed_out audit event")
	}
}

// emitPhaseTransitionAudit emits an audit event for phase transitions.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitPhaseTransitionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, fromPhase, toPhase string) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "phase.transitioned",
			"fromPhase", fromPhase,
			"toPhase", toPhase)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildPhaseTransitionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
		fromPhase,
		toPhase,
	)
	if err != nil {
		logger.Error(err, "Failed to build phase transition audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store phase transition audit event")
	}
}

// emitCompletionAudit emits an audit event for remediation completion.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
func (r *Reconciler) emitCompletionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string, durationMs int64) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.completed",
			"outcome", outcome,
			"durationMs", durationMs)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildCompletionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
		outcome,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build completion audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store completion audit event")
	}
}

// emitFailureAudit emits an audit event for remediation failure.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.failed (P1)
func (r *Reconciler) emitFailureAudit(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error, durationMs int64) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.failed",
			"failurePhase", failurePhase,
			"failureErr", failureErr,
			"durationMs", durationMs)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildFailureEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
		string(failurePhase),
		failureErr,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build failure audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store failure audit event")
	}
}

// emitRoutingBlockedAudit emits an audit event for routing blocked decisions.
// Per DD-RO-002: Centralized Routing Engine blocking conditions.
// Per ADR-032 §1: All phase transitions must be audited (Pending/Analyzing → Blocked).
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitRoutingBlockedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, fromPhase string, blocked *routing.BlockingCondition, workflowID string) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "routing.blocked",
			"blockReason", blocked.Reason)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Build routing blocked data
	requeueSeconds := int(blocked.RequeueAfter.Seconds())
	var blockedUntilStr *string
	if blocked.BlockedUntil != nil {
		str := blocked.BlockedUntil.Format(time.RFC3339)
		blockedUntilStr = &str
	}

	blockData := &roaudit.RoutingBlockedData{
		BlockReason:         blocked.Reason,
		BlockMessage:        blocked.Message,
		FromPhase:           fromPhase,
		ToPhase:             string(remediationv1.PhaseBlocked),
		WorkflowID:          workflowID,
		TargetResource:      rr.Spec.TargetResource.String(),
		RequeueAfterSeconds: requeueSeconds,
		BlockedUntil:        blockedUntilStr,
		BlockingWFE:         blocked.BlockingWorkflowExecution,
		DuplicateOf:         blocked.DuplicateOf,
		ConsecutiveFailures: rr.Status.ConsecutiveFailureCount,
	}

	// Calculate backoff seconds if NextAllowedExecution is set
	if rr.Status.NextAllowedExecution != nil {
		backoff := time.Until(rr.Status.NextAllowedExecution.Time)
		if backoff > 0 {
			blockData.BackoffSeconds = int(backoff.Seconds())
		}
	}

	event, err := r.auditManager.BuildRoutingBlockedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rr.Spec.ClusterID,
		fromPhase,
		blockData,
	)
	if err != nil {
		logger.Error(err, "Failed to build routing blocked audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store routing blocked audit event")
	}
}

// emitApprovalRequestedAudit emits an audit event for approval requested.
// Per DD-AUDIT-003: orchestrator.approval.requested (P2)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitApprovalRequestedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, confidence float64, workflowID string) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "approval.requested")
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Calculate RAR name using deterministic naming pattern
	rarName := fmt.Sprintf("rar-%s", rr.Name)

	// Calculate requiredBy timestamp (7 days from now per ADR-040)
	requiredBy := metav1.Now().Add(7 * 24 * time.Hour)

	// Build event using audit manager's build method (refactored per TODO comment)
	event, err := r.auditManager.BuildApprovalRequestedEvent(
		roaudit.ApprovalEventContext{
			CorrelationID: correlationID,
			Namespace:     rr.Namespace,
			RRName:        rr.Name,
			ClusterID:     rr.Spec.ClusterID,
			RARName:       rarName,
		},
		workflowID,
		fmt.Sprintf("%.2f", confidence),
		requiredBy,
	)
	if err != nil {
		logger.Error(err, "Failed to build approval requested audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store approval requested audit event")
	}
}

// emitTimeoutAudit emits an audit event for global or phase timeout.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed with outcome=failure (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitTimeoutAudit(ctx context.Context, rr *remediationv1.RemediationRequest, timeoutType, timeoutPhase string, durationMs int64) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.completed.timeout")
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Use audit helper to create event with proper timestamp (DD-AUDIT-002 V2.0)
	// Reuse lifecycle.completed event type with outcome=failure for timeouts
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, roaudit.EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, roaudit.CategoryOrchestration)
	audit.SetEventAction(event, roaudit.ActionCompleted)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", roaudit.ServiceName)
	audit.SetResource(event, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, rr.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if rr.Spec.ClusterID != "" {
		audit.SetClusterID(event, rr.Spec.ClusterID)
	}
	audit.SetDuration(event, int(durationMs))

	// Build payload using ogen types (timeout is represented as failure)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
		RrName:    rr.Name,
		Namespace: rr.Namespace,
	}
	payload.FailurePhase = roaudit.ToOptFailurePhase(timeoutPhase)
	payload.FailureReason = roaudit.ToOptFailureReason(fmt.Sprintf("%s timeout", timeoutType))
	payload.DurationMs.SetTo(durationMs)

	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload)

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store timeout audit event", "timeoutType", timeoutType)
	}
}

// emitRetentionCleanupAudit emits an audit event before deleting an expired RR (#265).
// Ensures the audit trail is complete before CRD removal — PostgreSQL is the long-term store.
// Non-blocking: failures are logged but do not prevent deletion.
func (r *Reconciler) emitRetentionCleanupAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		return
	}

	correlationID := rr.Name
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, roaudit.EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, roaudit.CategoryOrchestration)
	audit.SetEventAction(event, "retention_cleanup")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", roaudit.ServiceName)
	audit.SetResource(event, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, rr.Namespace)
	// DD-AUDIT-003 v2.2: Fleet cluster provenance (CC8.1)
	if rr.Spec.ClusterID != "" {
		audit.SetClusterID(event, rr.Spec.ClusterID)
	}

	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
		RrName:    rr.Name,
		Namespace: rr.Namespace,
	}
	if rr.Status.RetentionExpiryTime != nil {
		payload.DurationMs.SetTo(time.Since(rr.Status.RetentionExpiryTime.Time).Milliseconds())
	}

	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload)

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store retention cleanup audit event")
	}
}
