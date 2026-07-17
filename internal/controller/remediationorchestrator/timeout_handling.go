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

// Global-timeout handling (BR-ORCH-027) and EffectivenessAssessment creation
// (ADR-EM-001) on terminal phases. Split out of terminal_transitions.go per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file
// under the 700-line convention threshold. Pure structural move — no
// behavior change.
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// handleGlobalTimeout transitions the RR to TimedOut phase when global timeout exceeded.
// BR-ORCH-027: Global Timeout Management
// Business Value: Prevents stuck remediations from consuming resources indefinitely
// Default timeout: 1 hour from CreationTimestamp
func (r *Reconciler) handleGlobalTimeout(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	timeoutPhase, err := r.transitionToTimedOut(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to transition to TimedOut")
		return ctrl.Result{}, fmt.Errorf("failed to transition to TimedOut: %w", err)
	}

	// ========================================
	// CREATE TIMEOUT NOTIFICATION (BR-ORCH-027)
	// Business Value: Operators notified for manual intervention
	// ========================================
	nr, ok := r.createTimeoutNotification(ctx, rr, timeoutPhase, logger)
	if !ok {
		// Notification creation/wiring failed non-fatally - timeout transition is primary goal.
		return ctrl.Result{}, nil
	}

	r.trackTimeoutNotification(ctx, rr, nr, logger)

	// Issue #240: EA is NOT created on global timeout. See transitionToVerifying.

	return ctrl.Result{}, nil
}

// transitionToTimedOut implements handleGlobalTimeout's status-transition
// step: mark the RR TimedOut, record metrics/audit/K8s-event side effects,
// and return the phase the RR was in when the timeout fired (needed by the
// notification body built later in handleGlobalTimeout). Extracted from
// handleGlobalTimeout (Wave 6 6e-i GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (r *Reconciler) transitionToTimedOut(ctx context.Context, rr *remediationv1.RemediationRequest) (remediationv1.RemediationPhase, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	timeoutPhase := rr.Status.OverallPhase
	oldPhase := rr.Status.OverallPhase

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		now := metav1.Now()
		rr.Status.TimeoutTime = &now
		rr.Status.TimeoutPhase = &timeoutPhase
		rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

		// BR-ORCH-043: Set Ready condition (terminal timeout)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationTimedOut, "Remediation timed out", r.Metrics)

		return nil
	})
	if err != nil {
		return timeoutPhase, err
	}

	// Record metrics (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(remediationv1.PhaseTimedOut), rr.Namespace).Inc()
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, string(timeoutPhase)).Inc()
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "global", string(timeoutPhase), durationMs)
	}

	// DD-EVENT-001: Emit K8s event for global timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Global timeout exceeded during %s phase", string(timeoutPhase)))
	}

	logger.Info("Remediation timed out (global timeout exceeded)",
		"timeoutPhase", timeoutPhase,
		"creationTimestamp", rr.CreationTimestamp)

	return timeoutPhase, nil
}

// createTimeoutNotification implements handleGlobalTimeout's
// notification-creation step (BR-ORCH-027): build the escalation
// NotificationRequest, set its owner reference, and create it. Any failure
// here (missing UID, owner-ref error, create error other than
// AlreadyExists) is non-fatal to the timeout transition and reported via
// ok=false so the caller returns early without failing reconciliation.
// Extracted from handleGlobalTimeout (Wave 6 6e-i GREEN: funlen remediation)
// — pure code motion, no behavior change.
func (r *Reconciler) createTimeoutNotification(ctx context.Context, rr *remediationv1.RemediationRequest, timeoutPhase remediationv1.RemediationPhase, logger logr.Logger) (*notificationv1.NotificationRequest, bool) {
	notificationName := fmt.Sprintf("timeout-%s", rr.Name)
	nr := r.buildTimeoutNotificationRequest(rr, notificationName, timeoutPhase)

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference on timeout notification")
		return nil, false
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on timeout notification")
		return nil, false
	}

	// Create notification (non-blocking - timeout transition is primary goal)
	if err := r.client.Create(ctx, nr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Timeout notification already exists (concurrent create), continuing", "notificationName", notificationName)
		} else {
			logger.Error(err, "Failed to create timeout notification",
				"notificationName", notificationName)
			return nil, false
		}
	}

	logger.Info("Created timeout notification",
		"notificationName", notificationName,
		"priority", nr.Spec.Priority,
		"timeoutPhase", timeoutPhase)

	// DD-EVENT-001: Emit K8s event for notification creation (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Timeout notification created: %s", notificationName))
	}

	return nr, true
}

