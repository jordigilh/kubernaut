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

// Categorizing-phase reconciliation logic, split out of signalprocessing_controller.go
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file under
// the 700-line convention threshold. Pure structural move — no behavior change.
package signalprocessing

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"

	// BR-SP-110: Kubernetes Conditions
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

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
	bizClass := r.classifyBusiness(k8sCtx, envClass)

	// BR-SP-110: Prepare condition messages (will be set inside atomic update)
	categorizationMessage, processingMessage := categorizingCompletionMessages(sp, bizClass, envClass, priorityAssignment)

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

	if result, err := r.finalizeCategorizingCompletion(ctx, sp, oldPhase, processingMessage, categorizingStart); err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

// categorizingCompletionMessages builds the BR-SP-110 condition messages
// used inside the atomic status update for the Categorizing phase.
// Extracted from reconcileCategorizing (Wave 6 6e-iii GREEN: funlen
// remediation) — pure code motion, no behavior change.
func categorizingCompletionMessages(sp *signalprocessingv1alpha1.SignalProcessing, bizClass *signalprocessingv1alpha1.BusinessClassification, envClass *signalprocessingv1alpha1.EnvironmentClassification, priorityAssignment *signalprocessingv1alpha1.PriorityAssignment) (string, string) {
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

	return categorizationMessage, processingMessage
}

// finalizeCategorizingCompletion records the phase-transition and
// completion audit events (BR-SP-090, ADR-032 mandatory audit), emits the
// PhaseTransition/SignalProcessed K8s events (DD-EVENT-001 v1.1), and tracks
// per-phase and overall processing metrics (DD-005). On audit failure,
// returns a non-nil error result so the caller aborts before reaching the
// success path. Extracted from reconcileCategorizing (Wave 6 6e-iii GREEN:
// funlen remediation) — pure code motion, no behavior change.
//
//nolint:unparam // ctrl.Result is always the zero value here; signature matches the reconcile-chain (ctrl.Result, error) contract of its caller (Issue #1546 Tier 4)
func (r *SignalProcessingReconciler) finalizeCategorizingCompletion(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, oldPhase signalprocessingv1alpha1.SignalProcessingPhase, processingMessage string, categorizingStart time.Time) (ctrl.Result, error) {
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
// Unlike its sibling classify/assign helpers, this one has no fallible
// PolicyEvaluator call, so it needs no logger.
func (r *SignalProcessingReconciler) classifyBusiness(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) *signalprocessingv1alpha1.BusinessClassification {
	result := &signalprocessingv1alpha1.BusinessClassification{
		Criticality:    signalprocessingv1alpha1.CriticalityMedium,
		SLARequirement: signalprocessingv1alpha1.SLARequirementBronze,
	}

	// BLAST-A1 (BR-SP-112 R1): Extract business unit from namespace labels first,
	// then fall back to workload labels for cluster-scoped resources (e.g., Node).
	if k8sCtx != nil {
		labels := extractBusinessLabels(k8sCtx)
		if bu, ok := labels["kubernaut.ai/business-unit"]; ok {
			result.BusinessUnit = bu
		} else if team, ok := labels["kubernaut.ai/team"]; ok {
			result.BusinessUnit = team
		}
		if owner, ok := labels["kubernaut.ai/service-owner"]; ok {
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

// extractBusinessLabels returns the best available labels for business classification.
// Namespace labels take priority; workload labels are used as fallback for cluster-scoped resources.
func extractBusinessLabels(k8sCtx *signalprocessingv1alpha1.KubernetesContext) map[string]string {
	if k8sCtx.Namespace != nil && len(k8sCtx.Namespace.Labels) > 0 {
		return k8sCtx.Namespace.Labels
	}
	if k8sCtx.Workload != nil && len(k8sCtx.Workload.Labels) > 0 {
		return k8sCtx.Workload.Labels
	}
	return nil
}
