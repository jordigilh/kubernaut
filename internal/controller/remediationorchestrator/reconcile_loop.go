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

// The top-level Reconcile entrypoint, Pending-phase bootstrap, terminal-phase
// housekeeping (retention TTL, Ready safety net), and controller-runtime
// wiring (SetupWithManager + field indexes). Split out of reconciler.go per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file
// under the 700-line convention threshold. Pure structural move — no
// behavior change.
package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
)

// Reconcile implements the reconciliation loop for RemediationRequest.
// It handles phase transitions and delegates to appropriate handlers.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", req.NamespacedName)
	startTime := time.Now()

	// Fetch the RemediationRequest
	rr := &remediationv1.RemediationRequest{}
	if err := r.client.Get(ctx, req.NamespacedName, rr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("RemediationRequest not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch RemediationRequest")
		return ctrl.Result{}, err
	}

	// Record reconcile duration on exit (only if metrics are available)
	defer func() {
		r.Metrics.ReconcileDurationSeconds.WithLabelValues(
			rr.Namespace,
			string(rr.Status.OverallPhase),
		).Observe(time.Since(startTime).Seconds())
	}()

	// BR-AUDIT-005 Gap #7: Validate timeout configuration (fail fast on invalid config)
	// Per test requirement: Negative timeouts should be rejected with ERR_INVALID_TIMEOUT_CONFIG
	if err := r.validateTimeoutConfig(rr); err != nil {
		logger.Error(err, "Invalid timeout configuration, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, remediationv1.FailurePhaseConfiguration, err)
	}

	// OBSERVED GENERATION CHECK (DD-CONTROLLER-001 Pattern B - Phase-Aware).
	// See shouldSkipPendingReconcile for the full rationale.
	if r.shouldSkipPendingReconcile(rr, logger) {
		return ctrl.Result{}, nil
	}

	// Initialize phase if empty (new RemediationRequest from Gateway)
	// Per DD-GATEWAY-011: RO owns status.overallPhase, Gateway creates instances without status
	if rr.Status.OverallPhase == "" {
		return r.initializeNewRemediationRequest(ctx, rr, startTime, logger)
	}

	// Terminal-phase housekeeping (Issue #88, #265)
	if phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		return r.handleTerminalPhaseHousekeeping(ctx, rr, logger)
	}

	// Check for global timeout (BR-ORCH-027)
	// Supports per-RR override via spec.timeoutConfig.global (AC-027-4)
	// Business Value: Prevents stuck remediations from consuming resources indefinitely
	// Note: Uses status.StartTime (not CreationTimestamp) as StartTime is explicitly set by controller
	if rr.Status.StartTime != nil {
		globalTimeout := r.getEffectiveGlobalTimeout(rr)
		timeSinceStart := time.Since(rr.Status.StartTime.Time)
		if timeSinceStart > globalTimeout {
			logger.Info("RemediationRequest exceeded global timeout",
				"timeSinceStart", timeSinceStart,
				"globalTimeout", globalTimeout,
				"overridden", rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil,
				"startTime", rr.Status.StartTime.Time)
			return r.handleGlobalTimeout(ctx, rr)
		}
	}

	// Check for per-phase timeouts (BR-ORCH-028)
	// Enables faster detection of stuck phases without waiting for global timeout
	if err := r.checkPhaseTimeouts(ctx, rr); err != nil {
		return ctrl.Result{}, err
	}

	// Track notification status (BR-ORCH-029/030)
	// Updates RR status based on NotificationRequest phase changes
	if err := r.trackNotificationStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track notification status")
		// Non-fatal: Continue with reconciliation
	}

	// Issue #666: Check phase handler registry first
	currentPhase := phase.Phase(rr.Status.OverallPhase)
	if h, ok := r.phaseRegistry.Lookup(currentPhase); ok {
		intent, err := h.Handle(ctx, rr)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("phase handler %s: %w", currentPhase, err)
		}
		return r.ApplyTransition(ctx, rr, intent)
	}

	// All phase handlers are now registered in the registry. If we reach
	// here, the phase is genuinely unknown.
	logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
	return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil // REFACTOR-RO-003
}

