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

package handler

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecutionHandler handles WE status changes for Remediation Orchestrator.
// Reference: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking),
//            BR-ORCH-036 (manual review notification), DD-WE-004 (exponential backoff)
type WorkflowExecutionHandler struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionHandler creates a new WorkflowExecutionHandler.
func NewWorkflowExecutionHandler(c client.Client, s *runtime.Scheme) *WorkflowExecutionHandler {
	return &WorkflowExecutionHandler{
		client: c,
		scheme: s,
	}
}

// HandleSkipped handles WE Skipped phase per DD-WE-004 and BR-ORCH-032.
// Reference: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking), BR-ORCH-036 (manual review)
func (h *WorkflowExecutionHandler) HandleSkipped(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", we.Status.SkipDetails.Reason,
	)

	reason := we.Status.SkipDetails.Reason

	switch reason {
	case "ResourceBusy":
		// DUPLICATE: Another workflow running - requeue
		logger.Info("WE skipped: ResourceBusy - tracking as duplicate, requeueing")
		rr.Status.OverallPhase = "Skipped"
		rr.Status.SkipReason = reason
		if we.Status.SkipDetails.ConflictingWorkflow != nil {
			rr.Status.DuplicateOf = we.Status.SkipDetails.ConflictingWorkflow.Name
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case "RecentlyRemediated":
		// DUPLICATE: Cooldown active - requeue with fixed interval
		// Per WE Team Response Q6: RO should NOT calculate backoff, let WE re-evaluate
		logger.Info("WE skipped: RecentlyRemediated - tracking as duplicate, requeueing")
		rr.Status.OverallPhase = "Skipped"
		rr.Status.SkipReason = reason
		if we.Status.SkipDetails.RecentRemediation != nil {
			rr.Status.DuplicateOf = we.Status.SkipDetails.RecentRemediation.Name
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil

	case "ExhaustedRetries":
		// NOT A DUPLICATE: Manual review required
		logger.Info("WE skipped: ExhaustedRetries - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Retry limit exceeded - 5+ consecutive pre-execution failures")

	case "PreviousExecutionFailed":
		// NOT A DUPLICATE: Manual review required (cluster state unknown)
		logger.Info("WE skipped: PreviousExecutionFailed - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Previous execution failed during workflow run - cluster state may be inconsistent")

	default:
		logger.Error(nil, "Unknown skip reason", "reason", reason)
		return ctrl.Result{}, fmt.Errorf("unknown skip reason: %s", reason)
	}
}

// HandleFailed handles WE Failed phase per DD-WE-004 and BR-ORCH-032.
// Reference: BR-ORCH-032 (failure handling), DD-WE-004 (execution vs pre-execution failures)
func (h *WorkflowExecutionHandler) HandleFailed(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
	)

	if we.Status.FailureDetails == nil {
		logger.Error(nil, "WE Failed but FailureDetails is nil")
		rr.Status.OverallPhase = "Failed"
		rr.Status.Message = "Workflow failed with unknown reason"
		return ctrl.Result{}, nil
	}

	if we.Status.FailureDetails.WasExecutionFailure {
		// EXECUTION FAILURE: Cluster state may be modified - NO auto-retry
		logger.Info("WE failed during execution - manual review required",
			"failedTask", we.Status.FailureDetails.FailedTaskName,
			"reason", we.Status.FailureDetails.Reason,
		)

		rr.Status.OverallPhase = "Failed"
		rr.Status.RequiresManualReview = true
		rr.Status.Message = we.Status.FailureDetails.NaturalLanguageSummary

		// TODO: Create execution failure notification
		// This will be implemented in Day 7 (Escalation Manager)

		// NO requeue - manual intervention required
		return ctrl.Result{}, nil
	}

	// PRE-EXECUTION FAILURE: May consider recovery (V1.1+ feature)
	// For V1.0, just mark as failed without requiring manual review
	logger.Info("WE failed during pre-execution - may consider recovery in future",
		"failedTask", we.Status.FailureDetails.FailedTaskName,
		"reason", we.Status.FailureDetails.Reason,
	)

	rr.Status.OverallPhase = "Failed"
	rr.Status.RequiresManualReview = false // Pre-execution failures are recoverable
	rr.Status.Message = we.Status.FailureDetails.NaturalLanguageSummary

	// V1.0: No automatic recovery, just fail
	// V1.1+: Will call evaluateRecoveryOptions here
	return ctrl.Result{}, nil
}

// handleManualReviewRequired handles skip reasons requiring manual intervention.
// Reference: BR-ORCH-032 v1.1, BR-ORCH-036
func (h *WorkflowExecutionHandler) handleManualReviewRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
	skipReason string,
	message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Update RR status - FAILED, not Skipped (per BR-ORCH-032 v1.1)
	rr.Status.OverallPhase = "Failed"
	rr.Status.SkipReason = skipReason
	rr.Status.RequiresManualReview = true
	rr.Status.DuplicateOf = "" // NOT a duplicate
	rr.Status.Message = we.Status.SkipDetails.Message

	logger.Info("Manual review required",
		"skipReason", skipReason,
		"message", message,
	)

	// Create manual review notification (BR-ORCH-036)
	notificationName, err := h.CreateManualReviewNotification(ctx, rr, we, sp)
	if err != nil {
		logger.Error(err, "Failed to create manual review notification")
		// Continue even if notification fails - don't block the skip handling
	} else {
		logger.Info("Created manual review notification", "notification", notificationName)
	}

	// NO requeue - manual intervention required
	return ctrl.Result{}, nil
}

