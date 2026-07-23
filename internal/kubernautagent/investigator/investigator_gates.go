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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// submitOnlyRCATools returns the single-tool submit-only tool list used by
// gate retries to force the LLM to resubmit its RCA result.
func submitOnlyRCATools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}
}

// appendCorrectionMessage returns a copy of history with a user correction
// message appended, for use in gate-retry LLM calls.
func appendCorrectionMessage(history []llm.Message, correctionMsg string) []llm.Message {
	retryMessages := make([]llm.Message, len(history), len(history)+1)
	copy(retryMessages, history)
	return append(retryMessages, llm.Message{Role: "user", Content: correctionMsg})
}

// extractSubmitContent extracts the tool-call arguments (preferred) or falls
// back to the raw message content from a gate-retry chat response.
func extractSubmitContent(resp llm.ChatResponse) string {
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			return tc.Arguments
		}
	}
	return resp.Message.Content
}

func (inv *Investigator) sameKindValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	signal katypes.SignalContext,
	history []llm.Message,
	llmCtx LLMInvocationContext,
) *katypes.InvestigationResult {
	correlationID, modelName := llmCtx.CorrelationID, llmCtx.ModelName
	if signal.ResourceKind == "" || result.RemediationTarget.Kind == "" {
		return result
	}
	if !strings.EqualFold(result.RemediationTarget.Kind, signal.ResourceKind) {
		return result
	}

	inv.logger.Info("same-kind validation gate triggered: remediation_target.kind matches signal resource_kind",
		"target_kind", result.RemediationTarget.Kind,
		"signal_resource_kind", signal.ResourceKind,
		"correlation_id", correlationID)

	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionSameKindGate
	gateEvent.EventOutcome = audit.OutcomeSuccess
	gateEvent.Data["model"] = modelName
	gateEvent.Data["prompt_length"] = 0
	gateEvent.Data["prompt_preview"] = ""
	gateEvent.Data["signal_resource_kind"] = signal.ResourceKind
	gateEvent.Data["target_kind"] = result.RemediationTarget.Kind
	gateEvent.Data["target_name"] = result.RemediationTarget.Name
	audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	return inv.retryForSameKind(ctx, result, history, llmCtx)
}

// sameKindCorrectionMessage builds the LLM correction message asking it to
// re-evaluate whether a child resource (rather than targetKind, which
// matches the input signal's kind) is the true root cause.
func sameKindCorrectionMessage(targetKind string) string {
	return fmt.Sprintf(
		`Your remediation_target.kind is "%s", which is the same resource kind as the input signal. `+
			`Signals often propagate upward: workload-level issues manifest as conditions on parent resources `+
			`(e.g., pod memory leaks cause node DiskPressure, deployment misconfigurations appear as node conditions). `+
			`Please re-evaluate: is a child resource (Deployment, StatefulSet, DaemonSet, Pod) the actual root cause `+
			`whose configuration should be modified? If after re-evaluation you are confident the %s itself is the `+
			`correct remediation target, confirm by resubmitting with the same target and explain why in your `+
			`due_diligence.target_accuracy field.`,
		targetKind, targetKind,
	)
}

