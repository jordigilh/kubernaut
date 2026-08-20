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

package aianalysis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aaconditions "github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// ========================================
// Phase Handlers
// Pattern: P2 - Controller Decomposition
// ========================================
//
// These handlers implement phase-specific reconciliation logic.
// Each handler is responsible for one phase of the AIAnalysis lifecycle.
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md

// reconcilePending handles AIAnalysis in Pending phase.
//
// Business Requirements:
//   - BR-AI-010: Initialize analysis and transition to Investigating
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Pending", "name", analysis.Name)
	log.Info("Processing Pending phase")

	// Set StartedAt timestamp (per crd-schema.md)
	now := metav1.Now()
	analysis.Status.StartedAt = &now

	// Capture phase BEFORE transition for audit
	phaseBefore := analysis.Status.Phase

	// Transition to Investigating phase (first processing phase per CRD schema)
	// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Investigating handler after processing
	analysis.Status.Phase = PhaseInvestigating
	analysis.Status.Message = "AIAnalysis created, starting investigation"

	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to update status to Investigating")
		return ctrl.Result{}, err
	}

	// DD-AUDIT-003: Record phase transition AFTER status update (ensures audit reflects committed state)
	// IDEMPOTENCY: Only record if phase actually changed (prevents duplicate events in race conditions)
	// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
	if phaseBefore != PhaseInvestigating {
		r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, PhaseInvestigating)
	}

	r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonAIAnalysisCreated, "AIAnalysis processing started")

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for intermediate transitions
	r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
		fmt.Sprintf("Phase transition: %s → %s", phaseBefore, PhaseInvestigating))

	// Requeue after short delay to process Investigating phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// reconcileInvestigating handles AIAnalysis in Investigating phase.
//
// Business Requirements:
//   - BR-AI-023: KA integration
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileInvestigating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Investigating", "name", analysis.Name)
	log.Info("Processing Investigating phase")

	invHandler := r.InvestigatingHandler.Load()
	if invHandler == nil {
		// Issue #1116: Fail loudly — a nil handler means the controller was
		// misconfigured. SetupWithManager should have caught this, but defense
		// in depth prevents silent data loss.
		log.Error(nil, "InvestigatingHandler is nil — investigation phase cannot execute (BR-AI-023)")
		return ctrl.Result{}, fmt.Errorf("investigatingHandler is nil: cannot execute investigating phase")
	}

	// AA-BUG-007: Use optimistic locking with idempotency check
	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE with AA-BUG-007 fix
	// ========================================
	// Status writes (PollCount, LastPolled) are allowed here. The informer
	// predicate (aiAnalysisUpdatePredicate) filters out poll-tracking-only
	// updates, so they don't trigger re-reconciles. Only phase changes and
	// session creation pass the predicate filter.
	outcome := &investigatingUpdateOutcome{}
	if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
		return r.runInvestigatingHandler(ctx, analysis, invHandler, outcome, log)
	}); err != nil {
		return r.handleInvestigatingUpdateError(ctx, err, analysis, log)
	}
	r.clearSchemaRejectionRetryAnnotation(ctx, analysis, log)

	return r.finalizeInvestigatingTransition(ctx, analysis, outcome, log)
}

// investigatingUpdateOutcome captures the results of executing the
// InvestigatingHandler inside the AtomicStatusUpdate closure, so the caller
// can decide on requeue/audit/event behavior after the status write commits.
type investigatingUpdateOutcome struct {
	phaseBefore     string
	result          ctrl.Result
	handlerErr      error
	handlerExecuted bool
	// investigationTimeMs is non-nil only when THIS execution of the
	// AtomicStatusUpdate closure is the one that first computed
	// InvestigationMetadata.InvestigationTime (detected as a 0 -> >0
	// transition -- see runInvestigatingHandler). #2204 (2026-08-20): the
	// RecordAIAgentResult audit call keys off this field in
	// finalizeInvestigatingTransition, which runs AFTER AtomicStatusUpdate
	// durably commits, so a resourceVersion-Conflict retry of the closure
	// (which recomputes InvestigationTime from a fresh 0 baseline every
	// time, since the prior failed attempt's write never landed) cannot
	// double-record the event -- only the run whose outcome the caller
	// actually observes gets audited, exactly once.
	investigationTimeMs *int64
}

