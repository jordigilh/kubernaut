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

// Package handlers implements phase handlers for the AIAnalysis controller.
// This file contains response processing logic extracted from investigating.go.
//
// Refactoring: P1.1 - Extract ResponseProcessor (Dec 20, 2025)
// Purpose: Separate response processing concerns from handler orchestration
// Benefits: Improved testability, reduced file size, single responsibility
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ResponseProcessor handles processing of HolmesGPT-API responses
// BR-AI-008: Capture all response fields including RCA, workflow, and alternatives
// BR-HAPI-197: Check needs_human_review before proceeding
// BR-HAPI-200: Handle problem_resolved outcomes
type ResponseProcessor struct {
	log         logr.Logger
	metrics     *metrics.Metrics
	auditClient AuditClientInterface
}

// NewResponseProcessor creates a new ResponseProcessor
func NewResponseProcessor(log logr.Logger, m *metrics.Metrics, auditClient AuditClientInterface) *ResponseProcessor {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	return &ResponseProcessor{
		log:         log.WithName("response-processor"),
		metrics:     m,
		auditClient: auditClient,
	}
}

// ProcessIncidentResponse processes the IncidentResponse from generated client
// BR-AI-009: Reset failure counter on successful API call
// BR-HAPI-197: Check needs_human_review before proceeding
func (p *ResponseProcessor) ProcessIncidentResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	// BR-AI-009: Reset failure counter on successful API call
	analysis.Status.ConsecutiveFailures = 0

	// #388: All processed alerts are actionable by default; only the
	// NotActionable handler overrides this to "NotActionable".
	analysis.Status.Actionability = aianalysis.ActionabilityActionable

	// Check if NeedsHumanReview is set
	needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null

	p.log.Info("Processing successful incident response",
		"confidence", resp.Confidence,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", hasSelectedWorkflow,
		"needsHumanReview", needsHumanReview,
	)

	// BR-AI-OBSERVABILITY-004: Record confidence score for AI quality tracking
	p.metrics.RecordConfidenceScore(analysis.Spec.AnalysisRequest.SignalContext.SignalName, resp.Confidence)

	// BR-HAPI-197: Check if KA explicitly requires human review (Layer 1 - Primary)
	// CRITICAL: This MUST be checked FIRST. KA's explicit needs_human_review=true
	// takes priority over all other classification logic.
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
	}

	// BR-HAPI-200.6 Outcome A: Problem confidently resolved, no workflow needed
	// Detection per BR-HAPI-200.6: needs_human_review=false AND selected_workflow=null AND confidence >= 0.7
	// Defense-in-depth (Layer 2): also verify no warning signals that indicate an active
	// problem (inconclusive investigation, no matching workflows). This catches edge cases
	// where the LLM incorrectly overrides needs_human_review=false but KA still appends
	// diagnostic warnings from its investigation_outcome parsing.
	// #208 (Layer 3): If the LLM provided a substantive RCA (with contributing factors),
	// it identified a real problem. Route to human review since no workflow was selected,
	// rather than silently closing as "no action required."
	// #301: When KA's "Problem self-resolved" signal is present (from investigation_outcome=resolved),
	// bypass the hasSubstantiveRCA check — the RCA documents the transient condition for audit,
	// not an ongoing problem requiring intervention.
	// #607: When agent explicitly says "not actionable", Outcome D must win over Outcome A.
	// This matches Python KA's precedence where actionable=false is evaluated first.
	isResolved := hasProblemResolvedSignal(resp.Warnings)
	if !hasSelectedWorkflow && resp.Confidence >= 0.7 && !hasNoWorkflowWarningSignal(resp.Warnings) && !hasNotActionableSignal(resp.Warnings) && (isResolved || !hasSubstantiveRCA(resp.RootCauseAnalysis)) {
		return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
	}

	// #388 + #607 Outcome D: Alert not actionable — benign condition, no remediation warranted.
	// When the agent signals actionable=false (via warning + is_actionable field), this is an
	// authoritative LLM determination — same trust pattern as needs_human_review (line 94).
	// #607: Removed confidence >= 0.7 gate. The LLM's explicit actionable=false is trusted
	// regardless of confidence. The agent applies a confidence floor of 0.8 as defense-in-depth.
	isNotActionable := hasNotActionableSignal(resp.Warnings)
	isActionablePtr := GetOptNilBoolValue(resp.IsActionable)
	if !hasSelectedWorkflow && isNotActionable && isActionablePtr != nil && !*isActionablePtr {
		return p.handleNotActionableFromIncident(ctx, analysis, resp)
	}

	// BR-AI-050 + Issue #29: No workflow found (terminal failure requiring human review)
	// Reached when: (a) confidence < 0.7 with no workflow, OR
	// (b) confidence >= 0.7 but warning signals indicate an active problem (defense-in-depth)
	if !hasSelectedWorkflow {
		return p.handleNoWorkflowTerminalFailure(ctx, analysis, resp)
	}

	// BR-HAPI-197 AC-4 + Issue #28: AIAnalysis applies confidence threshold (V1.0: 70%)
	// KA returns confidence but does NOT enforce thresholds - AIAnalysis owns this logic
	const confidenceThreshold = 0.7 // TODO V1.1: Make configurable per BR-HAPI-198

	if hasSelectedWorkflow && resp.Confidence < confidenceThreshold {
		return p.handleLowConfidenceFailure(ctx, analysis, resp)
	}

	// All checks passed - store KA response metadata and continue processing
	analysis.Status.Warnings = resp.Warnings
	analysis.Status.InvestigationID = resp.IncidentID

	// ADR-056: Extract detected_labels from KA response into PostRCAContext
	p.populatePostRCAContext(analysis, resp.DetectedLabels.Value, resp.DetectedLabels.Set, resp.DetectedLabels.Null)

	// ADR-055: TargetInOwnerChain removed. remediationTarget is now a first-class
	// LLM RCA output, not derived from pre-computed owner chain.

	// Store root cause analysis (if present) - uses centralized helper with remediationTarget
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCause = rca.Summary
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// Store selected workflow (DD-CONTRACT-002)
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			sw := &aianalysisv1.SelectedWorkflow{
				WorkflowID:            GetStringFromMap(swMap, "workflow_id"),
				ActionType:            GetStringFromMap(swMap, "action_type"),
				Version:               GetStringFromMap(swMap, "version"),
				ExecutionBundle:       GetStringFromMap(swMap, "execution_bundle"),
				ExecutionBundleDigest: GetStringFromMap(swMap, "execution_bundle_digest"),
				Confidence:            GetFloat64FromMap(swMap, "confidence"),
				Rationale:             GetStringFromMap(swMap, "rationale"),
				ExecutionEngine:       GetStringFromMap(swMap, "execution_engine"),
			}
			// Map parameters if present (map[string]string)
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					sw.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
			// BR-WE-016: Extract engine_config as raw JSON for pass-through
			if ecRaw, ok := swMap["engine_config"]; ok && ecRaw != nil {
				if ecBytes, err := json.Marshal(ecRaw); err == nil {
					sw.EngineConfig = &apiextensionsv1.JSON{Raw: ecBytes}
				}
			}
			analysis.Status.SelectedWorkflow = sw
		}
	}

	// Store alternative workflows (INFORMATIONAL ONLY - NOT for execution)
	if len(resp.AlternativeWorkflows) > 0 {
		alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
		for _, alt := range resp.AlternativeWorkflows {
			executionBundle := ""
			if alt.ExecutionBundle.Set && !alt.ExecutionBundle.Null {
				executionBundle = alt.ExecutionBundle.Value
			}
			alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
				WorkflowID:      alt.WorkflowID,
				ExecutionBundle:  executionBundle,
				Confidence:      alt.Confidence,
				Rationale:       alt.Rationale,
			})
		}
		analysis.Status.AlternativeWorkflows = alternatives
	}

	// BR-HAPI-197: No human review needed for successful workflow selection
	analysis.Status.NeedsHumanReview = false

	// Set InvestigationComplete condition
	aianalysis.SetInvestigationComplete(analysis, true, "HolmesGPT-API investigation completed successfully")

	// Transition to Analyzing phase
	// Note: ObservedGeneration set by InvestigatingHandler before returning
	analysis.Status.Phase = aianalysis.PhaseAnalyzing
	analysis.Status.Message = "Investigation complete, starting analysis"

	// DD-AUDIT-003: Phase transition audit recorded by InvestigatingHandler (investigating.go:177)
	// NOT recorded here to avoid duplicates - handler records after status is committed

	return ctrl.Result{Requeue: true}, nil
}

