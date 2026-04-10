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
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// emitValidityExtendedEvent emits a warning K8s event when the runtime guard
// extends the ValidityDeadline because StabilizationWindow >= ValidityWindow.
func (r *Reconciler) emitValidityExtendedEvent(ea *eav1.EffectivenessAssessment) {
	r.Recorder.Eventf(ea, corev1.EventTypeWarning, "ValidityWindowExtended",
		"StabilizationWindow (%v) >= ValidityWindow (%v); extended deadline to %v",
		ea.Spec.Config.StabilizationWindow.Duration, r.Config.ValidityWindow, ea.Status.ValidityDeadline.Time)
}

// emitHashEvent emits K8s and audit events for the hash computation result.
// Uses the specialized RecordHashComputed to include pre/post hashes and match flag.
func (r *Reconciler) emitHashEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, result hash.ComputeResult) {
	if result.Component.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash assessment failed: %v", result.Component.Error))
	} else {
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component hash computed (match: %v)", result.Match))
	}

	if r.AuditManager != nil {
		hashData := emaudit.HashComputedData{
			PostHash: result.Hash,
			PreHash:  result.PreHash,
			Match:    result.Match,
		}
		if err := r.AuditManager.RecordHashComputed(ctx, ea, result.Component, hashData); err != nil {
			log.FromContext(ctx).V(1).Info("Failed to store hash computed audit event",
				"error", err)
		}
	}
}

// emitCompletionEvent emits a K8s event when the assessment completes.
func (r *Reconciler) emitCompletionEvent(ea *eav1.EffectivenessAssessment, reason string) {
	r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonEffectivenessAssessed,
		fmt.Sprintf("Assessment completed: %s (correlation: %s)", reason, ea.Spec.CorrelationID))
}

// emitK8sComponentEvent emits a K8s event for a component assessment result.
func (r *Reconciler) emitK8sComponentEvent(ea *eav1.EffectivenessAssessment, component string, result emtypes.ComponentResult) {
	if result.Error != nil {
		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonComponentAssessed,
			fmt.Sprintf("Component %s assessment failed: %v", component, result.Error))
	} else {
		msg := fmt.Sprintf("Component %s assessed", component)
		if result.Score != nil {
			msg = fmt.Sprintf("Component %s assessed (score: %.2f)", component, *result.Score)
		}
		r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonComponentAssessed, msg)
	}
}

// emitHealthEvent emits K8s event and health_checks typed audit event (DD-017 v2.5).
func (r *Reconciler) emitHealthEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, hr healthAssessResult) {
	r.emitK8sComponentEvent(ea, "health", hr.Component)

	r.emitAuditEvent(ctx, emtypes.AuditHealthAssessed, func() error {
		return r.AuditManager.RecordHealthAssessed(ctx, ea, hr.Component, emaudit.HealthAssessedData{
			TotalReplicas:            hr.Status.TotalReplicas,
			ReadyReplicas:            hr.Status.ReadyReplicas,
			RestartsSinceRemediation: hr.Status.RestartsSinceRemediation,
			CrashLoops:              hr.Status.CrashLoops,
			OOMKilled:               hr.Status.OOMKilled,
			PendingCount:            hr.Status.PendingCount,
		})
	})
}

// emitAlertEvent emits K8s event and alert_resolution typed audit event (DD-017 v2.5).
func (r *Reconciler) emitAlertEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, ar alertAssessResult) {
	r.emitK8sComponentEvent(ea, "alert", ar.Component)

	r.emitAuditEvent(ctx, emtypes.AuditAlertAssessed, func() error {
		return r.AuditManager.RecordAlertAssessed(ctx, ea, ar.Component, emaudit.AlertAssessedData{
			AlertResolved:         ar.AlertResolved,
			ActiveCount:           ar.ActiveCount,
			ResolutionTimeSeconds: ar.ResolutionTimeSeconds,
		})
	})
}

