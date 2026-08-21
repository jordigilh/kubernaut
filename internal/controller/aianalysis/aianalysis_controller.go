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
	"slices"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aianalysispkg "github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
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

	// Phase handlers (wired in via dependency injection).
	// InvestigatingHandler uses atomic.Pointer so integration tests can
	// swap a mock handler while the controller manager is running.
	InvestigatingHandler atomic.Pointer[handlers.InvestigatingHandler]
	AnalyzingHandler     *handlers.AnalyzingHandler

	// Audit client for recording audit events (DD-AUDIT-003)
	AuditClient *audit.AuditClient

	// #1421, amended #2214: AgentSession creator, reused for cascade
	// cancellation in the terminal branch. When RO externally patches AA to
	// Failed/ParentCancelled, the terminal branch deletes the AgentSession
	// (instead of writing InvestigationSession directly) -- AF's
	// AgentSessionTerminalCloseReconciler (watching AgentSession) closes the
	// correlated IS to Cancelled, and KA's Dispatcher.cancelOnDelete stops
	// the in-flight investigation goroutine.
	AgentSessionCreator *creator.AgentSessionCreator
}

// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubernaut.ai,resources=aianalyses/finalizers,verbs=update
// DD-AA-KA-001 Amendment #2214: AA no longer interacts with
// InvestigationSession at all (neither reads it for decision-making nor
// writes its terminal phase) -- AF's AgentSessionTerminalCloseReconciler now
// owns IS terminal-phase closure exclusively, driven by watching
// AgentSession. AA's only remaining external-cascade signal is deleting the
// AgentSession itself (see delete verb below).
// delete on agentsessions (BR-AI-009, DD-AA-KA-001 amendment): required by
// creator.AgentSessionCreator.DeleteForRetry, called from
// InvestigatingHandler.retryCapacityExceeded to discard a stale
// Failed+CapacityExceeded AgentSession so the next reconcile's GetOrCreate
// falls through to Create for the retry attempt; also required by
// DeleteForCascadeCancel (#2214) on external ParentCancelled.
// +kubebuilder:rbac:groups=kubernaut.ai,resources=agentsessions,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=kubernaut.ai,resources=agentsessions/status,verbs=get
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

	// AA-KA-001: Log reconcile state for debugging duplicate call issues
	log.V(1).Info("Reconcile state",
		"phase", analysis.Status.Phase,
		"generation", analysis.Generation,
		"observedGeneration", analysis.Status.ObservedGeneration,
		"investigationTime", analysis.Status.GetInvestigationMetadata().InvestigationTime)

	// 2. HANDLE DELETION
	if !analysis.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, analysis)
	}

	// 3. ADD FINALIZER IF NOT PRESENT
	if requeueResult, added, err := r.ensureFinalizer(ctx, analysis, log); added {
		return requeueResult, err
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
		return r.initializePendingPhase(ctx, analysis, log)
	}

	// 4. PHASE STATE MACHINE
	// Per reconciliation-phases.md v2.1: Pending → Investigating → Analyzing → Completed/Failed
	// NOTE: Recommending phase REMOVED in v1.8 - workflow data captured in Investigating phase
	// DD-AUDIT-003: Phase transition audits now emitted INSIDE phase handlers
	// (avoids race condition where status update triggers immediate reconcile before audit)
	result, err := r.dispatchPhase(ctx, analysis, currentPhase, log)

	// BR-AI-017: Record metrics and audit events after phase processing
	// AA-BUG-005: This must run for ALL phases including terminal states (Completed/Failed)
	// to record the analysis.completed audit event via RecordAnalysisComplete
	r.recordPhaseMetrics(ctx, currentPhase, analysis, err)

	return result, err
}

// ensureFinalizer adds FinalizerName to analysis if not already present,
// persisting the update. The third return value reports whether the
// finalizer was just added (and this reconcile should therefore return
// immediately via the returned Result/error). Extracted from Reconcile
// (Wave 6 6c GREEN: funlen remediation) — pure code motion, no behavior
// change.
func (r *AIAnalysisReconciler) ensureFinalizer(ctx context.Context, analysis *aianalysisv1.AIAnalysis, log logr.Logger) (ctrl.Result, bool, error) {
	if controllerutil.ContainsFinalizer(analysis, FinalizerName) {
		return ctrl.Result{}, false, nil
	}
	controllerutil.AddFinalizer(analysis, FinalizerName)
	if err := r.Update(ctx, analysis); err != nil {
		log.Error(err, "Failed to add finalizer")
		return ctrl.Result{}, true, err
	}
	// Requeue after short delay after adding finalizer
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, true, nil
}

