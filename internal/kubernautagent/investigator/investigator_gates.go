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

// gateRetryOutcome enumerates why a validation-gate retry ended the way it
// did, recorded in the gate's audit event Data["retry_outcome"] (#2120,
// BR-AI-2120, FedRAMP AU-3). Prior to #2120 only "resolved"/"exhausted"
// existed (apiVersionValidationGate only); the additional values give
// auditors finer-grained visibility into *why* a retry failed instead of a
// single opaque "exhausted" bucket.
type gateRetryOutcome string

const (
	// gateRetryResolved: the retry's first attempt returned a usable
	// submit_result content.
	gateRetryResolved gateRetryOutcome = "resolved"
	// gateRetryResolvedAfterReminder: the retry's first attempt called an
	// undeclared tool instead of submit_result; a second attempt (replaying
	// that call as a tool_result error plus an explicit reminder) recovered
	// a usable submit_result content (#2120).
	gateRetryResolvedAfterReminder gateRetryOutcome = "resolved_after_other_tool_retry"
	// gateRetryOtherToolExhausted: the LLM called an undeclared tool on both
	// the initial retry and the reminder retry (or the reminder retry itself
	// errored) — the correction was never answered with submit_result (#2120).
	gateRetryOtherToolExhausted gateRetryOutcome = "llm_requested_other_tool"
	// gateRetryEmptyResponse: the retry returned no tool calls and no message
	// content at all.
	gateRetryEmptyResponse gateRetryOutcome = "empty_response"
	// gateRetryParseError: the retry's content could not be parsed as an RCA result.
	gateRetryParseError gateRetryOutcome = "parse_error"
	// gateRetryExhausted: a generic/infrastructure failure (LLM call error, or
	// the parsed retry result still failed the gate's own check) with no more
	// specific outcome above applying.
	gateRetryExhausted gateRetryOutcome = "exhausted"
)

// gateRetrySubmitContent classifies a gate-retry LLM response: returns the
// submit_result tool-call arguments if the model called that tool, else
// falls back to the raw message content (mirroring the pre-#2120 behavior
// for models that respond with bare JSON text instead of a tool call).
// Any tool call to a name other than submit_result is reported in
// otherTools -- calling an undeclared tool instead of resubmitting the RCA
// result is the #2120 failure mode this enables detecting.
func gateRetrySubmitContent(resp llm.ChatResponse) (content string, otherTools []string) {
	for _, tc := range resp.ToolCalls {
		if tc.Name == SubmitResultToolName {
			return tc.Arguments, nil
		}
		otherTools = append(otherTools, tc.Name)
	}
	if resp.Message.Content != "" {
		return resp.Message.Content, otherTools
	}
	return "", otherTools
}

// retryGateOnUnexpectedTool re-issues a gate-retry call once more after the
// model called a tool other than submit_result instead of resubmitting the
// RCA result. It replays the wrong call(s) as synthetic tool_result "not
// available" errors, plus an explicit reminder to call submit_result --
// mirroring the real tool-execution message pattern used elsewhere in this
// package (see the tool-call loop in Investigate). A live-LLM spike against
// the production Claude/Vertex client (10/10 trials, see
// docs/testing/2120/TEST_PLAN.md) confirmed this recovers the correct
// submit_result call on the very next turn.
func (inv *Investigator) retryGateOnUnexpectedTool(
	ctx context.Context,
	client llm.Client,
	retryMessages []llm.Message,
	firstResp llm.ChatResponse,
	tools []llm.ToolDefinition,
	tokens *TokenAccumulator,
	runtimeParams llm.RuntimeParams,
	correlationID string,
) (content string, otherTools []string, err error) {
	assistantMsg := firstResp.Message
	assistantMsg.ToolCalls = firstResp.ToolCalls
	retryMessages = append(retryMessages, assistantMsg)

	var calledTools []string
	for _, tc := range firstResp.ToolCalls {
		calledTools = append(calledTools, tc.Name)
		retryMessages = append(retryMessages, llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("Tool %q is not available for this correction step.", tc.Name),
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
		})
	}
	retryMessages = append(retryMessages, llm.Message{
		Role: "user",
		Content: "You must call submit_result to resubmit your root cause analysis result. " +
			"No other tool is available for this correction step.",
	})

	inv.logger.Info("gate retry: LLM called an undeclared tool, retrying once with reminder",
		"tools_called", calledTools,
		"correlation_id", correlationID)

	resp, chatErr := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    tools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if chatErr != nil {
		return "", nil, chatErr
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	content, otherTools = gateRetrySubmitContent(resp)
	return content, otherTools, nil
}