// populatePostRCAContext extracts detected_labels from the KA response raw map
// and sets PostRCAContext on the AIAnalysis status.
// ADR-056: DetectedLabels flow from KA → PostRCAContext for Rego policy input.
func (p *ResponseProcessor) populatePostRCAContext(analysis *aianalysisv1.AIAnalysis, detectedLabelsRaw interface{}, isSet bool, isNull bool) {
	if !isSet || isNull {
		return
	}

	dlMap := GetMapFromOptNil(detectedLabelsRaw)
	if len(dlMap) == 0 {
		return
	}

	dl := extractDetectedLabels(dlMap)
	now := metav1.Now()
	analysis.Status.PostRCAContext = &aianalysisv1.PostRCAContext{
		DetectedLabels: dl,
		SetAt:          &now,
	}

	p.log.Info("Populated PostRCAContext from KA detected_labels",
		"gitOpsManaged", dl.GitOpsManaged,
		"stateful", dl.Stateful,
		"failedDetections", dl.FailedDetections,
	)
}

// extractDetectedLabels converts a raw map from the KA response into the
// strongly-typed DetectedLabels struct. Fields not present default to zero values.
func extractDetectedLabels(m map[string]interface{}) *sharedtypes.DetectedLabels {
	return &sharedtypes.DetectedLabels{
		GitOpsManaged:            GetBoolFromMap(m, "gitOpsManaged"),
		GitOpsTool:               GetStringFromMap(m, "gitOpsTool"),
		PDBProtected:             GetBoolFromMap(m, "pdbProtected"),
		HPAEnabled:               GetBoolFromMap(m, "hpaEnabled"),
		Stateful:                 GetBoolFromMap(m, "stateful"),
		HelmManaged:              GetBoolFromMap(m, "helmManaged"),
		NetworkIsolated:          GetBoolFromMap(m, "networkIsolated"),
		ServiceMesh:              GetStringFromMap(m, "serviceMesh"),
		ResourceQuotaConstrained: GetBoolFromMap(m, "resourceQuotaConstrained"),
		FailedDetections:         GetStringSliceFromMap(m, "failedDetections"),
	}
}

