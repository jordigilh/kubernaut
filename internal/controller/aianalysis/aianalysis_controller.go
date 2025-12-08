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

// Package aianalysis implements the AIAnalysis CRD controller.
// This controller orchestrates AI-based incident analysis using HolmesGPT-API
// and manages the workflow selection lifecycle.
//
// Business Requirements: BR-AI-001 to BR-AI-083 (V1.0)
// Architecture: DD-CONTRACT-002, DD-RECOVERY-002, DD-AIANALYSIS-001
package aianalysis

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

const (
	// FinalizerName is the finalizer for AIAnalysis resources
	FinalizerName = "aianalysis.kubernaut.ai/finalizer"

	// Phase constants for AIAnalysis lifecycle
	// Per CRD enum: Pending;Investigating;Analyzing;Recommending;Completed;Failed
	PhasePending       = "Pending"
	PhaseInvestigating = "Investigating"
	PhaseAnalyzing     = "Analyzing"
	PhaseRecommending  = "Recommending"
	PhaseCompleted     = "Completed"
	PhaseFailed        = "Failed"
)

// AIAnalysisReconciler reconciles an AIAnalysis object
// BR-AI-001: CRD Lifecycle Management
type AIAnalysisReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger

	// Phase handlers (wired in via dependency injection)
	InvestigatingHandler *handlers.InvestigatingHandler
	AnalyzingHandler     *handlers.AnalyzingHandler
}

// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AIAnalysis
// BR-AI-001: Phase state machine: Pending → Investigating → Analyzing → Recommending → Completed/Failed
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", req.NamespacedName)
	log.Info("Reconciling AIAnalysis")

	// 1. FETCH RESOURCE
	analysis := &aianalysisv1.AIAnalysis{}
	if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
		// Category A: AIAnalysis Not Found (normal during deletion)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. HANDLE DELETION
	if !analysis.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, analysis)
	}

	// 3. ADD FINALIZER IF NOT PRESENT
	if !controllerutil.ContainsFinalizer(analysis, FinalizerName) {
		controllerutil.AddFinalizer(analysis, FinalizerName)
		if err := r.Update(ctx, analysis); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. PHASE STATE MACHINE
	// Per CRD enum: Pending;Investigating;Analyzing;Recommending;Completed;Failed
	switch analysis.Status.Phase {
	case "", PhasePending:
		return r.reconcilePending(ctx, analysis)
	case PhaseInvestigating:
		return r.reconcileInvestigating(ctx, analysis)
	case PhaseAnalyzing:
		return r.reconcileAnalyzing(ctx, analysis)
	case PhaseRecommending:
		return r.reconcileRecommending(ctx, analysis)
	case PhaseCompleted, PhaseFailed:
		// Terminal states - no action needed
		log.Info("AIAnalysis in terminal state", "phase", analysis.Status.Phase)
		return ctrl.Result{}, nil
	default:
		log.Info("Unknown phase", "phase", analysis.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager
// DD-1: Use GenerationChangedPredicate to prevent reconciliation loops from status updates
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aianalysisv1.AIAnalysis{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

// reconcilePending handles AIAnalysis in Pending phase
// BR-AI-001: Initialize and transition to Investigating
// Note: The CRD schema has phases: Pending → Investigating → Analyzing → Recommending → Completed
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Pending", "name", analysis.Name)
	log.Info("Processing Pending phase")

	// Transition to Investigating phase (first processing phase per CRD schema)
	analysis.Status.Phase = PhaseInvestigating
	analysis.Status.Message = "AIAnalysis created, starting investigation"

	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to update status to Investigating")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(analysis, "Normal", "AIAnalysisCreated", "AIAnalysis processing started")

	return ctrl.Result{Requeue: true}, nil
}

// reconcileInvestigating handles AIAnalysis in Investigating phase
// BR-AI-023: HolmesGPT-API integration
func (r *AIAnalysisReconciler) reconcileInvestigating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Investigating", "name", analysis.Name)
	log.Info("Processing Investigating phase")

	// Use handler if wired in, otherwise stub for backward compatibility
	if r.InvestigatingHandler != nil {
		// Capture phase before handler
		phaseBefore := analysis.Status.Phase

		result, err := r.InvestigatingHandler.Handle(ctx, analysis)
		if err != nil {
			log.Error(err, "InvestigatingHandler failed")
			return result, err
		}
		// Update status after handler completes
		if err := r.Status().Update(ctx, analysis); err != nil {
			log.Error(err, "Failed to update status after Investigating phase")
			return ctrl.Result{}, err
		}
		// Requeue if phase changed to ensure next phase is processed
		// (GenerationChangedPredicate doesn't trigger on status-only updates)
		if analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing", "from", phaseBefore, "to", analysis.Status.Phase)
			return ctrl.Result{Requeue: true}, nil
		}
		return result, nil
	}

	// Stub fallback (for tests without handler wiring)
	log.Info("No InvestigatingHandler configured - using stub")
	return ctrl.Result{}, nil
}

// reconcileAnalyzing handles AIAnalysis in Analyzing phase
// BR-AI-030: Rego policy evaluation
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Analyzing", "name", analysis.Name)
	log.Info("Processing Analyzing phase")

	// Use handler if wired in, otherwise stub for backward compatibility
	if r.AnalyzingHandler != nil {
		// Capture phase before handler
		phaseBefore := analysis.Status.Phase

		result, err := r.AnalyzingHandler.Handle(ctx, analysis)
		if err != nil {
			log.Error(err, "AnalyzingHandler failed")
			return result, err
		}
		// Update status after handler completes
		if err := r.Status().Update(ctx, analysis); err != nil {
			log.Error(err, "Failed to update status after Analyzing phase")
			return ctrl.Result{}, err
		}
		// Requeue if phase changed to ensure next phase is processed
		// (GenerationChangedPredicate doesn't trigger on status-only updates)
		if analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing", "from", phaseBefore, "to", analysis.Status.Phase)
			return ctrl.Result{Requeue: true}, nil
		}
		return result, nil
	}

	// Stub fallback (for tests without handler wiring)
	log.Info("No AnalyzingHandler configured - using stub")
	return ctrl.Result{}, nil
}

// reconcileRecommending handles AIAnalysis in Recommending phase
// BR-AI-075: Workflow selection output
// TODO: Implement in Day 4
func (r *AIAnalysisReconciler) reconcileRecommending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Recommending", "name", analysis.Name)
	log.Info("Processing Recommending phase - stub")

	// TODO: Implement workflow recommendation in Day 4
	return ctrl.Result{}, nil
}

// handleDeletion handles AIAnalysis deletion
func (r *AIAnalysisReconciler) handleDeletion(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("name", analysis.Name)
	log.Info("Handling AIAnalysis deletion")

	// Cleanup logic here (audit trail, etc.)
	// TODO: Add audit event for deletion in Day 5

	// Remove finalizer
	controllerutil.RemoveFinalizer(analysis, FinalizerName)
	if err := r.Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
