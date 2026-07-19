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

// Enriching-phase reconciliation logic, split out of signalprocessing_controller.go
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file under
// the 700-line convention threshold. Pure structural move — no behavior change.
package signalprocessing

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"

	// BR-SP-110: Kubernetes Conditions
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// reconcileEnriching performs context enrichment based on the signal's target type.
//
// BR-SP-001: K8s Context Enrichment
// BR-SP-100: Owner Chain Traversal
// ADR-056: BR-SP-101 (Detected Labels) relocated to KA post-RCA
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

	if result, skip := r.checkEnrichingIdempotencyGuard(ctx, sp, logger); skip {
		return result, nil
	}

	// DD-005: Track phase processing attempt
	r.Metrics.IncrementProcessingTotal("enriching", "attempt")

	// RF-SP-003: Track enrichment duration for audit metrics
	enrichmentStart := time.Now()

	signal := &sp.Spec.Signal
	r.warnIfUnsupportedTargetType(sp, signal, logger)

	k8sCtx, result, failed, err := r.performK8sEnrichment(ctx, sp, signal, enrichmentStart, logger)
	if failed {
		return result, err
	}

	r.applyEnrichmentCustomLabels(ctx, k8sCtx, signal, logger)

	enrichmentReason, enrichmentMessage := buildEnrichmentMessage(k8sCtx, signal.TargetResource)

	return r.finalizeEnrichment(ctx, sp, k8sCtx, enrichmentReason, enrichmentMessage, enrichmentStart)
}

// checkEnrichingIdempotencyGuard prevents duplicate phase-transition work
// (SP-BUG-PHASE-TRANSITION-001) by re-fetching the current phase via the
// non-cached APIReader. Extracted from reconcileEnriching per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) checkEnrichingIdempotencyGuard(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, bool) {
	currentPhase, err := r.StatusManager.GetCurrentPhase(ctx, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch current phase for idempotency check, proceeding with caution")
		// Fail-safe: continue processing, but log the error
		return ctrl.Result{}, false
	}
	if currentPhase == signalprocessingv1alpha1.PhaseClassifying ||
		currentPhase == signalprocessingv1alpha1.PhaseCategorizing ||
		currentPhase == signalprocessingv1alpha1.PhaseCompleted ||
		currentPhase == signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("Skipping Enriching phase - already transitioned (non-cached check)",
			"current_phase", currentPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, true
	}
	return ctrl.Result{}, false
}

// warnIfUnsupportedTargetType emits a degraded-mode warning event for
// non-kubernetes target types (Issue #419 / V2.0: only "kubernetes"
// enrichment is implemented). Extracted from reconcileEnriching per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) warnIfUnsupportedTargetType(sp *signalprocessingv1alpha1.SignalProcessing, signal *signalprocessingv1alpha1.SignalData, logger logr.Logger) {
	if signal.TargetType == "" || signal.TargetType == "kubernetes" {
		return
	}
	logger.Info("Non-kubernetes targetType received; enrichment will run in degraded mode",
		"targetType", signal.TargetType)
	// E3: Guard against nil Recorder
	if r.Recorder != nil {
		r.Recorder.Eventf(sp, corev1.EventTypeWarning, events.EventReasonUnsupportedTargetType,
			"targetType %q is not yet supported for enrichment (V2.0); proceeding in degraded mode", signal.TargetType)
	}
}