// submitResultOnlyTools returns the single-tool declaration shared by both
// validation gates' retry calls: submit_result is the only tool the LLM is
// allowed to call when resubmitting its RCA result during a gate correction.
func submitResultOnlyTools() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}
}

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

	// #2118: the original wording below only asked the LLM to confirm or
	// change remediation_target.kind. Because that reads as a narrow
	// yes/no question, the LLM often responds with a minimal tool call
	// that answers only the target question and omits (or zeroes) the
	// separately-required confidence field -- silently overwriting a
	// real, previously-validated confidence with a placeholder. The
	// second paragraph makes the full-restatement requirement explicit.
	// This is a best-effort, defense-in-depth improvement: a live-LLM
	// spike (see docs/testing/2118/TEST_PLAN.md) confirmed prompt wording
	// alone is not reliably sufficient, which is why the guard below is
	// the actual, deterministic fix.
	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is "%s", which is the same resource kind as the input signal. `+
			`Signals often propagate upward: workload-level issues manifest as conditions on parent resources `+
			`(e.g., pod memory leaks cause node DiskPressure, deployment misconfigurations appear as node conditions). `+
			`Please re-evaluate: is a child resource (Deployment, StatefulSet, DaemonSet, Pod) the actual root cause `+
			`whose configuration should be modified? If after re-evaluation you are confident the %s itself is the `+
			`correct remediation target, confirm by resubmitting with the same target and explain why in your `+
			`due_diligence.target_accuracy field. `+
			`Whichever target you conclude is correct, you MUST resubmit a COMPLETE root cause analysis result: `+
			`restate confidence (genuinely recomputed from the evidence, not a placeholder or default value), `+
			`severity, and causal_chain at the same rigor as your original submission. Do not omit or zero out `+
			`confidence just because this is a confirmation -- an incomplete resubmission will be treated as a `+
			`loss of your prior analysis.`,
		result.RemediationTarget.Kind,
		result.RemediationTarget.Kind,
	)

	submitOnlyTools := submitResultOnlyTools()

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	// #1777 (BR-AUDIT-005, FedRAMP AU-3): the audit event must reflect the
	// retry prompt actually sent to the LLM. Building it before retryMessages
	// exists forced prompt_length/prompt_preview to a hardcoded 0/"" that
	// misrepresented every gate retry as an empty request.
	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionSameKindGate
	gateEvent.Data["model"] = modelName
	gateEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
	gateEvent.Data["prompt_preview"] = lastUserMessage(retryMessages, 500)
	gateEvent.Data["signal_resource_kind"] = signal.ResourceKind
	gateEvent.Data["target_kind"] = result.RemediationTarget.Kind
	gateEvent.Data["target_name"] = result.RemediationTarget.Name
	// #2120 (BR-AI-2120, FedRAMP AU-3): fire once, after the final outcome is
	// known, via defer -- StoreBestEffort reads gateEvent.Data/EventOutcome at
	// the time this deferred call actually executes (function return), not
	// at the time the defer statement runs. Storing immediately here (as the
	// pre-#2120 code did) meant retry_outcome/EventOutcome mutations made
	// below were set on the event in memory but never actually captured by a
	// production audit store, which serializes Data into an outbound request
	// synchronously at call time.
	defer audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	// #2086: this gate-retry LLM call is non-streamed (llm.ChatWithParams),
	// so it emits zero sink events for the duration of the round-trip. A
	// slow/retried backend call here can outlast AF's bridge-inactivity
	// timeout, causing AF to falsely report the investigation as
	// "completed" while the driving agent never calls discover_workflows.
	// This keepalive resets that timer without polluting the RCA summary
	// (EventTypeToolCallStart is never concatenated into summary text).
	emitGateRetryKeepalive(ctx, "revalidating_remediation_target")

	resp, err := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if err != nil {
		inv.logger.Error(err, "same-kind validation gate retry failed, keeping original result",
			"correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(gateRetryExhausted)
		return result
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	var usedReminder bool
	retryContent, otherTools := gateRetrySubmitContent(resp)
	if len(otherTools) > 0 {
		gateEvent.Data["other_tools_called"] = otherTools
	}
	if retryContent == "" && len(otherTools) > 0 {
		// #2120: the model called an undeclared tool instead of resubmitting
		// the RCA result. Retry once with a synthetic tool-error + reminder
		// (live-LLM spike: 10/10 recovery) before falling back to keeping
		// the original result.
		inv.logger.Info("same-kind validation gate: LLM called an undeclared tool instead of submit_result, retrying once with reminder",
			"tools_called", otherTools,
			"correlation_id", correlationID)
		usedReminder = true
		var reminderErr error
		retryContent, otherTools, reminderErr = inv.retryGateOnUnexpectedTool(ctx, client, retryMessages, resp, submitOnlyTools, tokens, runtimeParams, correlationID)
		if len(otherTools) > 0 {
			gateEvent.Data["other_tools_called"] = otherTools
		}
		if reminderErr != nil {
			inv.logger.Error(reminderErr, "same-kind validation gate: retry-on-unexpected-tool call failed, keeping original",
				"correlation_id", correlationID)
			gateEvent.EventOutcome = audit.OutcomeFailure
			gateEvent.Data["retry_outcome"] = string(gateRetryOtherToolExhausted)
			return result
		}
		if retryContent == "" {
			inv.logger.Info("same-kind validation gate: LLM called an undeclared tool on both attempts, keeping original",
				"tools_called", otherTools,
				"correlation_id", correlationID)
			gateEvent.EventOutcome = audit.OutcomeFailure
			gateEvent.Data["retry_outcome"] = string(gateRetryOtherToolExhausted)
			return result
		}
	}
	if retryContent == "" {
		inv.logger.Info("same-kind validation gate: no content in retry response, keeping original",
			"correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(gateRetryEmptyResponse)
		return result
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Error(parseErr, "same-kind validation gate: retry parse failed, keeping original",
			"correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(gateRetryParseError)
		return result
	}

	if retryResult.RemediationTarget.Kind == "" && result.RemediationTarget.Kind != "" {
		inv.logger.Info("same-kind validation gate: retry lost remediation_target, keeping original",
			"original_target", result.RemediationTarget.Kind+"/"+result.RemediationTarget.Name,
			"correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(gateRetryEmptyResponse)
		return result
	}

	// #2118: unlike the RemediationTarget.Kind guard above, a lost
	// confidence does not warrant discarding the whole retry -- the
	// retry's other content (e.g. an updated due_diligence.target_accuracy
	// narrative) is genuinely new and worth keeping. Only the regressed
	// field itself is backfilled from the pre-retry result.
	if retryResult.Confidence <= 0 && result.Confidence > 0 {
		inv.logger.Info("same-kind validation gate: retry lost confidence, keeping original",
			"original_confidence", result.Confidence,
			"correlation_id", correlationID)
		retryResult.Confidence = result.Confidence
	}

	gateEvent.EventOutcome = audit.OutcomeSuccess
	if usedReminder {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolvedAfterReminder)
	} else {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolved)
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

	submitOnlyTools := submitResultOnlyTools()

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "user", Content: correctionMsg},
	)

	// #1777 (BR-AUDIT-005, FedRAMP AU-3): the audit event must reflect the
	// retry prompt actually sent to the LLM. Building it before retryMessages
	// exists forced prompt_length/prompt_preview to a hardcoded 0/"" that
	// misrepresented every gate retry as an empty request.
	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionAPIVersionGate
	gateEvent.Data["model"] = modelName
	gateEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
	gateEvent.Data["prompt_preview"] = lastUserMessage(retryMessages, 500)
	gateEvent.Data["ambiguous_kind"] = kind
	gateEvent.Data["conflicting_groups"] = groupList
	// #2120 (BR-AI-2120, FedRAMP AU-3): fire once, after the final outcome is
	// known, via defer -- this also fixes a pre-existing bug (#1044) where
	// gateEvent.Data["retry_outcome"] was mutated below AFTER this store call
	// fired, so a production audit store (which serializes Data into an
	// outbound request synchronously at call time) never actually persisted
	// retry_outcome.
	defer audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	// #2086: same silent-gap risk as sameKindValidationGate above — this
	// retry LLM call is non-streamed and emits no sink events during the
	// round-trip.
	emitGateRetryKeepalive(ctx, "resolving_api_version_ambiguity")

	resp, retryErr := llm.ChatWithParams(ctx, client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, runtimeParams)
	if retryErr != nil {
		inv.logger.Error(retryErr, "apiVersionValidationGate: retry failed, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryExhausted)
	}
	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	var usedReminder bool
	retryContent, otherTools := gateRetrySubmitContent(resp)
	if len(otherTools) > 0 {
		gateEvent.Data["other_tools_called"] = otherTools
	}
	if retryContent == "" && len(otherTools) > 0 {
		// #2120: the model called an undeclared tool instead of resubmitting
		// the RCA result. Retry once with a synthetic tool-error + reminder
		// (live-LLM spike: 10/10 recovery) before triggering human review.
		inv.logger.Info("apiVersionValidationGate: LLM called an undeclared tool instead of submit_result, retrying once with reminder",
			"kind", kind, "tools_called", otherTools, "correlation_id", correlationID)
		usedReminder = true
		var reminderErr error
		retryContent, otherTools, reminderErr = inv.retryGateOnUnexpectedTool(ctx, client, retryMessages, resp, submitOnlyTools, tokens, runtimeParams, correlationID)
		if len(otherTools) > 0 {
			gateEvent.Data["other_tools_called"] = otherTools
		}
		if reminderErr != nil {
			inv.logger.Error(reminderErr, "apiVersionValidationGate: retry-on-unexpected-tool call failed, triggering human review",
				"kind", kind, "correlation_id", correlationID)
			return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryOtherToolExhausted)
		}
		if retryContent == "" {
			inv.logger.Info("apiVersionValidationGate: LLM called an undeclared tool on both attempts, triggering human review",
				"kind", kind, "tools_called", otherTools, "correlation_id", correlationID)
			return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryOtherToolExhausted)
		}
	}
	if retryContent == "" {
		inv.logger.Info("apiVersionValidationGate: empty retry response, triggering human review",
			"correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryEmptyResponse)
	}

	retryResult, parseErr := inv.resultParser.Parse(retryContent)
	if parseErr != nil {
		inv.logger.Error(parseErr, "apiVersionValidationGate: retry parse failed, triggering human review",
			"correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryParseError)
	}

	if retryResult.RemediationTarget.APIVersion == "" {
		inv.logger.Info("apiVersionValidationGate: retry still missing api_version, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryExhausted)
	}

	gateEvent.EventOutcome = audit.OutcomeSuccess
	if usedReminder {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolvedAfterReminder)
	} else {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolved)
	}
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
	outcome gateRetryOutcome,
) *katypes.InvestigationResult {
	gateEvent.Data["retry_outcome"] = string(outcome)
	gateEvent.EventOutcome = audit.OutcomeFailure
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
		"kind", kind, "conflicting_groups", groupList, "retry_outcome", outcome, "correlation_id", correlationID)
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
