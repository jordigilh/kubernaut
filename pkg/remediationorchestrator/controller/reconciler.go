// Package controller provides the Kubernetes controller implementation
// for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-025: Core orchestration workflow
// - BR-ORCH-026: Approval orchestration
// - BR-ORCH-027, BR-ORCH-028: Timeout management
// - BR-ORCH-032: Skipped phase handling
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// Reconciler reconciles RemediationRequest objects.
// It implements the central orchestration logic for the remediation lifecycle.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config remediationorchestrator.OrchestratorConfig

	// Child CRD creators
	spCreator *creator.SignalProcessingCreator
	aiCreator *creator.AIAnalysisCreator
}

// NewReconciler creates a new Reconciler with the given dependencies.
func NewReconciler(c client.Client, scheme *runtime.Scheme, config remediationorchestrator.OrchestratorConfig) *Reconciler {
	return &Reconciler{
		Client:    c,
		Scheme:    scheme,
		Config:    config,
		spCreator: creator.NewSignalProcessingCreator(c, scheme),
		aiCreator: creator.NewAIAnalysisCreator(c, scheme),
	}
}

// Reconcile implements the reconciliation loop for RemediationRequest.
// It handles phase transitions, child CRD creation, and status aggregation.
//
// +kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests/finalizers,verbs=update
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	defer func() {
		logger.V(1).Info("Reconciliation completed",
			"duration", time.Since(startTime).String())
	}()

	// 1. FETCH: Get the RemediationRequest
	rr := &remediationv1.RemediationRequest{}
	if err := r.Get(ctx, req.NamespacedName, rr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("RemediationRequest not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get RemediationRequest")
		return ctrl.Result{}, err
	}

	logger = logger.WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"phase", rr.Status.OverallPhase,
	)

	// 2. CHECK TERMINAL: Don't process terminal states
	if r.isTerminalPhase(rr.Status.OverallPhase) {
		logger.Info("RemediationRequest in terminal state, skipping")
		return ctrl.Result{}, nil
	}

	// 3. INITIALIZE: Set initial status if empty
	if rr.Status.OverallPhase == "" {
		return r.initializeStatus(ctx, rr)
	}

	// 4. PROCESS: Handle phase-specific logic
	return r.processPhase(ctx, rr)
}

// initializeStatus sets the initial status for a new RemediationRequest.
func (r *Reconciler) initializeStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Initializing RemediationRequest status")

	now := metav1.Now()
	rr.Status.OverallPhase = string(phase.Pending)
	rr.Status.StartTime = &now

	if err := r.Status().Update(ctx, rr); err != nil {
		logger.Error(err, "Failed to initialize status")
		return ctrl.Result{}, err
	}

	logger.Info("Status initialized to Pending")
	return ctrl.Result{Requeue: true}, nil
}

// processPhase handles the current phase and determines next actions.
func (r *Reconciler) processPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	currentPhase := phase.Phase(rr.Status.OverallPhase)

	switch currentPhase {
	case phase.Pending:
		return r.handlePendingPhase(ctx, rr)
	case phase.Processing:
		return r.handleProcessingPhase(ctx, rr)
	case phase.Analyzing:
		return r.handleAnalyzingPhase(ctx, rr)
	case phase.AwaitingApproval:
		return r.handleAwaitingApprovalPhase(ctx, rr)
	case phase.Executing:
		return r.handleExecutingPhase(ctx, rr)
	default:
		logger.Error(nil, "Unknown phase", "phase", currentPhase)
		return ctrl.Result{}, fmt.Errorf("unknown phase: %s", currentPhase)
	}
}

// handlePendingPhase transitions from Pending to Processing.
// This creates the SignalProcessing CRD.
func (r *Reconciler) handlePendingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling Pending phase, creating SignalProcessing CRD")

	// Create SignalProcessing CRD
	spName, err := r.spCreator.Create(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to create SignalProcessing CRD")
		return ctrl.Result{}, err
	}

	// Update status with reference to SignalProcessing
	rr.Status.OverallPhase = string(phase.Processing)
	rr.Status.RemediationProcessingRef = &corev1.ObjectReference{
		APIVersion: signalprocessingv1.GroupVersion.String(),
		Kind:       "SignalProcessing",
		Name:       spName,
		Namespace:  rr.Namespace,
	}

	if err := r.Status().Update(ctx, rr); err != nil {
		logger.Error(err, "Failed to update status to Processing")
		return ctrl.Result{}, err
	}

	logger.Info("Transitioned to Processing phase", "signalProcessing", spName)
	return ctrl.Result{Requeue: true}, nil
}