// runInvestigatingHandler is the AtomicStatusUpdate closure body for
// reconcileInvestigating: AA-BUG-009 idempotency checks (phase already
// changed / handler already executed, with AA-H4 recovery-poll exception for
// an active KA session), then executing the handler and setting the Ready
// condition on failure (Issue #79 Phase 7b). Extracted from
// reconcileInvestigating (Wave 6 6c GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (r *AIAnalysisReconciler) runInvestigatingHandler(ctx context.Context, analysis *aianalysisv1.AIAnalysis, invHandler *handlers.InvestigatingHandler, outcome *investigatingUpdateOutcome, log logr.Logger) error {
	// Capture phase after ATOMIC refetch
	outcome.phaseBefore = analysis.Status.Phase

	// AA-BUG-009: Enhanced idempotency - skip handler if phase already changed OR already executed
	if outcome.phaseBefore != PhaseInvestigating {
		log.Info("AA-KA-001: Phase already changed, skipping handler",
			"expected", PhaseInvestigating, "actual", outcome.phaseBefore,
			"observedGeneration", analysis.Status.ObservedGeneration)
		outcome.handlerExecuted = false
		return nil
	}

	// #2204: baseline read BEFORE the handler runs, so that after Handle()
	// returns we can detect whether THIS execution is the one that first
	// transitioned InvestigationTime from 0 -- see investigatingUpdateOutcome
	// .investigationTimeMs's doc comment. Read once here (not inside the
	// guard below) because the "AA-H4 recovery poll" branch can also reach
	// Handle() despite InvestigationTime already being > 0.
	investigationTimeBefore := analysis.Status.GetInvestigationMetadata().InvestigationTime

	if investigationTimeBefore > 0 {
		hasActiveSession := analysis.Status.KASession != nil && analysis.Status.KASession.ID != ""
		if !hasActiveSession {
			log.Info("AA-KA-001: Handler already executed, skipping duplicate call",
				"investigationTime", analysis.Status.InvestigationMetadata.InvestigationTime,
				"phase", outcome.phaseBefore)
			outcome.handlerExecuted = false
			return nil
		}
		log.Info("AA-H4: Active KA session detected, allowing recovery poll despite InvestigationTime > 0",
			"sessionID", analysis.Status.KASession.ID,
			"investigationTime", analysis.Status.InvestigationMetadata.InvestigationTime)
	}

	// #2080 recurrence: absorb any early wake-up (self-watch on
	// KASession.ID, the IS-watch on KACorrelationID, or a stray requeue)
	// that arrives before handleSessionLost's own RequeueAfter backoff has
	// actually elapsed. Checked here -- the single production entry point
	// into InvestigatingHandler -- so it applies regardless of what
	// triggered this reconcile. No status field is mutated on this path, so
	// the resulting (no-op) Status().Update() cannot itself re-trigger the
	// self-watch predicate.
	if ks := analysis.Status.KASession; ks != nil && ks.BackoffUntil != nil {
		if remaining := time.Until(ks.BackoffUntil.Time); remaining > 0 {
			log.V(1).Info("Session regeneration backoff still active, skipping handler (#2080 recurrence)",
				"remaining", remaining, "generation", ks.Generation)
			outcome.handlerExecuted = false
			outcome.result = ctrl.Result{RequeueAfter: remaining}
			return nil
		}
	}

	// Execute handler ONLY if phase check passed AND not already executed
	outcome.result, outcome.handlerErr = invHandler.Handle(ctx, analysis)
	outcome.handlerExecuted = true
	if outcome.handlerErr != nil {
		return outcome.handlerErr
	}

	// #2204: this run just transitioned InvestigationTime 0 -> >0, i.e. it is
	// the run whose handleSessionCompleted observed KA's Completed write.
	// Capture the value so finalizeInvestigatingTransition can record the
	// RecordAIAgentResult audit event exactly once, after this closure's
	// Status().Update() actually commits (DD-WE-009 convention) -- see
	// investigatingUpdateOutcome.investigationTimeMs's doc comment for why a
	// Conflict-retry of this same closure cannot cause a duplicate.
	if investigationTimeBefore == 0 {
		if t := analysis.Status.GetInvestigationMetadata().InvestigationTime; t > 0 {
			outcome.investigationTimeMs = &t
		}
	}

	// Issue #79 Phase 7b: Set Ready condition on terminal transitions
	if analysis.Status.Phase == PhaseFailed {
		aaconditions.SetReady(analysis, false, aaconditions.ReasonNotReady, "Analysis failed: "+analysis.Status.Message)
	}

	return nil
}