// handleWorkflowResolutionFailureFromIncident handles workflow resolution failure from IncidentResponse
// BR-HAPI-197: Workflow resolution failed, human must intervene
// #768: Delegates to handleNoMatchingWorkflowsCompleted when humanReviewReason=no_matching_workflows
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromIncident(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	humanReviewReason := ""
	if resp.HumanReviewReason.Set && !resp.HumanReviewReason.Null {
		humanReviewReason = string(resp.HumanReviewReason.Value)
	}

	// #768: no_matching_workflows is a successful investigation — route to Completed handler
	if humanReviewReason == "no_matching_workflows" {
		return p.handleNoMatchingWorkflowsCompleted(ctx, analysis, resp)
	}

	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null

	p.log.Info("Workflow resolution failed, requires human review",
		"warnings", resp.Warnings,
		"humanReviewReason", humanReviewReason,
		"hasPartialWorkflow", hasSelectedWorkflow,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysisv1.ReasonWorkflowResolutionFailed
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197: Store human review flag and reason in CRD status
	analysis.Status.NeedsHumanReview = true
	if humanReviewReason != "" {
		analysis.Status.HumanReviewReason = humanReviewReason
	}

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if resp.HumanReviewReason.IsSet() {
		subReason = string(resp.HumanReviewReason.Value)
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// DD-AUDIT-003: Record analysis failure audit event
	failureErr := fmt.Errorf("workflow resolution failed: %s", humanReviewReason)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Map HumanReviewReason enum to SubReason
	if humanReviewReason != "" {
		analysis.Status.SubReason = p.mapEnumToSubReason(humanReviewReason)
	} else {
		analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
	}

	// Handle ValidationAttemptsHistory from generated types
	var messageParts []string
	if len(resp.ValidationAttemptsHistory) > 0 {
		for _, genAttempt := range resp.ValidationAttemptsHistory {
			attempt := aianalysisv1.ValidationAttempt{
				Attempt: genAttempt.Attempt,
				IsValid: genAttempt.IsValid,
				Errors:  genAttempt.Errors,
			}
			if genAttempt.WorkflowID.Set {
				attempt.WorkflowID = genAttempt.WorkflowID.Value
			}
			// Parse timestamp string to metav1.Time
			if parsedTime, err := time.Parse(time.RFC3339, genAttempt.Timestamp); err == nil {
				attempt.Timestamp = metav1.NewTime(parsedTime)
			} else {
				// Fallback to current time if parsing fails
				attempt.Timestamp = metav1.Now()
			}
			analysis.Status.ValidationAttemptsHistory = append(analysis.Status.ValidationAttemptsHistory, attempt)

			// Build operator-friendly message from validation attempts
			if len(genAttempt.Errors) > 0 {
				messageParts = append(messageParts, fmt.Sprintf("Attempt %d: %s", genAttempt.Attempt, strings.Join(genAttempt.Errors, ", ")))
			}
		}
	}

	// Issue #588: Message contains only validation attempt errors.
	// Warnings are stored separately in Status.Warnings to prevent duplication
	// when buildManualReviewBody renders both Details and Warnings sections.
	analysis.Status.Message = strings.Join(messageParts, "; ")
	analysis.Status.Warnings = resp.Warnings

	// Preserve partial response if available
	if hasSelectedWorkflow {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:         GetStringFromMap(swMap, "workflow_id"),
				ExecutionBundle:    GetStringFromMap(swMap, "execution_bundle"),
				Confidence:         GetFloat64FromMap(swMap, "confidence"),
				Rationale:          GetStringFromMap(swMap, "rationale"),
				ExecutionEngine:    GetStringFromMap(swMap, "execution_engine"),
			}
		}
	}

	// Preserve RCA if available - Issue #97: uses centralized helper with remediationTarget
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	aianalysis.SetInvestigationComplete(analysis, false, fmt.Sprintf("workflow resolution failed: %s", humanReviewReason))
	aianalysis.SetAnalysisComplete(analysis, false, fmt.Sprintf("workflow resolution failed: %s", humanReviewReason))
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonWorkflowResolutionFailed, "Workflow resolution failed, requires human review")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No resolved workflow, approval not applicable")
	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleProblemResolvedFromIncident handles problem self-resolved from IncidentResponse