// shouldSkipPendingReconcile implements the OBSERVED GENERATION CHECK
// (DD-CONTROLLER-001 Pattern B - Phase-Aware), extracted from Reconcile per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
// Per OBSERVED_GENERATION_DEEP_ANALYSIS_JAN_01_2026.md
//
// Phase-Aware Pattern: Parent Controllers with Active Orchestration
//   - Remove GenerationChangedPredicate (allow child status updates) ✅ Already done
//   - Add phase-aware ObservedGeneration check (balance idempotency with orchestration)
//
// The Challenge:
//   - Annotation changes: Generation unchanged, should skip (wasteful)
//   - Child status updates: Generation unchanged, MUST reconcile (critical!)
//   - Polling checks: Generation unchanged, MUST reconcile (critical!)
//     → Generation-based check CANNOT distinguish these events!
//
// The Solution: Phase-Aware Skip Logic
// Skip reconcile ONLY when we're NOT actively orchestrating:
//  1. Initial state (OverallPhase == "") → Allow (initialization)
//  2. Pending phase → Skip (not yet orchestrating, wasteful)
//  3. Processing/Analyzing/Executing → Allow (active orchestration of child CRDs)
//  4. Terminal phases (Completed/Failed) → Allow (owned-resource housekeeping: Issue #88)
//     Terminal RRs must still process NT delivery status and EA completion events.
//     handleTerminalPhaseHousekeeping handles terminal phases with dedicated housekeeping.
//
// Tradeoff: Accepts extra reconciles during active and terminal phases
// Benefit: Allows critical polling, child status updates, and terminal housekeeping
func (r *Reconciler) shouldSkipPendingReconcile(rr *remediationv1.RemediationRequest, logger logr.Logger) bool {
	if rr.Status.ObservedGeneration == rr.Generation &&
		rr.Status.OverallPhase == phase.Pending &&
		rr.Status.SignalProcessingRef != nil {
		logger.V(1).Info("⏭️  SKIPPED: No orchestration needed in Pending phase",
			"phase", rr.Status.OverallPhase,
			"generation", rr.Generation,
			"observedGeneration", rr.Status.ObservedGeneration,
			"reason", "ObservedGeneration matches, phase is Pending, and SP already created")
		return true
	}

	// Log when we proceed during active orchestration (helps understand behavior)
	if rr.Status.ObservedGeneration == rr.Generation &&
		rr.Status.OverallPhase != "" &&
		!phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		logger.V(1).Info("✅ PROCEEDING: Active orchestration phase",
			"phase", rr.Status.OverallPhase,
			"generation", rr.Generation,
			"reason", "Child orchestration requires reconciliation")
	}
	return false
}

