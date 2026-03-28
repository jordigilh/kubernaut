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
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/status"

	// BR-SP-110: Kubernetes Conditions
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// SignalProcessingReconciler reconciles a SignalProcessing object.
// Per IMPLEMENTATION_PLAN_V1.31.md - E2E GREEN Phase + BR-SP-090 Audit
// Day 10 Integration: Wired with Rego-based classifiers from pkg/signalprocessing/classifier
type SignalProcessingReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AuditManager *audit.Manager      // BR-SP-090: Audit Manager (Phase 3 refactoring - 2026-01-22)

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

	// ========================================
	// OBSERVED GENERATION CHECK (DD-CONTROLLER-001)
	// ========================================
	// Skip reconcile if we've already processed this generation AND not in terminal phase
	if sp.Status.ObservedGeneration == sp.Generation &&
		sp.Status.Phase != "" &&
		sp.Status.Phase != signalprocessingv1alpha1.PhaseCompleted &&
		sp.Status.Phase != signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("✅ DUPLICATE RECONCILE PREVENTED: Generation already processed",
			"generation", sp.Generation,
			"observedGeneration", sp.Status.ObservedGeneration,
			"phase", sp.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Initialize status if needed
	if sp.Status.Phase == "" {
		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Initialize phase + timestamp in single API call
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - only after processing phase
		// ========================================
		err := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
			sp.Status.Phase = signalprocessingv1alpha1.PhasePending
			sp.Status.StartTime = &metav1.Time{Time: time.Now()}
			return nil
	})
	if err != nil {
		logger.Error(err, "Failed to initialize status")
		return ctrl.Result{}, err
	}
	// Requeue after short delay to process Pending phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

	// Skip if already completed or failed
	if sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
		sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
		return ctrl.Result{}, nil
	}

	// Process based on current phase
	var result ctrl.Result
	var err error

	switch sp.Status.Phase {
	case signalprocessingv1alpha1.PhasePending:
		result, err = r.reconcilePending(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseEnriching:
		result, err = r.reconcileEnriching(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseClassifying:
		result, err = r.reconcileClassifying(ctx, sp, logger)
	case signalprocessingv1alpha1.PhaseCategorizing:
		result, err = r.reconcileCategorizing(ctx, sp, logger)
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
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// DD-SHARED-001: Handle transient errors with exponential backoff
	// Transient errors get explicit backoff delays to prevent thundering herd
	if err != nil && isTransientError(err) {
		return r.handleTransientError(ctx, sp, err, logger)
	}

	// On success, reset consecutive failures
	if err == nil && result.Requeue {
		if resetErr := r.resetConsecutiveFailures(ctx, sp, logger); resetErr != nil {
			logger.V(1).Info("Failed to reset consecutive failures (non-fatal)", "error", resetErr)
		}
	}

	return result, err
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

// reconcileEnriching performs context enrichment based on the signal's target type.
//
// BR-SP-001: K8s Context Enrichment
// BR-SP-100: Owner Chain Traversal
// ADR-056: BR-SP-101 (Detected Labels) relocated to HAPI post-RCA
//
// ========================================
// V2.0 EXTENSIBILITY POINT: Multi-Provider Support
// ========================================
//
// Currently, this function only supports Kubernetes enrichment (targetType: "kubernetes").
// The CRD field `spec.signal.targetType` (enum: kubernetes|aws|azure|gcp|datadog) is already
// present and validated, providing the routing discriminator for future multi-provider support.
//
// When Kubernaut evolves to a full-stack AIOps platform, extend this function with:
//
//	switch sp.Spec.Signal.TargetType {
//	case "kubernetes":
//	    return r.enrichKubernetesContext(ctx, sp, logger)  // Current implementation
//	case "aws":
//	    return r.enrichAWSContext(ctx, sp, logger)         // CloudWatch, CloudTrail, EC2, EKS
//	case "azure":
//	    return r.enrichAzureContext(ctx, sp, logger)       // Azure Monitor, Activity Log, AKS
//	case "gcp":
//	    return r.enrichGCPContext(ctx, sp, logger)         // Cloud Monitoring, GKE
//	case "datadog":
//	    return r.enrichDatadogContext(ctx, sp, logger)     // Datadog API context
//	}
//
// Each provider enricher should:
// 1. Call provider-specific APIs to gather context
// 2. Populate the appropriate status fields (may require CRD status extension)
// 3. Handle provider-specific error scenarios and degraded mode
// 4. Set conditions with provider-specific reasons
//
// Related fields in spec.signal:
// - targetType: The platform to enrich from (routing discriminator)
// - type: Signal source (prometheus, kubernetes-event, aws-cloudwatch, etc.)
// - source: Gateway adapter that ingested the signal
//
// See: docs/architecture/decisions/DD-SP-003-multi-provider-extensibility.md (TODO: create when V2.0 begins)
// ========================================
func (r *SignalProcessingReconciler) reconcileEnriching(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Processing Enriching phase")

	// SP-BUG-PHASE-TRANSITION-001: Idempotency guard to prevent duplicate phase transitions
	// Use non-cached APIReader to get FRESH phase data (cached client may be stale)
	currentPhase, err := r.StatusManager.GetCurrentPhase(ctx, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch current phase for idempotency check, proceeding with caution")
		// Fail-safe: continue processing, but log the error
	} else if currentPhase == signalprocessingv1alpha1.PhaseClassifying ||
		currentPhase == signalprocessingv1alpha1.PhaseCategorizing ||
		currentPhase == signalprocessingv1alpha1.PhaseCompleted ||
		currentPhase == signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("Skipping Enriching phase - already transitioned (non-cached check)",
			"current_phase", currentPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	// DD-005: Track phase processing attempt
	r.Metrics.IncrementProcessingTotal("enriching", "attempt")

	// RF-SP-003: Track enrichment duration for audit metrics
	enrichmentStart := time.Now()

	signal := &sp.Spec.Signal

	// Issue #419 / V2.0: Only "kubernetes" enrichment is implemented. Non-kubernetes target
	// types are accepted by CRD validation but enrichment runs in degraded mode (no context).
	if signal.TargetType != "" && signal.TargetType != "kubernetes" {
		logger.Info("Non-kubernetes targetType received; enrichment will run in degraded mode",
			"targetType", signal.TargetType)
		r.Recorder.Eventf(sp, "Warning", "UnsupportedTargetType",
			"targetType %q is not yet supported for enrichment (V2.0); proceeding in degraded mode", signal.TargetType)
	}

	targetNs := signal.TargetResource.Namespace
	targetKind := signal.TargetResource.Kind
	targetName := signal.TargetResource.Name

	// BR-SP-001: K8sEnricher is MANDATORY - fail loudly if not wired or fails
	// No fallback path - enrichment failure should stop processing
	if r.K8sEnricher == nil {
		return ctrl.Result{}, fmt.Errorf("K8sEnricher is nil - this is a startup configuration error")
	}

	k8sCtx, err := r.K8sEnricher.Enrich(ctx, signal)
	if err != nil {
		logger.Error(err, "K8sEnricher failed", "targetKind", targetKind, "targetName", targetName)
		r.Metrics.IncrementProcessingTotal("enriching", "failure")
		r.Metrics.ObserveProcessingDuration("enriching", time.Since(enrichmentStart).Seconds())

		// BR-SP-110: Set EnrichmentComplete=False condition (best-effort, survives refetch)
		if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
			spconditions.SetEnrichmentComplete(sp, false, spconditions.ReasonEnrichmentFailed, err.Error())
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to persist EnrichmentComplete=False condition")
		}

		if r.AuditManager != nil {
			_ = r.AuditManager.RecordError(ctx, sp, "Enriching", err)
		}

		return ctrl.Result{}, fmt.Errorf("enrichment failed: %w", err)
	}

	// 4. Custom labels (BR-SP-102) - ADR-060: Unified evaluator
	customLabels := make(map[string][]string)
	if r.PolicyEvaluator != nil {
		policyInput := evaluator.BuildInput(k8sCtx, signal)
		labels, err := r.PolicyEvaluator.EvaluateCustomLabels(ctx, policyInput)
		if err != nil {
			logger.V(1).Info("Custom labels evaluation failed, using fallback", "error", err)
		} else {
			customLabels = labels
		}
	}

	// Fallback: Extract from namespace labels if Rego Engine not available or failed
	if len(customLabels) == 0 && k8sCtx.Namespace != nil {
		// Extract team label from namespace labels (production)
		if team, ok := k8sCtx.Namespace.Labels["kubernaut.ai/team"]; ok && team != "" {
			customLabels["team"] = []string{team}
		}
		// Extract tier label (BR-SP-102: multi-key extraction support)
		if tier, ok := k8sCtx.Namespace.Labels["kubernaut.ai/tier"]; ok && tier != "" {
			customLabels["tier"] = []string{tier}
		}
		// Extract cost-center label (BR-SP-102: use correct key name)
		if cost, ok := k8sCtx.Namespace.Labels["kubernaut.ai/cost-center"]; ok && cost != "" {
			customLabels["cost-center"] = []string{cost}
		}
		// Extract region label
		if region, ok := k8sCtx.Namespace.Labels["kubernaut.ai/region"]; ok && region != "" {
			customLabels["region"] = []string{region}
		}
	}

	if len(customLabels) > 0 {
		k8sCtx.CustomLabels = customLabels
	}

	// BR-SP-110: Prepare enrichment condition (will be set inside atomic update)
	var enrichmentReason, enrichmentMessage string
	if k8sCtx.DegradedMode {
		// Degraded mode - enrichment succeeded but with partial context
		enrichmentReason = spconditions.ReasonDegradedMode
		enrichmentMessage = fmt.Sprintf("Enrichment completed in degraded mode: %s %s/%s (K8s API unavailable)",
			targetKind, targetNs, targetName)
	} else {
		enrichmentReason = ""
		enrichmentMessage = fmt.Sprintf("K8s context enriched: %s %s/%s",
			targetKind, targetNs, targetName)
	}

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidate: KubernetesContext + Phase + Conditions → 1 API call
	// BEFORE: 4 separate fields in 1 update (but refetch+update pattern)
	// AFTER: Atomic refetch → apply all → single Status().Update()
	// ========================================
	oldPhase := sp.Status.Phase
	// SP-BUG-ENRICHMENT-001: Check if enrichment already completed BEFORE status update
	// This prevents duplicate audit events when controller reconciles same enrichment twice
	enrichmentAlreadyCompleted := spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)

	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		// Apply enrichment updates after refetch
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Classifying handler after processing
		sp.Status.KubernetesContext = k8sCtx
		sp.Status.Phase = signalprocessingv1alpha1.PhaseClassifying
		// BR-SP-110: Set condition AFTER refetch to prevent wipe
		spconditions.SetEnrichmentComplete(sp, true, enrichmentReason, enrichmentMessage)
		return nil
	})
	if updateErr != nil {
		// DD-005: Track phase processing failure
		r.Metrics.IncrementProcessingTotal("enriching", "failure")
		r.Metrics.ObserveProcessingDuration("enriching", time.Since(enrichmentStart).Seconds())
		return ctrl.Result{}, updateErr
	}

	// DD-EVENT-001 v1.1: K8s events for enrichment observability
	// SP-BUG-ENRICHMENT-001: Only emit events if enrichment wasn't already completed
	if r.Recorder != nil && !enrichmentAlreadyCompleted {
		if k8sCtx.DegradedMode {
			r.Recorder.Event(sp, corev1.EventTypeWarning, events.EventReasonEnrichmentDegraded,
				enrichmentMessage)
		} else {
			r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonSignalEnriched,
				enrichmentMessage)
		}
	}

	// Record enrichment completion audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	// RF-SP-003: Track actual enrichment duration for audit metrics
	// SP-BUG-ENRICHMENT-001: Only emit audit if enrichment wasn't already completed
	enrichmentDuration := int(time.Since(enrichmentStart).Milliseconds())
	if err := r.recordEnrichmentCompleteAudit(ctx, sp, k8sCtx, enrichmentDuration, enrichmentAlreadyCompleted); err != nil {
		// DD-005: Track phase processing failure (audit failure)
		r.Metrics.IncrementProcessingTotal("enriching", "failure")
		r.Metrics.ObserveProcessingDuration("enriching", time.Since(enrichmentStart).Seconds())
		return ctrl.Result{}, err
	}

	// Record phase transition audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseClassifying)); err != nil {
		// DD-005: Track phase processing failure
		r.Metrics.IncrementProcessingTotal("enriching", "failure")
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: PhaseTransition K8s event for observability
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, signalprocessingv1alpha1.PhaseClassifying))
	}

	// DD-005: Track phase processing success and duration
	r.Metrics.IncrementProcessingTotal("enriching", "success")
	r.Metrics.ObserveProcessingDuration("enriching", time.Since(enrichmentStart).Seconds())

	// Requeue quickly to continue to next phase
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// reconcileClassifying performs environment and priority classification.
// BR-SP-051-053: Environment Classification
// BR-SP-070-072: Priority Assignment
func (r *SignalProcessingReconciler) reconcileClassifying(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Processing Classifying phase")

	// SP-BUG-PHASE-TRANSITION-002: Idempotency guard to prevent duplicate classification.decision events
	// Use non-cached APIReader to get FRESH phase data (cached client may be stale)
	currentPhase, err := r.StatusManager.GetCurrentPhase(ctx, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch current phase for idempotency check, proceeding with caution")
		// Fail-safe: continue processing, but log the error
	} else if currentPhase == signalprocessingv1alpha1.PhaseCategorizing ||
		currentPhase == signalprocessingv1alpha1.PhaseCompleted ||
		currentPhase == signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("Skipping Classifying phase - already transitioned (non-cached check)",
			"current_phase", currentPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	// Issue #437 / SP-CACHE-002: Refetch sp via APIReader to get fresh status data.
	// The informer cache (r.Get in Reconcile) may be stale when the enriching phase
	// completes quickly (e.g., degraded mode with non-existent target). Without this,
	// KubernetesContext (set by enriching) can be nil, causing environment=unknown.
	if err := r.StatusManager.FreshGet(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
		logger.Error(err, "Failed to refetch SP for fresh KubernetesContext")
		return ctrl.Result{}, err
	}

	// Issue #437: Defensive guard — requeue if enrichment data not yet visible.
	// KubernetesContext and EnrichmentComplete are set in the same AtomicStatusUpdate
	// by the enriching phase. If KubernetesContext is nil or Namespace is nil after
	// FreshGet, enrichment data hasn't propagated to the API server yet.
	k8sCtx := sp.Status.KubernetesContext
	if k8sCtx == nil || k8sCtx.Namespace == nil {
		enrichmentDone := spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)
		if !enrichmentDone {
			safetyValveExceeded := sp.Status.StartTime != nil && time.Since(sp.Status.StartTime.Time) > 30*time.Second
			if safetyValveExceeded {
				logger.Error(nil, "Issue #437: KubernetesContext incomplete after 30s safety valve, proceeding with defaults",
					"k8sCtx_nil", k8sCtx == nil,
					"sp", sp.Name)
			} else {
				logger.Info("Issue #437: KubernetesContext not yet available after FreshGet, requeuing",
					"k8sCtx_nil", k8sCtx == nil,
					"namespace_nil", k8sCtx == nil || k8sCtx.Namespace == nil,
					"sp", sp.Name)
				return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
			}
		} else {
			logger.Error(nil, "Issue #437: EnrichmentComplete=True but KubernetesContext incomplete, proceeding with defaults",
				"k8sCtx_nil", k8sCtx == nil,
				"sp", sp.Name)
		}
	}

	// DD-005: Track phase processing attempt and duration
	r.Metrics.IncrementProcessingTotal("classifying", "attempt")
	classifyingStart := time.Now()

	signal := &sp.Spec.Signal

	// Issue #437: Diagnostic logging — capture namespace labels for classification debugging.
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		logger.V(1).Info("Classification input",
			"namespace", k8sCtx.Namespace.Name,
			"labels", k8sCtx.Namespace.Labels,
			"degradedMode", k8sCtx.DegradedMode,
			"sp", sp.Name)
	}

	// ADR-060: Build unified policy input once, reuse across all evaluations
	policyInput := evaluator.BuildInput(k8sCtx, signal)

	// 1. Environment Classification (BR-SP-051-053) - MANDATORY
	envClass, err := r.classifyEnvironment(ctx, policyInput, logger)
	if err != nil {
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())

		// BR-SP-110: Set ClassificationComplete=False condition (best-effort)
		if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
			spconditions.SetClassificationComplete(sp, false, spconditions.ReasonClassificationFailed, err.Error())
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to persist ClassificationComplete=False condition")
		}

		return ctrl.Result{}, err
	}

	// 2. Priority Assignment (BR-SP-070-072) - MANDATORY
	priorityAssignment, err := r.assignPriority(ctx, policyInput, logger)
	if err != nil {
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())

		// BR-SP-110: Set ClassificationComplete=False condition (best-effort)
		if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
			spconditions.SetClassificationComplete(sp, false, spconditions.ReasonClassificationFailed, err.Error())
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to persist ClassificationComplete=False condition")
		}

		return ctrl.Result{}, err
	}

	// 3. Severity Determination (BR-SP-105, DD-SEVERITY-001) - MANDATORY
	var severityResult *evaluator.SeverityResult
	if r.PolicyEvaluator != nil {
		severityResult, err = r.PolicyEvaluator.EvaluateSeverity(ctx, policyInput)
		if err != nil {
			r.Metrics.IncrementProcessingTotal("classifying", "failure")
			r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
			logger.Error(err, "Severity determination failed - transitioning to Failed phase",
				"externalSeverity", signal.Severity,
				"hint", "Check Rego policy has else clause for unmapped values")

			updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
				sp.Status.ObservedGeneration = sp.Generation
				sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
				sp.Status.Error = fmt.Sprintf("policy evaluation failed: %v", err)
				spconditions.SetClassificationComplete(sp, false, spconditions.ReasonRegoEvaluationError, err.Error())
				spconditions.SetReady(sp, false, spconditions.ReasonNotReady, "Signal processing failed")
				return nil
			})
			if updateErr != nil {
				logger.Error(updateErr, "Failed to update status to Failed phase")
				return ctrl.Result{}, updateErr
			}

			if r.Recorder != nil {
				r.Recorder.Event(sp, corev1.EventTypeWarning, events.EventReasonPolicyEvaluationFailed,
					fmt.Sprintf("Rego policy evaluation failed for external severity %q: %v", signal.Severity, err))
			}

			return ctrl.Result{}, nil
		}
	}

	// 4. Signal Mode Classification (BR-SP-106, ADR-054) - OPTIONAL (defaults to reactive)
	// Determines if the signal is proactive or reactive, and normalizes the signal name
	// for downstream workflow catalog matching.
	var signalModeResult classifier.SignalModeResult
	if r.SignalModeClassifier != nil {
		signalModeResult = r.SignalModeClassifier.Classify(signal.Name)
	} else {
		// Default: reactive mode, name unchanged (backwards compatible)
		signalModeResult = classifier.SignalModeResult{
			SignalMode:       "reactive",
			SignalName:       signal.Name,
			SourceSignalName: "",
		}
	}
	logger.V(1).Info("Signal mode classified",
		"signalMode", signalModeResult.SignalMode,
		"signalName", signalModeResult.SignalName,
		"sourceSignalName", signalModeResult.SourceSignalName)

	// BR-SP-110: Prepare classification condition message (will be set inside atomic update)
	classificationMessage := fmt.Sprintf("Classified: environment=%s (source=%s), priority=%s (source=%s)",
		envClass.Environment, envClass.Source,
		priorityAssignment.Priority, priorityAssignment.Source)

	// Add severity to classification message if determined
	if severityResult != nil {
		classificationMessage = fmt.Sprintf("Classified: environment=%s (source=%s), priority=%s (source=%s), severity=%s (source=%s)",
			envClass.Environment, envClass.Source,
			priorityAssignment.Priority, priorityAssignment.Source,
			severityResult.Severity, severityResult.Source)
	}

	// Add signal mode to classification message
	if signalModeResult.SignalMode == "proactive" {
		classificationMessage += fmt.Sprintf(", signalMode=proactive (normalized: %s → %s)",
			signalModeResult.SourceSignalName, signalModeResult.SignalName)
	}

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidate: EnvironmentClassification + PriorityAssignment + Severity + Phase + Conditions → 1 API call
	// ========================================
	oldPhase := sp.Status.Phase
	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Categorizing handler after processing
		// Apply classification updates after refetch
		sp.Status.EnvironmentClassification = envClass
		sp.Status.PriorityAssignment = priorityAssignment
		// DD-SEVERITY-001: Set normalized severity
		if severityResult != nil {
			sp.Status.Severity = severityResult.Severity
		}
		// ADR-060: Unified policy hash covers all rules
		if r.PolicyEvaluator != nil {
			sp.Status.PolicyHash = r.PolicyEvaluator.GetPolicyHash()
		}
		// BR-SP-106: Set signal mode and normalized signal name (ADR-054)
		// SignalType is set for ALL signals (not just proactive) — it is the
		// authoritative signal name for all downstream consumers (RO, AA, HAPI).
		sp.Status.SignalMode = signalModeResult.SignalMode
		sp.Status.SignalName = signalModeResult.SignalName
		sp.Status.SourceSignalName = signalModeResult.SourceSignalName
		sp.Status.Phase = signalprocessingv1alpha1.PhaseCategorizing
		// BR-SP-110: Set condition AFTER refetch to prevent wipe
		spconditions.SetClassificationComplete(sp, true, "", classificationMessage)
		return nil
	})
	if updateErr != nil {
		// DD-005: Track phase processing failure
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
		return ctrl.Result{}, updateErr
	}

	// Record classification decision audit event (BR-SP-105, DD-SEVERITY-001)
	// Must be called after atomic status update to include normalized severity
	// **Refactoring**: 2026-01-22 - Phase 3: Use AuditManager
	if r.AuditManager != nil && severityResult != nil {
		durationMs := int(time.Since(classifyingStart).Milliseconds())
		_ = r.AuditManager.RecordClassificationDecision(ctx, sp, durationMs)
	}

	// Record phase transition audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseCategorizing)); err != nil {
		// DD-005: Track phase processing failure (audit failure)
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: PhaseTransition K8s event for observability
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, signalprocessingv1alpha1.PhaseCategorizing))
	}

	// DD-005: Track phase processing success and duration
	r.Metrics.IncrementProcessingTotal("classifying", "success")
	r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())

	// Requeue quickly to continue to next phase
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// reconcileCategorizing performs business classification and completes processing.
// BR-SP-080, BR-SP-081: Business Classification
func (r *SignalProcessingReconciler) reconcileCategorizing(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Processing Categorizing phase")

	// SP-BUG-PHASE-TRANSITION-003: Idempotency guard to prevent duplicate audit events
	// Use non-cached APIReader to get FRESH phase data (cached client may be stale)
	currentPhase, err := r.StatusManager.GetCurrentPhase(ctx, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch current phase for idempotency check, proceeding with caution")
		// Fail-safe: continue processing, but log the error
	} else if currentPhase == signalprocessingv1alpha1.PhaseCompleted ||
		currentPhase == signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("Skipping Categorizing phase - already transitioned (non-cached check)",
			"current_phase", currentPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	// SP-CACHE-002: Refetch sp via APIReader for fresh status data (same rationale as classifying).
	if err := r.StatusManager.FreshGet(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
		logger.Error(err, "Failed to refetch SP for fresh classification data")
		return ctrl.Result{}, err
	}

	// DD-005: Track phase processing attempt and duration
	r.Metrics.IncrementProcessingTotal("categorizing", "attempt")
	categorizingStart := time.Now()

	k8sCtx := sp.Status.KubernetesContext
	envClass := sp.Status.EnvironmentClassification
	priorityAssignment := sp.Status.PriorityAssignment

	// Business classification
	bizClass := r.classifyBusiness(k8sCtx, envClass, logger)

	// BR-SP-110: Prepare condition messages (will be set inside atomic update)
	categorizationMessage := fmt.Sprintf("Categorized: businessUnit=%s, criticality=%s, sla=%s",
		bizClass.BusinessUnit, bizClass.Criticality, bizClass.SLARequirement)

	var duration float64
	if sp.Status.StartTime != nil {
		duration = time.Since(sp.Status.StartTime.Time).Seconds()
	}
	var priorityStr, envStr string
	if priorityAssignment != nil {
		priorityStr = string(priorityAssignment.Priority)
	}
	if envClass != nil {
		envStr = string(envClass.Environment)
	}
	processingMessage := fmt.Sprintf("Signal processed successfully in %.2fs: %s %s alert ready for remediation",
		duration, priorityStr, envStr)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidate: BusinessClassification + Phase + CompletionTime + 2 Conditions → 1 API call
	// BEFORE: 5 status fields in 1 update (but refetch+update pattern)
	// AFTER: Atomic refetch → apply all → single Status().Update()
	// ========================================
	oldPhase := sp.Status.Phase
	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ObservedGeneration = sp.Generation // DD-CONTROLLER-001: inside callback so it survives refetch
		sp.Status.BusinessClassification = bizClass
		sp.Status.Phase = signalprocessingv1alpha1.PhaseCompleted
		now := metav1.Now()
		sp.Status.CompletionTime = &now
		spconditions.SetCategorizationComplete(sp, true, "", categorizationMessage)
		spconditions.SetProcessingComplete(sp, true, "", processingMessage)
		spconditions.SetReady(sp, true, spconditions.ReasonReady, "Signal processing completed")
		return nil
	})
	if updateErr != nil {
		// DD-005: Track phase processing failure
		r.Metrics.IncrementProcessingTotal("categorizing", "failure")
		r.Metrics.ObserveProcessingDuration("categorizing", time.Since(categorizingStart).Seconds())
		return ctrl.Result{}, updateErr
	}

	// Record phase transition audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseCompleted)); err != nil {
		// DD-005: Track phase processing failure (audit failure)
		r.Metrics.IncrementProcessingTotal("categorizing", "failure")
		r.Metrics.ObserveProcessingDuration("categorizing", time.Since(categorizingStart).Seconds())
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: PhaseTransition K8s event for observability
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, signalprocessingv1alpha1.PhaseCompleted))
	}

	// BR-SP-090: Record audit event on completion
	// ADR-032: Audit is MANDATORY - not optional. AuditClient must be wired up.
	// DD-PERF-001: After atomic update, sp object has all persisted status including BusinessClassification
	if err := r.recordCompletionAudit(ctx, sp); err != nil {
		// DD-005: Track phase processing failure (audit failure)
		r.Metrics.IncrementProcessingTotal("categorizing", "failure")
		r.Metrics.ObserveProcessingDuration("categorizing", time.Since(categorizingStart).Seconds())
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: SignalProcessed K8s event when enrichment and classification complete successfully
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonSignalProcessed, processingMessage)
	}

	// DD-005: Track phase processing success and duration
	r.Metrics.IncrementProcessingTotal("categorizing", "success")
	r.Metrics.ObserveProcessingDuration("categorizing", time.Since(categorizingStart).Seconds())

	// DD-005: Track overall signal processing completion
	r.Metrics.IncrementProcessingTotal("completed", "success")
	if sp.Status.StartTime != nil {
		totalDuration := time.Since(sp.Status.StartTime.Time).Seconds()
		r.Metrics.ObserveProcessingDuration("completed", totalDuration)
	}

	return ctrl.Result{}, nil
}

