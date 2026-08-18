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

// DD-TIMEOUT-002 / Issue #2176: SignalProcessing self-enforcement of RO's
// authoritative absolute deadline (Spec.TimesOutAt, propagated from
// RemediationRequest.Status.TimeoutConfig.Processing at SP creation time).
// Checked once per Reconcile, ahead of phase dispatch, so it applies
// uniformly across every active phase (Enriching, Classifying, Categorizing)
// without each phase handler needing its own timeout-checking logic.
package signalprocessing

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// setActivePhaseConditionFalse sets the DD-SP-002 condition matching the
// pre-failure phase to False with ReasonTimedOut, so `kubectl describe` shows
// which specific phase was in flight when RO's deadline hit -- mirroring
// evaluateSeverityOrFail's existing phase-specific-condition pattern. The
// phase must be the value captured *before* the caller overwrites
// sp.Status.Phase to PhaseFailed (this deliberately takes it as a parameter
// rather than reading sp.Status.Phase, which would already be PhaseFailed by
// the time this runs inside failOnTimeout's AtomicStatusUpdate closure).
// Pending has no phase-specific condition (per DD-SP-002); Enriching
// deliberately excludes the pre-2176 K8sAPITimeout/ResourceNotFound reasons,
// since this is a wall-clock deadline, not an enrichment-specific failure.
func setActivePhaseConditionFalse(sp *signalprocessingv1alpha1.SignalProcessing, phase signalprocessingv1alpha1.SignalProcessingPhase, message string) {
	switch phase {
	case signalprocessingv1alpha1.PhaseEnriching:
		spconditions.SetEnrichmentComplete(sp, false, spconditions.ReasonTimedOut, message)
	case signalprocessingv1alpha1.PhaseClassifying:
		spconditions.SetClassificationComplete(sp, false, spconditions.ReasonTimedOut, message)
	case signalprocessingv1alpha1.PhaseCategorizing:
		spconditions.SetCategorizationComplete(sp, false, spconditions.ReasonTimedOut, message)
	case signalprocessingv1alpha1.PhasePending, signalprocessingv1alpha1.PhaseCompleted, signalprocessingv1alpha1.PhaseFailed:
		// No phase-specific condition to set: Pending precedes any active
		// phase (per DD-SP-002), and Completed/Failed are already terminal
		// by the time this timeout check would ever run.
	}
}

// hasTimedOut reports whether sp carries an authoritative Spec.TimesOutAt
// deadline (DD-TIMEOUT-002) that has already passed. Returns false when RO
// propagated no deadline, in which case SP relies solely on RO's outer
// backstop.
func hasTimedOut(sp *signalprocessingv1alpha1.SignalProcessing) bool {
	return sp.Spec.TimesOutAt != nil && metav1.Now().After(sp.Spec.TimesOutAt.Time)
}

// failOnTimeout fails sp in place, mirroring evaluateSeverityOrFail's
// terminal-failure pattern (status update + K8s Warning event + mandatory
// audit RecordError). The caller should return ctrl.Result{} alongside the
// returned error immediately (no requeue: a terminal Failed phase needs
// none).
func (r *SignalProcessingReconciler) failOnTimeout(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) error {
	activePhase := sp.Status.Phase
	phase := string(activePhase)
	var elapsed time.Duration
	if sp.Status.StartTime != nil {
		elapsed = time.Since(sp.Status.StartTime.Time)
	}
	timeoutErr := fmt.Errorf("signal processing timed out after %s during phase %s (deadline: %s)",
		elapsed.Truncate(time.Second), phase, sp.Spec.TimesOutAt.Format(time.RFC3339))

	logger.Info("SignalProcessing exceeded RO's authoritative deadline, failing",
		"phase", phase,
		"elapsed", elapsed,
		"timesOutAt", sp.Spec.TimesOutAt,
	)

	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ObservedGeneration = sp.Generation
		sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
		sp.Status.Error = timeoutErr.Error()
		// DD-SP-002: on FAILED, the phase-specific condition for whichever
		// phase was active when the deadline hit is also set False, mirroring
		// evaluateSeverityOrFail's existing terminal-failure pattern. Must use
		// the pre-failure activePhase captured above -- sp.Status.Phase was
		// just overwritten to PhaseFailed on the line above.
		setActivePhaseConditionFalse(sp, activePhase, timeoutErr.Error())
		spconditions.SetProcessingComplete(sp, false, spconditions.ReasonTimedOut, timeoutErr.Error())
		spconditions.SetReady(sp, false, spconditions.ReasonNotReady, "Signal processing timed out")
		return nil
	})
	if updateErr != nil {
		logger.Error(updateErr, "Failed to update status to Failed phase on timeout")
		return updateErr
	}

	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeWarning, events.EventReasonProcessingTimedOut, timeoutErr.Error())
	}

	// BR-AUDIT-005 / AU-2/AU-3: audit is MANDATORY for self-enforced timeout
	// failures, matching every other terminal-failure path in this controller
	// (e.g. evaluateSeverityOrFail).
	if r.AuditManager != nil {
		if auditErr := r.AuditManager.RecordError(ctx, sp, phase, timeoutErr); auditErr != nil {
			logger.Error(auditErr, "Failed to record timeout error audit event",
				"name", sp.Name, "namespace", sp.Namespace, "phase", phase)
		}
	}

	return nil
}
