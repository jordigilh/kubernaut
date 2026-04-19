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

	// If no EA ref, nothing to track
	if rr.Status.EffectivenessAssessmentRef == nil {
		logger.V(1).Info("No EffectivenessAssessmentRef to track")
		return nil
	}

	ref := rr.Status.EffectivenessAssessmentRef
	if ref.Name == "" || ref.Namespace == "" {
		logger.Info("Invalid EffectivenessAssessmentRef, skipping tracking",
			"refName", ref.Name,
			"refNamespace", ref.Namespace,
		)
		return nil
	}

	// Check if condition already set with a terminal reason (idempotency).
	// Allow transitioning from the initial "AssessmentInProgress" (GAP-2) to a
	// terminal reason (AssessmentCompleted/AssessmentFailed), but do not overwrite
	// a terminal reason once set.
	existingCondition := meta.FindStatusCondition(rr.Status.Conditions, ConditionEffectivenessAssessed)
	if existingCondition != nil && existingCondition.Reason != "AssessmentInProgress" {
		logger.V(1).Info("EffectivenessAssessed condition already set with terminal reason, skipping",
			"status", existingCondition.Status,
			"reason", existingCondition.Reason,
		)
		return nil
	}

	// Fetch the EA CRD
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

	// Only set condition for terminal EA phases
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
		return "Remediated"
	}

	components := ea.Status.Components
	if !components.AlertAssessed {
		return "Remediated"
	}

	if components.AlertScore != nil && *components.AlertScore == 0 {
		return "Inconclusive"
	}

	return "Remediated"
}

// completeVerificationIfNeeded transitions the RR from Verifying to Completed (#280, #722).
// Called inside a status update closure when EA reaches a terminal phase.
// Issue #722: Uses DeriveOutcomeFromEA for score-aware outcome instead of hardcoding "Remediated".
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

	logger.Info("Verification complete, RR transitioned to Completed",
		"outcome", outcome,
		"remediationRequest", rr.Name)
}