// classifyEnvironment determines the environment classification via unified evaluator.
// ADR-060: Uses PolicyEvaluator.EvaluateEnvironment instead of separate EnvironmentClassifier.
func (r *SignalProcessingReconciler) classifyEnvironment(ctx context.Context, input evaluator.PolicyInput, logger logr.Logger) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	if r.PolicyEvaluator == nil {
		return nil, fmt.Errorf("PolicyEvaluator is nil - this is a startup configuration error")
	}

	result, err := r.PolicyEvaluator.EvaluateEnvironment(ctx, input)
	if err != nil {
		logger.Error(err, "Environment classification failed")
		return nil, fmt.Errorf("environment classification failed: %w", err)
	}

	return result, nil
}

// assignPriority determines the priority via unified evaluator.
// ADR-060: Uses PolicyEvaluator.EvaluatePriority. Priority references environment
// internally within Rego -- no envClass parameter needed from Go.
func (r *SignalProcessingReconciler) assignPriority(ctx context.Context, input evaluator.PolicyInput, logger logr.Logger) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	if r.PolicyEvaluator == nil {
		return nil, fmt.Errorf("PolicyEvaluator is nil - this is a startup configuration error")
	}

	result, err := r.PolicyEvaluator.EvaluatePriority(ctx, input)
	if err != nil {
		logger.Error(err, "Priority assignment failed")
		return nil, fmt.Errorf("priority assignment failed: %w", err)
	}

	return result, nil
}

