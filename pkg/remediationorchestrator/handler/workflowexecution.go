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
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler/skip"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// WorkflowExecutionHandler handles WE status changes for Remediation Orchestrator.
// Reference: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking),
//
//	BR-ORCH-036 (manual review notification), DD-WE-004 (exponential backoff)
//
// REFACTOR-RO-002: Skip handlers extracted to pkg/remediationorchestrator/handler/skip/
type WorkflowExecutionHandler struct {
	client  client.Client
	scheme  *runtime.Scheme
	metrics *metrics.Metrics

	// Skip reason handlers (REFACTOR-RO-002)
	skipHandlers map[string]skip.Handler
}

// NewWorkflowExecutionHandler creates a new WorkflowExecutionHandler.
// REFACTOR-RO-002: Initializes skip handlers
func NewWorkflowExecutionHandler(c client.Client, s *runtime.Scheme, m *metrics.Metrics) *WorkflowExecutionHandler {
	h := &WorkflowExecutionHandler{
		client:  c,
		scheme:  s,
		metrics: m,
	}

	// Initialize skip handler context (REFACTOR-RO-002)
	skipCtx := &skip.Context{
		Client:              c,
		Metrics:             m,
		NotificationCreator: h, // WorkflowExecutionHandler implements the interface
	}

	// Initialize skip handlers (REFACTOR-RO-002)
	h.skipHandlers = map[string]skip.Handler{
		"ResourceBusy":            skip.NewResourceBusyHandler(skipCtx),
		"RecentlyRemediated":      skip.NewRecentlyRemediatedHandler(skipCtx),
		"ExhaustedRetries":        skip.NewExhaustedRetriesHandler(skipCtx),
		"PreviousExecutionFailed": skip.NewPreviousExecutionFailedHandler(skipCtx),
	}

	return h
}

