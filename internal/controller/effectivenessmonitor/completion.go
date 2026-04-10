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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
)

// completeAssessment finalizes the EA with Completed phase and assessment reason.
// It performs a single atomic status update and emits completion events.
// Used by both the normal completion path (Step 8-9) and the expired path (Step 4).
func (r *Reconciler) completeAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	reason := r.determineAssessmentReason(ea)
	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	return ctrl.Result{}, nil
}

// failAssessment transitions the EA to PhaseFailed for unrecoverable conditions (#573).
// Unlike completeAssessment (which uses PhaseCompleted), this sets PhaseFailed and
// reason "unrecoverable". The RO handles PhaseFailed in trackEffectivenessStatus.
func (r *Reconciler) failAssessment(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	now := metav1.Now()
	ea.Status.Phase = eav1.PhaseFailed
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = "unrecoverable"
	ea.Status.Message = fmt.Sprintf("Assessment failed: %s", reason)

	conditions.SetCondition(ea, conditions.ConditionAssessmentComplete, metav1.ConditionFalse, "ValidationFailed", ea.Status.Message)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Failed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.Recorder.Event(ea, corev1.EventTypeWarning, "AssessmentFailed", ea.Status.Message)
	logger.Info("Assessment failed due to unrecoverable condition",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	return ctrl.Result{}, nil
}

// completeAssessmentWithReason finalizes the EA with an explicit assessment reason.
// Unlike completeAssessment, which computes the reason from component state, this
// method uses the provided reason directly. Used by the spec drift guard (DD-EM-002 v1.1).
func (r *Reconciler) completeAssessmentWithReason(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	r.setCompletionFields(ea, reason)

	if err := r.Status().Update(ctx, ea); err != nil {
		logger.Error(err, "Failed to update EA to Completed",
			"reason", reason, "resourceVersion", ea.ResourceVersion)
		return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
	}

	r.emitCompletionMetricsAndEvents(ctx, ea, reason)

	logger.Info("Assessment completed",
		"reason", reason,
		"correlationID", ea.Spec.CorrelationID,
	)

	return ctrl.Result{}, nil
}

// setCompletionFields sets the in-memory status fields for assessment completion.
// Shared by completeAssessment, completeAssessmentWithReason, and finalizeReconcile.
// Also sets the AssessmentComplete condition per DD-CRD-002.
func (r *Reconciler) setCompletionFields(ea *eav1.EffectivenessAssessment, reason string) {
	now := metav1.Now()
	ea.Status.Phase = eav1.PhaseCompleted
	ea.Status.CompletedAt = &now
	ea.Status.AssessmentReason = reason
	ea.Status.Message = fmt.Sprintf("Assessment completed: %s", reason)

	// Issue #79 Phase 7b: Set Ready condition on terminal transitions
	conditions.SetReady(ea, true, conditions.ReasonReady, "Assessment completed")

	// Set AssessmentComplete condition (DD-CRD-002) for all completion paths.
	condReason := mapAssessmentReasonToConditionReason(reason)
	conditions.SetCondition(ea, conditions.ConditionAssessmentComplete,
		metav1.ConditionTrue, condReason,
		fmt.Sprintf("Assessment completed: %s", reason))

	// BR-EM-012: If the EA was in active decay monitoring (AlertDecayRetries > 0),
	// resolve the AlertDecayDetected condition on any terminal transition.
	if ea.Status.Components.AlertDecayRetries > 0 {
		decayReason := conditions.ReasonDecayResolved
		decayMsg := "Alert decay monitoring ended: assessment completed"
		if reason == eav1.AssessmentReasonAlertDecayTimeout {
			decayReason = conditions.ReasonDecayTimeout
			decayMsg = "Alert decay monitoring ended: validity window expired before alert resolved"
		}
		conditions.SetCondition(ea, conditions.ConditionAlertDecayDetected,
			metav1.ConditionFalse, decayReason, decayMsg)
	}
}

// mapAssessmentReasonToConditionReason maps an AssessmentReason value to the
// corresponding DD-CRD-002 condition reason constant.
func mapAssessmentReasonToConditionReason(reason string) string {
	switch reason {
	case eav1.AssessmentReasonFull:
		return conditions.ReasonAssessmentFull
	case eav1.AssessmentReasonPartial:
		return conditions.ReasonAssessmentPartial
	case eav1.AssessmentReasonExpired:
		return conditions.ReasonAssessmentExpired
	case eav1.AssessmentReasonSpecDrift:
		return conditions.ReasonSpecDrift
	case eav1.AssessmentReasonMetricsTimedOut:
		return conditions.ReasonMetricsTimedOut
	case eav1.AssessmentReasonNoExecution:
		return conditions.ReasonNoExecution
	case eav1.AssessmentReasonAlertDecayTimeout:
		return conditions.ReasonAlertDecayTimeout
	default:
		return reason
	}
}

// emitCompletionMetricsAndEvents records metrics and emits K8s + audit events for completion.
func (r *Reconciler) emitCompletionMetricsAndEvents(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	r.Metrics.RecordAssessmentCompleted(reason)
	r.emitCompletionEvent(ea, reason)
	r.emitCompletedAuditEvent(ctx, ea, reason)
}

// determineAssessmentReason computes the reason based on component assessment state.
//
// Reason hierarchy (highest priority first):
//   - full: All enabled components assessed
//   - metrics_timed_out: Core checks done (health+hash), Prometheus enabled but metrics
//     not assessed, validity expired (distinct from generic partial)
//   - partial: Some components assessed, some not (generic)
//   - expired: Validity expired with no data collected
func (r *Reconciler) determineAssessmentReason(ea *eav1.EffectivenessAssessment) string {
	components := &ea.Status.Components

	allAssessed := components.HealthAssessed && components.HashComputed &&
		(components.AlertAssessed || !r.Config.AlertManagerEnabled) &&
		(components.MetricsAssessed || !r.Config.PrometheusEnabled)

	anyAssessed := components.HealthAssessed || components.HashComputed ||
		components.AlertAssessed || components.MetricsAssessed

	if allAssessed {
		return eav1.AssessmentReasonFull
	}

	validityExpired := ea.Status.ValidityDeadline != nil &&
		r.validityChecker.TimeUntilExpired(ea.Status.ValidityDeadline.Time) == 0

	if validityExpired {
		// Issue #369, BR-EM-012: Distinguish alert_decay_timeout from generic partial.
		if components.AlertDecayRetries > 0 && !components.AlertAssessed {
			return eav1.AssessmentReasonAlertDecayTimeout
		}

		// ADR-EM-001, Batch 3: Distinguish metrics_timed_out from generic partial.
		if r.Config.PrometheusEnabled && !components.MetricsAssessed &&
			components.HealthAssessed && components.HashComputed &&
			(components.AlertAssessed || !r.Config.AlertManagerEnabled) {
			return eav1.AssessmentReasonMetricsTimedOut
		}

		if anyAssessed {
			return eav1.AssessmentReasonPartial
		}
		return eav1.AssessmentReasonExpired
	}

	if anyAssessed {
		return eav1.AssessmentReasonPartial
	}

	return eav1.AssessmentReasonExpired
}

// allComponentsDone checks if all enabled components have been assessed.
func (r *Reconciler) allComponentsDone(ea *eav1.EffectivenessAssessment) bool {
	if !ea.Status.Components.HealthAssessed {
		return false
	}
	if !ea.Status.Components.HashComputed {
		return false
	}
	if r.Config.AlertManagerEnabled && !ea.Status.Components.AlertAssessed {
		return false
	}
	if r.Config.PrometheusEnabled && !ea.Status.Components.MetricsAssessed {
		return false
	}
	return true
}