// initializePendingPhase sets analysis to Pending on first reconciliation and
// persists it, requeuing shortly after so the Pending phase gets processed.
// DD-CONTROLLER-001: ObservedGeneration is NOT set here - only after
// processing phase. Extracted from Reconcile (Wave 6 6c GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (r *AIAnalysisReconciler) initializePendingPhase(ctx context.Context, analysis *aianalysisv1.AIAnalysis, log logr.Logger) (ctrl.Result, error) {
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

// dispatchPhase executes the phase-specific reconcile logic for currentPhase.
// Extracted from Reconcile (Wave 6 6c GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (r *AIAnalysisReconciler) dispatchPhase(ctx context.Context, analysis *aianalysisv1.AIAnalysis, currentPhase string, log logr.Logger) (ctrl.Result, error) {
	switch currentPhase {
	case PhasePending:
		return r.reconcilePending(ctx, analysis)
	case PhaseInvestigating:
		return r.reconcileInvestigating(ctx, analysis)
	case PhaseAnalyzing:
		return r.reconcileAnalyzing(ctx, analysis)
	case PhaseCompleted, PhaseFailed:
		// Terminal states - no action needed
		log.Info("AIAnalysis in terminal state", "phase", currentPhase)
		// #1421, amended #2214: If externally cancelled (ParentCancelled),
		// cascade by deleting the AgentSession (KA session).
		if analysis.Status.Reason == aianalysisv1.ReasonParentCancelled {
			r.cascadeCancelAgentSession(ctx, analysis)
		}
		// AA-BUG-005: Must call recordPhaseMetrics for terminal states to record analysis.completed event
		return ctrl.Result{}, nil
	default:
		log.Error(nil, "Unknown phase - failing AIAnalysis to prevent stall", "phase", currentPhase)
		analysis.Status.Phase = PhaseFailed
		analysis.Status.Reason = "UnknownPhase"
		analysis.Status.Message = fmt.Sprintf("Unrecognized phase %q; failing to prevent stall", currentPhase)
		return ctrl.Result{}, nil
	}
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
	if r.InvestigatingHandler.Load() == nil {
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
//
// SPIKE (2026-08-20, #2204 RCA): maxConcurrentReconciles mirrors the
// EffectivenessMonitor/Notification variadic-option pattern
// (internal/controller/effectivenessmonitor/reconciler.go SetupWithManager).
// Unlike those two, this controller has run with controller-runtime's
// implicit MaxConcurrentReconciles=1 default since it was first scaffolded
// (no prior value to regress from) -- every AIAnalysis CR's reconcile
// (including the Investigating phase's 2s poll-and-requeue loop) is
// serialized through a single worker per controller-manager process. Under
// this PR's own capacity-retry E2E/IT specs and #2204's bursty concurrent
// investigations, a deep per-process workqueue backlog on that single
// worker is the leading hypothesis for the 60s+ Eventually timeouts.
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles ...int) error {
	if err := r.ValidateDependencies(); err != nil {
		return fmt.Errorf("aianalysis controller has nil dependencies: %w", err)
	}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&aianalysisv1.AIAnalysis{}).
		WatchesRawSource(source.Kind(mgr.GetCache(), &agentsessionv1.AgentSession{},
			handler.TypedEnqueueRequestsFromMapFunc(r.mapAgentSessionToAIAnalysis),
			AgentSessionEventPredicate(),
		)).
		WithEventFilter(aiAnalysisUpdatePredicate())

	if len(maxConcurrentReconciles) > 0 && maxConcurrentReconciles[0] > 0 {
		builder = builder.WithOptions(ctrlcontroller.TypedOptions[ctrl.Request]{
			MaxConcurrentReconciles: maxConcurrentReconciles[0],
		})
	}

	return builder.Complete(r)
}

// mapAgentSessionToAIAnalysis maps an AgentSession event to the AIAnalysis
// that owns it (DD-AA-KA-001: replaces the retired IS-watch). AgentSession's
// OwnerReference is set directly to the creating AIAnalysis
// (creator.AgentSessionCreator.GetOrCreate), so the owning AIAnalysis name is
// read straight off the controller reference -- no RR-name field-index List
// required, unlike the old IS-based mapping.
func (r *AIAnalysisReconciler) mapAgentSessionToAIAnalysis(_ context.Context, as *agentsessionv1.AgentSession) []reconcile.Request {
	owner := metav1.GetControllerOf(as)
	if owner == nil || owner.Kind != "AIAnalysis" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Name: owner.Name, Namespace: as.Namespace},
	}}
}

