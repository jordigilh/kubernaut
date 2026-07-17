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

// Classifying-phase reconciliation logic, split out of signalprocessing_controller.go
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
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"

	// BR-SP-110: Kubernetes Conditions
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// reconcileClassifying performs environment and priority classification.
// BR-SP-051-053: Environment Classification
// BR-SP-070-072: Priority Assignment
func (r *SignalProcessingReconciler) reconcileClassifying(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, error) {
	logger.V(1).Info("Processing Classifying phase")

	if result, skip := r.checkClassifyingIdempotencyGuard(ctx, sp, logger); skip {
		return result, nil
	}

	k8sCtx, result, blocked, err := r.refreshClassifyingContext(ctx, sp, logger)
	if blocked {
		return result, err
	}

	// DD-005: Track phase processing attempt and duration
	r.Metrics.IncrementProcessingTotal("classifying", "attempt")
	classifyingStart := time.Now()

	signal := &sp.Spec.Signal
	logClassificationInput(k8sCtx, sp, logger)

	// ADR-060: Build unified policy input once, reuse across all evaluations
	policyInput := evaluator.BuildInput(k8sCtx, signal)

	// 1. Environment Classification (BR-SP-051-053) - MANDATORY
	envClass, err := r.classifyEnvironment(ctx, policyInput, logger)
	if err != nil {
		return r.failClassifyingPhase(ctx, sp, classifyingStart,
			fmt.Sprintf("environment classification failed: %v", err),
			"Failed to transition to PhaseFailed on environment classification error",
			err, logger)
	}

	// 2. Priority Assignment (BR-SP-070-072) - MANDATORY
	priorityAssignment, err := r.assignPriority(ctx, policyInput, logger)
	if err != nil {
		return r.failClassifyingPhase(ctx, sp, classifyingStart,
			fmt.Sprintf("priority assignment failed: %v", err),
			"Failed to transition to PhaseFailed on priority assignment error",
			err, logger)
	}

	// 3. Severity Determination (BR-SP-105, DD-SEVERITY-001) - MANDATORY
	severityResult, sevResult, sevFailed, err := r.evaluateSeverityOrFail(ctx, sp, policyInput, signal, classifyingStart, logger)
	if sevFailed {
		return sevResult, err
	}

	// 4. Signal Mode Classification (BR-SP-106, ADR-054) - OPTIONAL (defaults to reactive)
	// Determines if the signal is proactive or reactive, and normalizes the signal name
	// for downstream workflow catalog matching.
	signalModeResult := r.resolveSignalMode(signal)
	logger.V(1).Info("Signal mode classified",
		"signalMode", signalModeResult.SignalMode,
		"signalName", signalModeResult.SignalName,
		"sourceSignalName", signalModeResult.SourceSignalName)

	// 5. Cluster Classification (BR-FLEET-003, #1511) - OPTIONAL, non-fatal on error
	// Unlike severity, a cluster evaluation error MUST NOT transition to PhaseFailed --
	// it is an optional targeting dimension, not a correctness gate (R2).
	clusterClassification := r.evaluateClusterOrSkip(ctx, policyInput, sp, logger)

	// BR-SP-110: Prepare classification condition message (will be set inside atomic update)
	classificationMessage := buildClassificationMessage(envClass, priorityAssignment, severityResult, signalModeResult)

	return r.finalizeClassification(ctx, sp, classificationInputs{
		envClass:              envClass,
		priorityAssignment:    priorityAssignment,
		severityResult:        severityResult,
		signalModeResult:      signalModeResult,
		clusterClassification: clusterClassification,
		classificationMessage: classificationMessage,
	}, classifyingStart, logger)
}