// BR-HAPI-200: Problem confirmed resolved, no workflow needed
func (p *ResponseProcessor) handleProblemResolvedFromIncident(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	p.log.Info("Problem confirmed resolved, no workflow needed",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysisv1.ReasonWorkflowNotNeeded
	analysis.Status.SubReason = "ProblemResolved"
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197: No human review needed for resolved problems
	analysis.Status.NeedsHumanReview = false

	if resp.Analysis != "" {
		analysis.Status.Message = resp.Analysis
	} else if len(resp.Warnings) > 0 {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	} else {
		analysis.Status.Message = "Problem self-resolved. No remediation required."
	}

	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available - Issue #97: uses centralized helper with remediationTarget
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	aianalysis.SetInvestigationComplete(analysis, true, "Investigation completed: problem self-resolved")
	aianalysis.SetAnalysisComplete(analysis, true, "Analysis completed: no workflow needed, problem resolved")
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonNoWorkflowNeeded, "Problem self-resolved, no workflow needed")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No workflow selected, approval not applicable")

	// BR-HAPI-200: Record analysis completion audit event
	// This is a successful completion even though no workflow was selected
	p.auditClient.RecordAnalysisComplete(ctx, analysis)

	return ctrl.Result{}, nil
}

// handleNotActionableFromIncident handles alert-not-actionable outcomes from IncidentResponse.
// #388: Alert is benign — condition may be present but is harmless (e.g., orphaned PVCs).
// Routes to Completed/WorkflowNotNeeded/NotActionable, analogous to handleProblemResolvedFromIncident
// but semantically distinct: resolved = problem went away, not-actionable = problem is harmless.
func (p *ResponseProcessor) handleNotActionableFromIncident(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	p.log.Info("Alert not actionable, no workflow needed",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysisv1.ReasonWorkflowNotNeeded
	analysis.Status.SubReason = "NotActionable"
	analysis.Status.InvestigationID = resp.IncidentID

	// #388: Benign alerts never require human review
	analysis.Status.NeedsHumanReview = false
	analysis.Status.Actionability = aianalysis.ActionabilityNotActionable

	if resp.Analysis != "" {
		analysis.Status.Message = resp.Analysis
	} else if len(resp.Warnings) > 0 {
		analysis.Status.Message = strings.Join(resp.Warnings, "; ")
	} else {
		analysis.Status.Message = "Alert not actionable. No remediation warranted."
	}

	analysis.Status.Warnings = resp.Warnings

	// Store RCA for audit trail — benign conditions still warrant documentation
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	aianalysis.SetInvestigationComplete(analysis, true, "Investigation completed: alert not actionable")
	aianalysis.SetAnalysisComplete(analysis, true, "Analysis completed: no workflow needed, alert not actionable")
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonNoWorkflowNeeded, "Alert not actionable, no workflow needed")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No workflow selected, approval not applicable")

	p.auditClient.RecordAnalysisComplete(ctx, analysis)

	return ctrl.Result{}, nil
}