// ========================================
// V1.0 TODO: FUNCTION DEPRECATED (DD-RO-002)
// HandleSkipped is part of the OLD routing flow (WE skips → reports to RO).
// In V1.0, RO makes routing decisions BEFORE creating WFE, so WFE never skips.
// This function will be REMOVED in Days 2-3 when new routing logic is implemented.
// ========================================
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
	)

	// V1.0: This code path will not execute (WFE never created if should be skipped)
	// Stubbed for Day 1 build compatibility
	logger.Info("V1.0: HandleSkipped called but WFE should never be in Skipped phase")
	return ctrl.Result{}, fmt.Errorf("V1.0: WFE should never be in Skipped phase (DD-RO-002)")
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
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		rr.Status.Message = "Workflow failed with unknown reason"
		return ctrl.Result{}, nil
	}

	if we.Status.FailureDetails.WasExecutionFailure {
		// EXECUTION FAILURE: Cluster state may be modified - NO auto-retry
		logger.Info("WE failed during execution - manual review required",
			"failedTask", we.Status.FailureDetails.FailedTaskName,
			"reason", we.Status.FailureDetails.Reason,
		)

		// Update RR status (DD-GATEWAY-011, BR-ORCH-038)
		// REFACTOR-RO-001: Using retry helper
		err := helpers.UpdateRemediationRequestStatus(ctx, h.client, h.metrics, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.OverallPhase = "Failed"
			rr.Status.RequiresManualReview = true
			rr.Status.Message = we.Status.FailureDetails.NaturalLanguageSummary
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to update RR status for execution failure")
			return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
		}

		// REFACTOR-RO-004: Create execution failure notification (BR-ORCH-036)
		// Execution failures require immediate operator attention (cluster state unknown)
		notificationName, err := h.CreateManualReviewNotification(ctx, rr, we, sp)
		if err != nil {
			logger.Error(err, "Failed to create execution failure notification")
			// Continue - notification is best-effort, don't block status update
		} else {
			logger.Info("Created execution failure notification",
				"notification", notificationName,
				"severity", "critical",
			)
		}

		// NO requeue - manual intervention required
		return ctrl.Result{}, nil
	}

	// PRE-EXECUTION FAILURE: May consider recovery (V1.1+ feature)
	// For V1.0, just mark as failed without requiring manual review
	logger.Info("WE failed during pre-execution - may consider recovery in future",
		"failedTask", we.Status.FailureDetails.FailedTaskName,
		"reason", we.Status.FailureDetails.Reason,
	)

	// Update RR status (DD-GATEWAY-011, BR-ORCH-038)
	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, h.client, h.metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		rr.Status.RequiresManualReview = false // Pre-execution failures are recoverable
		rr.Status.Message = we.Status.FailureDetails.NaturalLanguageSummary
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update RR status for pre-execution failure")
		return ctrl.Result{}, fmt.Errorf("failed to update RR status: %w", err)
	}

	// V1.0: No automatic recovery, just fail
	// V1.1+: Will call evaluateRecoveryOptions here
	return ctrl.Result{}, nil
}
// CalculateRequeueTime calculates requeue duration from NextAllowedExecution.
// REFACTOR-RO-003: Using centralized timeout constant for fallback
// Reference: DD-WE-004 (exponential backoff)
func (h *WorkflowExecutionHandler) CalculateRequeueTime(nextAllowed *metav1.Time) time.Duration {
	if nextAllowed == nil {
		return config.RequeueFallback // REFACTOR-RO-003
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

	// Update parent RR (DD-GATEWAY-011, BR-ORCH-038)
	// REFACTOR-RO-001: Using retry helper
	// Note: We need to fetch the parent RR first, then update it
	parentRR := &remediationv1.RemediationRequest{}
	if err := h.client.Get(ctx, client.ObjectKey{Name: parentRRName, Namespace: childRR.Namespace}, parentRR); err != nil {
		logger.Error(err, "Failed to fetch parent RR")
		return fmt.Errorf("failed to fetch parent RR: %w", err)
	}

	err := helpers.UpdateRemediationRequestStatus(ctx, h.client, h.metrics, parentRR, func(rr *remediationv1.RemediationRequest) error {
		// Update only RO-owned fields - Gateway fields preserved via refetch
		rr.Status.DuplicateCount++
		// Avoid duplicates in refs list
		alreadyTracked := false
		for _, ref := range rr.Status.DuplicateRefs {
			if ref == childRR.Name {
				alreadyTracked = true
				break
			}
		}
		if !alreadyTracked {
			rr.Status.DuplicateRefs = append(rr.Status.DuplicateRefs, childRR.Name)
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to update parent RR status with duplicate tracking")
		return fmt.Errorf("failed to update parent RR status: %w", err)
	}

	logger.Info("Tracked duplicate on parent RR",
		"duplicateCount", parentRR.Status.DuplicateCount,
	)
	return nil
}

// MapSkipReasonToSeverity maps skip reason to severity label per Notification team guidance.
// ExecutionFailure = critical (cluster state unknown, from HandleFailed)
// PreviousExecutionFailed = critical (cluster state unknown, from HandleSkipped)
// ExhaustedRetries = high (infrastructure issue, but state is known)
// Reference: BR-ORCH-036, REFACTOR-RO-004
func (h *WorkflowExecutionHandler) MapSkipReasonToSeverity(skipReason string) string {
	switch skipReason {
	case "ExecutionFailure", "PreviousExecutionFailed":
		return "critical"
	case "ExhaustedRetries":
		return "high"
	default:
		return "medium"
	}
}

// MapSkipReasonToPriority maps skip reason to NotificationPriority per Notification team guidance.
// Reference: BR-ORCH-036, REFACTOR-RO-004
func (h *WorkflowExecutionHandler) MapSkipReasonToPriority(skipReason string) notificationv1.NotificationPriority {
	switch skipReason {
	case "ExecutionFailure", "PreviousExecutionFailed":
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
	// V1.0: Determine reason from FailureDetails only (SkipDetails removed in DD-RO-002)
	reason := ""
	if we.Status.FailureDetails != nil {
		reason = "ExecutionFailure"
	}

	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"reason", reason,
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
	severity := h.MapSkipReasonToSeverity(reason)
	priority := h.MapSkipReasonToPriority(reason)

	// Build NotificationRequest for manual review
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "manual-review",
				"kubernaut.ai/severity":            severity,
				"kubernaut.ai/skip-reason":         reason,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeManualReview,
			Priority: priority,
			Subject:  fmt.Sprintf("Manual Review Required: %s - %s", rr.Spec.SignalName, reason),
			Body:     h.buildManualReviewBody(rr, we, sp, reason),
			Channels: []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}, // Default channels for manual review
			Metadata: map[string]string{
				"remediationRequest":   rr.Name,
				"workflowExecution":    we.Name,
				"skipReason":           reason,
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
// REFACTOR-RO-004: Updated to handle both skip and failure cases
func (h *WorkflowExecutionHandler) buildManualReviewBody(
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing,
	reason string,
) string {
	env := "unknown"
	if sp.Status.EnvironmentClassification != nil {
		env = sp.Status.EnvironmentClassification.Environment
	}

	// V1.0: Determine message from FailureDetails only (SkipDetails removed in DD-RO-002)
	message := ""
	if we.Status.FailureDetails != nil {
		message = we.Status.FailureDetails.NaturalLanguageSummary
	}

	return fmt.Sprintf(`Manual intervention required for remediation.

**Signal**: %s
**Severity**: %s
**Environment**: %s

**Reason**: %s
**Details**: %s

**Consecutive Failures**: %d

**Action Required**: Please investigate and manually resolve the issue.
`,
		rr.Spec.SignalName,
		rr.Spec.Severity,
		env,
		reason,
		message,
		we.Status.ConsecutiveFailures,
	)
}
