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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

const processingRequeueInterval = 10 * time.Second

// ProcessingHandler encapsulates the reconcile logic for the Processing phase.
// It monitors the SignalProcessing CRD, creates AIAnalysis on completion,
// and transitions to Analyzing.
//
// Internalizes logic from:
//   - reconciler.handleProcessingPhase
//   - handler.SignalProcessingHandler.HandleStatus
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type ProcessingHandler struct {
	client            client.Client
	aiAnalysisCreator *creator.AIAnalysisCreator
	statusManager     *status.Manager
	metrics           *metrics.Metrics
}

// NewProcessingHandler creates a new ProcessingHandler with all required dependencies.
func NewProcessingHandler(
	c client.Client,
	aic *creator.AIAnalysisCreator,
	sm *status.Manager,
	m *metrics.Metrics,
) *ProcessingHandler {
	return &ProcessingHandler{
		client:            c,
		aiAnalysisCreator: aic,
		statusManager:     sm,
		metrics:           m,
	}
}

// Phase returns the phase this handler manages.
func (h *ProcessingHandler) Phase() phase.Phase {
	return phase.Processing
}

// Handle processes a RemediationRequest in the Processing phase.
// It monitors the associated SignalProcessing CRD, creates AIAnalysis on
// SP completion, and returns a TransitionIntent to advance to Analyzing.
func (h *ProcessingHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.SignalProcessingRef == nil {
		logger.Error(nil, "Processing phase but no SignalProcessingRef - corrupted state")
		return phase.Fail(remediationv1.FailurePhaseSignalProcessing,
			fmt.Errorf("SignalProcessing not found"),
			"corrupted state: no SignalProcessingRef"), nil
	}

	sp := &signalprocessingv1.SignalProcessing{}
	if err := h.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.SignalProcessingRef.Name,
		Namespace: rr.Status.SignalProcessingRef.Namespace,
	}, sp); err != nil {
		logger.Error(err, "Failed to fetch SignalProcessing CRD")
		return phase.Requeue(config.RequeueGenericError, "SP fetch failed"), nil
	}

	switch sp.Status.Phase {
	case signalprocessingv1.PhaseCompleted:
		return h.handleSPCompleted(ctx, rr, sp)

	case signalprocessingv1.PhaseFailed:
		return h.handleSPFailed(ctx, rr)

	case signalprocessingv1.PhasePending, "Processing":
		logger.V(1).Info("SignalProcessing still in progress, requeuing")
		return phase.Requeue(processingRequeueInterval, "SP in progress"), nil

	case "":
		logger.V(1).Info("SignalProcessing has empty phase, waiting for controller to process")
		return phase.Requeue(processingRequeueInterval, "SP phase empty"), nil

	default:
		logger.Info("SignalProcessing has unknown phase, requeuing", "phase", sp.Status.Phase)
		return phase.Requeue(processingRequeueInterval, "SP unknown phase"), nil
	}
}

// handleSPCompleted creates AIAnalysis on SP completion and returns an Advance(Analyzing) intent.
func (h *ProcessingHandler) handleSPCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, sp *signalprocessingv1.SignalProcessing) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("SignalProcessing completed, creating AIAnalysis")

	aiName, err := h.aiAnalysisCreator.Create(ctx, rr, sp)
	if err != nil {
		logger.Error(err, "Failed to create AIAnalysis CRD")
		if updateErr := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetAIAnalysisReady(rr, false,
				fmt.Sprintf("Failed to create AIAnalysis: %v", err), h.metrics)
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update AIAnalysisReady condition")
		}
		return phase.Requeue(config.RequeueGenericError, "AI creation failed"), nil
	}
	logger.Info("Created AIAnalysis CRD", "aiName", aiName)

	h.metrics.ChildCRDCreationsTotal.WithLabelValues("AIAnalysis", rr.Namespace).Inc()

	err = helpers.UpdateRemediationRequestStatus(ctx, h.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{
			APIVersion: aianalysisv1.GroupVersion.String(),
			Kind:       "AIAnalysis",
			Name:       aiName,
			Namespace:  rr.Namespace,
		}
		remediationrequest.SetAIAnalysisReady(rr, true, fmt.Sprintf("AIAnalysis CRD %s created successfully", aiName), h.metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to set AIAnalysisRef in status")
		return phase.Requeue(config.RequeueGenericError, "AI ref update failed"), nil
	}
	logger.V(1).Info("Set AIAnalysisRef in status", "aiName", aiName)

	if err := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
		remediationrequest.SetSignalProcessingComplete(rr, true,
			remediationrequest.ReasonSignalProcessingSucceeded,
			"SignalProcessing completed successfully", h.metrics)
		return nil
	}); err != nil {
		logger.Error(err, "Failed to update SignalProcessingComplete condition")
	}

	return phase.Advance(phase.Analyzing, "AIAnalysis created, SP completed"), nil
}

// handleSPFailed sets the SP failure condition and returns a Failed intent.
func (h *ProcessingHandler) handleSPFailed(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("SignalProcessing failed, transitioning to Failed")

	if err := h.statusManager.AtomicStatusUpdate(ctx, rr, func() error {
		remediationrequest.SetSignalProcessingComplete(rr, false,
			remediationrequest.ReasonSignalProcessingFailed,
			"SignalProcessing failed", h.metrics)
		return nil
	}); err != nil {
		logger.Error(err, "Failed to update SignalProcessingComplete condition")
	}

	return phase.Fail(remediationv1.FailurePhaseSignalProcessing,
		fmt.Errorf("SignalProcessing failed"),
		"SignalProcessing failed"), nil
}
