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

// Package handlers provides phase-specific handlers for AIAnalysis reconciliation.
package handlers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// P1.3 Refactoring: RegoEvaluatorInterface and AnalyzingAuditClientInterface moved to interfaces.go

// AnalyzingHandler handles the Analyzing phase.
// BR-AI-012: Evaluate Rego approval policies
// BR-AI-014: Graceful degradation for policy failures
// BR-AI-019: Populate ApprovalContext if approval required
//
// Per reconciliation-phases.md v2.0: Analyzing transitions directly to Completed.
// The Recommending phase was removed in v1.8 as workflow data is captured in Investigating.
type AnalyzingHandler struct {
	log         logr.Logger
	evaluator   RegoEvaluatorInterface
	metrics     *metrics.Metrics              // DD-METRICS-001: Injected metrics
	auditClient AnalyzingAuditClientInterface // DD-AUDIT-003: Injected audit client
}

// NewAnalyzingHandler creates a new AnalyzingHandler.
func NewAnalyzingHandler(evaluator RegoEvaluatorInterface, log logr.Logger, m *metrics.Metrics, auditClient AnalyzingAuditClientInterface) *AnalyzingHandler {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	return &AnalyzingHandler{
		evaluator:   evaluator,
		metrics:     m,
		auditClient: auditClient,
		log:         log.WithName("analyzing-handler"),
	}
}

// Name returns the handler name.
func (h *AnalyzingHandler) Name() string {
	return "analyzing"
}