// initializeNewRemediationRequest handles the first reconcile of a new
// RemediationRequest from Gateway (status.overallPhase == ""), extracted from
// Reconcile per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
// Per DD-GATEWAY-011: RO owns status.overallPhase, Gateway creates instances without status.
func (r *Reconciler) initializeNewRemediationRequest(ctx context.Context, rr *remediationv1.RemediationRequest, startTime time.Time, logger logr.Logger) (ctrl.Result, error) {
	// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) to confirm the
	// phase is genuinely empty. The informer cache is eventually consistent — a second
	// reconcile may start with stale cache showing OverallPhase=="" even after a
	// previous reconcile already set it to Pending in etcd. Without this check, both
	// reconciles enter the initialization block and emit duplicate audit events.
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
		logger.Error(err, "Failed to refetch RemediationRequest via apiReader")
		return ctrl.Result{}, err
	}
	if rr.Status.OverallPhase != "" {
		// Cache was stale — another reconcile already initialized this RR.
		// Requeue to proceed with the now-initialized phase.
		logger.V(1).Info("Skipped duplicate initialization (stale cache)",
			"name", rr.Name, "phase", rr.Status.OverallPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	logger.Info("Initializing new RemediationRequest", "name", rr.Name)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Initialize phase + StartTime + TimeoutConfig in single API call
	// DD-CONTROLLER-001: ObservedGeneration NOT set here - only after processing phase
	// Gap #8: Initialize TimeoutConfig defaults on first reconcile
	// REFACTOR: Use extracted helper method for timeout initialization
	// ========================================
	if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
		// TOCTOU guard: AtomicStatusUpdate refetches the RR internally.
		// Between the apiReader check above and this refetch, a concurrent
		// reconcile or status update may have already set the phase.
		if rr.Status.OverallPhase != "" {
			return errPhaseAlreadySet
		}
		rr.Status.OverallPhase = phase.Pending
		rr.Status.StartTime = &metav1.Time{Time: startTime}

		// Gap #8: Initialize TimeoutConfig with controller defaults
		// REFACTOR: Delegated to populateTimeoutDefaults() for validation + reusability
		r.populateTimeoutDefaults(ctx, rr)

		return nil
	}); err != nil {
		if errors.Is(err, errPhaseAlreadySet) {
			logger.V(1).Info("Initialization aborted (phase set during TOCTOU window)",
				"name", rr.Name, "phase", rr.Status.OverallPhase)
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		logger.Error(err, "Failed to initialize RemediationRequest status")
		return ctrl.Result{}, err
	}

	// RO-AUDIT-IDEMPOTENCY: Safe to emit — the apiReader refetch above confirmed this
	// reconcile is the sole initializer. AtomicStatusUpdate handles optimistic locking
	// conflicts via RetryOnConflict, but the phase transition is guaranteed to have
	// been performed by this reconcile (not a stale-cache duplicate).
	r.emitLifecycleStartedAudit(ctx, rr)

	// Gap #8: NO REFETCH NEEDED - AtomicStatusUpdate already updated rr in-memory
	// The rr object passed to AtomicStatusUpdate is updated with the persisted status
	// (including TimeoutConfig) by the Status().Update() call inside AtomicStatusUpdate.
	// Refetching here would risk getting a cached/stale version.

	// Gap #8: Emit orchestrator.lifecycle.created event with TimeoutConfig
	// Per BR-AUDIT-005 Gap #8: Capture initial TimeoutConfig for RR reconstruction
	// This happens AFTER status initialization to capture actual defaults
	r.emitRemediationCreatedAudit(ctx, rr)

	// DD-EVENT-001: Emit K8s event for new RR acceptance (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonRemediationCreated,
			fmt.Sprintf("RemediationRequest accepted for signal %s", rr.Spec.SignalName))
	}

	// Requeue after short delay to process the Pending phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// handleTerminalPhaseHousekeeping handles Completed/Failed RemediationRequests
// (Issue #88, #265), extracted from Reconcile per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
// Handles: TTL enforcement (stamp RetentionExpiryTime, delete expired CRDs),
// Ready safety net, notification tracking, effectiveness assessment tracking,
// and cascading terminal phase to non-terminal child CRDs.
func (r *Reconciler) handleTerminalPhaseHousekeeping(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Terminal-phase housekeeping", "phase", rr.Status.OverallPhase)

	deleted, retentionRequeue, err := r.enforceRetentionTTL(ctx, rr, logger)
	if err != nil {
		return ctrl.Result{}, err
	}
	if deleted {
		return ctrl.Result{}, nil
	}

	r.applyReadySafetyNet(ctx, rr, logger)

	// BR-ORCH-029/030: Track notification delivery status for terminal RRs
	if err := r.trackNotificationStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track notification status in terminal phase")
		// Non-fatal: continue to return
	}

	// ADR-EM-001, GAP-RO-2: Track effectiveness assessment status for terminal RRs
	if err := r.trackEffectivenessStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track effectiveness assessment status in terminal phase")
		// Non-fatal: continue to return
	}

	// #1421: Cascade terminal phase to non-terminal child CRDs.
	// Kubernetes-native parent-manages-children: RO is responsible for
	// transitioning children to a terminal state when the parent RR terminates.
	if err := r.cascadeTerminalToChildren(ctx, rr); err != nil {
		logger.Error(err, "Failed to cascade terminal phase to children")
		// Non-fatal: continue to return
	}

	// BR-ORCH-044: Track routing decision - no action needed
	r.Metrics.NoActionNeededTotal.WithLabelValues(string(rr.Status.OverallPhase), rr.Namespace).Inc()

	// #265: Requeue for TTL cleanup at expiry time
	return ctrl.Result{RequeueAfter: retentionRequeue}, nil
}