// buildTimeoutNotificationRequest constructs the escalation
// NotificationRequest object (not yet persisted) for handleGlobalTimeout's
// timeout notification. Extracted from createTimeoutNotification (Wave 6
// 6e-i GREEN: funlen remediation) — pure code motion, no behavior change.
func (r *Reconciler) buildTimeoutNotificationRequest(rr *remediationv1.RemediationRequest, notificationName string, timeoutPhase remediationv1.RemediationPhase) *notificationv1.NotificationRequest {
	return &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notificationName,
			Namespace: rr.Namespace,
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityCritical,
			Severity: rr.Spec.Severity,
			Subject:  fmt.Sprintf("Remediation Timeout: %s", rr.Spec.SignalName),
			Body: r.notificationCreator.BuildGlobalTimeoutBody(
				rr.Spec.SignalName,
				rr.Name,
				string(timeoutPhase),
				r.getEffectiveGlobalTimeout(rr).String(),
				rr.Status.StartTime.Format(time.RFC3339),
				rr.Status.TimeoutTime.Format(time.RFC3339),
			),
			Context: buildTimeoutContext(rr.Name, string(timeoutPhase), "", rr.Spec.TargetResource),
		},
	}
}

// trackTimeoutNotification implements handleGlobalTimeout's best-effort
// status-tracking step (Recommendation #2, BR-ORCH-035): append nr to
// rr.Status.NotificationRequestRefs. Failure is logged but not propagated —
// the notification was already created successfully, so tracking is
// best-effort. Extracted from handleGlobalTimeout (Wave 6 6e-i GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (r *Reconciler) trackTimeoutNotification(ctx context.Context, rr *remediationv1.RemediationRequest, nr *notificationv1.NotificationRequest, logger logr.Logger) {
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		notifRef := corev1.ObjectReference{
			Kind:       "NotificationRequest",
			Namespace:  nr.Namespace,
			Name:       nr.Name,
			UID:        nr.UID,
			APIVersion: "notification.kubernaut.ai/v1alpha1",
		}
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, notifRef)
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to track notification in status (non-critical)",
			"notificationName", nr.Name)
	} else {
		logger.V(1).Info("Tracked notification in status",
			"notificationName", nr.Name,
			"totalNotifications", len(rr.Status.NotificationRequestRefs)+1)
	}
}

// createEffectivenessAssessmentIfNeeded creates an EA CRD if the eaCreator is wired.
// ADR-EM-001: EA creation is ALWAYS non-fatal. The terminal phase transition must succeed
// even if EA creation fails. Errors are logged but not propagated.
// BR-HAPI-191: Resolves the target from AIAnalysis.RemediationTarget when available,
// so the EA assesses the resource the workflow actually modified (not the signal Pod).
// Batch 3: After creating the EA, persists the EffectivenessAssessmentRef on the RR status
// so that trackEffectivenessStatus can find the EA for condition tracking.
func (r *Reconciler) createEffectivenessAssessmentIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) {
	if r.eaCreator == nil {
		return
	}

	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	dualTarget, isGitOpsManaged, ai := r.resolveEATargets(ctx, rr, logger)
	hashComputeDelay, alertCheckDelay, isCRD := r.computeEADelays(rr, dualTarget, ai, isGitOpsManaged, logger)

	name, err := r.eaCreator.CreateEffectivenessAssessment(ctx, rr, dualTarget, hashComputeDelay, alertCheckDelay)
	if err != nil {
		logger.Error(err, "Failed to create EffectivenessAssessment (non-fatal per ADR-EM-001)")
		return
	}
	logger.Info("EffectivenessAssessment created", "eaName", name, "rrPhase", rr.Status.OverallPhase)

	// #277: Emit orchestrator.ea.created audit event with propagation delay breakdown.
	r.emitEACreatedAudit(ctx, rr, name, hashComputeDelay, alertCheckDelay, isGitOpsManaged, isCRD)

	// ADR-EM-001, Batch 3: Persist EA ref on RR status for trackEffectivenessStatus.
	// Uses helpers.UpdateRemediationRequestStatus for atomic persistence (same pattern
	// as NT ref tracking in handleGlobalTimeout).
	// GAP-2 (ADR-EM-001 Section 9.4.15): Also set initial EffectivenessAssessed=False /
	// AssessmentInProgress so operators can distinguish "no EA yet" from "EA in progress."
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       name,
			Namespace:  rr.Namespace,
			APIVersion: eav1.GroupVersion.String(),
		}
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:    ConditionEffectivenessAssessed,
			Status:  metav1.ConditionFalse,
			Reason:  "AssessmentInProgress",
			Message: fmt.Sprintf("EffectivenessAssessment %s created, assessment in progress", name),
		})
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist EA ref on RR status (non-critical)", "eaName", name)
	}
}