// Handle processes the Analyzing phase.
// BR-AI-012: Evaluate Rego policies to determine approval requirement.
// BR-AI-014: If evaluation fails, default to approvalRequired=true (safe default).
// BR-AI-018: Validate workflow exists in status (from InvestigatingHandler).
// BR-AI-019: Populate ApprovalContext if approval is required.
//
// Per reconciliation-phases.md v2.0: Transitions directly to Completed (no Recommending phase).
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Analyzing phase", "name", analysis.Name)

	// Track phase for audit logging (used for idempotency check)
	oldPhase := analysis.Status.Phase

	// AA-BUG-009: Idempotency check #1 - Per RO pattern (RO_AUDIT_DUPLICATION_RISK_ANALYSIS_JAN_01_2026.md - Option C)
	// Skip if we're ALREADY in Completed state for this generation
	// This prevents duplicate processing and audit events when controller reconciles due to annotation/label changes
	if analysis.Status.ObservedGeneration == analysis.Generation && oldPhase == aianalysis.PhaseCompleted {
		h.log.Info("Already in Completed phase for this generation, skipping",
			"generation", analysis.Generation,
			"phase", oldPhase)
		return ctrl.Result{}, nil
	}

	// BR-AI-018: Validate workflow exists (captured by InvestigatingHandler)
	if analysis.Status.SelectedWorkflow == nil {
		h.log.Error(nil, "No workflow selected - investigation may have failed", "name", analysis.Name)
		analysis.Status.Phase = aianalysis.PhaseFailed
		analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
		analysis.Status.Message = "No workflow selected - investigation may have failed"
		analysis.Status.Reason = "NoWorkflowSelected"

		// BR-HAPI-197: Track failure metrics
		h.metrics.RecordFailure("NoWorkflowSelected", "InvestigationFailed") // P2.3: Use convenience method

		// DD-AUDIT-003: Record analysis failure audit event
		failureErr := fmt.Errorf("no workflow selected from investigation")
		if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
			h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
		}

		// Set WorkflowResolved=False and ApprovalRequired=False before AnalysisComplete
		aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonWorkflowResolutionFailed, "No workflow selected from investigation")
		aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No workflow to approve")
		// Set AnalysisComplete=False condition
		aianalysis.SetAnalysisComplete(analysis, false, "No workflow selected from investigation")

		// AA-BUG-008: Phase transition recorded by CONTROLLER ONLY (phase_handlers.go:215)
		// Handler changes phase but does NOT record transition (follows InvestigatingHandler pattern)
		return ctrl.Result{}, nil
	}

	// Build policy input from analysis
	input := h.buildPolicyInput(analysis)

	// Evaluate Rego policy - track duration for audit
	regoStartTime := metav1.Now()
	result, err := h.evaluator.Evaluate(ctx, input)
	regoDuration := metav1.Now().Sub(regoStartTime.Time).Milliseconds()

	// Ensure minimum duration of 1ms for audit trail (evaluation time may round to 0)
	if regoDuration == 0 {
		regoDuration = 1
	}

	if err != nil {
		// This shouldn't happen - evaluator should handle errors gracefully
		// But if it does, use safe defaults
		h.log.Error(err, "Rego evaluation returned error, using safe default")

		// Record Rego evaluation failure metric
		h.metrics.RecordRegoEvaluation("error", true)

		// DD-AUDIT-003: Record Rego evaluation audit event
		h.auditClient.RecordRegoEvaluation(ctx, analysis, "error", true, int(regoDuration), "Rego evaluation failed unexpectedly")

		analysis.Status.Phase = aianalysis.PhaseFailed
		analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
		analysis.Status.Message = "Rego evaluation failed unexpectedly"
		analysis.Status.Reason = "RegoEvaluationError"

	// BR-HAPI-197: Track failure metrics
	h.metrics.RecordFailure("RegoEvaluationError", "PolicyEvaluationFailed") // P2.3: Use convenience method

	// DD-AUDIT-003: Record analysis failure audit event
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Set WorkflowResolved=False and ApprovalRequired=False before AnalysisComplete
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonWorkflowResolutionFailed, "Rego evaluation failed, cannot resolve workflow")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "Rego evaluation failed, approval status unknown")
	// Set AnalysisComplete=False condition
	aianalysis.SetAnalysisComplete(analysis, false, "Rego policy evaluation failed: "+err.Error())

		// AA-BUG-008: Phase transition recorded by CONTROLLER ONLY (phase_handlers.go:215)
		// Handler changes phase but does NOT record transition (follows InvestigatingHandler pattern)
		return ctrl.Result{}, nil
	}

	// Record Rego evaluation metric
	outcome := "approved"
	if result.ApprovalRequired {
		outcome = "requires_approval"
	}
	h.metrics.RecordRegoEvaluation(outcome, result.Degraded)

	// DD-AUDIT-003: Record Rego evaluation audit event
	h.auditClient.RecordRegoEvaluation(ctx, analysis, outcome, result.Degraded, int(regoDuration), result.Reason)

	// Store evaluation results in status
	analysis.Status.ApprovalRequired = result.ApprovalRequired
	analysis.Status.ApprovalReason = result.Reason
	analysis.Status.DegradedMode = result.Degraded

	h.log.Info("Rego evaluation complete",
		"approvalRequired", result.ApprovalRequired,
		"degraded", result.Degraded,
		"reason", result.Reason,
	)

	// ========================================
	// IDEMPOTENCY CHECK: Only record approval decision once
	// ========================================
	// Check if ApprovalRequired condition is already set to prevent duplicate audit events
	// This can happen when controller reconciles multiple times in Analyzing phase
	// (e.g., status update triggers immediate re-reconcile before ObservedGeneration is updated)
	approvalCondition := aianalysis.GetCondition(analysis, aianalysis.ConditionApprovalRequired)
	alreadyRecorded := approvalCondition != nil && approvalCondition.Status != ""

	// BR-AI-019: Populate ApprovalContext if approval is required
	if result.ApprovalRequired {
		h.populateApprovalContext(analysis, result)
		// Set ApprovalRequired condition
		aianalysis.SetApprovalRequired(analysis, true, aianalysis.ReasonPolicyRequiresApproval, result.Reason)

		// Record approval decision metric and audit event ONLY if not already recorded
		if !alreadyRecorded {
			environment := getEnvironment(analysis)
			h.metrics.RecordApprovalDecision("requires_approval", environment)

			// DD-AUDIT-003: Record approval decision audit event (idempotent - only once)
			h.auditClient.RecordApprovalDecision(ctx, analysis, "requires_approval", result.Reason)
			h.log.V(1).Info("Approval decision recorded", "decision", "requires_approval")
		} else {
			h.log.V(1).Info("Approval decision already recorded, skipping duplicate", "decision", "requires_approval")
		}
	} else {
		// Set ApprovalRequired=False condition (auto-approved)
		aianalysis.SetApprovalRequired(analysis, false, "AutoApproved", "Policy evaluation does not require manual approval")

		// Record approval decision metric and audit event ONLY if not already recorded
		if !alreadyRecorded {
			environment := getEnvironment(analysis)
			h.metrics.RecordApprovalDecision("auto_approved", environment)

			// DD-AUDIT-003: Record approval decision audit event (idempotent - only once)
			h.auditClient.RecordApprovalDecision(ctx, analysis, "auto_approved", "Policy evaluation does not require manual approval")
			h.log.V(1).Info("Approval decision recorded", "decision", "auto_approved")
		} else {
			h.log.V(1).Info("Approval decision already recorded, skipping duplicate", "decision", "auto_approved")
		}
	}

	// Set WorkflowResolved condition (we already validated workflow exists above)
	aianalysis.SetWorkflowResolved(analysis, true, aianalysis.ReasonWorkflowSelected,
		"Workflow "+analysis.Status.SelectedWorkflow.WorkflowID+" selected with confidence "+
			formatConfidence(analysis.Status.SelectedWorkflow.Confidence))

	// Set AnalysisComplete condition
	aianalysis.SetAnalysisComplete(analysis, true, "Rego policy evaluation completed successfully")

	// Transition directly to Completed (per reconciliation-phases.md v2.0)
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	if analysis.Status.StartedAt != nil {
		analysis.Status.TotalAnalysisTime = int64(now.Sub(analysis.Status.StartedAt.Time).Seconds())
	}
	analysis.Status.Message = "Analysis complete"

	// DD-AUDIT-003: Record analysis completion when transitioning to Completed
	// AA-BUG-008: Phase transition recorded by CONTROLLER ONLY (phase_handlers.go:215)
	// Handler should NOT record phase transition to avoid duplicates (follows InvestigatingHandler pattern)
	if analysis.Status.Phase != oldPhase {
		// AA-BUG-006: Record analysis.completed here (on transition), not in recordPhaseMetrics
		// This ensures it's only recorded ONCE when transitioning TO Completed, not on every reconcile
		h.auditClient.RecordAnalysisComplete(ctx, analysis)
	}

	h.log.Info("Analysis completed",
		"name", analysis.Name,
		"approvalRequired", result.ApprovalRequired,
	)

	return ctrl.Result{}, nil // Final phase - no requeue
}

