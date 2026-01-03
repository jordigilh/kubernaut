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
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// ========================================
// Phase Handlers
// Pattern: P2 - Controller Decomposition
// ========================================
//
// These handlers implement phase-specific reconciliation logic.
// Each handler is responsible for one phase of the AIAnalysis lifecycle.
//
// Reference: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md

// reconcilePending handles AIAnalysis in Pending phase.
//
// Business Requirements:
//   - BR-AI-010: Initialize analysis and transition to Investigating
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Pending", "name", analysis.Name)
	log.Info("Processing Pending phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		r.Metrics.RecordReconcileDuration(PhasePending, time.Since(phaseStart).Seconds())
	}()

	// Set StartedAt timestamp (per crd-schema.md)
	now := metav1.Now()
	analysis.Status.StartedAt = &now

	// Capture phase BEFORE transition for audit
	phaseBefore := analysis.Status.Phase

	// Transition to Investigating phase (first processing phase per CRD schema)
	// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Investigating handler after processing
	analysis.Status.Phase = PhaseInvestigating
	analysis.Status.Message = "AIAnalysis created, starting investigation"

	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to update status to Investigating")
		return ctrl.Result{}, err
	}

	// DD-AUDIT-003: Record phase transition AFTER status update (ensures audit reflects committed state)
	if r.AuditClient != nil {
		r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, PhaseInvestigating)
	}

	r.Recorder.Event(analysis, "Normal", "AIAnalysisCreated", "AIAnalysis processing started")

	return ctrl.Result{Requeue: true}, nil
}

// reconcileInvestigating handles AIAnalysis in Investigating phase.
//
// Business Requirements:
//   - BR-AI-023: HolmesGPT-API integration
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileInvestigating(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Investigating", "name", analysis.Name)
	log.Info("Processing Investigating phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		r.Metrics.RecordReconcileDuration(PhaseInvestigating, time.Since(phaseStart).Seconds())
	}()

	// Use handler if wired in, otherwise stub for backward compatibility
	if r.InvestigatingHandler != nil {
		// Capture phase before handler for requeue detection
		var phaseBefore string
		var result ctrl.Result
		var handlerErr error

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Handler modifies status fields, then atomic update commits all changes
		// BEFORE: Handler modifies → Status().Update() (separate API call)
		// AFTER: Atomic refetch → Handler modifies → Single Status().Update()
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
			// Capture phase after refetch
			phaseBefore = analysis.Status.Phase

			// Execute handler (modifies analysis.Status in memory after refetch)
			result, handlerErr = r.InvestigatingHandler.Handle(ctx, analysis)
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		}); err != nil {
			log.Error(err, "Failed to atomically update status after Investigating phase")
			return ctrl.Result{}, err
		}

		// Requeue if phase changed to ensure next phase is processed
		// (GenerationChangedPredicate doesn't trigger on status-only updates)
		if analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing (atomic update)", "from", phaseBefore, "to", analysis.Status.Phase)

			// DD-AUDIT-003: Phase transition already recorded INSIDE handler
			// (investigating.go:142, 177 - no duplicate recording needed here)

			return ctrl.Result{Requeue: true}, nil
		}
		return result, nil
	}

	// Stub fallback (for tests without handler wiring)
	log.Info("No InvestigatingHandler configured - using stub")
	return ctrl.Result{}, nil
}

// reconcileAnalyzing handles AIAnalysis in Analyzing phase.
//
// Business Requirements:
//   - BR-AI-030: Rego policy evaluation
//   - BR-AI-017: Track phase timing
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	log := r.Log.WithValues("phase", "Analyzing", "name", analysis.Name)
	log.Info("Processing Analyzing phase")

	// BR-AI-017: Track phase timing
	phaseStart := time.Now()
	defer func() {
		r.Metrics.RecordReconcileDuration(PhaseAnalyzing, time.Since(phaseStart).Seconds())
	}()

	// Use handler if wired in, otherwise stub for backward compatibility
	if r.AnalyzingHandler != nil {
		// Capture phase before handler for requeue detection
		var phaseBefore string
		var result ctrl.Result
		var handlerErr error

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Handler modifies status fields, then atomic update commits all changes
		// BEFORE: Handler modifies → Status().Update() (separate API call)
		// AFTER: Atomic refetch → Handler modifies → Single Status().Update()
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
			// Capture phase after refetch
			phaseBefore = analysis.Status.Phase

			// Execute handler (modifies analysis.Status in memory after refetch)
			result, handlerErr = r.AnalyzingHandler.Handle(ctx, analysis)
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		}); err != nil {
			log.Error(err, "Failed to atomically update status after Analyzing phase")
			return ctrl.Result{}, err
		}

		// Requeue if phase changed to ensure next phase is processed
		// (GenerationChangedPredicate doesn't trigger on status-only updates)
		if analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing (atomic update)", "from", phaseBefore, "to", analysis.Status.Phase)

			// DD-AUDIT-003: Phase transition already recorded INSIDE handler
			// (analyzing.go:97, 134, 220 - no duplicate recording needed here)

			return ctrl.Result{Requeue: true}, nil
		}
		return result, nil
	}

	// Stub fallback (for tests without handler wiring)
	log.Info("No AnalyzingHandler configured - using stub")
	return ctrl.Result{}, nil
}