// evaluateClusterOrSkip runs the optional BR-FLEET-003 (#1511) cluster
// classification. Unlike evaluateSeverityOrFail, ANY failure here (nil
// PolicyEvaluator, evaluation error, or an empty/no-match result) degrades
// gracefully to an empty classification -- it never blocks or fails the
// Classifying phase. This mirrors the K8sEnricher's graceful degradation for
// unregistered clusters (SI-10): cluster classification is best-effort.
func (r *SignalProcessingReconciler) evaluateClusterOrSkip(ctx context.Context, policyInput evaluator.PolicyInput, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) string {
	if r.PolicyEvaluator == nil {
		return ""
	}

	result, err := r.PolicyEvaluator.EvaluateCluster(ctx, policyInput)
	if err != nil {
		// BR-FLEET-003 R2: non-fatal -- log and continue with no classification.
		logger.V(1).Info("Cluster classification evaluation failed, continuing without classification",
			"error", err.Error(), "sp", sp.Name)
		return ""
	}
	if result == nil {
		return ""
	}
	return result.Classification
}

// logClassificationInput emits Issue #437 diagnostic logging of namespace
// labels used as classification input, when a namespace context is
// available. Extracted from reconcileClassifying (Wave 6 6e-iii GREEN:
// funlen remediation) — pure code motion, no behavior change.
func logClassificationInput(k8sCtx *signalprocessingv1alpha1.KubernetesContext, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) {
	if k8sCtx == nil || k8sCtx.Namespace == nil {
		return
	}
	logger.V(1).Info("Classification input",
		"namespace", k8sCtx.Namespace.Name,
		"labels", k8sCtx.Namespace.Labels,
		"degradedMode", k8sCtx.DegradedMode,
		"sp", sp.Name)
}

// classificationInputs bundles the classification outputs computed by
// reconcileClassifying so finalizeClassification can persist them via a
// single atomic status update (DD-PERF-001). Extracted per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep
// reconcileClassifying's parameter count low (argument-limit).
type classificationInputs struct {
	envClass              *signalprocessingv1alpha1.EnvironmentClassification
	priorityAssignment    *signalprocessingv1alpha1.PriorityAssignment
	severityResult        *evaluator.SeverityResult
	signalModeResult      classifier.SignalModeResult
	classificationMessage string
	// clusterClassification is the optional BR-FLEET-003 (#1511) cluster
	// business classification. Empty when fleet mode is disabled, the
	// cluster is unregistered, no Rego `cluster` rule matched, or
	// evaluation failed (non-fatal, unlike severity).
	clusterClassification string
}

// finalizeClassification persists the classification outputs (DD-PERF-001
// atomic update: EnvironmentClassification + PriorityAssignment + Severity +
// Phase + Conditions in one API call), records the classification-decision
// and phase-transition audit events (BR-SP-105, BR-SP-090, ADR-032 mandatory
// audit), emits the PhaseTransition K8s event (DD-EVENT-001 v1.1), and
// tracks processing metrics (DD-005). Extracted from reconcileClassifying
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) finalizeClassification(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, in classificationInputs, classifyingStart time.Time, logger logr.Logger) (ctrl.Result, error) {
	oldPhase := sp.Status.Phase
	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - will be set by Categorizing handler after processing
		sp.Status.EnvironmentClassification = in.envClass
		sp.Status.PriorityAssignment = in.priorityAssignment
		// DD-SEVERITY-001: Set normalized severity
		if in.severityResult != nil {
			sp.Status.Severity = in.severityResult.Severity
		}
		// ADR-060: Unified policy hash covers all rules
		if r.PolicyEvaluator != nil {
			sp.Status.PolicyHash = r.PolicyEvaluator.GetPolicyHash()
		}
		// BR-SP-106: Set signal mode and normalized signal name (ADR-054)
		// SignalType is set for ALL signals (not just proactive) — it is the
		// authoritative signal name for all downstream consumers (RO, AA, KA).
		sp.Status.SignalMode = in.signalModeResult.SignalMode
		sp.Status.SignalName = in.signalModeResult.SignalName
		sp.Status.SourceSignalName = in.signalModeResult.SourceSignalName
		// BR-FLEET-003 (#1511): optional cluster business classification.
		sp.Status.ClusterClassification = in.clusterClassification
		sp.Status.Phase = signalprocessingv1alpha1.PhaseCategorizing
		// BR-SP-110: Set condition AFTER refetch to prevent wipe
		spconditions.SetClassificationComplete(sp, true, "", in.classificationMessage)
		return nil
	})
	if updateErr != nil {
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
		return ctrl.Result{}, updateErr
	}

	// Record classification decision audit event (BR-SP-105, DD-SEVERITY-001)
	// Must be called after atomic status update to include normalized severity
	if r.AuditManager != nil && in.severityResult != nil {
		durationMs := int(time.Since(classifyingStart).Milliseconds())
		if auditErr := r.AuditManager.RecordClassificationDecision(ctx, sp, durationMs); auditErr != nil {
			logger.Error(auditErr, "Failed to record classification decision audit event",
				"name", sp.Name, "namespace", sp.Namespace, "phase", "Classifying")
		}
	}

	// Record phase transition audit event (BR-SP-090)
	// ADR-032: Audit is MANDATORY - return error if not configured
	if err := r.recordPhaseTransitionAudit(ctx, sp, string(oldPhase), string(signalprocessingv1alpha1.PhaseCategorizing)); err != nil {
		r.Metrics.IncrementProcessingTotal("classifying", "failure")
		r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
		return ctrl.Result{}, err
	}

	// DD-EVENT-001 v1.1: PhaseTransition K8s event for observability
	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, signalprocessingv1alpha1.PhaseCategorizing))
	}

	r.Metrics.IncrementProcessingTotal("classifying", "success")
	r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())

	// Requeue quickly to continue to next phase
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}