// handleNoMatchingWorkflowsCompleted handles the case where the investigation succeeded
// but no workflow matched the incident. This is Phase=Completed (not Failed) because the
// analysis was successful — it correctly concluded that no automated remediation is available.
//
// Issue #768: Phase should be Completed when humanReviewReason=no_matching_workflows
// Issue #769: rootCauseAnalysis must be preserved (rootCause must be populated)
func (p *ResponseProcessor) handleNoMatchingWorkflowsCompleted(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	humanReviewReason := ""
	if resp.HumanReviewReason.Set && !resp.HumanReviewReason.Null {
		humanReviewReason = string(resp.HumanReviewReason.Value)
	}

	p.log.Info("Investigation succeeded, no matching workflows — completing with human review",
		"confidence", resp.Confidence,
		"humanReviewReason", humanReviewReason,
		"warnings", resp.Warnings,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseCompleted
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysisv1.ReasonAnalysisCompleted
	analysis.Status.SubReason = "NoMatchingWorkflows"
	analysis.Status.InvestigationID = resp.IncidentID

	// #768: NeedsHumanReview remains true — still requires human intervention
	analysis.Status.NeedsHumanReview = true
	if humanReviewReason != "" {
		analysis.Status.HumanReviewReason = humanReviewReason
	}

	// Build operator-friendly message
	analysis.Status.Message = "Investigation completed: no matching workflows found"
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// #769: Preserve RCA — both rootCause (summary) and rootCauseAnalysis (full struct)
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCause = rca.Summary
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// #768: Conditions reflect successful investigation, no workflow match
	aianalysis.SetInvestigationComplete(analysis, true, "Investigation completed successfully")
	aianalysis.SetAnalysisComplete(analysis, true, "Analysis completed: no matching workflows found")
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonNoMatchingWorkflows, "No matching workflows found, human review required")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No workflow selected, approval not applicable")

	// #768: Audit as completion (not failure) — this also feeds RR reconstruction
	p.auditClient.RecordAnalysisComplete(ctx, analysis)

	return ctrl.Result{}, nil
}