// populateApprovalContext populates the ApprovalContext for approval notifications.
// BR-AI-019: Rich context for approval notification
func (h *AnalyzingHandler) populateApprovalContext(analysis *aianalysisv1.AIAnalysis, result *rego.PolicyResult) {
	if analysis.Status.ApprovalContext == nil {
		analysis.Status.ApprovalContext = &aianalysisv1.ApprovalContext{}
	}

	ctx := analysis.Status.ApprovalContext
	ctx.Reason = result.Reason
	ctx.WhyApprovalRequired = result.Reason

	// Get confidence from SelectedWorkflow
	if analysis.Status.SelectedWorkflow != nil {
		ctx.ConfidenceScore = analysis.Status.SelectedWorkflow.Confidence

		// Set confidence level based on score
		switch {
		case ctx.ConfidenceScore >= 0.8:
			ctx.ConfidenceLevel = "high"
		case ctx.ConfidenceScore >= 0.6:
			ctx.ConfidenceLevel = "medium"
		default:
			ctx.ConfidenceLevel = "low"
		}

		// Populate RecommendedActions from SelectedWorkflow
		ctx.RecommendedActions = []aianalysisv1.RecommendedAction{
			{
				Action:    analysis.Status.SelectedWorkflow.WorkflowID,
				Rationale: analysis.Status.SelectedWorkflow.Rationale,
			},
		}
	}

	// Include investigation summary from RCA
	if analysis.Status.RootCauseAnalysis != nil {
		ctx.InvestigationSummary = analysis.Status.RootCauseAnalysis.Summary

		// Populate EvidenceCollected from contributing factors
		if len(analysis.Status.RootCauseAnalysis.ContributingFactors) > 0 {
			ctx.EvidenceCollected = analysis.Status.RootCauseAnalysis.ContributingFactors
		}
	}

	// Populate AlternativesConsidered from AlternativeWorkflows
	if len(analysis.Status.AlternativeWorkflows) > 0 {
		ctx.AlternativesConsidered = make([]aianalysisv1.AlternativeApproach, 0, len(analysis.Status.AlternativeWorkflows))
		for _, alt := range analysis.Status.AlternativeWorkflows {
			ctx.AlternativesConsidered = append(ctx.AlternativesConsidered, aianalysisv1.AlternativeApproach{
				Approach: alt.WorkflowID,
				ProsCons: alt.Rationale,
			})
		}
	}

	// Populate PolicyEvaluation with Rego details
	ctx.PolicyEvaluation = &aianalysisv1.PolicyEvaluation{
		PolicyName: "aianalysis.approval",
		Decision:   "manual_review_required",
	}
	if result.Degraded {
		ctx.PolicyEvaluation.Decision = "degraded_mode"
	}
}