// CalculateRequeueTime calculates requeue duration from NextAllowedExecution.
// Reference: DD-WE-004 (exponential backoff)
func (h *WorkflowExecutionHandler) CalculateRequeueTime(nextAllowed *metav1.Time) time.Duration {
	if nextAllowed == nil {
		return 1 * time.Minute // Default fallback
	}
	duration := time.Until(nextAllowed.Time)
	if duration < 0 {
		return 0 // Already expired, requeue immediately
	}
	return duration
}

// TrackDuplicate tracks a duplicate RR on the parent (BR-ORCH-033).
// It updates the parent RR's DuplicateCount and DuplicateRefs.
func (h *WorkflowExecutionHandler) TrackDuplicate(
	ctx context.Context,
	childRR *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	parentRRName string,
) error {
	logger := log.FromContext(ctx).WithValues(
		"childRR", childRR.Name,
		"parentRR", parentRRName,
	)

	// Fetch the parent RR
	parentRR := &remediationv1.RemediationRequest{}
	if err := h.client.Get(ctx, client.ObjectKey{Name: parentRRName, Namespace: childRR.Namespace}, parentRR); err != nil {
		logger.Error(err, "Failed to fetch parent RR for duplicate tracking")
		return fmt.Errorf("failed to fetch parent RR: %w", err)
	}

	// Update duplicate count and refs
	parentRR.Status.DuplicateCount++
	parentRR.Status.DuplicateRefs = append(parentRR.Status.DuplicateRefs, childRR.Name)

	// Update the parent's status
	if err := h.client.Status().Update(ctx, parentRR); err != nil {
		logger.Error(err, "Failed to update parent RR status with duplicate tracking")
		return fmt.Errorf("failed to update parent RR status: %w", err)
	}

	logger.Info("Tracked duplicate on parent RR",
		"duplicateCount", parentRR.Status.DuplicateCount,
	)
	return nil
}

// MapSkipReasonToSeverity maps skip reason to severity label per Notification team guidance.
// PreviousExecutionFailed = critical (cluster state unknown)
// ExhaustedRetries = high (infrastructure issue, but state is known)
// Reference: BR-ORCH-036
func (h *WorkflowExecutionHandler) MapSkipReasonToSeverity(skipReason string) string {
	switch skipReason {
	case "PreviousExecutionFailed":
		return "critical"
	case "ExhaustedRetries":
		return "high"
	default:
		return "medium"
	}
}

// MapSkipReasonToPriority maps skip reason to NotificationPriority per Notification team guidance.
// Reference: BR-ORCH-036
func (h *WorkflowExecutionHandler) MapSkipReasonToPriority(skipReason string) notificationv1.NotificationPriority {
	switch skipReason {
	case "PreviousExecutionFailed":
		return notificationv1.NotificationPriorityCritical
	case "ExhaustedRetries":
		return notificationv1.NotificationPriorityHigh
	default:
		return notificationv1.NotificationPriorityMedium
	}
}

// CreateManualReviewNotification creates a NotificationRequest for manual review scenarios.
// Reference: BR-ORCH-036 (manual review notification)
func (h *WorkflowExecutionHandler) CreateManualReviewNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", we.Status.SkipDetails.Reason,
	)

	// Generate deterministic name
	name := fmt.Sprintf("nr-manual-review-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Manual review notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Get severity and priority from skip reason
	severity := h.MapSkipReasonToSeverity(we.Status.SkipDetails.Reason)
	priority := h.MapSkipReasonToPriority(we.Status.SkipDetails.Reason)

	// Build NotificationRequest for manual review
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "manual-review",
				"kubernaut.ai/severity":            severity,
				"kubernaut.ai/skip-reason":         we.Status.SkipDetails.Reason,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeManualReview,
			Priority: priority,
			Subject:  fmt.Sprintf("Manual Review Required: %s - %s", rr.Spec.SignalName, we.Status.SkipDetails.Reason),
			Body:     h.buildManualReviewBody(rr, we, sp),
			Channels: []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}, // Default channels for manual review
			Metadata: map[string]string{
				"remediationRequest":   rr.Name,
				"workflowExecution":    we.Name,
				"skipReason":           we.Status.SkipDetails.Reason,
				"skipMessage":          we.Status.SkipDetails.Message,
				"consecutiveFailures":  fmt.Sprintf("%d", we.Status.ConsecutiveFailures),
				"requiresManualReview": "true",
			},
		},
	}

	// Set owner reference for cascade deletion
	if err := controllerutil.SetControllerReference(rr, nr, h.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the notification
	if err := h.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create manual review notification")
		return "", fmt.Errorf("failed to create manual review notification: %w", err)
	}

	logger.Info("Created manual review notification", "name", name, "severity", severity)
	return name, nil
}

// buildManualReviewBody builds the notification body for manual review.
func (h *WorkflowExecutionHandler) buildManualReviewBody(
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
) string {
	env := "unknown"
	if sp.Status.EnvironmentClassification != nil {
		env = sp.Status.EnvironmentClassification.Environment
	}

	return fmt.Sprintf(`Manual intervention required for remediation.

**Signal**: %s
**Severity**: %s
**Environment**: %s

**Skip Reason**: %s
**Details**: %s

**Consecutive Failures**: %d

**Action Required**: Please investigate and manually resolve the issue.
`,
		rr.Spec.SignalName,
		rr.Spec.Severity,
		env,
		we.Status.SkipDetails.Reason,
		we.Status.SkipDetails.Message,
		we.Status.ConsecutiveFailures,
	)
}


