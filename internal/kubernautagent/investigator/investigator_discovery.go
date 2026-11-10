/*
Copyright 2026 Jordi Gil.

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

package investigator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// RunWorkflowDiscoveryFromRCA executes Phase 3 (workflow selection) using a
// structured RCA result. This reuses the exact autonomous Phase 3 pipeline
// (BuildPhase1Context + runWorkflowSelection) so that interactive and
// autonomous workflow discovery produce consistent results.
//
// F5/F6 (#1374): When the investigator has an enricher wired, this function
// resolves enrichment internally (pre-RCA + post-RCA re-enrichment), mirroring
// the autonomous Investigate() pipeline. The caller-provided enrichData is used
// as fallback only when the enricher is nil.
func (inv *Investigator) RunWorkflowDiscoveryFromRCA(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (result *katypes.InvestigationResult, retErr error) {
	if rcaResult == nil {
		return nil, fmt.Errorf("RunWorkflowDiscoveryFromRCA: rcaResult must not be nil")
	}
	ctx = audit.WithCorrelationID(ctx, correlationID)
	ctx, evidenceScope := ensureEvidenceStatusScope(ctx)
	defer func() { attachEvidenceStatus(evidenceScope, &result, &retErr) }()

	rcaCopy := *rcaResult
	rcaResult = &rcaCopy
	// This entry point accepts a Phase-1 result from an interactive session;
	// replace any model-provided severity before it enters discovery context.
	applySignalSeverity(rcaResult, signal)

	inv.anomalyDetectorFor(correlationID).Reset()
	// #2387 (BR-KA-OBSERVABILITY-001): cumulative per-RR accounting — NO
	// reset here. Message turns, the extraction turn, and (in takeover
	// flows) the autonomous legs all share this scope; resetting would wipe
	// them. Assembly below sets cumulative totals from the scope.

	preKind := signal.ResourceKind
	var err error
	ctx, err = inv.prescopeFleetOverlay(ctx, signal.ClusterID, correlationID)
	if err != nil {
		return nil, err
	}
	client, modelName, runtimeParams := inv.resolveForPhase(katypes.PhaseWorkflowDiscovery)
	if rcaResult.HumanReviewNeeded {
		// A pre-existing review decision is authoritative. Interactive
		// workflow discovery must not clear it merely because a caller asks
		// for a catalog lookup.
		ApplyTotals(rcaResult, inv.InvestigationTotals(correlationID))
		return rcaResult, nil
	}
	rcaClient, _, rcaRuntimeParams := inv.resolveForPhase(katypes.PhaseRCA)
	rcaResult = inv.apiVersionValidationGate(ctx, rcaResult, nil, LLMInvocationContext{
		Client: rcaClient, RuntimeParams: rcaRuntimeParams, CorrelationID: correlationID,
	})
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	if rcaResult.HumanReviewNeeded ||
		(rcaResult.RemediationTarget.Kind != "" && !TargetAPIVersionValidationMatches(rcaResult, clusterID)) {
		// Target verification failure invalidates any stale typed outcome from
		// the preceding RCA leg. Cancellation is authoritative and must remain
		// the terminal outcome for the interactive request.
		if !rcaResult.Cancelled {
			markRCAIncomplete(rcaResult)
		}
		ApplyTotals(rcaResult, inv.InvestigationTotals(correlationID))
		return rcaResult, nil
	}

	signal = syncSignalFromRCAForWorkflowDiscovery(signal, rcaResult.RemediationTarget)
	signal.Namespace = inv.normalizeNamespace(signal.ResourceKind, signal.Namespace)

	inv.logWorkflowDiscoveryTarget(preKind, signal, rcaResult, correlationID)

	// F5 (#1374): Resolve enrichment when the enricher is wired, mirroring
	// the autonomous path (Investigate). Pre-RCA enrichment uses the signal
	// target; post-RCA re-enrichment uses the RCA-identified target when it
	// differs (cross-resource RCA).
	rawEnrichData, enricherWired, hardFailResult := inv.resolveRCAWorkflowDiscoveryEnrichment(ctx, signal, rcaResult, correlationID)
	if hardFailResult != nil {
		// #2387: hard-fail skips FinalizeWorkflowResult, so apply the
		// cumulative scope totals (and tokens) here directly.
		inv.applyMetricsFromScope(hardFailResult, correlationID)
		inv.setTokenUsageFromScope(hardFailResult, correlationID)
		return hardFailResult, nil
	}
	if enricherWired {
		if rawEnrichData != nil && (rawEnrichData.HistoryUnavailable || isHistoryUnavailableStatus(rawEnrichData.HistoryStatus)) {
			return inv.finalizeHistoryUnavailableDiscovery(ctx, rcaResult, signal, rawEnrichData, correlationID, modelName), nil
		}
		enrichData = toPromptEnrichment(rawEnrichData)
	} else if enrichData != nil && enrichData.HistoryUnavailable {
		// Interactive callers may provide only the serialized enrichment
		// snapshot, without an Enricher to attach source status. Preserve the
		// same typed terminal evidence contract as the autonomous path.
		status := enrichment.HistoryEvidenceStatusForError(
			enrichment.ErrHistoryUnavailable,
			signal.ResourceKind, signal.ResourceName, signal.Namespace,
			signal.ClusterID, correlationID,
		)
		evidenceScope.MergeSource(status)
	}

	// F6 (#1374): Attach DetectedLabelsJSON to the signal so workflow
	// discovery tools forward labels to DS catalog queries, activating
	// GitOps-aware scoring (parity with autonomous path).
	inv.attachRCADetectedLabelsJSON(&signal, rawEnrichData, correlationID)

	return inv.executeWorkflowSelectionFromRCA(ctx, workflowSelectionFromRCAParams{
		signal:        signal,
		rcaResult:     rcaResult,
		enrichData:    enrichData,
		rawEnrichData: rawEnrichData,
		client:        client,
		modelName:     modelName,
		runtimeParams: runtimeParams,
		correlationID: correlationID,
	})
}

func (inv *Investigator) logWorkflowDiscoveryTarget(
	preKind string,
	signal katypes.SignalContext,
	rcaResult *katypes.InvestigationResult,
	correlationID string,
) {
	inv.logger.Info("RunWorkflowDiscoveryFromRCA: RCA target state",
		"rca_target_kind", rcaResult.RemediationTarget.Kind,
		"rca_target_api_version", rcaResult.RemediationTarget.APIVersion,
		"signal_kind_before", preKind,
		"signal_kind", signal.ResourceKind,
		"signal_api_version", signal.ResourceAPIVersion,
		"signal_namespace", signal.Namespace,
		"signal_severity", signal.Severity,
		"signal_environment", signal.Environment,
		"signal_priority", signal.Priority,
		"signal_cluster", signal.ClusterClassification,
		"correlation_id", correlationID)
}

type workflowSelectionFromRCAParams struct {
	signal        katypes.SignalContext
	rcaResult     *katypes.InvestigationResult
	enrichData    *prompt.EnrichmentData
	rawEnrichData *enrichment.EnrichmentResult
	client        llm.Client
	modelName     string
	runtimeParams llm.RuntimeParams
	correlationID string
}

func (inv *Investigator) executeWorkflowSelectionFromRCA(ctx context.Context, p workflowSelectionFromRCAParams) (*katypes.InvestigationResult, error) {
	p1Ctx := BuildPhase1Context(p.rcaResult)
	markWorkflowDiscoveryStarted(p.rcaResult)
	workflowResult, err := inv.runWorkflowSelection(ctx, p.signal, p.rcaResult.RCASummary, p.enrichData, p1Ctx, LLMInvocationContext{
		CorrelationID:      p.correlationID,
		Client:             p.client,
		ModelName:          p.modelName,
		RuntimeParams:      p.runtimeParams,
		workflowEnrichment: p.rawEnrichData,
	})
	if err != nil {
		return nil, err
	}
	applyWorkflowDiscoveryTypedStatus(workflowResult, p.rcaResult, p.rawEnrichData)
	FinalizeWorkflowResult(workflowResult, p.signal, p.rcaResult, p.rawEnrichData)
	workflowResult = inv.revalidateFinalizedWorkflowTarget(ctx, workflowResult, p.rcaResult, p.client, p.runtimeParams, p.correlationID)
	// #2387: cumulative per-RR totals straight from the scope (message +
	// extraction + Phase 3 legs, plus any prior autonomous legs in takeover
	// flows). No rcaResult summation: under scope-source-of-truth results
	// never pre-carry counts.
	inv.applyMetricsFromScope(workflowResult, p.correlationID)
	inv.setTokenUsageFromScope(workflowResult, p.correlationID)
	return workflowResult, nil
}

// revalidateFinalizedWorkflowTarget closes the handoff gap between RCA target
// verification and authoritative enrichment. Enrichment may resolve a model
// identified child (for example, Pod) to its owning remediation target (for
// example, Deployment), which invalidates RCA provenance for the new target.
// The Phase 3 proposal is not executable until this final target has fresh
// target-cluster GVK verification; an unverifiable target is cleared by
// enforceVerifiedWorkflowTarget and requires human review.
func (inv *Investigator) revalidateFinalizedWorkflowTarget(
	ctx context.Context,
	result *katypes.InvestigationResult,
	rcaResult *katypes.InvestigationResult,
	client llm.Client,
	runtimeParams llm.RuntimeParams,
	correlationID string,
) *katypes.InvestigationResult {
	if result == nil || rcaResult == nil || result.WorkflowID == "" || result.RemediationTarget.Kind == "" || !completeAPIVersionValidation(rcaResult) {
		return result
	}
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	if TargetAPIVersionValidationMatches(result, clusterID) {
		return result
	}

	result = inv.apiVersionValidationGate(ctx, result, nil, LLMInvocationContext{
		Client: client, RuntimeParams: runtimeParams, CorrelationID: correlationID,
	})
	inv.enforceVerifiedWorkflowTarget(ctx, result)
	return result
}

func syncSignalFromRCAForWorkflowDiscovery(signal katypes.SignalContext, target katypes.RemediationTarget) katypes.SignalContext {
	if !target.APIVersionValidated {
		// A non-empty model value is not authoritative. Keep the original
		// signal version when the target kind is unchanged; clear stale signal
		// GVK data when RCA changes kind without a validated version.
		target.APIVersion = ""
	}
	return SyncSignalFromRCA(signal, target)
}

func ensureEvidenceStatusScope(ctx context.Context) (context.Context, *katypes.EvidenceStatusScope) {
	if scope, ok := katypes.EvidenceStatusScopeFromContext(ctx); ok {
		return ctx, scope
	}
	scope := katypes.NewEvidenceStatusScope()
	return katypes.WithEvidenceStatusScope(ctx, scope), scope
}

func attachEvidenceStatus(scope *katypes.EvidenceStatusScope, result **katypes.InvestigationResult, retErr *error) {
	status := scope.Snapshot()
	if status == nil {
		return
	}
	if *result != nil {
		(*result).EvidenceStatus = status
	} else if *retErr != nil {
		*retErr = katypes.NewInvestigationError(*retErr, status)
	}
}

// applyWorkflowDiscoveryTypedStatus applies the server-owned discovery gate to
// a workflow-selection result. A result that reaches this helper without an
// explicit selection or no-match sentinel is a discovery failure.
func applyWorkflowDiscoveryTypedStatus(result, rcaResult *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	if result.HistoryStatus == "" {
		switch {
		case enrichData != nil && enrichData.HistoryStatus != "":
			result.HistoryStatus = enrichData.HistoryStatus
		case rcaResult != nil:
			result.HistoryStatus = rcaResult.HistoryStatus
		}
	}
	if result.WorkflowDiscoveryStatus == "" {
		if result.WorkflowID != "" {
			markWorkflowDiscoverySelected(result)
		} else if status, outcome, ok := classifyWorkflowDiscoveryNoWorkflow(result); ok {
			result.WorkflowDiscoveryStatus = status
			result.TerminalOutcome = outcome
		} else {
			markWorkflowDiscoveryFailed(result)
		}
	}
	if result.WorkflowDiscoveryStatus == sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch &&
		result.TerminalOutcome == "" && (result.IsActionable == nil || *result.IsActionable) {
		result.TerminalOutcome = sharedtypes.TerminalOutcomeNoMatchingWorkflows
	}
}

// finalizeHistoryUnavailableDiscovery preserves the accepted RCA and exits
// before workflow discovery when the required target-scoped history could not
// be acquired. This is also used for an RCA target shift whose owner/label
// enrichment succeeded but whose history source failed.
func (inv *Investigator) finalizeHistoryUnavailableDiscovery(ctx context.Context, result *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult, correlationID, modelName string) *katypes.InvestigationResult {
	markHistoryEnrichmentUnavailable(result, enrichData)
	applySignalSeverity(result, signal)
	attachDetectedLabels(result, enrichData)
	injectFinalizedRemediationTarget(result, signal, result, enrichData)
	InjectTargetResourceParameters(result)
	inv.applyMetricsFromScope(result, correlationID)
	inv.setTokenUsageFromScope(result, correlationID)
	inv.emitResponseComplete(ctx, result, &TokenAccumulator{}, correlationID, firstNonEmpty(modelName, inv.modelName))
	return result
}

// resolveRCAWorkflowDiscoveryEnrichment resolves (and, when the RCA target
// differs from the signal target, re-resolves) enrichment for the interactive
// workflow-discovery path. enricherWired is false when inv.enricher is nil,
// signaling that the caller must leave its own enrichData parameter
// untouched. hardFailResult is non-nil when re-enrichment hard-failed; the
// caller must return it immediately (it is already fully finalized).
func (inv *Investigator) resolveRCAWorkflowDiscoveryEnrichment(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, correlationID string) (rawEnrichData *enrichment.EnrichmentResult, enricherWired bool, hardFailResult *katypes.InvestigationResult) {
	if inv.enricher == nil {
		return nil, false, nil
	}

	enrichmentCache := make(map[string]*enrichment.EnrichmentResult)
	signalKind, signalName, signalNS := ResolveEnrichmentTarget(signal, nil)
	signalNS = inv.normalizeNamespace(signalKind, signalNS)
	signalTarget := enrichmentTargetRef{Kind: signalKind, Name: signalName, Namespace: signalNS, APIVersion: signal.ResourceAPIVersion}
	rawEnrichData = inv.resolveEnrichmentCached(ctx, enrichmentCache, signalTarget, signal.ClusterID, signal.IncidentID)

	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	postRCANS = inv.normalizeNamespace(postRCAKind, postRCANS)
	postRCAAPIVersion := signal.ResourceAPIVersion
	if rcaResult != nil && rcaResult.RemediationTarget.Kind != "" {
		postRCAAPIVersion = rcaResult.RemediationTarget.APIVersion
	}
	if postRCAKind == signalKind && postRCAName == signalName && postRCANS == signalNS && postRCAAPIVersion == signal.ResourceAPIVersion {
		if rawEnrichData != nil && rawEnrichData.TargetResourceDeleted {
			rcaResult.Warnings = append(rcaResult.Warnings,
				deletedResourceWarning(signalKind, signalName, signalNS))
		}
		return rawEnrichData, true, nil
	}

	return inv.reEnrichForRCATargetShift(ctx, reEnrichForRCATargetShiftParams{
		EnrichmentCache: enrichmentCache, Signal: signal, RCAResult: rcaResult, RawEnrichData: rawEnrichData,
		SignalTarget:  signalTarget,
		PostRCATarget: enrichmentTargetRef{Kind: postRCAKind, Name: postRCAName, Namespace: postRCANS, APIVersion: postRCAAPIVersion},
		CorrelationID: correlationID,
	})
}

// reEnrichForRCATargetShiftParams groups the fields needed by
// reEnrichForRCATargetShift. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type reEnrichForRCATargetShiftParams struct {
	EnrichmentCache             map[string]*enrichment.EnrichmentResult
	Signal                      katypes.SignalContext
	RCAResult                   *katypes.InvestigationResult
	RawEnrichData               *enrichment.EnrichmentResult
	SignalTarget, PostRCATarget enrichmentTargetRef
	CorrelationID               string
}

// reEnrichForRCATargetShift re-runs enrichment against the RCA-resolved
// remediation target when it differs from the signal's original target
// (e.g. the LLM identified a different owning resource as the true root
// cause). Returns a non-nil hardFailResult when the re-enrichment
// hard-fails, in which case rcaResult has already been mutated to request
// human review with the rca_incomplete reason.
func (inv *Investigator) reEnrichForRCATargetShift(ctx context.Context, p reEnrichForRCATargetShiftParams) (*enrichment.EnrichmentResult, bool, *katypes.InvestigationResult) {
	enrichmentCache, signal, rcaResult, rawEnrichData := p.EnrichmentCache, p.Signal, p.RCAResult, p.RawEnrichData
	signalTarget, postRCATarget, correlationID := p.SignalTarget, p.PostRCATarget, p.CorrelationID
	inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enriching with RCA target",
		"signal", signalTarget.Kind+"/"+signalTarget.Name,
		"rca_target", postRCATarget.Kind+"/"+postRCATarget.Name,
		"correlation_id", correlationID)
	reEnriched := inv.resolveEnrichmentCached(ctx, enrichmentCache, postRCATarget, signal.ClusterID, signal.IncidentID)

	if reEnriched != nil && reEnriched.HardFail {
		inv.logger.Error(reEnriched.OwnerChainError,
			"RunWorkflowDiscoveryFromRCA: enrichment hard-failed, triggering rca_incomplete",
			"correlation_id", correlationID)
		if isHistoryUnavailableStatus(reEnriched.HistoryStatus) || reEnriched.HistoryUnavailable {
			markHistoryEnrichmentUnavailable(rcaResult, reEnriched)
		} else {
			applyEnrichmentHistoryStatus(rcaResult, reEnriched)
			rcaResult.HumanReviewNeeded = true
			rcaResult.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete
			markDiscoverySkippedDueToFailure(rcaResult, rcaResult.HistoryStatus, false)
		}
		applySignalSeverity(rcaResult, signal)
		attachDetectedLabels(rcaResult, rawEnrichData)
		injectFinalizedRemediationTarget(rcaResult, signal, rcaResult, rawEnrichData)
		InjectTargetResourceParameters(rcaResult)
		return rawEnrichData, true, rcaResult
	}

	if reEnriched == nil {
		// A failed target lookup must not return the signal target's history
		// under the RCA target identity. Keep only explicitly allowed label
		// fallback data in a target-scoped shell.
		targetOnly := &enrichment.EnrichmentResult{
			ResourceKind:      postRCATarget.Kind,
			ResourceName:      postRCATarget.Name,
			ResourceNamespace: postRCATarget.Namespace,
		}
		targetOnly = preserveSignalTargetLabelsOnFailedRCAEnrichment(targetOnly, rawEnrichData)
		return targetOnly, true, nil
	}

	if reEnriched.TargetResourceDeleted {
		rcaResult.Warnings = append(rcaResult.Warnings,
			deletedResourceWarning(postRCATarget.Kind, postRCATarget.Name, postRCATarget.Namespace))
	}

	if allLabelDetectionsFailed(reEnriched.DetectedLabels) {
		inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enrichment labels all failed, preserving signal-target labels",
			"rca_target", postRCATarget.Kind+"/"+postRCATarget.Name, "correlation_id", correlationID)
		return preserveSignalTargetLabelsOnFailedRCAEnrichment(reEnriched, rawEnrichData), true, nil
	}

	return reEnriched, true, nil
}

// attachRCADetectedLabelsJSON marshals rawEnrichData's DetectedLabels onto
// signal.DetectedLabelsJSON so workflow discovery tools forward labels to DS
// catalog queries, activating GitOps-aware scoring (parity with the
// autonomous Investigate path). No-op when rawEnrichData or its labels are
// unset (which is always the case when the enricher is not wired).
func (inv *Investigator) attachRCADetectedLabelsJSON(signal *katypes.SignalContext, rawEnrichData *enrichment.EnrichmentResult, correlationID string) {
	if rawEnrichData == nil || rawEnrichData.DetectedLabels == nil {
		return
	}
	dlJSON, err := json.Marshal(rawEnrichData.DetectedLabels)
	if err != nil {
		inv.logger.Error(err, "RunWorkflowDiscoveryFromRCA: failed to marshal detected labels",
			"correlation_id", correlationID)
		return
	}
	signal.DetectedLabelsJSON = string(dlJSON)
	dl := rawEnrichData.DetectedLabels
	trueCount := countTrueLabels(dl.GitOpsManaged, dl.PDBProtected, dl.HPAEnabled,
		dl.Stateful, dl.HelmManaged, dl.NetworkIsolated, dl.ResourceQuotaConstrained)
	inv.logger.V(1).Info("RunWorkflowDiscoveryFromRCA: detected labels attached for workflow scoring",
		"correlation_id", correlationID,
		"true_label_count", trueCount,
		"gitops_tool", dl.GitOpsTool)
}
