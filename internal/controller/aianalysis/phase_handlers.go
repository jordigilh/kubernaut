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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
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
	// IDEMPOTENCY: Only record if phase actually changed (prevents duplicate events in race conditions)
	// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
	if phaseBefore != PhaseInvestigating {
		r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, PhaseInvestigating)
	}

	r.Recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonAIAnalysisCreated, "AIAnalysis processing started")

	// Requeue after short delay to process Investigating phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
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
		// AA-BUG-007: Use optimistic locking with idempotency check
		// Handler runs INSIDE updateFunc, checked AFTER each atomic refetch

		var phaseBefore string
		var result ctrl.Result
		var handlerErr error
		var handlerExecuted bool

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE with AA-BUG-007 fix
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
			// Capture phase after ATOMIC refetch
			phaseBefore = analysis.Status.Phase

		// AA-BUG-009: Enhanced idempotency - skip handler if phase already changed OR already executed
			// InvestigationTime > 0 means handler already executed for this Investigating phase
			// This prevents duplicate holmesgpt.call audit events from concurrent reconciles
			if phaseBefore != PhaseInvestigating {
				log.Info("AA-HAPI-001: Phase already changed, skipping handler",
					"expected", PhaseInvestigating, "actual", phaseBefore,
					"observedGeneration", analysis.Status.ObservedGeneration)
				handlerExecuted = false
				return nil
			}

			if analysis.Status.InvestigationTime > 0 {
				log.Info("AA-HAPI-001: Handler already executed, skipping duplicate call",
					"investigationTime", analysis.Status.InvestigationTime,
					"phase", phaseBefore)
				handlerExecuted = false
				return nil
			}

			// Execute handler ONLY if phase check passed AND not already executed
			result, handlerErr = r.InvestigatingHandler.Handle(ctx, analysis)
			handlerExecuted = true
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		}); err != nil {
			log.Error(err, "Failed to atomically update status after Investigating phase")
			return ctrl.Result{}, err
		}

		// Only requeue if handler actually executed and changed phase
		if handlerExecuted && analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing", "from", phaseBefore, "to", analysis.Status.Phase)

			// DD-AUDIT-003: Record phase transition AFTER status committed (AA-BUG-001 fix)
			// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
			r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, analysis.Status.Phase)

			// Requeue quickly after phase transition
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
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
		// AA-BUG-007: Use optimistic locking with idempotency check
		// The key insight: Move handler execution BEFORE AtomicStatusUpdate's refetch
		// so we can check the phase ONCE and decide whether to proceed

		var phaseBefore string
		var result ctrl.Result
		var handlerErr error
		var handlerExecuted bool

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE with AA-BUG-007 fix
		// Handler runs INSIDE updateFunc, but ONLY ONCE per phase value
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() error {
			// Capture phase after ATOMIC refetch (part of AtomicStatusUpdate)
			phaseBefore = analysis.Status.Phase

			// AA-BUG-007: Idempotency - skip handler if phase already changed
			// This check happens AFTER each refetch, preventing duplicate execution
			if phaseBefore != PhaseAnalyzing {
				log.V(1).Info("Phase already changed, skipping handler",
					"expected", PhaseAnalyzing, "actual", phaseBefore)
				handlerExecuted = false
				return nil // No-op, phase already processed
			}

			// AA-BUG-007: Execute handler ONLY if phase check passed
			// Handler modifies analysis.Status in memory
			result, handlerErr = r.AnalyzingHandler.Handle(ctx, analysis)
			handlerExecuted = true
			if handlerErr != nil {
				return handlerErr
			}
			return nil
		}); err != nil {
			log.Error(err, "Failed to atomically update status after Analyzing phase")
			return ctrl.Result{}, err
		}

		// Only requeue if handler actually executed and changed phase
		if handlerExecuted && analysis.Status.Phase != phaseBefore {
			log.Info("Phase changed, requeuing", "from", phaseBefore, "to", analysis.Status.Phase)

			// DD-AUDIT-003: Record phase transition AFTER status committed (AA-BUG-001 fix)
			// BR-AI-090: AuditClient is P0, guaranteed non-nil (controller exits if init fails)
			r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, analysis.Status.Phase)

			// Requeue quickly after phase transition
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		return result, nil
	}

	// Stub fallback (for tests without handler wiring)
	log.Info("No AnalyzingHandler configured - using stub")
	return ctrl.Result{}, nil
}