// checkClassifyingIdempotencyGuard prevents duplicate classification.decision
// events on reconcile re-entry (SP-BUG-PHASE-TRANSITION-002) by re-fetching
// the current phase via the non-cached APIReader. Extracted from
// reconcileClassifying per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) checkClassifyingIdempotencyGuard(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (ctrl.Result, bool) {
	currentPhase, err := r.StatusManager.GetCurrentPhase(ctx, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch current phase for idempotency check, proceeding with caution")
		// Fail-safe: continue processing, but log the error
		return ctrl.Result{}, false
	}
	if currentPhase == signalprocessingv1alpha1.PhaseCategorizing ||
		currentPhase == signalprocessingv1alpha1.PhaseCompleted ||
		currentPhase == signalprocessingv1alpha1.PhaseFailed {
		logger.V(1).Info("Skipping Classifying phase - already transitioned (non-cached check)",
			"current_phase", currentPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, true
	}
	return ctrl.Result{}, false
}

// refreshClassifyingContext refetches sp via the non-cached APIReader (Issue
// #437 / SP-CACHE-002) so KubernetesContext set by the enriching phase is
// visible even when enrichment completed very quickly, then applies the
// defensive #437 guard: requeue (or, past a 30s safety valve, proceed with
// defaults) if enrichment data hasn't propagated to the API server yet.
// Extracted from reconcileClassifying per GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 2 (issue #1520). Returns blocked=true when the caller must return
// (result, err) immediately.
func (r *SignalProcessingReconciler) refreshClassifyingContext(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, logger logr.Logger) (*signalprocessingv1alpha1.KubernetesContext, ctrl.Result, bool, error) {
	if err := r.StatusManager.FreshGet(ctx, client.ObjectKeyFromObject(sp), sp); err != nil {
		logger.Error(err, "Failed to refetch SP for fresh KubernetesContext")
		return nil, ctrl.Result{}, true, err
	}

	// Issue #437: Defensive guard — requeue if enrichment data not yet visible.
	// KubernetesContext and EnrichmentComplete are set in the same AtomicStatusUpdate
	// by the enriching phase. If KubernetesContext is nil or Namespace is nil after
	// FreshGet, enrichment data hasn't propagated to the API server yet.
	k8sCtx := sp.Status.KubernetesContext
	// BLAST-A3 (BR-SP-112 R3): nil Namespace is valid for cluster-scoped resources (e.g., Node).
	// Only apply the #437 guard when namespace is expected (namespace-scoped kinds).
	isClusterScoped := sp.Spec.Signal.TargetResource.Namespace == ""
	if k8sCtx != nil && (isClusterScoped || k8sCtx.Namespace != nil) {
		return k8sCtx, ctrl.Result{}, false, nil
	}

	enrichmentDone := spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)
	if !enrichmentDone {
		safetyValveExceeded := sp.Status.StartTime != nil && time.Since(sp.Status.StartTime.Time) > 30*time.Second
		if safetyValveExceeded {
			logger.Error(nil, "Issue #437: KubernetesContext incomplete after 30s safety valve, proceeding with defaults",
				"k8sCtx_nil", k8sCtx == nil,
				"sp", sp.Name)
			return k8sCtx, ctrl.Result{}, false, nil
		}
		logger.Info("Issue #437: KubernetesContext not yet available after FreshGet, requeuing",
			"k8sCtx_nil", k8sCtx == nil,
			"namespace_nil", k8sCtx == nil || k8sCtx.Namespace == nil,
			"sp", sp.Name)
		return nil, ctrl.Result{RequeueAfter: 500 * time.Millisecond}, true, nil
	}

	logger.Error(nil, "Issue #437: EnrichmentComplete=True but KubernetesContext incomplete, proceeding with defaults",
		"k8sCtx_nil", k8sCtx == nil,
		"sp", sp.Name)
	return k8sCtx, ctrl.Result{}, false, nil
}