// resolveEATargets implements DD-EM-003's dual-target resolution step of
// createEffectivenessAssessmentIfNeeded: the signal target always comes from
// rr, while the remediation target comes from AIAnalysis.RemediationTarget
// when the AIAnalysis is fetchable, falling back to the RR target otherwise
// (non-fatal — a fetch error just means dualTarget/ai stay nil). Also
// surfaces GitOps-managed detection from the RCA pipeline (DD-EM-004,
// BR-RO-103.2). Extracted from createEffectivenessAssessmentIfNeeded (Wave 6
// 6e-i GREEN: cyclomatic-complexity remediation) — pure code motion, no
// behavior change.
func (r *Reconciler) resolveEATargets(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (dualTarget *creator.DualTarget, isGitOpsManaged bool, ai *aianalysisv1.AIAnalysis) {
	if rr.Status.AIAnalysisRef == nil {
		return nil, false, nil
	}

	ai = &aianalysisv1.AIAnalysis{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}, ai); err != nil {
		logger.V(1).Info("Could not fetch AIAnalysis for target resolution (non-fatal), using RR target",
			"error", err)
		return nil, false, nil
	}

	dualTarget = resolveDualTargets(rr, ai)
	if ai.Status.PostRCAContext != nil &&
		ai.Status.PostRCAContext.DetectedLabels != nil &&
		ai.Status.PostRCAContext.DetectedLabels.GitOpsManaged {
		isGitOpsManaged = true
	}
	return dualTarget, isGitOpsManaged, ai
}

// computeEADelays implements the Duration-delay computation steps of
// createEffectivenessAssessmentIfNeeded: DD-EM-004 v2.0/BR-RO-103/#253/#277's
// async-managed-target hash-compute delay, and #277's proactive-signal alert
// check delay. Extracted from createEffectivenessAssessmentIfNeeded (Wave 6
// 6e-i GREEN: cyclomatic-complexity remediation) — pure code motion, no
// behavior change.
func (r *Reconciler) computeEADelays(rr *remediationv1.RemediationRequest, dualTarget *creator.DualTarget, ai *aianalysisv1.AIAnalysis, isGitOpsManaged bool, logger logr.Logger) (hashComputeDelay, alertCheckDelay *metav1.Duration, isCRD bool) {
	remediationKind := rr.Spec.TargetResource.Kind
	if dualTarget != nil {
		remediationKind = dualTarget.Remediation.Kind
	}

	gvk, err := k8sutil.ResolveGVKForKind(r.restMapper, remediationKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, treating as sync target for hash timing",
			"kind", remediationKind, "error", err)
	} else if !creator.IsBuiltInGroup(gvk.Group) {
		isCRD = true
	}

	asyncCfg := r.getAsyncPropagation()
	propagationDelay := asyncCfg.ComputePropagationDelay(isGitOpsManaged, isCRD)
	if propagationDelay > 0 {
		hashComputeDelay = &metav1.Duration{Duration: propagationDelay}
		logger.Info("Async-managed target detected, setting hash check delay",
			"kind", remediationKind,
			"gitOps", isGitOpsManaged,
			"isCRD", isCRD,
			"hashComputeDelay", propagationDelay)
	}

	// #277: Detect proactive signals via AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalMode.
	// Proactive alerts (e.g. predict_linear) need extra time to resolve.
	if ai != nil && ai.Spec.AnalysisRequest.SignalContext.SignalMode == signalprocessingv1alpha1.SignalModeProactive && asyncCfg.ProactiveAlertDelay > 0 {
		alertCheckDelay = &metav1.Duration{Duration: asyncCfg.ProactiveAlertDelay}
		logger.Info("Proactive signal detected, setting alert check delay",
			"signalMode", ai.Spec.AnalysisRequest.SignalContext.SignalMode,
			"alertCheckDelay", asyncCfg.ProactiveAlertDelay)
	}

	return hashComputeDelay, alertCheckDelay, isCRD
}
