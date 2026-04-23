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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// WorkflowExecutionHandler handles WorkflowExecution CRD status updates.
// This handler encapsulates all WorkflowExecution-specific logic for consistency with other service handlers.
//
// Reference:
// - BR-ORCH-025: WorkflowExecution orchestration
// - BR-ORCH-032: Failure handling
// - DD-PERF-001: Atomic status updates with conditions
// - Handler Consistency Refactoring (2026-01-22): Extract inline logic for maintainability
//
// Historical Note:
// Previous implementation had HandleSkipped() and HandleFailed() methods that were never called by the reconciler.
// This refactored version extracts the actual inline logic from handleExecutingPhase() into a dedicated handler
// for consistency with AIAnalysisHandler and SignalProcessingHandler patterns.
type WorkflowExecutionHandler struct {
	client              client.Client
	scheme              *runtime.Scheme
	metrics             *metrics.Metrics
	notificationCreator *creator.NotificationCreator
	Recorder            record.EventRecorder
	transitionToFailed  func(context.Context, *remediationv1.RemediationRequest, remediationv1.FailurePhase, error) (ctrl.Result, error)
	transitionToVerifying func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error)
}

// NewWorkflowExecutionHandler creates a new WorkflowExecutionHandler.
func NewWorkflowExecutionHandler(
	c client.Client,
	s *runtime.Scheme,
	nc *creator.NotificationCreator,
	m *metrics.Metrics,
	rec record.EventRecorder,
	ttf func(context.Context, *remediationv1.RemediationRequest, remediationv1.FailurePhase, error) (ctrl.Result, error),
	ttv func(context.Context, *remediationv1.RemediationRequest, string) (ctrl.Result, error),
) *WorkflowExecutionHandler {
	return &WorkflowExecutionHandler{
		client:              c,
		scheme:              s,
		notificationCreator: nc,
		metrics:             m,
		Recorder:            rec,
		transitionToFailed:  ttf,
		transitionToVerifying: ttv,
	}
}

// HandleStatus processes WorkflowExecution CRD status updates and triggers appropriate RemediationRequest transitions.
//
// Flow:
// 1. Check WorkflowExecution phase
// 2. If Completed → set WorkflowExecutionComplete condition + transition RR to Verifying (#280)
// 3. If Failed → set WorkflowExecutionComplete condition (false) + transition RR to Failed
// 4. If empty phase → handle as missing/deleted WE (error case)
// 5. If Pending/Running → requeue and wait
//
// Reference:
// - BR-ORCH-025: Executing phase waits for WorkflowExecution completion
// - DD-PERF-001: Atomic status updates with conditions
// - DD-AUDIT-003: Terminal state transitions emit audit events via callbacks
//
// Returns:
// - ctrl.Result: Requeue configuration
// - error: Nil on success, error on terminal state transitions
func (h *WorkflowExecutionHandler) HandleStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"wePhase", we.Status.Phase,
	)

	switch we.Status.Phase {
	case workflowexecutionv1.PhaseCompleted:
		logger.Info("WorkflowExecution completed, transitioning to Verifying")

		// Note: WorkflowExecutionComplete condition is set by the reconciler
		// via DD-PERF-001 atomic status update before calling transitionToVerifying.
		// This handler focuses on phase transition logic only.

		// Delegate to reconciler's transitionToVerifying() for audit emission (DD-AUDIT-003)
		return h.transitionToVerifying(ctx, rr, "Remediated") // CRD enum: Remediated, NoActionRequired, ManualReviewRequired

	case workflowexecutionv1.PhaseFailed:
		logger.Info("WorkflowExecution failed, transitioning to Failed")

		// GAP-3 / #807: Create ManualReview NR before terminal transition (BR-ORCH-036).
		nrName := fmt.Sprintf("nr-manual-review-%s", rr.Name)
		if !hasNotificationRef(rr, nrName) {
			failureMsg := "WorkflowExecution failed"
			if we.Status.FailureReason != "" {
				failureMsg = we.Status.FailureReason
			}
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceWorkflowExecution,
				Reason:  "ExecutionFailure",
				Message: failureMsg,
			}
			notifName, notifErr := h.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create manual review notification for WFE failure")
			} else {
				logger.Info("Created manual review notification for WFE failure", "notification", notifName)
				ref := h.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist WFE failure NR ref (non-critical)", "notification", notifName)
				}
				if h.Recorder != nil {
					h.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Manual review notification created: %s", notifName))
				}
			}
		}

		return h.transitionToFailed(ctx, rr, remediationv1.FailurePhaseWorkflowExecution, fmt.Errorf("WorkflowExecution failed"))

	case "":
		// Empty phase means WE was just created but controller hasn't set phase yet
		// Requeue to check status again after controller processes it
		logger.V(1).Info("WorkflowExecution has empty phase, waiting for controller to process")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case workflowexecutionv1.PhasePending, workflowexecutionv1.PhaseRunning:
		// Still in progress, requeue to check status again
		logger.V(1).Info("WorkflowExecution still in progress, requeuing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		// Unknown phase - log warning and requeue
		// WorkflowExecution controller might have introduced a new phase
		logger.Info("WorkflowExecution has unknown phase, requeuing",
			"phase", we.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

func hasNotificationRef(rr *remediationv1.RemediationRequest, name string) bool {
	for i := range rr.Status.NotificationRequestRefs {
		if rr.Status.NotificationRequestRefs[i].Name == name {
			return true
		}
	}
	return false
}

func (h *WorkflowExecutionHandler) buildNotificationRef(ctx context.Context, name, namespace string) corev1.ObjectReference {
	ref := corev1.ObjectReference{
		Kind:       "NotificationRequest",
		Name:       name,
		Namespace:  namespace,
		APIVersion: "notification.kubernaut.ai/v1alpha1",
	}
	nr := &notificationv1.NotificationRequest{}
	if err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, nr); err == nil {
		ref.UID = nr.UID
	}
	return ref
}

