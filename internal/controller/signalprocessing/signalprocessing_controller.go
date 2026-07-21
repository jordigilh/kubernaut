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

// Package signalprocessing implements the SignalProcessing CRD controller.
// Per IMPLEMENTATION_PLAN_V1.31.md - E2E GREEN Phase + BR-SP-090 Audit
//
// Reconciliation Flow:
//  1. Pending → Enriching: K8s context enrichment + owner chain + custom labels
//  2. Enriching → Classifying: Environment + Priority classification
//  3. Classifying → Categorizing: Business classification
//  4. Categorizing → Completed: Final status update + audit event
//
// Business Requirements:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-051-053: Environment Classification
//   - BR-SP-070-072: Priority Assignment
//   - BR-SP-090: Categorization Audit Trail
//   - BR-SP-100: Owner Chain Traversal
//   - BR-SP-102: Custom Labels
package signalprocessing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	// BR-SP-110: Kubernetes Conditions
)

// SignalProcessingReconciler reconciles a SignalProcessing object.
// Per IMPLEMENTATION_PLAN_V1.31.md - E2E GREEN Phase + BR-SP-090 Audit
// Day 10 Integration: Wired with Rego-based classifiers from pkg/signalprocessing/classifier
type SignalProcessingReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AuditManager *audit.Manager // BR-SP-090: Audit Manager (Phase 3 refactoring - 2026-01-22)

	// V1.0 Maturity Requirements (per SERVICE_MATURITY_REQUIREMENTS.md)
	Metrics  *metrics.Metrics     // DD-005: Observability - metrics wired to controller
	Recorder record.EventRecorder // K8s best practice: EventRecorder for debugging

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
	// - 66-75% API call reduction (3-4 updates → 1 atomic update per reconcile)
	// - Eliminates race conditions from sequential updates
	// - Reduces etcd write load and watch events
	//
	// WIRED IN: cmd/signalprocessing/main.go
	// USAGE: r.StatusManager.AtomicStatusUpdate(ctx, sp, func() { ... })
	StatusManager *status.Manager

	// ADR-060: Unified Rego evaluator replaces individual classifiers
	// Covers: environment (BR-SP-051), priority (BR-SP-070), severity (BR-SP-105), custom labels (BR-SP-102)
	PolicyEvaluator PolicyEvaluator // MANDATORY - fail loudly if nil

	SignalModeClassifier *classifier.SignalModeClassifier // BR-SP-106: Proactive signal mode classification (ADR-054)

	// K8sEnricher provides sophisticated Kubernetes context enrichment
	// This is MANDATORY - fail loudly if nil or on error
	K8sEnricher K8sEnricher // BR-SP-001: K8s context enrichment (interface for testability)
}

// +kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=remediation.kubernaut.ai,resources=remediationrequests,verbs=get;list;watch

// Reconcile implements the reconciliation loop for SignalProcessing.
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Reconciling SignalProcessing", "name", req.Name, "namespace", req.Namespace)

	// Fetch the SignalProcessing instance
	sp := &signalprocessingv1alpha1.SignalProcessing{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// O5: Defense-in-depth nil guard for PolicyEvaluator (after resource exists)
	if r.PolicyEvaluator == nil {
		return ctrl.Result{}, fmt.Errorf("PolicyEvaluator is nil - SP controller misconfigured (should be caught by SetupWithManager)")
	}

	// OBSERVED GENERATION CHECK (DD-CONTROLLER-001): skip reconcile if we've
	// already processed this generation AND not in a terminal phase.
	if isDuplicateGenerationReconcile(sp) {
		logger.V(1).Info("✅ DUPLICATE RECONCILE PREVENTED: Generation already processed",
			"generation", sp.Generation,
			"observedGeneration", sp.Status.ObservedGeneration,
			"phase", sp.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Initialize status if needed
	if sp.Status.Phase == "" {
		return r.initializeNewSignalProcessing(ctx, sp, logger)
	}

	// Skip if already completed or failed
	if sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
		sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
		return ctrl.Result{}, nil
	}

	result, err := r.dispatchPhase(ctx, sp, logger)
	if errors.Is(err, errUnknownPhase) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// DD-SHARED-001: Handle transient errors with exponential backoff
	// Transient errors get explicit backoff delays to prevent thundering herd
	if err != nil && isTransientError(err) {
		return r.handleTransientError(ctx, sp, err, logger)
	}

	// BR-SP-111: On success, reset consecutive failures
	// E1 fix: Success paths use RequeueAfter (not Requeue), so check both.
	if err == nil && (result.Requeue || result.RequeueAfter > 0) {
		if resetErr := r.resetConsecutiveFailures(ctx, sp); resetErr != nil {
			logger.V(1).Info("Failed to reset consecutive failures (non-fatal)", "error", resetErr)
		}
	}

	return result, err
}

// isDuplicateGenerationReconcile reports whether sp's current generation was
// already observed by a prior reconcile while sp is not in a terminal phase
// (DD-CONTROLLER-001). Extracted from Reconcile per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func isDuplicateGenerationReconcile(sp *signalprocessingv1alpha1.SignalProcessing) bool {
	return sp.Status.ObservedGeneration == sp.Generation &&
		sp.Status.Phase != "" &&
		sp.Status.Phase != signalprocessingv1alpha1.PhaseCompleted &&
		sp.Status.Phase != signalprocessingv1alpha1.PhaseFailed
}

