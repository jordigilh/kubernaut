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
// This controller orchestrates AI-based incident analysis using the Kubernaut Agent
// and manages the workflow selection lifecycle.
//
// Business Requirements: BR-AI-001 to BR-AI-083 (V1.0)
// Architecture: DD-CONTRACT-002, DD-AIANALYSIS-001
package aianalysis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	aianalysispkg "github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/status"
)

const (
	// FinalizerName is the finalizer for AIAnalysis resources
	FinalizerName = "kubernaut.ai/finalizer"
)

// Phase constants: Imported from pkg/aianalysis/handler.go (single source of truth)
// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
const (
	PhasePending       = aianalysispkg.PhasePending
	PhaseInvestigating = aianalysispkg.PhaseInvestigating
	PhaseAnalyzing     = aianalysispkg.PhaseAnalyzing
	PhaseCompleted     = aianalysispkg.PhaseCompleted
	PhaseFailed        = aianalysispkg.PhaseFailed

	ActionabilityActionable    = aianalysispkg.ActionabilityActionable
	ActionabilityNotActionable = aianalysispkg.ActionabilityNotActionable
)

// AIAnalysisReconciler reconciles an AIAnalysis object
// BR-AI-001: CRD Lifecycle Management
// DD-AUDIT-003: P0 priority for audit traces
type AIAnalysisReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger

	// DD-METRICS-001: Metrics wired to controller (V1.0 Maturity Requirement - P0)
	// Per DD-METRICS-001: Dependency injection pattern for testability
	Metrics *metrics.Metrics

	// ========================================
	// STATUS MANAGER (DD-PERF-001)
	// 📋 Design Decision: DD-PERF-001 | ✅ Atomic Status Updates Pattern
	// See: docs/architecture/decisions/DD-PERF-001-atomic-status-updates-mandate.md
	// ========================================
	//
	// StatusManager manages atomic status updates to reduce K8s API calls
	// Consolidates multiple status field updates into single atomic operations
	//
	// BENEFITS:
	// - 50-75% API call reduction (multiple updates → 1 atomic update)
	// - Eliminates race conditions from sequential updates
	// - Reduces etcd write load and watch events
	//
	// WIRED IN: cmd/aianalysis/main.go
	// USAGE: r.StatusManager.AtomicStatusUpdate(ctx, analysis, func() { ... })
	StatusManager *status.Manager

	// Phase handlers (wired in via dependency injection)
	InvestigatingHandler *handlers.InvestigatingHandler
	AnalyzingHandler     *handlers.AnalyzingHandler

	// Audit client for recording audit events (DD-AUDIT-003)
	AuditClient *audit.AuditClient
}

// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubernaut.ai,resources=investigationsessions,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AIAnalysis
// BR-AI-001: Phase state machine: Pending → Investigating → Analyzing → Completed/Failed
// Per reconciliation-phases.md v2.1: Recommending phase REMOVED in v1.8
// BR-AI-017: Track service performance metrics
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("aianalysis", req.NamespacedName)
	log.Info("Reconciling AIAnalysis")

	// 1. FETCH RESOURCE
	analysis := &aianalysisv1.AIAnalysis{}
	if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
		// Category A: AIAnalysis Not Found (normal during deletion)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// AA-HAPI-001: Log reconcile state for debugging duplicate call issues
	log.V(1).Info("Reconcile state",
		"phase", analysis.Status.Phase,
		"generation", analysis.Generation,
		"observedGeneration", analysis.Status.ObservedGeneration,
		"investigationTime", analysis.Status.InvestigationTime)

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
		// Requeue after short delay after adding finalizer
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	// ========================================
	// NO OBSERVED GENERATION CHECK FOR AIAnalysis
	// ========================================
	// AIAnalysis progresses through multiple phases (Pending→Investigating→Analyzing→Completed)
	// within a SINGLE generation via status-only updates.
	// ObservedGeneration checks would block phase progression!
	// See SetupWithManager comment: "GenerationChangedPredicate removed to allow phase progression"
	// ========================================

	// Capture current phase for metrics
	currentPhase := analysis.Status.Phase
	if currentPhase == "" {
		// Initialize phase to Pending on first reconciliation
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - only after processing phase
		analysis.Status.Phase = PhasePending
	analysis.Status.Message = "AIAnalysis created"
	if err := r.Status().Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to initialize phase to Pending")
		return ctrl.Result{}, err
	}
	// Requeue after short delay to process Pending phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

	// 4. PHASE STATE MACHINE
	// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
	// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
	var result ctrl.Result
	var err error

	// DD-AUDIT-003: Phase transition audits now emitted INSIDE phase handlers
	// (avoids race condition where status update triggers immediate reconcile before audit)
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
		// AA-BUG-005: Must call recordPhaseMetrics for terminal states to record analysis.completed event
		result = ctrl.Result{}
		err = nil
	default:
		log.Info("Unknown phase", "phase", currentPhase)
		result = ctrl.Result{}
		err = nil
	}

	// BR-AI-017: Record metrics and audit events after phase processing
	// AA-BUG-005: This must run for ALL phases including terminal states (Completed/Failed)
	// to record the analysis.completed audit event via RecordAnalysisComplete
	r.recordPhaseMetrics(ctx, currentPhase, analysis, err)

	return result, err
}

// recordPhaseMetrics records metrics and audit events after phase processing
// BR-AI-017: Track reconciliation outcomes and failures
// DD-AUDIT-003: Record audit events for terminal states