// enforceRetentionTTL implements Issue #265 TTL enforcement for terminal
// RemediationRequests: it deletes CRDs past their RetentionExpiryTime, stamps
// expiry on the first terminal reconcile, and reports the required requeue
// interval for CRDs still within their retention window. Extracted from
// handleTerminalPhaseHousekeeping per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
// Returns (deleted, requeueAfter, error). When deleted is true, the caller
// must stop housekeeping immediately (the RR no longer exists).
func (r *Reconciler) enforceRetentionTTL(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (bool, time.Duration, error) {
	// Expired CRDs are deleted immediately (before housekeeping).
	// Non-expired terminal RRs proceed through housekeeping, then requeue at expiry.
	if rr.Status.RetentionExpiryTime != nil && time.Now().After(rr.Status.RetentionExpiryTime.Time) {
		r.emitRetentionCleanupAudit(ctx, rr)
		if err := r.client.Delete(ctx, rr); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete expired RemediationRequest")
				return false, 0, err
			}
		}
		logger.Info("Deleted expired RemediationRequest (#265)",
			"retentionExpiryTime", rr.Status.RetentionExpiryTime.Format(time.RFC3339))
		return true, 0, nil
	}

	// Stamp expiry on first terminal reconcile (non-blocking — housekeeping continues)
	retention := r.getRetentionPeriod()
	if rr.Status.RetentionExpiryTime == nil {
		expiry := metav1.NewTime(time.Now().Add(retention))
		if err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.RetentionExpiryTime = &expiry
			return nil
		}); err != nil {
			logger.Error(err, "Failed to set RetentionExpiryTime")
		} else {
			logger.Info("RetentionExpiryTime set (#265)", "expiry", expiry.Format(time.RFC3339))
		}
		return false, retention, nil
	}

	return false, time.Until(rr.Status.RetentionExpiryTime.Time), nil
}

// applyReadySafetyNet implements Issue #79 Phase 7c: for terminal RRs that
// never got a Ready condition set (e.g., externally cancelled), stamp one
// based on the terminal phase outcome. Extracted from
// handleTerminalPhaseHousekeeping per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func (r *Reconciler) applyReadySafetyNet(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) {
	readyCondition := remediationrequest.GetCondition(rr, remediationrequest.ConditionReady)
	if readyCondition != nil {
		return
	}

	isSuccess := rr.Status.OverallPhase == remediationv1.PhaseCompleted || rr.Status.OverallPhase == remediationv1.PhaseSkipped
	ready, reason := false, remediationrequest.ReasonNotReady
	if isSuccess {
		ready, reason = true, remediationrequest.ReasonReady
	}
	if updateErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		remediationrequest.SetReady(rr, ready, reason, "Terminal phase: "+string(rr.Status.OverallPhase), r.Metrics)
		return nil
	}); updateErr != nil {
		logger.Error(updateErr, "Failed to set Ready safety net")
	}
}

// SetupWithManager sets up the controller with the Manager.
// Creates field index on spec.signalFingerprint for O(1) consecutive failure lookups.
// Reference: BR-ORCH-042, BR-GATEWAY-185 v1.1
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// BR-ORCH-042, BR-GATEWAY-185 v1.1: signal fingerprint index for O(1) consecutive-failure lookups.
	if err := registerFingerprintIndex(mgr); err != nil {
		return err
	}

	// DD-RO-002, V1.0: WorkflowExecution target-resource index for centralized routing queries.
	if err := registerWFETargetResourceIndex(mgr); err != nil {
		return err
	}

	// Issue #91: child-CRD remediationRequestRef.name indexes for parent lookups.
	if err := registerChildCRDIndexes(mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Owns(&signalprocessingv1.SignalProcessing{}).
		Owns(&aianalysisv1.AIAnalysis{}).
		Owns(&workflowexecutionv1.WorkflowExecution{}).
		Owns(&remediationv1.RemediationApprovalRequest{}).
		Owns(&notificationv1.NotificationRequest{}). // BR-ORCH-029/030: Watch notification lifecycle
		Owns(&eav1.EffectivenessAssessment{}).       // ADR-EM-001, GAP-RO-3: Watch EA for EffectivenessAssessed condition
		// V1.0 P1 FIX: GenerationChangedPredicate removed to allow child CRD status changes
		// Previous optimization filtered status updates, breaking integration tests (RO_INTEGRATION_CRITICAL_BUG_JAN_01_2026.md)
		// Rationale: Correctness > Performance for P0 orchestration service
		// WithEventFilter(predicate.GenerationChangedPredicate{}). // ❌ REMOVED - breaks integration tests
		Complete(r)
}