// errUnknownPhase is a sentinel returned by dispatchPhase when sp.Status.Phase
// doesn't match any known phase handler (SP-BUG-005). Reconcile treats it as
// a requeue-without-transition signal rather than a propagated error.
var errUnknownPhase = errors.New("unexpected signalprocessing phase")

// initializeNewSignalProcessing sets the initial Pending phase + StartTime
// (DD-PERF-001 atomic update) for a freshly created SignalProcessing, and
// records the "" → Pending audit transition (BR-SP-090, ADR-032 mandatory
// audit). Extracted from Reconcile per GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) initializeNewSignalProcessing(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	err := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.Phase = signalprocessingv1alpha1.PhasePending
		sp.Status.StartTime = &metav1.Time{Time: time.Now()}
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to initialize status")
		return ctrl.Result{}, err
	}

	// O2 (BR-SP-090): Record phase transition audit for "" → Pending
	// ADR-032: Audit is MANDATORY - return error if not configured
	if err := r.recordPhaseTransitionAudit(ctx, sp, "", string(signalprocessingv1alpha1.PhasePending)); err != nil {
		logger.Error(err, "Failed to record initial phase transition audit")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// dispatchPhase routes to the reconcile handler for sp's current phase.
// Extracted from Reconcile per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). Returns errUnknownPhase (SP-BUG-005) for a phase value that
// doesn't match any known handler (corrupted status, cache race, or a test
// fixture with an invalid phase) — all valid phases are handled explicitly.
func (r *SignalProcessingReconciler) dispatchPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	switch sp.Status.Phase {
	case signalprocessingv1alpha1.PhasePending:
		return r.reconcilePending(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseEnriching:
		return r.reconcileEnriching(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseClassifying:
		return r.reconcileClassifying(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseCategorizing:
		return r.reconcileCategorizing(ctx, sp, logger)
	default:
		// SP-BUG-005: Unexpected phase encountered
		// All valid phases are handled above. If we reach here, it indicates:
		// 1. Phase value was corrupted
		// 2. Race condition in K8s cache
		// 3. Test created resource with invalid phase
		// Log error and requeue without emitting audit event (prevents extra transitions)
		logger.Error(fmt.Errorf("unexpected phase: %s", sp.Status.Phase),
			"Unknown phase encountered - requeueing without transition",
			"phase", sp.Status.Phase,
			"resourceVersion", sp.ResourceVersion,
			"generation", sp.Generation)
		return ctrl.Result{}, errUnknownPhase
	}
}

// reconcilePending transitions from Pending to Enriching.
func (r *SignalProcessingReconciler) reconcilePending(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Processing Pending phase")

	// DD-005: Track phase processing attempt
	r.Metrics.IncrementProcessingTotal("pending", "attempt")

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Transition to Enriching in single API call
	// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Enriching handler after processing
	// ========================================
	oldPhase := sp.Status.Phase
	err := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.Phase = signalprocessingv1alpha1.PhaseEnriching
		return nil
	})
	if err != nil {
		// DD-005: Track phase processing failure
		r.Metrics.IncrementProcessingTotal("pending", "failure")
		return ctrl.Result{}, err
	}

	// Record phase transition audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	// FIX: SP-BUG-001 - Missing audit event for Pending→Enriching transition
	if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseEnriching)); err != nil {
		// DD-005: Track phase processing failure (audit failure)
		r.Metrics.IncrementProcessingTotal("pending", "failure")
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: PhaseTransition K8s event for observability
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, signalprocessingv1alpha1.PhaseEnriching))
	}

	// DD-005: Track phase processing success
	r.Metrics.IncrementProcessingTotal("pending", "success")
	// Requeue quickly to continue to next phase
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// SetupWithManager sets up the controller with the Manager.
// DD-CONTROLLER-001: ObservedGeneration provides idempotency without blocking status updates
// GenerationChangedPredicate removed to allow phase progression via status updates
// O5: PolicyEvaluator nil check — fail-fast at startup
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.PolicyEvaluator == nil {
		return fmt.Errorf("PolicyEvaluator is nil - cannot register SP controller without policy evaluator")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&signalprocessingv1alpha1.SignalProcessing{}).
		Named(fmt.Sprintf("signalprocessing-%s", "controller")).
		Complete(r)
}