// buildPolicyInput constructs the Rego policy input from AIAnalysis.
// BR-AI-012: Build input from status fields populated by InvestigatingHandler.
// Per IMPLEMENTATION_PLAN_V1.0.md lines 1756-1785 (ApprovalInput schema)
func (h *AnalyzingHandler) buildPolicyInput(analysis *aianalysisv1.AIAnalysis) *rego.PolicyInput {
	input := &rego.PolicyInput{
		// Signal context (from Spec.AnalysisRequest.SignalContext)
		SignalType:       analysis.Spec.AnalysisRequest.SignalContext.SignalName,
		Severity:         analysis.Spec.AnalysisRequest.SignalContext.Severity,
		Environment:      analysis.Spec.AnalysisRequest.SignalContext.Environment,
		BusinessPriority: analysis.Spec.AnalysisRequest.SignalContext.BusinessPriority,

		// Target resource
		TargetResource: rego.TargetResourceInput{
			Kind:      analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Kind,
			Name:      analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Name,
			Namespace: analysis.Spec.AnalysisRequest.SignalContext.TargetResource.Namespace,
		},

		// HolmesGPT-API response data
		Warnings: analysis.Status.Warnings,

		// Recovery context (from Spec)
		IsRecoveryAttempt:     analysis.Spec.IsRecoveryAttempt,
		RecoveryAttemptNumber: analysis.Spec.RecoveryAttemptNumber,
	}

	// Get confidence from SelectedWorkflow (populated by InvestigatingHandler)
	if analysis.Status.SelectedWorkflow != nil {
		input.Confidence = analysis.Status.SelectedWorkflow.Confidence
	}

	// ADR-055: Populate AffectedResource for Rego policy evaluation
	if analysis.Status.RootCauseAnalysis != nil && analysis.Status.RootCauseAnalysis.AffectedResource != nil {
		ar := analysis.Status.RootCauseAnalysis.AffectedResource
		input.AffectedResource = &rego.AffectedResourceInput{
			Kind:      ar.Kind,
			Name:      ar.Name,
			Namespace: ar.Namespace,
		}
	}

	// ADR-056: DetectedLabels read exclusively from PostRCAContext (HAPI post-RCA).
	dl := h.resolveDetectedLabels(analysis)
	input.DetectedLabels = h.detectedLabelsToMap(dl)

	if dl != nil {
		input.FailedDetections = dl.FailedDetections
	}

	// Populate CustomLabels from EnrichmentResults (Issue #113: now on KubernetesContext)
	er := analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults
	if er.KubernetesContext != nil && er.KubernetesContext.CustomLabels != nil {
		input.CustomLabels = er.KubernetesContext.CustomLabels
	} else {
		input.CustomLabels = make(map[string][]string)
	}

	// Populate BusinessClassification from EnrichmentResults (BR-SP-002, BR-SP-080, BR-SP-081)
	if bc := analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.BusinessClassification; bc != nil {
		input.BusinessClassification = make(map[string]string)
		if bc.BusinessUnit != "" {
			input.BusinessClassification["business_unit"] = bc.BusinessUnit
		}
		if bc.ServiceOwner != "" {
			input.BusinessClassification["service_owner"] = bc.ServiceOwner
		}
		if bc.Criticality != "" {
			input.BusinessClassification["criticality"] = bc.Criticality
		}
		if bc.SLARequirement != "" {
			input.BusinessClassification["sla_requirement"] = bc.SLARequirement
		}
	}

	return input
}

// formatConfidence formats a confidence score as a percentage string.
func formatConfidence(confidence float64) string {
	return fmt.Sprintf("%.0f%%", confidence*100)
}

// getEnvironment extracts environment from analysis request
func getEnvironment(analysis *aianalysisv1.AIAnalysis) string {
	return analysis.Spec.AnalysisRequest.SignalContext.Environment
}

// resolveDetectedLabels returns DetectedLabels from PostRCAContext.
// ADR-056: PostRCAContext.DetectedLabels are computed by HAPI at RCA time
// and are the sole source of cluster characteristics for Rego policy input.
func (h *AnalyzingHandler) resolveDetectedLabels(analysis *aianalysisv1.AIAnalysis) *sharedtypes.DetectedLabels {
	if analysis.Status.PostRCAContext != nil {
		return analysis.Status.PostRCAContext.DetectedLabels
	}
	return nil
}

// detectedLabelsToMap converts the typed DetectedLabels struct to a map for Rego.
// Per DD-WORKFLOW-001 v2.2: DetectedLabels uses snake_case field names in JSON.
// Rego policies access these as input.detected_labels.stateful, etc.
func (h *AnalyzingHandler) detectedLabelsToMap(dl *sharedtypes.DetectedLabels) map[string]interface{} {
	labels := make(map[string]interface{})
	if dl == nil {
		return labels
	}

	labels["git_ops_managed"] = dl.GitOpsManaged
	labels["git_ops_tool"] = dl.GitOpsTool
	labels["pdb_protected"] = dl.PDBProtected
	labels["hpa_enabled"] = dl.HPAEnabled
	labels["stateful"] = dl.Stateful
	labels["helm_managed"] = dl.HelmManaged
	labels["network_isolated"] = dl.NetworkIsolated
	labels["service_mesh"] = dl.ServiceMesh

	return labels
}