// registerFingerprintIndex creates the field index on spec.signalFingerprint
// (BR-ORCH-042, BR-GATEWAY-185 v1.1), used for O(1) consecutive-failure
// lookups. Uses the immutable spec field (64 chars) instead of mutable labels
// (63 chars max). Extracted from SetupWithManager per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func registerFingerprintIndex(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&remediationv1.RemediationRequest{},
		FingerprintFieldIndex, // "spec.signalFingerprint"
		func(obj client.Object) []string {
			rr := obj.(*remediationv1.RemediationRequest)
			if rr.Spec.SignalFingerprint == "" {
				return nil
			}
			return []string{rr.Spec.SignalFingerprint}
		},
	); err != nil {
		return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
	}
	return nil
}

// registerWFETargetResourceIndex creates the field index on
// WorkflowExecution.Spec.TargetResource for O(1) routing lookups (DD-RO-002,
// V1.0). Used by RO routing logic to find recent/active WFEs for the same
// target (e.g. "Find all WFEs targeting deployment/my-app"). Pattern mirrors
// the WE controller's own index registration.
//
// NOTE: This index may already exist if the WE controller was set up first;
// an "indexer conflict" error is therefore safely ignored since both
// controllers need the same index for the same purpose. Extracted from
// SetupWithManager per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func registerWFETargetResourceIndex(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&workflowexecutionv1.WorkflowExecution{},
		"spec.targetResource", // Field to index
		func(obj client.Object) []string {
			wfe := obj.(*workflowexecutionv1.WorkflowExecution)
			if wfe.Spec.TargetResource == "" {
				return nil
			}
			return []string{wfe.Spec.TargetResource}
		},
	); err != nil {
		// Ignore "indexer conflict" error - means WE controller already created this index.
		if !k8serrors.IsIndexerConflict(err) {
			return fmt.Errorf("failed to create field index on WorkflowExecution.spec.targetResource: %w", err)
		}
	}
	return nil
}

// registerChildCRDIndexes registers field indexes on all child CRDs for
// spec.remediationRequestRef.name (Issue #91), enabling MatchingFields
// queries and `kubectl --field-selector` for child lookups by parent RR.
// Extracted from SetupWithManager per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func registerChildCRDIndexes(mgr ctrl.Manager) error {
	childCRDIndexes := []struct {
		obj       client.Object
		extractor func(client.Object) []string
	}{
		{
			obj: &aianalysisv1.AIAnalysis{},
			extractor: func(obj client.Object) []string {
				aa := obj.(*aianalysisv1.AIAnalysis)
				if aa.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{aa.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &notificationv1.NotificationRequest{},
			extractor: func(obj client.Object) []string {
				nr := obj.(*notificationv1.NotificationRequest)
				if nr.Spec.RemediationRequestRef == nil || nr.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{nr.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &signalprocessingv1.SignalProcessing{},
			extractor: func(obj client.Object) []string {
				sp := obj.(*signalprocessingv1.SignalProcessing)
				if sp.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{sp.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &remediationv1.RemediationApprovalRequest{},
			extractor: func(obj client.Object) []string {
				rar := obj.(*remediationv1.RemediationApprovalRequest)
				if rar.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{rar.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &workflowexecutionv1.WorkflowExecution{},
			extractor: func(obj client.Object) []string {
				wfe := obj.(*workflowexecutionv1.WorkflowExecution)
				if wfe.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{wfe.Spec.RemediationRequestRef.Name}
			},
		},
	}

	for _, idx := range childCRDIndexes {
		if err := mgr.GetFieldIndexer().IndexField(
			context.Background(),
			idx.obj,
			RemediationRequestRefNameIndex,
			idx.extractor,
		); err != nil {
			if !k8serrors.IsIndexerConflict(err) {
				return fmt.Errorf("failed to create field index %s on %T: %w", RemediationRequestRefNameIndex, idx.obj, err)
			}
		}
	}
	return nil
}
