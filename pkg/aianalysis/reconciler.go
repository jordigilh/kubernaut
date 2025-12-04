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

package aianalysis

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// ========================================
// RECONCILER (BR-AI-001, DD-CONTRACT-002)
// 📋 Design Decision: DD-CONTRACT-002 | ✅ Self-Contained CRD Pattern
// See: docs/architecture/DESIGN_DECISIONS.md
// ========================================

// Reconciler reconciles an AIAnalysis object.
// It implements the controller-runtime Reconciler interface and routes
// to phase-specific handlers based on the current status.phase.
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder

	// Phase handlers (injected for testability)
	ValidatingHandler   PhaseHandler
	InvestigatingHandler PhaseHandler
	AnalyzingHandler    PhaseHandler
	RecommendingHandler PhaseHandler
}

// ========================================
// RETRY CONFIGURATION (APPENDIX_B)
// ========================================

const (
	// TransientRetryDelay is the initial retry delay for transient errors.
	TransientRetryDelay = 30 * time.Second

	// MaxTransientRetries is the maximum number of retries for transient errors.
	MaxTransientRetries = 5

	// PermanentFailureReason is the condition reason for permanent failures.
	PermanentFailureReason = "PermanentFailure"
)

// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the main reconciliation loop for AIAnalysis CRDs.
// It routes to phase-specific handlers based on status.phase.
//
// Error Categories (per APPENDIX_B):
// - Category A: CRD deleted during reconciliation → return nil (no requeue)
// - Category B: Transient errors → requeue with exponential backoff
// - Category C: Permanent errors → update status to Failed, emit event
// - Category D: Validation errors → update status to Failed (no retry)
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", req.NamespacedName)
	log.V(1).Info("Starting reconciliation")

	// Fetch the AIAnalysis instance
	analysis := &aianalysisv1.AIAnalysis{}
	if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
		if apierrors.IsNotFound(err) {
			// Category A: CRD deleted during reconciliation
			log.V(1).Info("AIAnalysis not found, likely deleted")
			return ctrl.Result{}, nil
		}
		// Transient error fetching CRD
		log.Error(err, "Failed to fetch AIAnalysis")
		return ctrl.Result{}, err
	}

	// Skip if already completed or failed
	if analysis.Status.Phase == PhaseCompleted || analysis.Status.Phase == PhaseFailed {
		log.V(1).Info("AIAnalysis already in terminal phase", "phase", analysis.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Initialize status if empty (new CRD)
	if analysis.Status.Phase == "" {
		return r.initializeStatus(ctx, analysis)
	}

	// Route to appropriate phase handler
	handler := r.getHandler(analysis.Status.Phase)
	if handler == nil {
		log.Error(nil, "Unknown phase", "phase", analysis.Status.Phase)
		return r.handlePermanentError(ctx, analysis, "UnknownPhase", "Unknown phase: "+analysis.Status.Phase)
	}

	// Execute phase handler
	log.V(1).Info("Executing phase handler", "phase", analysis.Status.Phase, "handler", handler.Name())
	result, err := handler.Handle(ctx, analysis)

	// Handle errors by category
	if err != nil {
		return r.handleError(ctx, analysis, err)
	}

	return result, nil
}

// initializeStatus sets the initial status for a new AIAnalysis CRD.
func (r *Reconciler) initializeStatus(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", client.ObjectKeyFromObject(analysis))
	log.Info("Initializing AIAnalysis status")

	now := metav1.Now()
	analysis.Status.Phase = PhaseValidating
	analysis.Status.Message = "Starting validation"
	analysis.Status.StartedAt = &now

	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to initialize status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(analysis, "Normal", "Initialized", "AIAnalysis initialized, starting validation")
	return ctrl.Result{Requeue: true}, nil
}

// getHandler returns the appropriate handler for the given phase.
func (r *Reconciler) getHandler(phase string) PhaseHandler {
	switch phase {
	case PhaseValidating:
		return r.ValidatingHandler
	case PhaseInvestigating:
		return r.InvestigatingHandler
	case PhaseAnalyzing:
		return r.AnalyzingHandler
	case PhaseRecommending:
		return r.RecommendingHandler
	default:
		return nil
	}
}

// handleError categorizes and handles errors from phase handlers.
func (r *Reconciler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", client.ObjectKeyFromObject(analysis))

	// Check error type
	var transientErr *TransientError
	var permanentErr *PermanentError
	var validationErr *ValidationError

	switch {
	case errors.As(err, &transientErr):
		// Category B: Transient error - requeue with delay
		log.Info("Transient error, will retry", "error", err.Error())
		r.Recorder.Event(analysis, "Warning", "TransientError", transientErr.Message)
		return ctrl.Result{RequeueAfter: TransientRetryDelay}, nil

	case errors.As(err, &permanentErr):
		// Category C: Permanent error - fail without retry
		log.Error(err, "Permanent error", "reason", permanentErr.Reason)
		return r.handlePermanentError(ctx, analysis, permanentErr.Reason, permanentErr.Message)

	case errors.As(err, &validationErr):
		// Category D: Validation error - fail without retry
		log.Error(err, "Validation error", "field", validationErr.Field)
		return r.handlePermanentError(ctx, analysis, "ValidationFailed", validationErr.Error())

	default:
		// Unknown error type - treat as transient
		log.Error(err, "Unknown error type, treating as transient")
		return ctrl.Result{RequeueAfter: TransientRetryDelay}, nil
	}
}

// handlePermanentError updates status to Failed and returns without requeue.
func (r *Reconciler) handlePermanentError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, reason, message string) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", client.ObjectKeyFromObject(analysis))

	now := metav1.Now()
	analysis.Status.Phase = PhaseFailed
	analysis.Status.Message = message
	analysis.Status.Reason = reason
	analysis.Status.CompletedAt = &now

	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to update status to Failed")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(analysis, "Warning", reason, message)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aianalysisv1.AIAnalysis{}).
		Complete(r)
}