// retryForSameKind re-submits the RCA request with a correction message
// asking the LLM to re-evaluate a same-kind remediation target, and parses
// the retry response. Falls back to the original result at every failure
// point: LLM error, empty response, parse error, or a retry result that
// lost the remediation target.
func (inv *Investigator) retryForSameKind(ctx context.Context, result *katypes.InvestigationResult, history []llm.Message, llmCtx LLMInvocationContext) *katypes.InvestigationResult {
	tokens, correlationID, client, runtimeParams := llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.RuntimeParams
	correctionMsg := sameKindCorrectionMessage(result.RemediationTarget.Kind)

	resp, err := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: appendCorrectionMessage(history, correctionMsg),
		Tools:    submitOnlyRCATools(),
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if err != nil {
		inv.logger.Error(err, "same-kind validation gate retry failed, keeping original result",
			"correlation_id", correlationID)
		return result
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	retryContent := extractSubmitContent(resp)
	if retryContent == "" {
		inv.logger.Info("same-kind validation gate: no content in retry response, keeping original",
			"correlation_id", correlationID)
		return result
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Error(parseErr, "same-kind validation gate: retry parse failed, keeping original",
			"correlation_id", correlationID)
		return result
	}

	if retryResult.RemediationTarget.Kind == "" && result.RemediationTarget.Kind != "" {
		inv.logger.Info("same-kind validation gate: retry lost remediation_target, keeping original",
			"original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name,
			"correlation_id", correlationID)
		return result
	}

	// The retry is a fresh submission on its own turn: its own Reasoning
	// block (if any) is authoritative for the accepted result, mirroring
	// runRCA's "winning turn" semantics (BR-AI-086 AC6, #1578). Without this,
	// resp.Message.Reasoning from the retry call is silently discarded and
	// the gate-accepted result loses reasoning captured by the LLM client.
	retryResult.Reasoning = toReasoningSummary(resp.Message.Reasoning)

	inv.logger.Info("same-kind validation gate: accepted retry result",
		"original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name,
		"retry_target", retryResult.RemediationTarget.Kind+"/"+retryResult.RemediationTarget.Name,
		"correlation_id", correlationID)
	return retryResult
}

// apiVersionValidationGate rejects ambiguous kinds missing api_version (Issue #1044).
// When the LLM's remediation_target.kind exists in multiple API groups (e.g.
// Subscription in operators.coreos.com and messaging.knative.dev) but api_version
// is empty, the gate injects a correction message naming the conflicting groups
// and retries once. On exhaustion, sets HumanReviewNeeded=true to prevent incorrect
// RBAC grants from resolving to the wrong API group.
func (inv *Investigator) apiVersionValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	history []llm.Message,
	llmCtx LLMInvocationContext,
) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	if inv.scopeResolver == nil {
		return result
	}
	kind := result.RemediationTarget.Kind
	if kind == "" {
		return result
	}
	if result.RemediationTarget.APIVersion != "" {
		return result
	}

	ambiguous, gvrs, err := inv.scopeResolver.IsAmbiguousKind(kind)
	if err != nil {
		inv.logger.Error(err, "apiVersionValidationGate: IsAmbiguousKind failed, skipping gate",
			"kind", kind, "correlation_id", correlationID)
		return result
	}
	if !ambiguous {
		return inv.autoResolveNonAmbiguousAPIVersion(result, gvrs, kind, correlationID)
	}

	groupList := conflictingGroupList(gvrs)

	inv.logger.Info("apiVersionValidationGate triggered: ambiguous kind missing api_version",
		"kind", kind, "conflicting_groups", groupList, "correlation_id", correlationID)

	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionAPIVersionGate
	gateEvent.EventOutcome = audit.OutcomeSuccess
	gateEvent.Data["model"] = modelName
	gateEvent.Data["prompt_length"] = 0
	gateEvent.Data["prompt_preview"] = ""
	gateEvent.Data["ambiguous_kind"] = kind
	gateEvent.Data["conflicting_groups"] = groupList
	audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	return inv.retryForAPIVersion(ctx, retryForAPIVersionParams{
		Result: result, History: history, Client: client, RuntimeParams: runtimeParams, Tokens: tokens,
		Kind: kind, GroupList: groupList, CorrelationID: correlationID, GateEvent: gateEvent,
	})
}

// autoResolveNonAmbiguousAPIVersion auto-resolves apiVersion for a
// non-ambiguous kind using the REST mapper (#1051), guaranteeing
// RemediationTarget.APIVersion is populated after RCA so GVK-format workflow
// component matching can proceed. PROD-2: when len(gvrs) > 1 but not
// ambiguous (multiple versions in the same group, e.g. v1 and v1beta1),
// auto-resolve is skipped because the preferred version is not
// deterministic; the fallback in custom tools uses lowercase kind for
// discovery in this edge case.
func (inv *Investigator) autoResolveNonAmbiguousAPIVersion(result *katypes.InvestigationResult, gvrs []schema.GroupVersionResource, kind, correlationID string) *katypes.InvestigationResult {
	if len(gvrs) == 1 {
		result.RemediationTarget.APIVersion = gvrToAPIVersion(gvrs[0])
		inv.logger.Info("apiVersionValidationGate: auto-resolved apiVersion for non-ambiguous kind",
			"kind", kind,
			"api_version", result.RemediationTarget.APIVersion,
			"correlation_id", correlationID)
	}
	return result
}

