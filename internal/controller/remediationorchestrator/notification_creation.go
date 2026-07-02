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
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// hasNotificationRef returns true if a NotificationRequest with the given name
// is already tracked in rr.Status.NotificationRequestRefs.
func hasNotificationRef(rr *remediationv1.RemediationRequest, name string) bool {
	for i := range rr.Status.NotificationRequestRefs {
		if rr.Status.NotificationRequestRefs[i].Name == name {
			return true
		}
	}
	return false
}

// ensureNotificationsCreated creates completion and bulk-duplicate notifications
// if they are not yet tracked in NotificationRequestRefs.
// Idempotent: deterministic names + ref check prevent duplicates across reconciles.
// Non-blocking: errors are logged but never propagated.
// #304: Called ONLY after Outcome is set (completeVerificationIfNeeded or timeout transitions).
// Previously called from transitionToVerifying before Outcome was populated (BR-ORCH-045 violation).
// Reference: BR-ORCH-045 (completion), BR-ORCH-034 (bulk duplicate), #281 (retry).
func (r *Reconciler) ensureNotificationsCreated(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	completionName := fmt.Sprintf("nr-completion-%s", rr.Name)
	if !hasNotificationRef(rr, completionName) {
		r.createCompletionNotification(ctx, rr, completionName, logger)
	}

	bulkName := fmt.Sprintf("nr-bulk-%s", rr.Name)
	if rr.Status.DuplicateCount > 0 && !hasNotificationRef(rr, bulkName) {
		r.createBulkDuplicateNotification(ctx, rr, logger)
	}
}

// createCompletionNotification gathers the AIAnalysis, WFE execution engine,
// and EA verification summary (all best-effort/graceful-degradation) needed
// to render the completion notification, then creates it (BR-ORCH-045).
// Extracted from ensureNotificationsCreated per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) createCompletionNotification(ctx context.Context, rr *remediationv1.RemediationRequest, completionName string, logger logr.Logger) {
	aiName := fmt.Sprintf("ai-%s", rr.Name)
	ai := &aianalysisv1.AIAnalysis{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: aiName, Namespace: rr.Namespace}, ai); err != nil {
		logger.Error(err, "Failed to fetch AIAnalysis for completion notification, will retry", "aiAnalysis", aiName)
		return
	}

	// Issue #518: Read executionEngine from WFE status (resolved at runtime by WE controller).
	executionEngine := ""
	if rr.Status.WorkflowExecutionRef != nil {
		we := &workflowexecutionv1.WorkflowExecution{}
		weKey := client.ObjectKey{Name: rr.Status.WorkflowExecutionRef.Name, Namespace: rr.Status.WorkflowExecutionRef.Namespace}
		if weErr := r.client.Get(ctx, weKey, we); weErr != nil {
			logger.V(1).Info("Could not fetch WFE for executionEngine (best-effort)", "error", weErr)
		} else {
			executionEngine = we.Status.ExecutionEngine
		}
	}

	// #318: Fetch EA for verification summary (graceful degradation: nil if unavailable)
	var ea *eav1.EffectivenessAssessment
	if rr.Status.EffectivenessAssessmentRef != nil {
		eaObj := &eav1.EffectivenessAssessment{}
		eaKey := client.ObjectKey{
			Name:      rr.Status.EffectivenessAssessmentRef.Name,
			Namespace: rr.Namespace,
		}
		if err := r.client.Get(ctx, eaKey, eaObj); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("EA not found for verification summary, notification will show 'not available'",
					"ea", eaKey.Name)
			} else {
				logger.Error(err, "Failed to fetch EA for verification summary, notification will show 'not available'",
					"ea", eaKey.Name)
			}
		} else {
			ea = eaObj
		}
	}

	notifName, notifErr := r.notificationCreator.CreateCompletionNotification(ctx, rr, ai, executionEngine, ea)
	if notifErr != nil {
		logger.Error(notifErr, "Failed to create completion notification, will retry")
		return
	}
	logger.Info("Created completion notification", "notification", notifName)
	ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist completion NT ref (non-critical)", "notification", notifName)
	}
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Completion notification created: %s", notifName))
	}
}