// handleProcessingPhase handles the Processing phase.
// This monitors SignalProcessing CRD and transitions to Analyzing when complete.
func (r *Reconciler) handleProcessingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handling Processing phase")

	// Get SignalProcessing CRD reference
	if rr.Status.RemediationProcessingRef == nil {
		// This shouldn't happen - recreate if missing
		logger.Info("SignalProcessing reference missing, recreating")
		return r.handlePendingPhase(ctx, rr)
	}

	// Fetch SignalProcessing status
	sp := &signalprocessingv1.SignalProcessing{}
	spKey := client.ObjectKey{
		Name:      rr.Status.RemediationProcessingRef.Name,
		Namespace: rr.Namespace,
	}
	if err := r.Get(ctx, spKey, sp); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SignalProcessing CRD not found, recreating")
			return r.handlePendingPhase(ctx, rr)
		}
		return ctrl.Result{}, err
	}

	// Check SignalProcessing status
	switch sp.Status.Phase {
	case "Completed":
		logger.Info("SignalProcessing completed, transitioning to Analyzing")
		return r.transitionToAnalyzing(ctx, rr, sp)
	case "Failed":
		logger.Info("SignalProcessing failed, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, "signal_processing", "SignalProcessing failed")
	default:
		// Still in progress, requeue
		logger.V(1).Info("SignalProcessing still in progress", "phase", sp.Status.Phase)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// transitionToAnalyzing creates AIAnalysis CRD and updates status.
func (r *Reconciler) transitionToAnalyzing(ctx context.Context, rr *remediationv1.RemediationRequest, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create AIAnalysis CRD
	aiName, err := r.aiCreator.Create(ctx, rr, sp)
	if err != nil {
		logger.Error(err, "Failed to create AIAnalysis CRD")
		return ctrl.Result{}, err
	}

	// Update status
	rr.Status.OverallPhase = string(phase.Analyzing)
	rr.Status.AIAnalysisRef = &corev1.ObjectReference{
		APIVersion: aianalysisv1.GroupVersion.String(),
		Kind:       "AIAnalysis",
		Name:       aiName,
		Namespace:  rr.Namespace,
	}

	if err := r.Status().Update(ctx, rr); err != nil {
		logger.Error(err, "Failed to update status to Analyzing")
		return ctrl.Result{}, err
	}

	logger.Info("Transitioned to Analyzing phase", "aiAnalysis", aiName)
	return ctrl.Result{Requeue: true}, nil
}

// transitionToFailed updates status to Failed with phase and reason.
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase, reason string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rr.Status.OverallPhase = string(phase.Failed)
	rr.Status.FailurePhase = &failurePhase
	rr.Status.FailureReason = &reason
	now := metav1.Now()
	rr.Status.CompletedAt = &now

	if err := r.Status().Update(ctx, rr); err != nil {
		logger.Error(err, "Failed to update status to Failed")
		return ctrl.Result{}, err
	}

	logger.Info("Transitioned to Failed phase", "failurePhase", failurePhase, "reason", reason)
	return ctrl.Result{}, nil
}

// handleAnalyzingPhase handles the Analyzing phase.
// This monitors AIAnalysis CRD and transitions based on approval requirements.
func (r *Reconciler) handleAnalyzingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handling Analyzing phase")

	// TODO: Day 2-7 implementation
	// - Check AIAnalysis CRD status
	// - Create AIAnalysis if not exists
	// - Check if approval is required (BR-ORCH-026)
	// - Transition to AwaitingApproval or Executing

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleAwaitingApprovalPhase handles the AwaitingApproval phase.
// This creates approval notifications and waits for approval.
// Reference: BR-ORCH-001, BR-ORCH-026
func (r *Reconciler) handleAwaitingApprovalPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handling AwaitingApproval phase")

	// TODO: Day 2-7 implementation
	// - Create NotificationRequest for approval (BR-ORCH-001)
	// - Monitor for approval response
	// - Transition to Executing on approval
	// - Transition to Failed on rejection

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// handleExecutingPhase handles the Executing phase.
// This monitors WorkflowExecution CRD and handles completion/failure/skip.
// Reference: BR-ORCH-032 for Skipped handling
func (r *Reconciler) handleExecutingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Handling Executing phase")

	// TODO: Day 2-7 implementation
	// - Check WorkflowExecution CRD status
	// - Create WorkflowExecution if not exists
	// - Handle Skipped status (BR-ORCH-032)
	// - Transition to Completed, Failed, or Skipped

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// isTerminalPhase returns true if the phase is a terminal state.
func (r *Reconciler) isTerminalPhase(p string) bool {
	return phase.IsTerminal(phase.Phase(p))
}