// ValidateDependencies verifies that all mandatory dependencies are non-nil.
// Returns a joined error listing every missing dependency.
// Issue #1116: Prevents the controller from silently skipping core business
// logic (Rego evaluation, investigation) when handlers are nil.
func (r *AIAnalysisReconciler) ValidateDependencies() error {
	var errs []error
	if r.InvestigatingHandler == nil {
		errs = append(errs, fmt.Errorf("investigatingHandler is nil: investigation phase will be skipped (BR-AI-023)"))
	}
	if r.AnalyzingHandler == nil {
		errs = append(errs, fmt.Errorf("analyzingHandler is nil: Rego policy evaluation will be skipped (BR-AI-012, BR-AI-030)"))
	}
	if r.Metrics == nil {
		errs = append(errs, fmt.Errorf("metrics is nil: observability will panic on phase transitions (DD-METRICS-001)"))
	}
	if r.StatusManager == nil {
		errs = append(errs, fmt.Errorf("statusManager is nil: atomic status updates will panic (DD-PERF-001)"))
	}
	if r.AuditClient == nil {
		errs = append(errs, fmt.Errorf("auditClient is nil: audit trail will panic on phase transitions (DD-AUDIT-003)"))
	}
	return errors.Join(errs...)
}

// SetupWithManager sets up the controller with the Manager.
//
// DD-CONTROLLER-001: Uses a custom predicate that filters Update events to only
// enqueue reconciles when the generation changed (spec update) or the phase
// changed (meaningful status transition). Status-only updates that only touch
// poll tracking fields (PollCount, LastPolled) do NOT trigger re-reconciles,
// allowing RequeueAfter backoff intervals to work correctly.
//
// Issue #1116: Validates all mandatory dependencies before registering.
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.ValidateDependencies(); err != nil {
		return fmt.Errorf("aianalysis controller has nil dependencies: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&aianalysisv1.AIAnalysis{}).
		WatchesRawSource(source.Kind(mgr.GetCache(), &isv1alpha1.InvestigationSession{},
			handler.TypedEnqueueRequestsFromMapFunc(r.mapISToAIAnalysis),
			isEventPredicate(),
		)).
		WithEventFilter(aiAnalysisUpdatePredicate()).
		Complete(r)
}

// mapISToAIAnalysis maps an InvestigationSession event to the AIAnalysis that
// references the same RemediationRequest. BR-INTERACTIVE-010: enables takeover/deletion detection.
func (r *AIAnalysisReconciler) mapISToAIAnalysis(ctx context.Context, is *isv1alpha1.InvestigationSession) []reconcile.Request {
	rrName := is.Spec.RemediationRequestRef.Name
	if rrName == "" {
		return nil
	}

	var list aianalysisv1.AIAnalysisList
	if err := r.List(ctx, &list,
		client.InNamespace(is.Namespace),
		client.MatchingFields{aiAnalysisRRNameIndex: rrName},
	); err != nil {
		r.Log.Error(err, "failed to map IS to AIAnalysis", "is", is.Name, "rrName", rrName)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		aa := &list.Items[i]
		if aa.Status.Phase == PhaseInvestigating {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: aa.Name, Namespace: aa.Namespace},
			})
		}
	}
	return requests
}

// isEventPredicate filters IS events to only create and delete (phase changes are not relevant).
func isEventPredicate() predicate.TypedPredicate[*isv1alpha1.InvestigationSession] {
	return predicate.TypedFuncs[*isv1alpha1.InvestigationSession]{
		CreateFunc: func(e event.TypedCreateEvent[*isv1alpha1.InvestigationSession]) bool {
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*isv1alpha1.InvestigationSession]) bool {
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*isv1alpha1.InvestigationSession]) bool {
			return false
		},
		GenericFunc: func(e event.TypedGenericEvent[*isv1alpha1.InvestigationSession]) bool {
			return false
		},
	}
}

// aiAnalysisRRNameIndex is the field index key for AIAnalysis's spec.remediationRequestRef.name.
const aiAnalysisRRNameIndex = "spec.remediationRequestRef.name"

// AIAnalysisRRNameIndex returns the field index key for external registration.
func AIAnalysisRRNameIndex() string { return aiAnalysisRRNameIndex }

// aiAnalysisUpdatePredicate returns a predicate that filters Update events.
// Create, Delete, and Generic events always pass through.
// Update events only pass if the generation changed (spec update) or the
// phase changed (meaningful status transition). This prevents status-only
// writes (PollCount, LastPolled) from triggering immediate re-reconciles
// that would bypass the intended RequeueAfter backoff intervals.
func aiAnalysisUpdatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return true
			}

			// Always reconcile on generation change (spec update)
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}

			// Reconcile on phase change (meaningful status transition)
			oldAIA, okOld := e.ObjectOld.(*aianalysisv1.AIAnalysis)
			newAIA, okNew := e.ObjectNew.(*aianalysisv1.AIAnalysis)
			if !okOld || !okNew {
				return true // Can't cast, let it through
			}

			if oldAIA.Status.Phase != newAIA.Status.Phase {
				return true
			}

			// Reconcile when session ID changes (new session created)
			oldSessionID := ""
			newSessionID := ""
			if oldAIA.Status.KASession != nil {
				oldSessionID = oldAIA.Status.KASession.ID
			}
			if newAIA.Status.KASession != nil {
				newSessionID = newAIA.Status.KASession.ID
			}
			if oldSessionID != newSessionID {
				return true
			}

			// Skip: status-only update with no meaningful change
			// (e.g., PollCount/LastPolled updates during poll-pending)
			return false
		},
	}
}

// reconcilePending handles AIAnalysis in Pending phase
// BR-AI-001: Initialize and transition to Investigating
// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed

// reconcileInvestigating handles AIAnalysis in Investigating phase
// BR-AI-023: KA integration
// BR-AI-017: Track phase timing

// reconcileAnalyzing handles AIAnalysis in Analyzing phase
// BR-AI-030: Rego policy evaluation
// BR-AI-017: Track phase timing

// handleDeletion handles AIAnalysis deletion