// classifyBusiness performs business classification.
// BR-SP-080, BR-SP-081: Business Classification
func (r *SignalProcessingReconciler) classifyBusiness(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, logger logr.Logger) *signalprocessingv1alpha1.BusinessClassification {
	result := &signalprocessingv1alpha1.BusinessClassification{
		Criticality:    signalprocessingv1alpha1.CriticalityMedium,
		SLARequirement: signalprocessingv1alpha1.SLARequirementBronze,
	}

	// Extract business unit from labels
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		if bu, ok := k8sCtx.Namespace.Labels["kubernaut.ai/business-unit"]; ok {
			result.BusinessUnit = bu
		} else if team, ok := k8sCtx.Namespace.Labels["kubernaut.ai/team"]; ok {
			result.BusinessUnit = team
		}
		if owner, ok := k8sCtx.Namespace.Labels["kubernaut.ai/service-owner"]; ok {
			result.ServiceOwner = owner
		}
	}

	if envClass != nil {
		switch envClass.Environment {
		case signalprocessingv1alpha1.EnvironmentProduction:
			result.Criticality = signalprocessingv1alpha1.CriticalityHigh
			result.SLARequirement = signalprocessingv1alpha1.SLARequirementGold
		case signalprocessingv1alpha1.EnvironmentStaging:
			result.Criticality = signalprocessingv1alpha1.CriticalityMedium
			result.SLARequirement = signalprocessingv1alpha1.SLARequirementSilver
		case signalprocessingv1alpha1.EnvironmentDevelopment:
			result.Criticality = signalprocessingv1alpha1.CriticalityLow
			result.SLARequirement = signalprocessingv1alpha1.SLARequirementBronze
		}
	}

	return result
}

