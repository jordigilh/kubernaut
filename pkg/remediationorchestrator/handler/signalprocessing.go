package handler

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SignalProcessingHandler handles SignalProcessing CRD status updates.
// This handler encapsulates all SignalProcessing-specific logic for consistency with other service handlers.
//
// Reference:
// - BR-ORCH-025: SignalProcessing orchestration
// - Handler Consistency Refactoring (2026-01-22): Extract inline logic for maintainability
type SignalProcessingHandler struct {
	client            client.Client
	scheme            *runtime.Scheme
	transitionToPhase func(context.Context, *remediationv1.RemediationRequest, remediationv1.RemediationPhase) (ctrl.Result, error)
}

// NewSignalProcessingHandler creates a new SignalProcessingHandler.
//
// Parameters:
// - c: Kubernetes client for API operations
// - s: Scheme for runtime type information
// - ttp: Callback to reconciler's transitionPhase() for audit emission
//
// The transitionToPhase callback allows the handler to trigger phase transitions
// while preserving the reconciler's responsibility for audit event emission.
func NewSignalProcessingHandler(
	c client.Client,
	s *runtime.Scheme,
	ttp func(context.Context, *remediationv1.RemediationRequest, remediationv1.RemediationPhase) (ctrl.Result, error),
) *SignalProcessingHandler {
	return &SignalProcessingHandler{
		client:            c,
		scheme:            s,
		transitionToPhase: ttp,
	}
}

// HandleStatus processes SignalProcessing CRD status updates and triggers appropriate RemediationRequest transitions.
//
// Flow:
// 1. Check SignalProcessing phase
// 2. If Completed → transition RR to Analyzing phase
// 3. If Failed → handle failure (future implementation)
// 4. If Pending/Processing → requeue and wait
//
// Reference:
// - BR-ORCH-025: Processing phase waits for SignalProcessing completion
// - DD-AUDIT-003: Phase transitions emit audit events via callback
//
// Returns:
// - ctrl.Result: Requeue configuration
// - error: Nil on success, error on failure
func (h *SignalProcessingHandler) HandleStatus(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"signalProcessing", sp.Name,
		"spPhase", sp.Status.Phase,
	)

	switch sp.Status.Phase {
	case signalprocessingv1.PhaseCompleted:
		logger.Info("SignalProcessing completed, transitioning to Analyzing")
		// Delegate to reconciler's transitionPhase() for audit emission (DD-AUDIT-003)
		return h.transitionToPhase(ctx, rr, phase.Analyzing)

	case signalprocessingv1.PhaseFailed:
		// SignalProcessing failed - this is rare but possible (e.g., validation errors)
		// The reconciler's handleProcessingPhase will call transitionToFailed before this handler
		logger.Info("SignalProcessing failed, already transitioned to Failed")
		// Return without requeue - terminal state
		return ctrl.Result{}, nil

	case signalprocessingv1.PhasePending, "Processing":
		// Still in progress, requeue to check status again
		logger.V(1).Info("SignalProcessing still in progress, requeuing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case "":
		// Empty phase means SP was just created but controller hasn't set phase yet
		// Requeue to check status again after controller processes it
		logger.V(1).Info("SignalProcessing has empty phase, waiting for controller to process")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		// Unknown phase - log warning and requeue
		logger.Info("SignalProcessing has unknown phase, requeuing",
			"phase", sp.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// getLogger returns a logger with handler-specific context.
func (h *SignalProcessingHandler) getLogger(ctx context.Context, rr *remediationv1.RemediationRequest) logr.Logger {
	return log.FromContext(ctx).WithValues(
		"handler", "SignalProcessingHandler",
		"remediationRequest", rr.Name,
	)
}
