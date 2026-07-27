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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
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
func (inv *Investigator) RunWorkflowDiscoveryFromRCA(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (*katypes.InvestigationResult, error) {
	if rcaResult == nil {
		return nil, fmt.Errorf("RunWorkflowDiscoveryFromRCA: rcaResult must not be nil")
	}

	rcaCopy := *rcaResult
	rcaResult = &rcaCopy

	inv.pipeline.AnomalyDetector.Reset()

	inv.autoResolveRCATargetAPIVersion(rcaResult, correlationID)

	preKind := signal.ResourceKind
	signal = SyncSignalFromRCA(signal, rcaResult.RemediationTarget)
	signal.Namespace = inv.normalizeNamespace(signal.ResourceKind, signal.Namespace)

	inv.logger.Info("RunWorkflowDiscoveryFromRCA: RCA target state",
		"rca_target_kind", rcaResult.RemediationTarget.Kind,
		"rca_target_api_version", rcaResult.RemediationTarget.APIVersion,
		"signal_kind_before", preKind,
		"signal_kind", signal.ResourceKind,
		"signal_api_version", signal.ResourceAPIVersion,
		"signal_namespace", signal.Namespace,
		"correlation_id", correlationID)

	// F5 (#1374): Resolve enrichment when the enricher is wired, mirroring
	// the autonomous path (Investigate). Pre-RCA enrichment uses the signal
	// target; post-RCA re-enrichment uses the RCA-identified target when it
	// differs (cross-resource RCA).
	rawEnrichData, enricherWired, hardFailResult := inv.resolveRCAWorkflowDiscoveryEnrichment(ctx, signal, rcaResult, correlationID)
	if hardFailResult != nil {
		return hardFailResult, nil
	}
	if enricherWired {
		enrichData = toPromptEnrichment(rawEnrichData)
	}

	// F6 (#1374): Attach DetectedLabelsJSON to the signal so workflow
	// discovery tools forward labels to DS catalog queries, activating
	// GitOps-aware scoring (parity with autonomous path).
	inv.attachRCADetectedLabelsJSON(&signal, rawEnrichData, correlationID)

	client, modelName, runtimeParams := inv.resolveForPhase(katypes.PhaseWorkflowDiscovery)

	p1Ctx := BuildPhase1Context(rcaResult)
	workflowResult, err := inv.runWorkflowSelection(ctx, signal, rcaResult.RCASummary, enrichData, p1Ctx, LLMInvocationContext{
		CorrelationID: correlationID,
		Client:        client,
		ModelName:     modelName,
		RuntimeParams: runtimeParams,
	})
	if err != nil {
		return nil, err
	}

	FinalizeWorkflowResult(workflowResult, signal, rcaResult, rawEnrichData)
	return workflowResult, nil
}

// autoResolveRCATargetAPIVersion auto-resolves rcaResult.RemediationTarget.APIVersion
// when the LLM omitted it, mirroring the autonomous apiVersionValidationGate in
// runRCA. It mutates rcaResult in place. Uses the REST mapper via
// scopeResolver: if the kind maps to exactly one API group, the apiVersion
// can be inferred deterministically without an extra LLM turn.
func (inv *Investigator) autoResolveRCATargetAPIVersion(rcaResult *katypes.InvestigationResult, correlationID string) {
	if rcaResult.RemediationTarget.APIVersion != "" || rcaResult.RemediationTarget.Kind == "" || inv.scopeResolver == nil {
		return
	}
	ambiguous, gvrs, err := inv.scopeResolver.IsAmbiguousKind(rcaResult.RemediationTarget.Kind)
	if err != nil {
		inv.logger.Error(err, "RunWorkflowDiscoveryFromRCA: IsAmbiguousKind failed, skipping apiVersion auto-resolve",
			"kind", rcaResult.RemediationTarget.Kind, "correlation_id", correlationID)
		return
	}
	if !ambiguous && len(gvrs) == 1 {
		rcaResult.RemediationTarget.APIVersion = gvrToAPIVersion(gvrs[0])
		inv.logger.Info("RunWorkflowDiscoveryFromRCA: auto-resolved apiVersion for non-ambiguous kind",
			"kind", rcaResult.RemediationTarget.Kind,
			"api_version", rcaResult.RemediationTarget.APIVersion,
			"correlation_id", correlationID)
	}
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
	rawEnrichData = inv.resolveEnrichmentCached(ctx, enrichmentCache, signalKind, signalName, signalNS, signal.IncidentID)

	postRCAKind, postRCAName, postRCANS := ResolveEnrichmentTarget(signal, rcaResult)
	postRCANS = inv.normalizeNamespace(postRCAKind, postRCANS)
	if postRCAKind == signalKind && postRCAName == signalName && postRCANS == signalNS {
		if rawEnrichData != nil && rawEnrichData.TargetResourceDeleted {
			rcaResult.Warnings = append(rcaResult.Warnings,
				deletedResourceWarning(signalKind, signalName, signalNS))
		}
		return rawEnrichData, true, nil
	}

	return inv.reEnrichForRCATargetShift(ctx, reEnrichForRCATargetShiftParams{
		EnrichmentCache: enrichmentCache, Signal: signal, RCAResult: rcaResult, RawEnrichData: rawEnrichData,
		SignalKind: signalKind, SignalName: signalName,
		PostRCAKind: postRCAKind, PostRCAName: postRCAName, PostRCANS: postRCANS,
		CorrelationID: correlationID,
	})
}

// reEnrichForRCATargetShiftParams groups the fields needed by
// reEnrichForRCATargetShift. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type reEnrichForRCATargetShiftParams struct {
	EnrichmentCache                                    map[string]*enrichment.EnrichmentResult
	Signal                                             katypes.SignalContext
	RCAResult                                          *katypes.InvestigationResult
	RawEnrichData                                      *enrichment.EnrichmentResult
	SignalKind, SignalName                             string
	PostRCAKind, PostRCAName, PostRCANS, CorrelationID string
}