// AgentSessionEventPredicate filters AgentSession events: passes create and
// delete unconditionally, and update events only when a field AA's
// InvestigatingHandler actually branches on changed -- Phase (terminal
// transition or dispatch ack), Interactive (Gap 1 ack signal), SessionID
// (KA correlation onset), or ActingUser/ActingUserGroups (#2204: MCP
// takeover identity onset, see below). Status-only churn that AA doesn't
// act on (e.g. a future observability-only field) is dropped to avoid
// unnecessary reconciles, mirroring the old ISEventPredicate's
// terminal-only filter but widened to the fields
// InvestigatingHandler.syncKASessionStatus reads.
//
// #2204 (2026-08-20, E2E-1293-001 CI RCA): ActingUser/ActingUserGroups were
// missing from this filter. Dispatcher.OnInteractiveUpgrade
// (internal/kubernautagent/agentsession/dispatcher.go) writes
// Interactive=true, ActingUser, and ActingUserGroups together in one
// Status().Update() -- but for an AgentSession that was already
// interactive-from-start (Interactive already true at creation, e.g. an
// IS created before AA's first reconcile), that write leaves Phase,
// Interactive, and SessionID all unchanged, so only ActingUser/
// ActingUserGroups actually changed. Before this PR removed AA's
// fixed-interval sessionPollInterval poll, a missed watch event here was
// still caught by the next poll tick (2s test / 15s prod); now the only
// paths to reconcile are this predicate and the deadline-driven backstop
// (typically minutes away), so a dropped event here left
// AIAnalysis.Status.InteractiveSession.ActingUser (populated by
// syncKASessionStatus, which reads exactly these two fields) never set
// until the investigation's full timeout elapsed -- confirmed live via
// E2E-1293-001 timing out its 90s Eventually waiting for exactly this.
func AgentSessionEventPredicate() predicate.TypedPredicate[*agentsessionv1.AgentSession] {
	return predicate.TypedFuncs[*agentsessionv1.AgentSession]{
		CreateFunc: func(e event.TypedCreateEvent[*agentsessionv1.AgentSession]) bool {
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*agentsessionv1.AgentSession]) bool {
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*agentsessionv1.AgentSession]) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			oldStatus, newStatus := e.ObjectOld.Status, e.ObjectNew.Status
			if oldStatus.Phase != newStatus.Phase {
				return true
			}
			if oldStatus.Interactive != newStatus.Interactive {
				return true
			}
			if oldStatus.SessionID != newStatus.SessionID {
				return true
			}
			if oldStatus.ActingUser != newStatus.ActingUser {
				return true
			}
			return !slices.Equal(oldStatus.ActingUserGroups, newStatus.ActingUserGroups)
		},
		GenericFunc: func(e event.TypedGenericEvent[*agentsessionv1.AgentSession]) bool {
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

// cascadeCancelAgentSession deletes the AgentSession backing this
// investigation when the AIAnalysis is externally terminated (e.g.,
// ParentCancelled from RO). #1421: Kubernetes-native cascade — parent
// manages child lifecycle. #2214 / DD-AA-KA-001 Amendment: replaces the
// retired direct InvestigationSession write -- AF's
// AgentSessionTerminalCloseReconciler (watching AgentSession) closes the
// correlated IS to Cancelled, and KA's already-proven
// Dispatcher.cancelOnDelete stops the in-flight investigation goroutine, both
// reacting independently to the same delete signal.
func (r *AIAnalysisReconciler) cascadeCancelAgentSession(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	log := r.Log.WithValues("aianalysis", analysis.Name)

	if r.AgentSessionCreator == nil {
		return
	}

	rrName := analysis.Spec.RemediationRequestRef.Name
	if rrName == "" {
		return
	}

	if err := r.AgentSessionCreator.DeleteForCascadeCancel(ctx, rrName, analysis.Namespace); err != nil {
		log.Error(err, "Failed to cascade cancel AgentSession (best-effort)",
			"rrName", rrName)
	} else {
		log.Info("Cascaded ParentCancelled by deleting AgentSession",
			"rrName", rrName)
	}
}

// reconcileInvestigating handles AIAnalysis in Investigating phase
// BR-AI-023: KA integration
// BR-AI-017: Track phase timing

// reconcileAnalyzing handles AIAnalysis in Analyzing phase
// BR-AI-030: Rego policy evaluation
// BR-AI-017: Track phase timing

// handleDeletion handles AIAnalysis deletion