// handleInvestigatingUpdateError classifies the AtomicStatusUpdate error:
// CRD-schema-rejected updates retry with backoff instead of failing closed
// (#2030 Part A), everything else is a standard reconcile error. Extracted
// from reconcileInvestigating (Wave 6 6c GREEN: funlen remediation) — pure
// code motion, no behavior change.
func (r *AIAnalysisReconciler) handleInvestigatingUpdateError(ctx context.Context, err error, analysis *aianalysisv1.AIAnalysis, log logr.Logger) (ctrl.Result, error) {
	if apierrors.IsInvalid(err) {
		// #2030 Part A: retry-then-escalate instead of fail-closing forever
		// on the first CRD schema rejection.
		return r.handleSchemaRejectedStatusUpdate(ctx, analysis, log, err)
	}
	log.Error(err, "Failed to atomically update status after Investigating phase")
	return ctrl.Result{}, err
}

// handleSchemaRejectedStatusUpdate handles a Status().Update() rejection
// caused by CRD schema drift (apierrors.IsInvalid) -- e.g. the live
// cluster's installed CRD lagging behind a new enum value the Go source
// already defines (#2030 Part A). Previously both call sites fail-closed
// forever on the first rejection (return ctrl.Result{}, nil, no requeue),
// silently abandoning the AIAnalysis with the controller never touching it
// again.
//
// Retries a bounded number of times with backoff. The retry counter is
// persisted via a plain Update() (metadata annotation only), not
// Status().Update(): a CRD with the status subresource enabled silently
// drops any .status diff on a non-status Update, so that write still
// succeeds even though Status().Update() is being rejected. If the cap is
// exceeded, escalates to a terminal Failed phase using valid enum values
// (Reason=APIError/SubReason=TransientError) instead of retrying forever.
func (r *AIAnalysisReconciler) handleSchemaRejectedStatusUpdate(ctx context.Context, analysis *aianalysisv1.AIAnalysis, log logr.Logger, rejectionErr error) (ctrl.Result, error) {
	count := 0
	if v := analysis.Annotations[handlers.SchemaRejectionRetryCountAnnotation]; v != "" {
		if parsed, parseErr := strconv.Atoi(v); parseErr == nil {
			count = parsed
		}
	}
	count++

	if count <= handlers.MaxSchemaRejectionRetries {
		if analysis.Annotations == nil {
			analysis.Annotations = map[string]string{}
		}
		analysis.Annotations[handlers.SchemaRejectionRetryCountAnnotation] = strconv.Itoa(count)

		if updErr := r.Update(ctx, analysis); updErr != nil {
			log.Error(updErr, "Failed to persist schema-rejection retry count annotation; will retry same attempt next reconcile",
				"name", analysis.Name)
		}

		delay := backoff.CalculateWithDefaults(int32(count))
		log.Error(rejectionErr, "CRD schema rejected status update — retrying with backoff",
			"name", analysis.Name, "attempt", count, "maxAttempts", handlers.MaxSchemaRejectionRetries, "backoff", delay)
		r.Recorder.Event(analysis, corev1.EventTypeWarning, "SchemaValidationFailed",
			fmt.Sprintf("Status update rejected by CRD schema (attempt %d/%d), retrying in %s: %v",
				count, handlers.MaxSchemaRejectionRetries, delay, rejectionErr))
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	log.Error(rejectionErr, "CRD schema rejected status update — retries exhausted, escalating to Failed",
		"name", analysis.Name, "attempts", count-1)
	r.Recorder.Event(analysis, corev1.EventTypeWarning, "SchemaValidationFailed",
		fmt.Sprintf("Status update permanently rejected by CRD schema after %d attempts, escalating to Failed: %v",
			count-1, rejectionErr))

	// Refetch fresh rather than reusing the in-memory analysis: the failed
	// handler run may have left other in-memory-only status fields in a
	// state we don't want to carry into the escalation write.
	fresh := &aianalysisv1.AIAnalysis{}
	if getErr := r.Get(ctx, client.ObjectKeyFromObject(analysis), fresh); getErr != nil {
		log.Error(getErr, "Failed to refetch AIAnalysis for schema-rejection escalation", "name", analysis.Name)
		return ctrl.Result{}, nil
	}
	now := metav1.Now()
	fresh.Status.Phase = PhaseFailed
	fresh.Status.Reason = aianalysisv1.ReasonAPIError
	fresh.Status.SubReason = "TransientError"
	fresh.Status.Message = fmt.Sprintf("CRD schema permanently rejected status update after %d attempts: %v", count-1, rejectionErr)
	fresh.Status.CompletedAt = &now
	fresh.Status.ObservedGeneration = fresh.Generation

	if updErr := r.Status().Update(ctx, fresh); updErr != nil {
		log.Error(updErr, "Failed to escalate to Failed phase after schema-rejection retries exhausted", "name", analysis.Name)
		return ctrl.Result{}, nil
	}
	*analysis = *fresh
	return ctrl.Result{}, nil
}

// clearSchemaRejectionRetryAnnotation removes the #2030 Part A retry-count
// annotation once a Status().Update() succeeds again after one or more
// prior CRD-schema rejections. Without this, a leftover non-zero count from
// an earlier, resolved schema-lag episode would make handleSchemaRejectedStatusUpdate
// start counting from that stale value on some later, unrelated rejection
// episode, tripping handlers.MaxSchemaRejectionRetries prematurely.
//
// Called only from the success path (after AtomicStatusUpdate returns nil),
// so analysis already reflects the just-committed state. Best-effort: a
// failure here is logged but does not affect the reconcile result, since the
// stale annotation is merely a minor inefficiency, not a correctness hazard,
// until another unrelated rejection episode occurs.
func (r *AIAnalysisReconciler) clearSchemaRejectionRetryAnnotation(ctx context.Context, analysis *aianalysisv1.AIAnalysis, log logr.Logger) {
	if _, present := analysis.Annotations[handlers.SchemaRejectionRetryCountAnnotation]; !present {
		return
	}
	delete(analysis.Annotations, handlers.SchemaRejectionRetryCountAnnotation)
	if err := r.Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to clear stale schema-rejection retry-count annotation (best-effort)",
			"name", analysis.Name)
	}
}

