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
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

const (
	// FinalizerName is the finalizer for AIAnalysis resources
	FinalizerName = "aianalysis.kubernaut.ai/finalizer"
)

// Phase constants imported from pkg/aianalysis/handler.go to avoid duplication
// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
const (
	PhasePending       = "Pending"
	PhaseInvestigating = "Investigating"
	PhaseAnalyzing     = "Analyzing"
	PhaseCompleted     = "Completed"
	PhaseFailed        = "Failed"
)

// AIAnalysisReconciler reconciles an AIAnalysis object
// BR-AI-001: CRD Lifecycle Management
// DD-AUDIT-003: P0 priority for audit traces
type AIAnalysisReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger

	// Phase handlers (wired in via dependency injection)
	InvestigatingHandler *handlers.InvestigatingHandler
	AnalyzingHandler     *handlers.AnalyzingHandler

	// Audit client for recording audit events (DD-AUDIT-003)
	AuditClient *audit.AuditClient
}

// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.ai,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AIAnalysis
// BR-AI-001: Phase state machine: Pending → Investigating → Analyzing → Completed/Failed
// Per reconciliation-phases.md v2.1: Recommending phase REMOVED in v1.8
// BR-AI-017: Track service performance metrics
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", req.NamespacedName)
	log.Info("Reconciling AIAnalysis")

	// BR-AI-017: Track reconciliation timing
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.RecordReconcileDuration("overall", duration)
	}()

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

	// Capture current phase for metrics
	currentPhase := analysis.Status.Phase
	if currentPhase == "" {
		currentPhase = PhasePending
	}

	// 4. PHASE STATE MACHINE
	// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
	// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
	var result ctrl.Result
	var err error

	switch currentPhase {
	case PhasePending:
		result, err = r.reconcilePending(ctx, analysis)
	case PhaseInvestigating:
		result, err = r.reconcileInvestigating(ctx, analysis)
	case PhaseAnalyzing:
		result, err = r.reconcileAnalyzing(ctx, analysis)
	case PhaseCompleted, PhaseFailed:
		// Terminal states - no action needed
		log.Info("AIAnalysis in terminal state", "phase", currentPhase)
		return ctrl.Result{}, nil
	default:
		log.Info("Unknown phase", "phase", currentPhase)
		return ctrl.Result{}, nil
	}

	// BR-AI-017: Record metrics and audit events after phase processing
	r.recordPhaseMetrics(ctx, currentPhase, analysis, err)

	return result, err
}

// recordPhaseMetrics records metrics and audit events after phase processing
// BR-AI-017: Track reconciliation outcomes and failures
// DD-AUDIT-003: Record audit events for terminal states
func (r *AIAnalysisReconciler) recordPhaseMetrics(ctx context.Context, phase string, analysis *aianalysisv1.AIAnalysis, err error) {
	result := "success"
	if err != nil {
		result = "error"
	}
	metrics.RecordReconciliation(phase, result)

	// Track failures with reason and sub-reason
	if analysis.Status.Phase == PhaseFailed {
		reason := analysis.Status.Reason
		if reason == "" {
			reason = "Unknown"
		}
		subReason := analysis.Status.SubReason
		if subReason == "" {
			subReason = "Unknown"
		}
		metrics.RecordFailure(reason, subReason)

		// DD-AUDIT-003: Record error audit event
		if r.AuditClient != nil && err != nil {
			r.AuditClient.RecordError(ctx, analysis, phase, err)
		}
	}

	// Track confidence scores for successful analyses
	if analysis.Status.Phase == PhaseCompleted && analysis.Status.SelectedWorkflow != nil {
		signalType := analysis.Spec.AnalysisRequest.SignalContext.SignalType
		confidence := analysis.Status.SelectedWorkflow.Confidence
		metrics.RecordConfidenceScore(signalType, confidence)
	}

	// DD-AUDIT-003: Record analysis complete audit event for terminal states
	if r.AuditClient != nil && (analysis.Status.Phase == PhaseCompleted || analysis.Status.Phase == PhaseFailed) {
		r.AuditClient.RecordAnalysisComplete(ctx, analysis)
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
// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Pending", "name", analysis.Name)
	log.Info("Processing Pending phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		metrics.RecordReconcileDuration(PhasePending, time.Since(phaseStart).Seconds())
	}()

	// Set StartedAt timestamp (per crd-schema.md)
	now := metav1.Now()
	analysis.Status.StartedAt = &now

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
// BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileInvestigating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Investigating", "name", analysis.Name)
	log.Info("Processing Investigating phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		metrics.RecordReconcileDuration(PhaseInvestigating, time.Since(phaseStart).Seconds())
	}()

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
// BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Analyzing", "name", analysis.Name)
	log.Info("Processing Analyzing phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		metrics.RecordReconcileDuration(PhaseAnalyzing, time.Since(phaseStart).Seconds())
	}()

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