// emitAlertDecayEvent emits a one-time audit event when alert decay is first detected.
// Called only when AlertDecayRetries == 0 (before increment). Follows the metrics
// component precedent: silent retries after the first detection (Issue #369, BR-EM-012).
func (r *Reconciler) emitAlertDecayEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, ar alertAssessResult) {
	r.emitK8sComponentEvent(ea, "alert_decay", emtypes.ComponentResult{
		Component: emtypes.ComponentAlertDecay,
		Assessed:  false,
		Score:     ar.Component.Score,
		Details:   "Alert decay detected: resource healthy but alert still firing",
	})

	var healthScore float64
	if ea.Status.Components.HealthScore != nil {
		healthScore = *ea.Status.Components.HealthScore
	}
	var alertScore float64
	if ar.Component.Score != nil {
		alertScore = *ar.Component.Score
	}

	r.emitAuditEvent(ctx, emtypes.AuditAlertDecayDetected, func() error {
		return r.AuditManager.RecordAlertDecayDetected(ctx, ea, emaudit.AlertDecayDetectedData{
			HealthScore: healthScore,
			AlertScore:  alertScore,
			RetryCount:  ea.Status.Components.AlertDecayRetries + 1,
		})
	})
}

// emitMetricsEvent emits K8s event and metric_deltas typed audit event (DD-017 v2.5).
func (r *Reconciler) emitMetricsEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, mr metricsAssessResult) {
	r.emitK8sComponentEvent(ea, "metrics", mr.Component)

	r.emitAuditEvent(ctx, emtypes.AuditMetricsAssessed, func() error {
		return r.AuditManager.RecordMetricsAssessed(ctx, ea, mr.Component, emaudit.MetricsAssessedData{
			CPUBefore:           mr.CPUBefore,
			CPUAfter:            mr.CPUAfter,
			MemoryBefore:        mr.MemoryBefore,
			MemoryAfter:         mr.MemoryAfter,
			LatencyP95BeforeMs:  mr.LatencyP95BeforeMs,
			LatencyP95AfterMs:   mr.LatencyP95AfterMs,
			ErrorRateBefore:     mr.ErrorRateBefore,
			ErrorRateAfter:      mr.ErrorRateAfter,
			ThroughputBeforeRPS: mr.ThroughputBeforeRPS,
			ThroughputAfterRPS:  mr.ThroughputAfterRPS,
		})
	})
}

// emitScheduledEventIfFirst emits the assessment.scheduled audit event and K8s event
// when ValidityDeadline is first set (WFP or Stabilizing transition).
// Called exactly once per EA lifecycle at the point where ValidityDeadline is persisted
// (#573, ADR-EM-001 §9.2.0). NOT called from the Assessing transition to avoid duplicates.
func (r *Reconciler) emitScheduledEventIfFirst(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentScheduled, func() error {
		return r.AuditManager.RecordAssessmentScheduled(ctx, ea, r.Config.ValidityWindow)
	})

	r.Recorder.Event(ea, corev1.EventTypeNormal, "AssessmentScheduled",
		fmt.Sprintf("Assessment scheduled for correlation %s (deadline: %s)",
			ea.Spec.CorrelationID, ea.Status.ValidityDeadline.Format("15:04:05")))
}

// emitAssessingTransitionEvents emits the AssessmentStarted K8s event for the
// transition to Assessing phase. Called after the status update succeeds.
func (r *Reconciler) emitAssessingTransitionEvents(ctx context.Context, ea *eav1.EffectivenessAssessment) {
	r.Recorder.Event(ea, corev1.EventTypeNormal, events.EventReasonAssessmentStarted,
		fmt.Sprintf("Assessment started for correlation %s", ea.Spec.CorrelationID))
}

// emitCompletedAuditEvent emits the assessment.completed audit event to DataStorage.
func (r *Reconciler) emitCompletedAuditEvent(ctx context.Context, ea *eav1.EffectivenessAssessment, reason string) {
	r.emitAuditEvent(ctx, emtypes.AuditAssessmentCompleted, func() error {
		return r.AuditManager.RecordAssessmentCompleted(ctx, ea, reason)
	})
}

// emitAuditEvent is a helper that handles the nil-check and error logging common to
// all audit event emissions. The recordFn performs the actual audit call.
func (r *Reconciler) emitAuditEvent(ctx context.Context, eventType emtypes.AuditEventType, recordFn func() error) {
	if r.AuditManager == nil {
		log.FromContext(ctx).V(1).Info("AuditManager not configured, skipping audit event",
			"eventType", string(eventType))
		return
	}

	if err := recordFn(); err != nil {
		log.FromContext(ctx).Error(err, "Failed to emit audit event", "eventType", string(eventType))
		return
	}
}

// resultStatus returns a metric label for a component result.
func resultStatus(result emtypes.ComponentResult) string {
	if result.Error != nil {
		return "error"
	}
	if result.Assessed {
		return "success"
	}
	return "skipped"
}