// failClassifyingPhase transitions sp to terminal Failed with all downstream
// conditions (Categorization/Processing/Ready) set False (S1, DD-SP-002),
// shared by the environment-classification and priority-assignment failure
// paths (S2: non-transient errors transition to PhaseFailed). Extracted from
// reconcileClassifying per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func (r *SignalProcessingReconciler) failClassifyingPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, classifyingStart time.Time, statusErrMsg, updateErrLogMsg string, err error, logger logr.Logger) (ctrl.Result, error) {
	r.Metrics.IncrementProcessingTotal("classifying", "failure")
	r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())

	if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ObservedGeneration = sp.Generation
		sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
		sp.Status.Error = statusErrMsg
		spconditions.SetClassificationComplete(sp, false, spconditions.ReasonClassificationFailed, err.Error())
		spconditions.SetCategorizationComplete(sp, false, spconditions.ReasonCategorizationFailed, "Skipped due to classification failure")
		spconditions.SetProcessingComplete(sp, false, spconditions.ReasonProcessingFailed, "Signal processing failed during classification")
		spconditions.SetReady(sp, false, spconditions.ReasonNotReady, "Signal processing failed")
		return nil
	}); updateErr != nil {
		logger.Error(updateErr, updateErrLogMsg)
		return ctrl.Result{}, updateErr
	}
	r.Metrics.IncrementProcessingTotal("completed", "failure")
	return ctrl.Result{}, nil
}