// handleNoWorkflowTerminalFailure handles terminal failure when no workflow selected with low confidence
// Issue #29: BR-AI-050 - AIAnalysis must detect terminal failure per BR-HAPI-197 AC-4
func (p *ResponseProcessor) handleNoWorkflowTerminalFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	p.log.Info("No workflow selected, terminal failure",
		"confidence", resp.Confidence,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "NoMatchingWorkflows" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for terminal failures
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "no_matching_workflows"

	// Build operator-friendly message
	analysis.Status.Message = "No workflow selected for remediation"
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Store RCA if available (for human review context) - Issue #97: centralized helper
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// BR-AI-050: Emit audit event for terminal failure
	failureErr := fmt.Errorf("no workflow selected: no matching workflows found")
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoMatchingWorkflows").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "NoMatchingWorkflows")

	aianalysis.SetInvestigationComplete(analysis, false, "no workflow selected: no matching workflows found")
	aianalysis.SetAnalysisComplete(analysis, false, "No workflow selected for remediation")
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonWorkflowResolutionFailed, "No matching workflows found")
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "No workflow selected, approval not applicable")
	return ctrl.Result{}, nil // Terminal - no requeue
}

// handleLowConfidenceFailure handles workflow selection with confidence below threshold
// Issue #28: BR-HAPI-197 AC-4 - AIAnalysis applies confidence threshold (not KA)
func (p *ResponseProcessor) handleLowConfidenceFailure(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *agentclient.IncidentResponse) (ctrl.Result, error) {
	const confidenceThreshold = 0.7 // V1.0: global 70% default

	p.log.Info("Low confidence workflow, requires human review",
		"confidence", resp.Confidence,
		"threshold", confidenceThreshold,
		"warnings", resp.Warnings,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	setTotalAnalysisTime(analysis, now)
	analysis.Status.Reason = aianalysis.ReasonWorkflowResolutionFailed
	analysis.Status.SubReason = "LowConfidence" // Maps to CRD SubReason enum
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197 AC-4: AIAnalysis sets needs_human_review for low confidence
	analysis.Status.NeedsHumanReview = true
	analysis.Status.HumanReviewReason = "low_confidence"

	// Build operator-friendly message
	analysis.Status.Message = fmt.Sprintf("Workflow confidence %.2f below threshold %.2f (low_confidence)", resp.Confidence, confidenceThreshold)
	if len(resp.Warnings) > 0 {
		analysis.Status.Message += "; " + strings.Join(resp.Warnings, "; ")
	}
	analysis.Status.Warnings = resp.Warnings

	// Store workflow info for human review (partial information for operator context)
	if resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null {
		swMap := GetMapFromOptNil(resp.SelectedWorkflow.Value)
		if swMap != nil {
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:            GetStringFromMap(swMap, "workflow_id"),
				Version:               GetStringFromMap(swMap, "version"),
				ExecutionBundle:       GetStringFromMap(swMap, "execution_bundle"),
				ExecutionBundleDigest: GetStringFromMap(swMap, "execution_bundle_digest"),
				Confidence:            GetFloat64FromMap(swMap, "confidence"),
				Rationale:             GetStringFromMap(swMap, "rationale"),
				ExecutionEngine:       GetStringFromMap(swMap, "execution_engine"),
			}
			// Map parameters if present
			if paramsRaw, ok := swMap["parameters"]; ok {
				if paramsMapIface, ok := paramsRaw.(map[string]interface{}); ok {
					analysis.Status.SelectedWorkflow.Parameters = convertMapToStringMap(paramsMapIface)
				}
			}
		}
	}

	// Store RCA if available (for human review context) - Issue #97: centralized helper
	if len(resp.RootCauseAnalysis) > 0 {
		if rca := ExtractRootCauseAnalysis(resp.RootCauseAnalysis); rca != nil {
			analysis.Status.RootCauseAnalysis = rca
		}
	}

	// Store alternative workflows if available (for human review context)
	if len(resp.AlternativeWorkflows) > 0 {
		alternatives := make([]aianalysisv1.AlternativeWorkflow, 0, len(resp.AlternativeWorkflows))
		for _, alt := range resp.AlternativeWorkflows {
			executionBundle := ""
			if alt.ExecutionBundle.Set && !alt.ExecutionBundle.Null {
				executionBundle = alt.ExecutionBundle.Value
			}
			alternatives = append(alternatives, aianalysisv1.AlternativeWorkflow{
				WorkflowID:      alt.WorkflowID,
				ExecutionBundle:  executionBundle,
				Confidence:      alt.Confidence,
				Rationale:       alt.Rationale,
			})
		}
		analysis.Status.AlternativeWorkflows = alternatives
	}

	// BR-AI-050: Emit audit event for low confidence failure
	failureErr := fmt.Errorf("low confidence: %.2f below threshold %.2f", resp.Confidence, confidenceThreshold)
	if auditErr := p.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		p.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	// Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LowConfidence").Inc()
	p.metrics.RecordFailure("WorkflowResolutionFailed", "LowConfidence")

	aianalysis.SetInvestigationComplete(analysis, false, fmt.Sprintf("low confidence: %.2f below threshold %.2f", resp.Confidence, confidenceThreshold))
	aianalysis.SetAnalysisComplete(analysis, false, fmt.Sprintf("Workflow confidence %.2f below threshold %.2f", resp.Confidence, confidenceThreshold))
	aianalysis.SetWorkflowResolved(analysis, false, aianalysis.ReasonWorkflowResolutionFailed, fmt.Sprintf("Workflow confidence %.2f below threshold", resp.Confidence))
	aianalysis.SetApprovalRequired(analysis, false, "NotApplicable", "Workflow not approved due to low confidence")
	return ctrl.Result{}, nil // Terminal - no requeue
}