// performK8sEnrichment invokes the mandatory K8sEnricher (BR-SP-001) and
// handles the full hard-failure path (metrics, condition, audit, K8s event)
// on error. Extracted from reconcileEnriching per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520). Returns failed=true
// when the caller must return (result, err) immediately.
//
//nolint:unparam // ctrl.Result is always the zero value here; signature matches the "caller must return (result, err)" contract shared with sibling extracted helpers (Issue #1546 Tier 4)
func (r *SignalProcessingReconciler) performK8sEnrichment(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, signal *signalprocessingv1alpha1.SignalData, enrichmentStart time.Time, logger logr.Logger) (*signalprocessingv1alpha1.KubernetesContext, ctrl.Result, bool, error) {
	// BR-SP-001: K8sEnricher is MANDATORY - fail loudly if not wired or fails
	// No fallback path - enrichment failure should stop processing
	if r.K8sEnricher == nil {
		return nil, ctrl.Result{}, true, fmt.Errorf("K8sEnricher is nil - this is a startup configuration error")
	}

	targetNs := signal.TargetResource.Namespace
	targetKind := signal.TargetResource.Kind
	targetName := signal.TargetResource.Name

	k8sCtx, err := r.K8sEnricher.Enrich(ctx, signal)
	if err == nil {
		return k8sCtx, ctrl.Result{}, false, nil
	}

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
		if auditErr := r.AuditManager.RecordError(ctx, sp, "Enriching", err); auditErr != nil {
			logger.Error(auditErr, "Failed to record enrichment error audit event",
				"name", sp.Name, "namespace", sp.Namespace, "phase", "Enriching")
		}
	}

	// O6 (DD-EVENT-001): Emit Warning K8s event on hard enrichment failure
	if r.Recorder != nil {
		var enrichFailMsg string
		if targetNs == "" {
			enrichFailMsg = fmt.Sprintf("K8s enrichment failed for %s %s: %v", targetKind, targetName, err)
		} else {
			enrichFailMsg = fmt.Sprintf("K8s enrichment failed for %s %s/%s: %v", targetKind, targetNs, targetName, err)
		}
		r.Recorder.Event(sp, corev1.EventTypeWarning, events.EventReasonEnrichmentFailed, enrichFailMsg)
	}

	return nil, ctrl.Result{}, true, fmt.Errorf("enrichment failed: %w", err)
}

// applyEnrichmentCustomLabels resolves custom labels via the policy evaluator
// (BR-SP-102, ADR-060) with a namespace/workload-label fallback (BLAST-A2,
// BR-SP-112 R2), mutating k8sCtx.CustomLabels in place. Extracted from
// reconcileEnriching per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) applyEnrichmentCustomLabels(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData, logger logr.Logger) {
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

	// BLAST-A2 (BR-SP-112 R2): Fallback label source — namespace first, then workload.
	if len(customLabels) == 0 {
		fallbackLabels := extractBusinessLabels(k8sCtx)
		if team, ok := fallbackLabels["kubernaut.ai/team"]; ok && team != "" {
			customLabels["team"] = []string{team}
		}
		if tier, ok := fallbackLabels["kubernaut.ai/tier"]; ok && tier != "" {
			customLabels["tier"] = []string{tier}
		}
		if cost, ok := fallbackLabels["kubernaut.ai/cost-center"]; ok && cost != "" {
			customLabels["cost-center"] = []string{cost}
		}
		if region, ok := fallbackLabels["kubernaut.ai/region"]; ok && region != "" {
			customLabels["region"] = []string{region}
		}
	}

	if len(customLabels) > 0 {
		k8sCtx.CustomLabels = customLabels
	}
}

// buildEnrichmentMessage derives the EnrichmentComplete condition's
// reason/message from the enrichment outcome (degraded vs. full). Extracted
// from reconcileEnriching per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). Pure function — no side effects.
func buildEnrichmentMessage(k8sCtx *signalprocessingv1alpha1.KubernetesContext, target signalprocessingv1alpha1.ResourceIdentifier) (string, string) {
	targetNs := target.Namespace
	targetKind := target.Kind
	targetName := target.Name

	if k8sCtx.DegradedMode {
		// BLAST-B2: Use clean format for cluster-scoped resources (no empty namespace prefix)
		if targetNs == "" {
			return spconditions.ReasonDegradedMode, fmt.Sprintf("Enrichment completed in degraded mode: %s %s (K8s API unavailable)",
				targetKind, targetName)
		}
		return spconditions.ReasonDegradedMode, fmt.Sprintf("Enrichment completed in degraded mode: %s %s/%s (K8s API unavailable)",
			targetKind, targetNs, targetName)
	}

	if targetNs == "" {
		return "", fmt.Sprintf("K8s context enriched: %s %s", targetKind, targetName)
	}
	return "", fmt.Sprintf("K8s context enriched: %s %s/%s", targetKind, targetNs, targetName)
}

// finalizeEnrichment persists the enriched context + phase transition
// (DD-PERF-001 atomic update), then emits the observability trail (K8s
// events, completion audit, phase-transition audit) and success metrics.
// All failures are returned directly to the caller (which logs via the
// reconcile-chain contract), so this needs no logger of its own.
// Extracted from reconcileEnriching per GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) finalizeEnrichment(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, k8sCtx *signalprocessingv1alpha1.KubernetesContext, enrichmentReason, enrichmentMessage string, enrichmentStart time.Time) (ctrl.Result, error) {
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