// evaluateSeverityOrFail runs BR-SP-105/DD-SEVERITY-001 severity determination
// (skipped entirely when no PolicyEvaluator is wired) and, on failure,
// transitions sp to terminal Failed with the Rego-specific error reason plus
// a Warning K8s event and error audit event. Extracted from
// reconcileClassifying per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). Returns failed=true when the caller must return
// (result, err) immediately.
func (r *SignalProcessingReconciler) evaluateSeverityOrFail(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing, policyInput evaluator.PolicyInput, signal *signalprocessingv1alpha1.SignalData, classifyingStart time.Time, logger logr.Logger) (*evaluator.SeverityResult, ctrl.Result, bool, error) {
	if r.PolicyEvaluator == nil {
		return nil, ctrl.Result{}, false, nil
	}

	severityResult, err := r.PolicyEvaluator.EvaluateSeverity(ctx, policyInput)
	if err == nil {
		return severityResult, ctrl.Result{}, false, nil
	}

	r.Metrics.IncrementProcessingTotal("classifying", "failure")
	r.Metrics.ObserveProcessingDuration("classifying", time.Since(classifyingStart).Seconds())
	// O3: Record terminal failure at the "completed" level for dashboard rollup
	r.Metrics.IncrementProcessingTotal("completed", "failure")
	logger.Error(err, "Severity determination failed - transitioning to Failed phase",
		"externalSeverity", signal.Severity,
		"hint", "Check Rego policy has else clause for unmapped values")

	updateErr := r.StatusManager.AtomicStatusUpdate(ctx, sp, func() error {
		sp.Status.ObservedGeneration = sp.Generation
		sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
		sp.Status.Error = fmt.Sprintf("policy evaluation failed: %v", err)
		spconditions.SetClassificationComplete(sp, false, spconditions.ReasonRegoEvaluationError, err.Error())
		// S1 (DD-SP-002): Set all downstream conditions to False on terminal failure
		spconditions.SetCategorizationComplete(sp, false, spconditions.ReasonCategorizationFailed, "Skipped due to classification failure")
		spconditions.SetProcessingComplete(sp, false, spconditions.ReasonProcessingFailed, "Signal processing failed during classification")
		spconditions.SetReady(sp, false, spconditions.ReasonNotReady, "Signal processing failed")
		return nil
	})
	if updateErr != nil {
		logger.Error(updateErr, "Failed to update status to Failed phase")
		return nil, ctrl.Result{}, true, updateErr
	}

	if r.Recorder != nil {
		r.Recorder.Event(sp, corev1.EventTypeWarning, events.EventReasonPolicyEvaluationFailed,
			fmt.Sprintf("Rego policy evaluation failed for external severity %q: %v", signal.Severity, err))
	}

	// O1 (BR-SP-090): Emit error audit event on classification failure
	if r.AuditManager != nil {
		if auditErr := r.AuditManager.RecordError(ctx, sp, "Classifying", err); auditErr != nil {
			logger.Error(auditErr, "Failed to record classification error audit event",
				"name", sp.Name, "namespace", sp.Namespace, "phase", "Classifying")
		}
	}

	return nil, ctrl.Result{}, true, nil
}

// resolveSignalMode classifies proactive vs. reactive signal mode (BR-SP-106,
// ADR-054), defaulting to reactive/unchanged name when no SignalModeClassifier
// is wired (backwards compatible). Extracted from reconcileClassifying per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *SignalProcessingReconciler) resolveSignalMode(signal *signalprocessingv1alpha1.SignalData) classifier.SignalModeResult {
	if r.SignalModeClassifier != nil {
		return r.SignalModeClassifier.Classify(signal.Name)
	}
	return classifier.SignalModeResult{
		SignalMode:       signalprocessingv1alpha1.SignalModeReactive,
		SignalName:       signal.Name,
		SourceSignalName: "",
	}
}

// buildClassificationMessage renders the ClassificationComplete condition
// message from the environment/priority/severity/signal-mode results
// (BR-SP-110). Extracted from reconcileClassifying per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520). Pure function — no
// side effects.
func buildClassificationMessage(envClass *signalprocessingv1alpha1.EnvironmentClassification, priorityAssignment *signalprocessingv1alpha1.PriorityAssignment, severityResult *evaluator.SeverityResult, signalModeResult classifier.SignalModeResult) string {
	classificationMessage := fmt.Sprintf("Classified: environment=%s (source=%s), priority=%s (source=%s)",
		envClass.Environment, envClass.Source,
		priorityAssignment.Priority, priorityAssignment.Source)

	if severityResult != nil {
		classificationMessage = fmt.Sprintf("Classified: environment=%s (source=%s), priority=%s (source=%s), severity=%s (source=%s)",
			envClass.Environment, envClass.Source,
			priorityAssignment.Priority, priorityAssignment.Source,
			severityResult.Severity, severityResult.Source)
	}

	if signalModeResult.SignalMode == signalprocessingv1alpha1.SignalModeProactive {
		classificationMessage += fmt.Sprintf(", signalMode=proactive (normalized: %s → %s)",
			signalModeResult.SourceSignalName, signalModeResult.SignalName)
	}

	return classificationMessage
}
