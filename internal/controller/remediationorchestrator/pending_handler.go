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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	k8serrors "github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
)

// PendingHandler encapsulates the reconcile logic for the Pending phase.
// It checks routing conditions, creates SignalProcessing, and transitions to Processing.
//
// Internalizes logic from reconciler.handlePendingPhase.
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type PendingHandler struct {
	client        client.Client
	routingEngine routing.Engine
	spCreator     *creator.SignalProcessingCreator
	statusManager *status.Manager
	metrics       *metrics.Metrics
}

// NewPendingHandler creates a new PendingHandler with all required dependencies.
func NewPendingHandler(
	c client.Client,
	re routing.Engine,
	sp *creator.SignalProcessingCreator,
	sm *status.Manager,
	m *metrics.Metrics,
) *PendingHandler {
	return &PendingHandler{
		client:        c,
		routingEngine: re,
		spCreator:     sp,
		statusManager: sm,
		metrics:       m,
	}
}

// Phase returns the phase this handler manages.
func (h *PendingHandler) Phase() phase.Phase {
	return phase.Pending
}

// Handle processes a RemediationRequest in the Pending phase.
// It checks pre-analysis routing conditions, creates SignalProcessing, and
// returns a TransitionIntent to advance to Processing.
func (h *PendingHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("Handling Pending phase - checking routing conditions")

	blocked, err := h.routingEngine.CheckPreAnalysisConditions(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to check routing conditions")
		return phase.Requeue(config.RequeueGenericError, "routing check failed"), nil
	}

	if blocked != nil {
		logger.Info("Routing blocked - will not create SignalProcessing",
			"reason", blocked.Reason,
			"message", blocked.Message,
			"requeueAfter", blocked.RequeueAfter)
		return phase.Block(&phase.BlockMeta{
			FromPhase:                 phase.Pending,
			Reason:                    blocked.Reason,
			Message:                   blocked.Message,
			RequeueAfter:              blocked.RequeueAfter,
			BlockedUntil:              blocked.BlockedUntil,
			BlockingWorkflowExecution: blocked.BlockingWorkflowExecution,
			DuplicateOf:               blocked.DuplicateOf,
		}), nil
	}

	logger.Info("Routing checks passed, creating SignalProcessing")

	spName, err := h.spCreator.Create(ctx, rr)
	if err != nil {
		if k8serrors.IsNamespaceTerminating(err) {
			logger.V(1).Info("Namespace is terminating, skipping reconciliation",
				"namespace", rr.Namespace, "reason", "async_cleanup")
			return phase.NoOp("namespace terminating"), nil
		}

		logger.Error(err, "Failed to create SignalProcessing CRD")
		if updateErr := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetSignalProcessingReady(rr, false,
				fmt.Sprintf("Failed to create SignalProcessing: %v", err), h.metrics)
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update SignalProcessingReady condition")
		}
		return phase.Requeue(config.RequeueGenericError, "SP creation failed"), nil
	}
	logger.Info("Created SignalProcessing CRD", "spName", spName)

	h.metrics.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", rr.Namespace).Inc()

	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.SignalProcessingRef = &corev1.ObjectReference{
			APIVersion: signalprocessingv1.GroupVersion.String(),
			Kind:       "SignalProcessing",
			Name:       spName,
			Namespace:  rr.Namespace,
		}
		remediationrequest.SetSignalProcessingReady(rr, true, fmt.Sprintf("SignalProcessing CRD %s created successfully", spName), h.metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to set SignalProcessingRef in status")
		return phase.Requeue(config.RequeueGenericError, "SP ref update failed"), nil
	}
	logger.V(1).Info("Set SignalProcessingRef in status", "spName", spName)

	return phase.Advance(phase.Processing, "SignalProcessing created"), nil
}
