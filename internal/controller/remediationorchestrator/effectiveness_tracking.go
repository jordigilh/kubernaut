/*
Copyright 2026 Jordi Gil.

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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// ========================================
// EFFECTIVENESS ASSESSMENT STATUS TRACKING (ADR-EM-001, GAP-RO-2)
// ========================================

// ConditionEffectivenessAssessed is the condition type set on RR when the EA completes.
const ConditionEffectivenessAssessed = "EffectivenessAssessed"

// trackEffectivenessStatus updates RemediationRequest conditions based on EffectivenessAssessment phase.
// Called during terminal-phase housekeeping to keep EA status in sync with the parent RR.
//
// When the EA reaches a terminal phase (Completed or Failed), this method sets the
// EffectivenessAssessed condition on the RR:
//   - Completed: Status=True, Reason=AssessmentCompleted, Message includes assessment reason
//   - Failed: Status=False, Reason=AssessmentFailed, Message includes failure details
//
// Business Requirements:
//   - ADR-EM-001: RO must track EA lifecycle
//   - GAP-RO-2: Set EffectivenessAssessed condition on RR
//
// #280: When the RR is in Verifying phase and EA reaches terminal, this method also
// transitions the RR from Verifying to Completed with Outcome="Remediated" and sets CompletedAt.
func (r *Reconciler) trackEffectivenessStatus(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	if rr == nil {
		return fmt.Errorf("RemediationRequest cannot be nil")
	}

	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	ref, ok := effectivenessTrackingRef(rr, logger)
	if !ok {
		return nil
	}

	ea := r.fetchEffectivenessAssessmentForTracking(ctx, ref, logger)
	if ea == nil {
		return nil
	}

	return r.applyEffectivenessAssessmentOutcome(ctx, rr, ea, logger)
}

// effectivenessTrackingRef resolves the EA ref to track and reports whether
// tracking should proceed: false is returned when there is no ref, the ref
// is malformed, or a terminal EffectivenessAssessed condition is already set
// (idempotency — a terminal reason, once set, is never overwritten, but the
// initial "AssessmentInProgress" reason (GAP-2) may still transition).
// Extracted from trackEffectivenessStatus (Wave 6 6e-i GREEN: funlen
// remediation) — pure code motion, no behavior change.
func effectivenessTrackingRef(rr *remediationv1.RemediationRequest, logger logr.Logger) (*corev1.ObjectReference, bool) {
	ref := rr.Status.EffectivenessAssessmentRef
	if ref == nil {
		logger.V(1).Info("No EffectivenessAssessmentRef to track")
		return nil, false
	}

	if ref.Name == "" || ref.Namespace == "" {
		logger.Info("Invalid EffectivenessAssessmentRef, skipping tracking",
			"refName", ref.Name,
			"refNamespace", ref.Namespace,
		)
		return nil, false
	}

	existingCondition := meta.FindStatusCondition(rr.Status.Conditions, ConditionEffectivenessAssessed)
	if existingCondition != nil && existingCondition.Reason != "AssessmentInProgress" {
		logger.V(1).Info("EffectivenessAssessed condition already set with terminal reason, skipping",
			"status", existingCondition.Status,
			"reason", existingCondition.Reason,
		)
		return nil, false
	}

	return ref, true
}

// fetchEffectivenessAssessmentForTracking fetches the EA CRD referenced by
// ref. A nil EA is returned when the EA is not found (deleted) or the fetch
// failed non-fatally — both cases mean "nothing more to do this reconcile",
// so the fetch error itself is never propagated to the caller (Issue #1546
// Tier 4: dropped the vestigial error return that was always nil; the
// Tier 2 nilnil sentinel rationale below still applies to the two internal
// nil returns). Extracted from trackEffectivenessStatus (Wave 6 6e-i GREEN:
// funlen remediation) — pure code motion, no behavior change.
func (r *Reconciler) fetchEffectivenessAssessmentForTracking(ctx context.Context, ref *corev1.ObjectReference, logger logr.Logger) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, ea)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("EffectivenessAssessment not found (deleted)", "eaName", ref.Name)
			return nil
		}
		logger.Error(err, "Failed to fetch EffectivenessAssessment", "eaName", ref.Name)
		return nil // Non-fatal: don't block reconciliation
	}
	return ea
}

// applyEffectivenessAssessmentOutcome sets the EffectivenessAssessed
// condition on the RR once the EA reaches a terminal phase (Completed or
// Failed) and, per #280/#722, completes a Verifying RR with a score-aware
// outcome. Non-terminal EA phases (Pending/Assessing) are a no-op. Extracted
// from trackEffectivenessStatus (Wave 6 6e-i GREEN: funlen remediation) —
// pure code motion, no behavior change.
func (r *Reconciler) applyEffectivenessAssessmentOutcome(ctx context.Context, rr *remediationv1.RemediationRequest, ea *eav1.EffectivenessAssessment, logger logr.Logger) error {
	switch ea.Status.Phase {
	case eav1.PhaseCompleted:
		reason := "AssessmentCompleted"
		if ea.Status.AssessmentReason == eav1.AssessmentReasonExpired {
			reason = "AssessmentExpired"
		}

		logger.Info("EffectivenessAssessment completed, setting condition",
			"eaName", ea.Name,
			"assessmentReason", ea.Status.AssessmentReason,
			"conditionReason", reason,
		)
		return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
			meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
				Type:               ConditionEffectivenessAssessed,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: rr.Generation,
				Reason:             reason,
				Message:            fmt.Sprintf("Effectiveness assessment completed (reason: %s)", ea.Status.AssessmentReason),
				LastTransitionTime: metav1.Now(),
			})
			// #280/#722: If RR is in Verifying, complete with score-aware outcome
			r.completeVerificationIfNeeded(rr, ea, logger)
			return nil
		})

	case eav1.PhaseFailed:
		logger.Info("EffectivenessAssessment failed, setting condition",
			"eaName", ea.Name,
			"failureReason", ea.Status.AssessmentReason,
		)
		return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
			meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
				Type:               ConditionEffectivenessAssessed,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: rr.Generation,
				Reason:             "AssessmentFailed",
				Message:            fmt.Sprintf("Effectiveness assessment failed (reason: %s)", ea.Status.AssessmentReason),
				LastTransitionTime: metav1.Now(),
			})
			// #280/#722: If RR is in Verifying, complete with score-aware outcome
			r.completeVerificationIfNeeded(rr, ea, logger)
			return nil
		})

	default:
		// EA still in progress (Pending or Assessing) — no action
		logger.V(1).Info("EffectivenessAssessment still in progress",
			"eaName", ea.Name,
			"phase", ea.Status.Phase,
		)
	}

	return nil
}

// DeriveOutcomeFromEA computes the RR outcome from EA component scores (Issue #722).
//
// Decision logic:
//   - alertAssessed && alertScore == 0 → "Inconclusive" (alert still firing)
//   - alertAssessed && alertScore > 0  → "Remediated" (alert resolved)
//   - !alertAssessed                   → "Remediated" (fail-open: AM unavailable)
//
// The !alertAssessed fail-open preserves current behavior for environments without
// AlertManager. EM alert/alert.go sets Assessed=false when AM is unreachable or disabled.
func DeriveOutcomeFromEA(ea *eav1.EffectivenessAssessment) string {
	if ea == nil {
		return remediationv1.OutcomeRemediated
	}

	components := ea.Status.Components
	if !components.AlertAssessed {
		return remediationv1.OutcomeRemediated
	}

	if components.AlertScore != nil && *components.AlertScore == 0 {
		return "Inconclusive"
	}

	return remediationv1.OutcomeRemediated
}

// completeVerificationIfNeeded transitions the RR from Verifying to Completed (#280, #722).
// Called inside a status update closure when EA reaches a terminal phase.
// Issue #722: Uses DeriveOutcomeFromEA for score-aware outcome instead of hardcoding "Remediated".
//
// Issue #1091 (BR-ORCH-042.6): When outcome is "Inconclusive" (EA confirms alert still firing,
// alertScore=0), this method also sets exponential backoff fields. Inconclusive is architecturally
// a Completed RR (OverallPhase=Completed) but functionally a failure — automatic retry without
// changed conditions is futile. ConsecutiveFailureCount increments despite PhaseCompleted because
// it tracks "functional failures" (including Inconclusive), not just phase failures.
func (r *Reconciler) completeVerificationIfNeeded(rr *remediationv1.RemediationRequest, ea *eav1.EffectivenessAssessment, logger interface{ Info(string, ...interface{}) }) {
	if rr.Status.OverallPhase != phase.Verifying {
		return
	}

	outcome := DeriveOutcomeFromEA(ea)

	now := metav1.Now()
	rr.Status.OverallPhase = phase.Completed
	rr.Status.Outcome = outcome
	rr.Status.CompletedAt = &now
	rr.Status.ObservedGeneration = rr.Generation

	startTime := rr.CreationTimestamp.Time
	durationMs := time.Since(startTime).Milliseconds()
	_ = durationMs

	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(phase.Verifying), string(phase.Completed), rr.Namespace).Inc()
	}

	// #1091 (BR-ORCH-042.6): Inconclusive outcomes get exponential backoff to prevent RR flood.
	// Mirrors transitionToFailed backoff pattern — count always increments, backoff only set below threshold.
	if outcome == "Inconclusive" {
		rr.Status.ConsecutiveFailureCount++

		if rr.Status.ConsecutiveFailureCount < int32(r.routingEngine.Config().ConsecutiveFailureThreshold) {
			backoff := r.routingEngine.CalculateExponentialBackoff(rr.Status.ConsecutiveFailureCount)
			if backoff > 0 {
				nextAllowed := metav1.NewTime(time.Now().Add(backoff))
				rr.Status.NextAllowedExecution = &nextAllowed
				logger.Info("Inconclusive outcome: set exponential backoff",
					"consecutiveFailures", rr.Status.ConsecutiveFailureCount,
					"backoff", backoff.Round(time.Second),
					"nextAllowedExecution", nextAllowed.Format(time.RFC3339))
			}
		} else {
			logger.Info("Inconclusive outcome: at/above threshold, skipping backoff (routing engine will block)",
				"consecutiveFailures", rr.Status.ConsecutiveFailureCount,
				"threshold", r.routingEngine.Config().ConsecutiveFailureThreshold)
		}
	}

	logger.Info("Verification complete, RR transitioned to Completed",
		"outcome", outcome,
		"remediationRequest", rr.Name,
		"consecutiveFailures", rr.Status.ConsecutiveFailureCount)
}