// finalizeInvestigatingTransition requeues and emits the phase-transition
// audit/event pair when the handler actually executed and changed phase;
// otherwise it returns the handler's own result unchanged. Extracted from
// reconcileInvestigating (Wave 6 6c GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (r *AIAnalysisReconciler) finalizeInvestigatingTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, outcome *investigatingUpdateOutcome, log logr.Logger) (ctrl.Result, error) {
	// DD-AUDIT-003 / DD-WE-009 ("audit outside the retryable closure"), #2204
	// fix: record result-retrieval audit exactly once, now that
	// AtomicStatusUpdate has durably committed -- see
	// investigatingUpdateOutcome.investigationTimeMs's doc comment.
	// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
	if outcome.investigationTimeMs != nil {
		r.AuditClient.RecordAIAgentResult(ctx, analysis, *outcome.investigationTimeMs)
	}

	if outcome.handlerExecuted && analysis.Status.Phase != outcome.phaseBefore {
		log.Info("Phase changed, requeuing", "from", outcome.phaseBefore, "to", analysis.Status.Phase)

		// DD-AUDIT-003: Record phase transition AFTER status committed (AA-BUG-001 fix)
		// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
		r.AuditClient.RecordPhaseTransition(ctx, analysis, outcome.phaseBefore, analysis.Status.Phase)

		// DD-EVENT-001 v1.1: Emit K8s events based on the new phase
		r.emitInvestigatingPhaseEvents(analysis, outcome.phaseBefore)

		// Requeue quickly after phase transition
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}
	return outcome.result, nil
}

// reconcileAnalyzing handles AIAnalysis in Analyzing phase.
//
// Business Requirements:
//   - BR-AI-030: Rego policy evaluation
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Analyzing", "name", analysis.Name)
	log.Info("Processing Analyzing phase")

	// Use handler if wired in, otherwise stub for backward compatibility
	if r.AnalyzingHandler != nil {
		// AA-BUG-007: Use optimistic locking with idempotency check
		// The key insight: Move handler execution BEFORE AtomicStatusUpdate's refetch
		// so we can check the phase ONCE and decide whether to proceed

		var phaseBefore string
		var result ctrl.Result
		var handlerErr error
		var handlerExecuted bool

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE with AA-BUG-007 fix
		// Handler runs INSIDE updateFunc, but ONLY ONCE per phase value
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
			// Capture phase after ATOMIC refetch (part of AtomicStatusUpdate)
			phaseBefore = analysis.Status.Phase

			// AA-BUG-007: Idempotency - skip handler if phase already changed
			// This check happens AFTER each refetch, preventing duplicate execution
			if phaseBefore != PhaseAnalyzing {
				log.V(1).Info("Phase already changed, skipping handler",
					"expected", PhaseAnalyzing, "actual", phaseBefore)
				handlerExecuted = false
				return nil // No-op, phase already processed
			}

			// AA-BUG-007: Execute handler ONLY if phase check passed
			// Handler modifies analysis.Status in memory
			result, handlerErr = r.AnalyzingHandler.Handle(ctx, analysis)
			handlerExecuted = true
			if handlerErr != nil {
				return handlerErr
			}

			// Issue #79 Phase 7b: Set Ready condition on terminal transitions
			switch analysis.Status.Phase {
			case PhaseCompleted:
				aaconditions.SetReady(analysis, true, aaconditions.ReasonReady, "Analysis completed")
			case PhaseFailed:
				aaconditions.SetReady(analysis, false, aaconditions.ReasonNotReady, "Analysis failed: "+analysis.Status.Message)
			}

			return nil
		}); err != nil {
			if apierrors.IsInvalid(err) {
				// #2030 Part A: retry-then-escalate instead of fail-closing
				// forever on the first CRD schema rejection.
				return r.handleSchemaRejectedStatusUpdate(ctx, analysis, log, err)
			}
			log.Error(err, "Failed to atomically update status after Analyzing phase")
			return ctrl.Result{}, err
		}
		r.clearSchemaRejectionRetryAnnotation(ctx, analysis, log)

		// Only requeue if handler actually executed and changed phase
		if handlerExecuted && analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing", "from", phaseBefore, "to", analysis.Status.Phase)

			// DD-AUDIT-003: Record phase transition AFTER status committed (AA-BUG-001 fix)
			// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
			r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, analysis.Status.Phase)

			// DD-EVENT-001 v1.1: Emit K8s events based on the new phase
			r.emitAnalyzingPhaseEvents(analysis, phaseBefore)

			// Requeue quickly after phase transition
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		return result, nil
	}

	// Issue #1116: Fail loudly — a nil handler means the controller was
	// misconfigured. SetupWithManager should have caught this, but defense
	// in depth prevents silent data loss.
	log.Error(nil, "AnalyzingHandler is nil — Rego evaluation cannot execute (BR-AI-012, BR-AI-030)")
	return ctrl.Result{}, fmt.Errorf("analyzingHandler is nil: cannot execute analyzing phase")
}