// setTotalAnalysisTime calculates and sets TotalAnalysisTime from StartedAt.
// Safe to call when StartedAt is nil (no-op).
func setTotalAnalysisTime(analysis *aianalysisv1.AIAnalysis, now metav1.Time) {
	if analysis.Status.StartedAt != nil {
		analysis.Status.TotalAnalysisTime = int64(now.Sub(analysis.Status.StartedAt.Time).Seconds())
	}
}

// mapEnumToSubReason maps KA HumanReviewReason enum to CRD SubReason
// This is the preferred method - direct enum-to-enum mapping (Dec 6, 2025)
// Updated Dec 7, 2025: Added investigation_inconclusive per BR-HAPI-200
func (p *ResponseProcessor) mapEnumToSubReason(reason string) string {
	mapping := map[string]string{
		"workflow_not_found":          "WorkflowNotFound",
		"image_mismatch":              "ImageMismatch",
		"parameter_validation_failed": "ParameterValidationFailed",
		"no_matching_workflows":       "NoMatchingWorkflows",
		"low_confidence":              "LowConfidence",
		"llm_parsing_error":           "LLMParsingError",
		"investigation_inconclusive":  "InvestigationInconclusive", // BR-HAPI-200
		"rca_incomplete":              "RcaIncomplete",             // BR-496 v2: root_owner missing from session_state
	}
	if subReason, ok := mapping[reason]; ok {
		return subReason
	}
	p.log.Info("Unknown human_review_reason, defaulting to WorkflowNotFound", "reason", reason)
	return "WorkflowNotFound"
}

// hasNoWorkflowWarningSignal checks if KA's warnings contain signals that indicate
// the absence of a selected workflow is due to an active problem (inconclusive investigation
// or no matching workflows in the catalog), NOT because the problem self-resolved.
//
// This is a defense-in-depth check (Layer 2) for BR-HAPI-200.6. It catches edge cases
// where the LLM incorrectly sets needs_human_review=false but KA's result_parser
// still appends diagnostic warnings from investigation_outcome processing.
//
// Warning signals checked (from KA result_parser.py):
//   - "inconclusive": investigation_outcome == "inconclusive"
//   - "no workflows matched": selected_workflow is None and outcome is not "resolved"
//   - "human review recommended": general KA safety signal
func hasNoWorkflowWarningSignal(warnings []string) bool {
	for _, w := range warnings {
		lower := strings.ToLower(w)
		if strings.Contains(lower, "inconclusive") ||
			strings.Contains(lower, "no workflows matched") ||
			strings.Contains(lower, "human review recommended") {
			return true
		}
	}
	return false
}

// hasProblemResolvedSignal checks if KA emitted the "Problem self-resolved" warning.
// This warning is only produced when investigation_outcome == "resolved" (result_parser.py),
// making it an authoritative signal that the problem is no longer occurring.
// #301: Used to bypass hasSubstantiveRCA when the RCA documents a resolved transient
// condition rather than an active problem.
func hasProblemResolvedSignal(warnings []string) bool {
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "problem self-resolved") {
			return true
		}
	}
	return false
}