// conflictingGroupList returns the deduplicated, comma-joined list of API
// groups an ambiguous kind was found in, for use in the LLM correction
// message and audit event.
func conflictingGroupList(gvrs []schema.GroupVersionResource) string {
	groupNames := make([]string, 0, len(gvrs))
	seen := make(map[string]struct{})
	for _, gvr := range gvrs {
		if _, ok := seen[gvr.Group]; !ok {
			groupNames = append(groupNames, gvr.Group)
			seen[gvr.Group] = struct{}{}
		}
	}
	return strings.Join(groupNames, ", ")
}

// retryForAPIVersionParams groups the fields needed by retryForAPIVersion.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type retryForAPIVersionParams struct {
	Result                         *katypes.InvestigationResult
	History                        []llm.Message
	Client                         llm.Client
	RuntimeParams                  llm.RuntimeParams
	Tokens                         *TokenAccumulator
	Kind, GroupList, CorrelationID string
	GateEvent                      *audit.AuditEvent
}

// retryForAPIVersion re-submits the RCA request with a correction message
// asking the LLM to disambiguate the API group for kind, and parses the
// retry response. Falls back to apiVersionGateExhaustion (forcing human
// review) at every failure point: LLM error, empty response, parse error, or
// a retry result that still lacks api_version.
func (inv *Investigator) retryForAPIVersion(ctx context.Context, p retryForAPIVersionParams) *katypes.InvestigationResult {
	result, history, client, runtimeParams, tokens := p.Result, p.History, p.Client, p.RuntimeParams, p.Tokens
	kind, groupList, correlationID, gateEvent := p.Kind, p.GroupList, p.CorrelationID, p.GateEvent
	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is %q, which exists in multiple API groups: %s. `+
			`Without an explicit api_version, the system cannot determine the correct API group `+
			`and may grant RBAC permissions to the wrong resource. `+
			`You MUST re-submit your result with the api_version field set to the correct `+
			`"group/version" (e.g. "operators.coreos.com/v1alpha1"). `+
			`Review your investigation context (kubectl_describe output) to determine which `+
			`API group the target %s/%s belongs to.`,
		kind, groupList, kind, result.RemediationTarget.Name,
	)

	resp, retryErr := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: appendCorrectionMessage(history, correctionMsg),
		Tools:    submitOnlyRCATools(),
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if retryErr != nil {
		inv.logger.Error(retryErr, "apiVersionValidationGate: retry failed, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent)
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	retryContent := extractSubmitContent(resp)
	if retryContent == "" {
		inv.logger.Info("apiVersionValidationGate: empty retry response, triggering human review",
			"correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent)
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Error(parseErr, "apiVersionValidationGate: retry parse failed, triggering human review",
			"correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent)
	}

	if retryResult.RemediationTarget.APIVersion == "" {
		inv.logger.Info("apiVersionValidationGate: retry still missing api_version, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent)
	}

	gateEvent.Data["retry_outcome"] = "resolved"
	// Clear parser-set HumanReviewNeeded from the retry result. The gate's
	// decision is authoritative: if the retry provided api_version, the
	// pipeline should continue to workflow selection, not abort.
	retryResult.HumanReviewNeeded = false
	retryResult.HumanReviewReason = ""
	// The retry is a fresh submission on its own turn: its own Reasoning
	// block (if any) is authoritative for the accepted result, mirroring
	// runRCA's "winning turn" semantics (BR-AI-086 AC6, #1578). Without this,
	// resp.Message.Reasoning from the retry call is silently discarded and
	// the gate-accepted result loses reasoning captured by the LLM client.
	retryResult.Reasoning = toReasoningSummary(resp.Message.Reasoning)
	inv.logger.Info("apiVersionValidationGate: retry provided api_version, accepted",
		"kind", kind,
		"api_version", retryResult.RemediationTarget.APIVersion,
		"correlation_id", correlationID)
	return retryResult
}

func (inv *Investigator) apiVersionGateExhaustion(
	result *katypes.InvestigationResult,
	groupList, kind, correlationID string,
	gateEvent *audit.AuditEvent,
) *katypes.InvestigationResult {
	gateEvent.Data["retry_outcome"] = "exhausted"
	result.HumanReviewNeeded = true
	result.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete

	// Clear workflow fields: the workflow was selected based on an ambiguous
	// kind without api_version, so it may target the wrong API group and lead
	// to incorrect RBAC grants. Leaving it populated would cause the API
	// response to include selected_workflow despite human review escalation.
	result.WorkflowID = ""
	result.ExecutionEngine = ""
	result.ExecutionBundle = ""
	result.ExecutionBundleDigest = ""
	result.WorkflowRationale = ""
	result.WorkflowVersion = ""
	result.Confidence = 0
	result.AlternativeWorkflows = nil

	result.Warnings = append(result.Warnings,
		fmt.Sprintf("apiVersionValidationGate: kind %q is ambiguous (API groups: %s) "+
			"but LLM did not provide api_version after retry — human review required to prevent "+
			"incorrect RBAC grants", kind, groupList))
	inv.logger.Info("apiVersionValidationGate: exhausted, human review required",
		"kind", kind, "conflicting_groups", groupList, "correlation_id", correlationID)
	return result
}

func enrichFromCatalog(result *katypes.InvestigationResult, v *parser.Validator) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}
	if result.ExecutionEngine == "" {
		result.ExecutionEngine = meta.ExecutionEngine
	}
	if result.ExecutionBundle == "" {
		result.ExecutionBundle = meta.ExecutionBundle
	}
	if result.ExecutionBundleDigest == "" {
		result.ExecutionBundleDigest = meta.ExecutionBundleDigest
	}
	if result.ServiceAccountName == "" {
		result.ServiceAccountName = meta.ServiceAccountName
	}
	result.WorkflowVersion = meta.Version

	// Issue #1661 Change 11a (DD-WORKFLOW-018): catalog-authoritative, not
	// LLM-suppliable -- always overwrite from meta rather than the
	// if-empty guards above, mirroring WorkflowVersion's unconditional
	// assignment.
	result.Dependencies = meta.Dependencies
	result.Resources = meta.Resources
	result.DeclaredParameterNames = meta.DeclaredParameterNames

	// Issue #1661 Change 12: ActionType/WorkflowName are likewise
	// catalog-authoritative -- always overwrite, same pattern as
	// Dependencies/Resources/DeclaredParameterNames above. This closes the
	// gap where KA never populated either field, breaking
	// workflowexecution.execution.started's audit payload (Change 11f).
	result.ActionType = meta.ActionType
	result.WorkflowName = meta.WorkflowName
}

// gvrToAPIVersion converts a GroupVersionResource to the apiVersion string
// format used by Kubernetes: "group/version" for named groups, "version" for
// core group (empty group). Issue #1051.
func gvrToAPIVersion(gvr schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return gvr.Version
	}
	return gvr.Group + "/" + gvr.Version
}

// CheckWorkflowTargetAlignment verifies that the selected workflow's component
// scope includes the RCA remediation target kind. Emits an audit event and
// appends a warning on mismatch. This is a WARNING-level gate: misalignment
// does not force human review because the LLM may have legitimate cross-type
// reasoning (Issue #934).
func CheckWorkflowTargetAlignment(ctx context.Context, result *katypes.InvestigationResult, v *parser.Validator, correlationID string, auditStore audit.AuditStore, logger logr.Logger) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}

	aligned := meta.MatchesTargetKind(result.RemediationTarget.Kind)

	ev := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	ev.EventAction = audit.ActionWorkflowAlignmentGate
	if aligned {
		ev.EventOutcome = audit.OutcomeSuccess
	} else {
		ev.EventOutcome = audit.OutcomeFailure
	}
	ev.Data["model"] = ""
	ev.Data["prompt_length"] = 0
	ev.Data["prompt_preview"] = ""
	ev.Data["workflow_id"] = result.WorkflowID
	ev.Data["target_kind"] = result.RemediationTarget.Kind
	ev.Data["workflow_components"] = meta.Component
	ev.Data["aligned"] = aligned
	audit.StoreBestEffort(ctx, auditStore, ev, logger)

	if !aligned {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Workflow %q target kind %q is not in the workflow's component scope %v",
				result.WorkflowID, result.RemediationTarget.Kind, meta.Component))
	}
}