// ========================================
// DD-EVENT-001 v1.1: K8s Event Emission Helpers
// BR-AA-095: K8s Event Observability for AIAnalysis Controller
// ========================================

// emitInvestigatingPhaseEvents emits K8s events after the Investigating handler runs.
// Called only when the phase has actually changed (handlerExecuted && phase != phaseBefore).
func (r *AIAnalysisReconciler) emitInvestigatingPhaseEvents(analysis *aianalysisv1.AIAnalysis, phaseBefore string) {
	newPhase := analysis.Status.Phase

	// P1: Terminal state events
	switch newPhase {
	case PhaseAnalyzing:
		// Successful investigation → transitioning to Analyzing
		r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonInvestigationComplete,
			"Investigation completed successfully, transitioning to Analyzing")
	case PhaseFailed:
		// Investigation failure → terminal state
		r.Recorder.Event(analysis, corev1.EventTypeWarning, events.EventReasonAnalysisFailed,
			fmt.Sprintf("Analysis failed during investigation: %s", analysis.Status.Message))
	}

	// P2: Decision point events
	if analysis.Status.GetReview().NeedsHumanReview {
		reason := analysis.Status.Review.HumanReviewReason
		if reason == "" {
			reason = "unspecified"
		}
		r.Recorder.Event(analysis, corev1.EventTypeWarning, events.EventReasonHumanReviewRequired,
			fmt.Sprintf("Human review required: %s", reason))
	}

	// P3: PhaseTransition breadcrumb for all transitions
	r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
		fmt.Sprintf("Phase transition: %s → %s", phaseBefore, newPhase))
}

// emitAnalyzingPhaseEvents emits K8s events after the Analyzing handler runs.
// Called only when the phase has actually changed (handlerExecuted && phase != phaseBefore).
func (r *AIAnalysisReconciler) emitAnalyzingPhaseEvents(analysis *aianalysisv1.AIAnalysis, phaseBefore string) {
	newPhase := analysis.Status.Phase

	// P1: Terminal state events
	switch newPhase {
	case PhaseCompleted:
		// Successful analysis → terminal state
		r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonAnalysisCompleted,
			"Analysis completed successfully")
	case PhaseFailed:
		// Analysis failure → terminal state
		r.Recorder.Event(analysis, corev1.EventTypeWarning, events.EventReasonAnalysisFailed,
			fmt.Sprintf("Analysis failed: %s", analysis.Status.Message))
	}

	// P2: Decision point events
	if analysis.Status.GetApproval().ApprovalRequired {
		reason := analysis.Status.Approval.ApprovalReason
		if reason == "" {
			reason = "policy evaluation"
		}
		r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonApprovalRequired,
			fmt.Sprintf("Human approval required: %s", reason))
	}

	// P3: PhaseTransition breadcrumb for all transitions
	r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
		fmt.Sprintf("Phase transition: %s → %s", phaseBefore, newPhase))
}