// ========================================
// SHARED BACKOFF INTEGRATION (DD-SHARED-001)
// Uses pkg/shared/backoff for exponential retry delays
// with jitter to prevent thundering herd
// ========================================

// calculateBackoffDelay returns the exponential backoff duration for the current failure count.
// Uses shared backoff library with ±10% jitter for anti-thundering herd.
// DD-SHARED-001: Shared Exponential Backoff Library
func (r *SignalProcessingReconciler) calculateBackoffDelay(failures int32) time.Duration {
	return backoff.CalculateWithDefaults(failures)
}

// handleTransientError records a transient failure and returns a Result with backoff delay.
// This implements exponential backoff with jitter for transient errors.
// DD-SHARED-001: Shared Exponential Backoff Library
//
// Use this for transient errors that may succeed on retry:
// - K8s API timeouts
// - Network issues
// - Temporary service unavailability
//
// Do NOT use for:
// - Fatal errors (invalid input, business logic errors)
// - Permanent failures (resource deleted, RBAC denied)
func (r *SignalProcessingReconciler) handleTransientError(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
	err error,
	logger logr.Logger,
) (ctrl.Result, error) {
	// Increment consecutive failures
	sp.Status.ConsecutiveFailures++
	sp.Status.LastFailureTime = &metav1.Time{Time: time.Now()}

	// Calculate backoff delay
	delay := r.calculateBackoffDelay(sp.Status.ConsecutiveFailures)

	logger.V(1).Info("Transient error, scheduling retry with backoff",
		"error", err,
		"consecutiveFailures", sp.Status.ConsecutiveFailures,
		"backoffDelay", delay,
	)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidate: ConsecutiveFailures + LastFailureTime + Error → 1 API call
	// ========================================
	consecutiveFailures := sp.Status.ConsecutiveFailures
	lastFailureTime := sp.Status.LastFailureTime
	errorMsg := err.Error()

	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ConsecutiveFailures = consecutiveFailures
		sp.Status.LastFailureTime = lastFailureTime
		sp.Status.Error = errorMsg
		return nil
	})
	if updateErr != nil {
		logger.Error(updateErr, "Failed to update failure count")
		// Return the original error to let controller-runtime handle it
		return ctrl.Result{}, err
	}

	// Return with explicit backoff delay instead of immediate requeue
	return ctrl.Result{RequeueAfter: delay}, nil
}

// resetConsecutiveFailures resets the failure counter on successful operation.
// Call this after successful phase transitions. Its error is logged by the
// sole caller, so it needs no logger of its own.
// DD-SHARED-001: Shared Exponential Backoff Library
// DD-PERF-001: Atomic status updates
func (r *SignalProcessingReconciler) resetConsecutiveFailures(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
) error {
	if sp.Status.ConsecutiveFailures == 0 {
		return nil // Already reset, skip update
	}

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidate: ConsecutiveFailures + Error → 1 API call
	// ========================================
	return r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ConsecutiveFailures = 0
		sp.Status.Error = ""
		return nil
	})
}

// isTransientError determines if an error is transient and should trigger backoff retry.
// Transient errors are temporary and may succeed on retry.
// DD-SHARED-001: Shared Exponential Backoff Library
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// K8s API transient errors
	if apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) {
		return true
	}

	// E2 fix: Use errors.Is for wrapped context errors.
	// context.DeadlineExceeded is transient (timeout, retry with backoff).
	// context.Canceled is NOT transient (caller-initiated abort, e.g., shutdown).
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}