// createBulkDuplicateNotification creates the bulk-duplicate-signal
// notification (BR-ORCH-034). Extracted from ensureNotificationsCreated per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) createBulkDuplicateNotification(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) {
	name, bulkErr := r.notificationCreator.CreateBulkDuplicateNotification(ctx, rr)
	if bulkErr != nil {
		logger.Error(bulkErr, "Failed to create bulk duplicate notification, will retry")
		return
	}
	logger.Info("Created bulk duplicate notification", "notification", name)
	ref := r.buildNotificationRef(ctx, name, rr.Namespace)
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist bulk NT ref (non-critical)", "notification", name)
	}
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Bulk duplicate notification created: %s", name))
	}
}

// buildNotificationRef fetches the NotificationRequest by name to obtain its UID
// and returns a fully populated ObjectReference (BR-ORCH-035 AC-6).
// If the fetch fails, UID is omitted (best-effort; Name+Namespace still sufficient for lookup).
func (r *Reconciler) buildNotificationRef(ctx context.Context, name, namespace string) corev1.ObjectReference {
	ref := corev1.ObjectReference{
		Kind:       "NotificationRequest",
		Name:       name,
		Namespace:  namespace,
		APIVersion: "notification.kubernaut.ai/v1alpha1",
	}
	nr := &notificationv1.NotificationRequest{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, nr); err == nil {
		ref.UID = nr.UID
	}
	return ref
}

// createPhaseTimeoutNotification creates a notification for phase timeout.
// Non-blocking - logs errors but doesn't fail reconciliation.
// Reference: BR-ORCH-028 (Per-phase timeout escalation)
// buildTimeoutContext constructs the typed notification context for timeout notifications.
// Shared by both global timeout and per-phase timeout notification creation.
func buildTimeoutContext(rrName, timeoutPhase, phaseTimeout string, target remediationv1.ResourceIdentifier) *notificationv1.NotificationContext {
	ctx := &notificationv1.NotificationContext{
		Lineage: &notificationv1.LineageContext{
			RemediationRequest: rrName,
		},
		Execution: &notificationv1.ExecutionContext{
			TimeoutPhase: timeoutPhase,
		},
		Target: &notificationv1.TargetContext{
			TargetResource: fmt.Sprintf("%s/%s", target.Kind, target.Name),
		},
	}
	if phaseTimeout != "" {
		ctx.Execution.PhaseTimeout = phaseTimeout
	}
	return ctx
}

func (r *Reconciler) createPhaseTimeoutNotification(ctx context.Context, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase, timeout time.Duration) {
	logger := log.FromContext(ctx)

	// Defensive: Refresh RR to get latest status (including TimeoutTime)
	latest := &remediationv1.RemediationRequest{}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), latest); err != nil {
		logger.Error(err, "Failed to refresh RR for phase timeout notification")
		return
	}
	rr = latest // Use refreshed version

	// Kubernetes names must be lowercase RFC 1123
	phaseLower := strings.ToLower(string(phase))
	notificationName := fmt.Sprintf("phase-timeout-%s-%s", phaseLower, rr.Name)

	nr := &notificationv1.NotificationRequest{
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
			Priority: notificationv1.NotificationPriorityHigh,
			Severity: rr.Spec.Severity,
			Phase:    string(phase),
			Subject:  fmt.Sprintf("Phase Timeout: %s - %s", phase, rr.Spec.SignalName),
			Body: r.notificationCreator.BuildPhaseTimeoutBody(
				rr.Spec.SignalName,
				rr.Name,
				string(phase),
				timeout.String(),
				safeFormatTime(rr.Status.StartTime),
				safeFormatTime(rr.Status.TimeoutTime),
			),
			Context: buildTimeoutContext(rr.Name, string(phase), timeout.String(), rr.Spec.TargetResource),
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on phase timeout notification")
		return
	}

	// Create notification (non-blocking)
	if err := r.client.Create(ctx, nr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Phase timeout notification already exists (concurrent create), continuing",
				"notificationName", notificationName, "phase", phase)
		} else {
			logger.Error(err, "Failed to create phase timeout notification",
				"notificationName", notificationName,
				"phase", phase)
			return
		}
	}

	logger.Info("Created phase timeout notification",
		"notificationName", notificationName,
		"phase", phase,
		"timeout", timeout)

	// BR-ORCH-035 AC-4: Track timeout notification ref (non-blocking)
	ref := r.buildNotificationRef(ctx, notificationName, rr.Namespace)
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist timeout NT ref (non-critical)", "notification", notificationName)
	}
}