// hasNotActionableSignal checks if KA emitted the "Alert not actionable" warning.
// This warning is only produced when actionable == false (result_parser.py),
// making it an authoritative signal that the alert is benign.
// #388: Used to route to Completed/WorkflowNotNeeded/NotActionable, bypassing
// the hasSubstantiveRCA check — the RCA documents a benign condition for audit,
// not an ongoing problem requiring intervention.
func hasNotActionableSignal(warnings []string) bool {
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "alert not actionable") {
			return true
		}
	}
	return false
}

// hasSubstantiveRCA checks if the LLM response contains a root cause analysis
// with contributing factors, indicating a real problem was identified.
// #208: When no workflow is selected but a real problem exists, the system should
// escalate to human review rather than silently completing as "NoActionRequired."
func hasSubstantiveRCA(rca agentclient.IncidentResponseRootCauseAnalysis) bool {
	if len(rca) == 0 {
		return false
	}
	extracted := ExtractRootCauseAnalysis(rca)
	if extracted == nil {
		return false
	}
	return extracted.Summary != "" && len(extracted.ContributingFactors) > 0
}

// mapWarningsToSubReason extracts SubReason from KA warnings
// DEPRECATED: Use mapEnumToSubReason when HumanReviewReason is available
// Kept for backward compatibility with older KA versions
func mapWarningsToSubReason(warnings []string) string {
	warningsStr := strings.ToLower(strings.Join(warnings, " "))

	switch {
	case strings.Contains(warningsStr, "not found") || strings.Contains(warningsStr, "does not exist"):
		return "WorkflowNotFound"
	case strings.Contains(warningsStr, "no workflows matched") || strings.Contains(warningsStr, "no matching"):
		return "NoMatchingWorkflows"
	case strings.Contains(warningsStr, "confidence") && strings.Contains(warningsStr, "below"):
		return "LowConfidence"
	case strings.Contains(warningsStr, "parameter validation") || strings.Contains(warningsStr, "missing required"):
		return "ParameterValidationFailed"
	case strings.Contains(warningsStr, "image mismatch") || strings.Contains(warningsStr, "container image"):
		return "ImageMismatch"
	case strings.Contains(warningsStr, "parse") || strings.Contains(warningsStr, "invalid json"):
		return "LLMParsingError"
	default:
		return "WorkflowNotFound" // Default to most common case
	}
}

// ExtractRootCauseAnalysis extracts RCA from an IncidentResponse, including remediationTarget.
// Issue #97: Centralizes RCA extraction (was duplicated in 5 handler functions).
// BR-496 v2: remediationTarget is KA-injected from K8s-verified root_owner, not LLM-provided.
// #542: KA emits "remediationTarget" in JSON; CRD stores it as RemediationTarget.
func ExtractRootCauseAnalysis(rcaData interface{}) *aianalysisv1.RootCauseAnalysis {
	rcaMap := GetMapFromOptNil(rcaData)
	if rcaMap == nil {
		return nil
	}
	rca := &aianalysisv1.RootCauseAnalysis{
		Summary:             GetStringFromMap(rcaMap, "summary"),
		Severity:            GetStringFromMap(rcaMap, "severity"),
		SignalType:          GetStringFromMap(rcaMap, "signal_name"),
		ContributingFactors: GetStringSliceFromMap(rcaMap, "contributing_factors"),
	}

	// #542: KA emits "remediationTarget"; maps to CRD's RemediationTarget field.
	if arRaw, ok := rcaMap["remediationTarget"]; ok {
		if arMap, ok := arRaw.(map[string]interface{}); ok {
			kind, _ := arMap["kind"].(string)
			name, _ := arMap["name"].(string)
			ns, _ := arMap["namespace"].(string)
			if kind != "" && name != "" {
				rca.RemediationTarget = &aianalysisv1.RemediationTarget{
					Kind:      kind,
					Name:      name,
					Namespace: ns,
				}
			}
		}
	}

	return rca
}