// SetupWithManager sets up the controller with the Manager.
// DD-CONTROLLER-001: ObservedGeneration provides idempotency without blocking status updates
// GenerationChangedPredicate removed to allow phase progression via status updates
func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
// Call this after successful phase transitions.
// DD-SHARED-001: Shared Exponential Backoff Library
// DD-PERF-001: Atomic status updates
func (r *SignalProcessingReconciler) resetConsecutiveFailures(
	ctx context.Context,
	sp *signalprocessingv1alpha1.SignalProcessing,
	logger logr.Logger,
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

	// Context deadline/cancellation (often network issues)
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	return false
}

// ========================================
// ADR-032 COMPLIANT AUDIT FUNCTIONS
// ========================================
// ADR-032 §2: "No Audit Loss" - Audit writes are MANDATORY, not best-effort
// Services MUST NOT implement "graceful degradation" that silently skips audit
// Services MUST return error if audit client is nil

// recordPhaseTransitionAudit records a phase transition audit event.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// SP-BUG-002: Prevents duplicate audit events when phase hasn't actually changed (race condition mitigation).
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordPhaseTransitionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, oldPhase, newPhase string) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}
	return r.AuditManager.RecordPhaseTransition(ctx, sp, oldPhase, newPhase)
}

// recordEnrichmentCompleteAudit records an enrichment completion audit event.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// RF-SP-003: Now tracks actual enrichment duration for audit metrics.
// SP-BUG-ENRICHMENT-001: Prevents duplicate audit events when enrichment already completed (race condition mitigation).
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordEnrichmentCompleteAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, k8sCtx *signalprocessingv1alpha1.KubernetesContext, durationMs int, alreadyCompleted bool) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}

	// Create a temporary SP with enriched context for audit
	auditSP := sp.DeepCopy()
	auditSP.Status.KubernetesContext = k8sCtx
	return r.AuditManager.RecordEnrichmentComplete(ctx, auditSP, durationMs, alreadyCompleted)
}

// recordCompletionAudit records the final signal processed and classification decision audit events.
// ADR-032: Returns error if AuditManager is nil (no silent skip allowed).
// AUDIT-06: Now also emits business.classified event for granular audit trail.
//
// **Refactoring**: 2026-01-22 - Phase 3: Delegates to AuditManager for ADR-032 enforcement
func (r *SignalProcessingReconciler) recordCompletionAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) error {
	if r.AuditManager == nil {
		return fmt.Errorf("AuditManager is nil - audit is MANDATORY per ADR-032")
	}
	return r.AuditManager.RecordCompletion(ctx, sp)
}