// reEnrichForRCATargetShift re-runs enrichment against the RCA-resolved
// remediation target when it differs from the signal's original target
// (e.g. the LLM identified a different owning resource as the true root
// cause). Returns a non-nil hardFailResult when the re-enrichment
// hard-fails, in which case rcaResult has already been mutated to request
// human review with the rca_incomplete reason.
func (inv *Investigator) reEnrichForRCATargetShift(ctx context.Context, p reEnrichForRCATargetShiftParams) (*enrichment.EnrichmentResult, bool, *katypes.InvestigationResult) {
	enrichmentCache, signal, rcaResult, rawEnrichData := p.EnrichmentCache, p.Signal, p.RCAResult, p.RawEnrichData
	signalKind, signalName := p.SignalKind, p.SignalName
	postRCAKind, postRCAName, postRCANS, correlationID := p.PostRCAKind, p.PostRCAName, p.PostRCANS, p.CorrelationID
	inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enriching with RCA target",
		"signal", signalKind+"/"+signalName,
		"rca_target", postRCAKind+"/"+postRCAName,
		"correlation_id", correlationID)
	reEnriched := inv.resolveEnrichmentCached(ctx, enrichmentCache, postRCAKind, postRCAName, postRCANS, signal.IncidentID)

	if reEnriched != nil && reEnriched.HardFail {
		inv.logger.Error(reEnriched.OwnerChainError,
			"RunWorkflowDiscoveryFromRCA: enrichment hard-failed, triggering rca_incomplete",
			"correlation_id", correlationID)
		rcaResult.HumanReviewNeeded = true
		rcaResult.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete
		backfillSeverity(rcaResult, signal)
		attachDetectedLabels(rcaResult, rawEnrichData)
		InjectRemediationTarget(rcaResult, signal, rawEnrichData)
		InjectTargetResourceParameters(rcaResult)
		return rawEnrichData, true, rcaResult
	}

	if reEnriched == nil {
		return rawEnrichData, true, nil
	}

	if reEnriched.TargetResourceDeleted {
		rcaResult.Warnings = append(rcaResult.Warnings,
			deletedResourceWarning(postRCAKind, postRCAName, postRCANS))
	}

	if allLabelDetectionsFailed(reEnriched.DetectedLabels) {
		inv.logger.Info("RunWorkflowDiscoveryFromRCA: re-enrichment labels all failed, preserving signal-target labels",
			"rca_target", postRCAKind+"/"+postRCAName, "correlation_id", correlationID)
		return rawEnrichData, true, nil
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
