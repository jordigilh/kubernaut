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

func (inv *Investigator) sameKindValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	signal katypes.SignalContext,
	history []llm.Message,
	tokens *TokenAccumulator,
	correlationID string,
	client llm.Client,
	modelName string,
	runtimeParams llm.RuntimeParams,
) *katypes.InvestigationResult {
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

	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is "%s", which is the same resource kind as the input signal. `+
			`Signals often propagate upward: workload-level issues manifest as conditions on parent resources `+
			`(e.g., pod memory leaks cause node DiskPressure, deployment misconfigurations appear as node conditions). `+
			`Please re-evaluate: is a child resource (Deployment, StatefulSet, DaemonSet, Pod) the actual root cause `+
			`whose configuration should be modified? If after re-evaluation you are confident the %s itself is the `+
			`correct remediation target, confirm by resubmitting with the same target and explain why in your `+
			`due_diligence.target_accuracy field.`,
		result.RemediationTarget.Kind,
		result.RemediationTarget.Kind,
	)

	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	resp, err := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
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

	var retryContent string
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			retryContent = tc.Arguments
			break
		}
	}
	if retryContent == "" && resp.Message.Content != "" {
		retryContent = resp.Message.Content
	}
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
	tokens *TokenAccumulator,
	correlationID string,
	client llm.Client,
	modelName string,
	runtimeParams llm.RuntimeParams,
) *katypes.InvestigationResult {
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
		// #1051: auto-resolve apiVersion for non-ambiguous kinds using the
		// REST mapper. This guarantees RemediationTarget.APIVersion is always
		// populated after RCA, enabling GVK-format workflow component matching.
		// PROD-2: when len(gvrs) > 1 but not ambiguous (multiple versions in
		// the same group, e.g. v1 and v1beta1), we skip auto-resolve because
		// the preferred version is not deterministic. The fallback in custom
		// tools will use lowercase kind for discovery in this edge case.
		if len(gvrs) == 1 {
			result.RemediationTarget.APIVersion = gvrToAPIVersion(gvrs[0])
			inv.logger.Info("apiVersionValidationGate: auto-resolved apiVersion for non-ambiguous kind",
				"kind", kind,
				"api_version", result.RemediationTarget.APIVersion,
				"correlation_id", correlationID)
		}
		return result
	}

	groupNames := make([]string, 0, len(gvrs))
	seen := make(map[string]struct{})
	for _, gvr := range gvrs {
		if _, ok := seen[gvr.Group]; !ok {
			groupNames = append(groupNames, gvr.Group)
			seen[gvr.Group] = struct{}{}
		}
	}
	groupList := strings.Join(groupNames, ", ")

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

	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	resp, retryErr := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
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

	var retryContent string
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			retryContent = tc.Arguments
			break
		}
	}
	if retryContent == "" && resp.Message.Content != "" {
		retryContent = resp.Message.Content
	}
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
	result.HumanReviewReason = "rca_incomplete"

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
