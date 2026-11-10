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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// gateRetryOutcome enumerates why a validation-gate retry ended the way it
// did, recorded in the gate's audit event Data["retry_outcome"] (#2121,
// BR-AI-2121, FedRAMP AU-3). Prior to #2121 only "resolved"/"exhausted"
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
	// a usable submit_result content (#2121).
	gateRetryResolvedAfterReminder gateRetryOutcome = "resolved_after_other_tool_retry"
	// gateRetryOtherToolExhausted: the LLM called an undeclared tool on both
	// the initial retry and the reminder retry (or the reminder retry itself
	// errored) — the correction was never answered with submit_result (#2121).
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
	// gateRetryCancelled: the parent investigation context ended while the
	// gate was verifying or correcting the target.
	gateRetryCancelled gateRetryOutcome = "cancelled"
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

// extractSubmitContent classifies a gate-retry LLM response: returns the
// submit_result tool-call arguments if the model called that tool, else
// falls back to the raw message content (mirroring the pre-#2121 behavior
// for models that respond with bare JSON text instead of a tool call). Any
// tool call to a name other than submit_result is reported in otherTools —
// calling an undeclared tool instead of resubmitting the RCA result is the
// #2121 failure mode this enables detecting.
func extractSubmitContent(resp llm.ChatResponse) (content string, otherTools []string) {
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
// docs/testing/2121/TEST_PLAN.md) confirmed this recovers the correct
// submit_result call on the very next turn.
func (inv *Investigator) retryGateOnUnexpectedTool(
	ctx context.Context,
	llmCtx LLMInvocationContext,
	retryMessages []llm.Message,
	firstResp llm.ChatResponse,
	tools []llm.ToolDefinition,
) (content string, otherTools []string, err error) {
	assistantMsg := firstResp.Message
	assistantMsg.ToolCalls = firstResp.ToolCalls
	retryMessages = append(retryMessages, assistantMsg)

	calledTools := make([]string, 0, len(firstResp.ToolCalls))
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
		"correlation_id", llmCtx.CorrelationID)

	resp, chatErr := llm.ChatWithParams(ctx, llmCtx.Client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    tools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, llmCtx.RuntimeParams)
	if chatErr != nil {
		return "", nil, chatErr
	}
	if llmCtx.Tokens != nil {
		llmCtx.Tokens.Add(resp.Usage)
	}
	// #2387: gate reminder retry is its own LLM turn.
	inv.metricsFor(llmCtx.CorrelationID).IncLLMTurns()
	inv.recordTokenUsage(llmCtx.CorrelationID, resp.Usage)

	content, otherTools = extractSubmitContent(resp)
	return content, otherTools, nil
}

// resolveGateRetrySubmission classifies a gate-retry response and, if the
// model called an undeclared tool instead of submit_result, attempts one
// reminder retry via retryGateOnUnexpectedTool (#2121). Shared by
// retryForSameKind and retryForAPIVersion to avoid duplicating the
// classify-then-maybe-retry-once branching in both gates. ok is false when
// the retry must be treated as failed (empty response either originally or
// after the reminder, or the undeclared-tool call persisted through the
// reminder); outcome then names why, for the caller to record on gateEvent
// before returning its own fallback result.
func (inv *Investigator) resolveGateRetrySubmission(
	ctx context.Context,
	llmCtx LLMInvocationContext,
	retryMessages []llm.Message,
	resp llm.ChatResponse,
	tools []llm.ToolDefinition,
	gateEvent *audit.AuditEvent,
	logPrefix string,
) (content string, usedReminder bool, outcome gateRetryOutcome, ok bool) {
	correlationID := llmCtx.CorrelationID
	retryContent, otherTools := extractSubmitContent(resp)
	if len(otherTools) > 0 {
		gateEvent.Data["other_tools_called"] = otherTools
	}
	if retryContent == "" && len(otherTools) > 0 {
		// #2121: the model called an undeclared tool instead of resubmitting
		// the RCA result. Retry once with a synthetic tool-error + reminder
		// (live-LLM spike: 10/10 recovery) before giving up.
		inv.logger.Info(logPrefix+": LLM called an undeclared tool instead of submit_result, retrying once with reminder",
			"tools_called", otherTools, "correlation_id", correlationID)
		usedReminder = true
		var reminderErr error
		retryContent, otherTools, reminderErr = inv.retryGateOnUnexpectedTool(ctx, llmCtx, retryMessages, resp, tools)
		if len(otherTools) > 0 {
			gateEvent.Data["other_tools_called"] = otherTools
		}
		if reminderErr != nil {
			inv.logger.Error(reminderErr, logPrefix+": retry-on-unexpected-tool call failed, giving up",
				"correlation_id", correlationID)
			return "", usedReminder, gateRetryOtherToolExhausted, false
		}
		if retryContent == "" {
			inv.logger.Info(logPrefix+": LLM called an undeclared tool on both attempts, giving up",
				"tools_called", otherTools, "correlation_id", correlationID)
			return "", usedReminder, gateRetryOtherToolExhausted, false
		}
	}
	if retryContent == "" {
		inv.logger.Info(logPrefix+": no content in retry response, giving up",
			"correlation_id", correlationID)
		return "", usedReminder, gateRetryEmptyResponse, false
	}
	return retryContent, usedReminder, "", true
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

	// #1777 (BR-AUDIT-005, FedRAMP AU-3): prompt_length/prompt_preview are
	// populated in retryForSameKind, once the actual retry prompt exists —
	// see the audit.StoreBestEffort call there for why.
	gateEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	gateEvent.EventAction = audit.ActionSameKindGate
	gateEvent.EventOutcome = audit.OutcomeSuccess
	gateEvent.Data["model"] = modelName
	gateEvent.Data["signal_resource_kind"] = signal.ResourceKind
	gateEvent.Data["target_kind"] = result.RemediationTarget.Kind
	gateEvent.Data["target_name"] = result.RemediationTarget.Name

	return inv.retryForSameKind(ctx, result, history, llmCtx, gateEvent)
}

// sameKindCorrectionMessage builds the LLM correction message asking it to
// re-evaluate whether a child resource (rather than targetKind, which
// matches the input signal's kind) is the true root cause.
//
// #2119 (BR-AI-2119, FedRAMP SI-11/AU-3): the original wording below only
// asked the LLM to confirm or change remediation_target.kind. Because that
// reads as a narrow yes/no question, the LLM often responds with a minimal
// tool call that answers only the target question and omits (or zeroes) the
// separately-required confidence field -- silently overwriting a real,
// previously-validated confidence with a placeholder. The final paragraph
// makes the full-restatement requirement explicit. This is a best-effort,
// defense-in-depth improvement: a live-LLM spike (see
// docs/testing/2119/TEST_PLAN.md) confirmed prompt wording alone is not
// reliably sufficient, which is why the confidence-backfill guard in
// retryForSameKind is the actual, deterministic fix.
func sameKindCorrectionMessage(targetKind string) string {
	return fmt.Sprintf(
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
		targetKind, targetKind,
	)
}

// retryForSameKind re-submits the RCA request with a correction message
// asking the LLM to re-evaluate a same-kind remediation target, and parses
// the retry response. Falls back to the original result at every failure
// point: LLM error, empty response, parse error, or a retry result that
// lost the remediation target.
func (inv *Investigator) retryForSameKind(ctx context.Context, result *katypes.InvestigationResult, history []llm.Message, llmCtx LLMInvocationContext, gateEvent *audit.AuditEvent) *katypes.InvestigationResult {
	correlationID := llmCtx.CorrelationID
	correctionMsg := sameKindCorrectionMessage(result.RemediationTarget.Kind)
	retryMessages := appendCorrectionMessage(history, correctionMsg)
	submitOnlyTools := submitOnlyRCATools()

	// #1777 (BR-AUDIT-005, FedRAMP AU-3): the audit event must reflect the
	// retry prompt actually sent to the LLM. Populating these fields before
	// retryMessages exists forced prompt_length/prompt_preview to a
	// hardcoded 0/"" that misrepresented every gate retry as an empty
	// request.
	gateEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
	gateEvent.Data["prompt_preview"] = lastUserMessage(retryMessages)
	// #2121 (BR-AI-2121, FedRAMP AU-3): fire once, after the final outcome is
	// known, via defer -- StoreBestEffort reads gateEvent.Data/EventOutcome
	// at the time this deferred call actually executes (function return),
	// not at the time the defer statement runs. Storing immediately here (as
	// the pre-#2121 code did) meant retry_outcome/EventOutcome mutations
	// made below were set on the event in memory but never actually
	// captured by a production audit store, which serializes Data into an
	// outbound request synchronously at call time.
	defer audit.StoreBestEffort(ctx, inv.auditStore, gateEvent, inv.auditLog())

	// #2088 (main port of #2086): this gate-retry LLM call is non-streamed
	// (llm.ChatWithParams), so it emits zero sink events for the duration of
	// the round-trip. A slow/retried backend call here can outlast AF's
	// bridge-inactivity timeout, causing AF to falsely report the
	// investigation as "completed" while the driving agent never calls
	// discover_workflows. This keepalive resets that timer without
	// polluting the RCA summary (EventTypeToolCallStart is never
	// concatenated into summary text).
	emitGateRetryKeepalive(ctx, "revalidating_remediation_target")

	resp, err := llm.ChatWithParams(ctx, llmCtx.Client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    submitOnlyTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, llmCtx.RuntimeParams)
	if err != nil {
		inv.logger.Error(err, "same-kind validation gate retry failed, keeping original result",
			"correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(gateRetryExhausted)
		return result
	}
	if llmCtx.Tokens != nil {
		llmCtx.Tokens.Add(resp.Usage)
	}
	// #2387: gate retry is its own LLM turn on top of the loop-counted turns.
	inv.metricsFor(correlationID).IncLLMTurns()
	inv.recordTokenUsage(correlationID, resp.Usage)

	retryContent, usedReminder, failOutcome, ok := inv.resolveGateRetrySubmission(
		ctx, llmCtx, retryMessages, resp, submitOnlyTools, gateEvent, "same-kind validation gate")
	if !ok {
		inv.logger.Info("same-kind validation gate: keeping original result after unresolved retry",
			"retry_outcome", failOutcome, "correlation_id", correlationID)
		gateEvent.EventOutcome = audit.OutcomeFailure
		gateEvent.Data["retry_outcome"] = string(failOutcome)
		return result
	}

	retryResult, parseErr := inv.parseRCA(retryContent)
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

	// #2119: unlike the RemediationTarget.Kind guard above, a lost
	// confidence does not warrant discarding the whole retry -- the
	// retry's other content (e.g. an updated due_diligence.target_accuracy
	// narrative) is genuinely new and worth keeping. Only the regressed
	// field itself is backfilled from the pre-retry result.
	if !retryResult.ConfidenceSet && result.ConfidenceSet {
		inv.logger.Info("same-kind validation gate: retry lost confidence, keeping original",
			"original_confidence", result.Confidence,
			"correlation_id", correlationID)
		preserveGateConfidence(result, retryResult)
	}

	// The retry is a fresh submission on its own turn: its own Reasoning
	// block (if any) is authoritative for the accepted result, mirroring
	// runRCA's "winning turn" semantics (BR-AI-086 AC6, #1578). Without this,
	// resp.Message.Reasoning from the retry call is silently discarded and
	// the gate-accepted result loses reasoning captured by the LLM client.
	retryResult.Reasoning = toReasoningSummary(resp.Message.Reasoning)

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

type apiVersionVerification struct {
	validated bool
	cancelled bool
	source    string
	reason    string
}

type apiResourceDiscoveryPayload struct {
	Resources []struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	} `json:"resources"`
	Warning string `json:"warning"`
}

const maxAPIVersionCorrectionDiscoveryRounds = 2

// The model may request both target-scoped discovery endpoints in one retry
// response, but an oversized batch must not consume the general tool budget.
const maxAPIVersionCorrectionDiscoveryCallsPerResponse = 2

func (inv *Investigator) executeTargetScopedDiscoveryTool(ctx context.Context, name string, args json.RawMessage, correlationID string, auditIndex int) (string, error) {
	// Gate-owned discovery is still a normal investigator tool call. Keep the
	// same anomaly budget, sanitizer/alignment handling, timeout, audit record,
	// observer events and per-investigation metrics as model-dispatched calls.
	callCtx := ctx
	if inv.toolCallTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(callCtx, inv.toolCallTimeout)
		defer cancel()
	}
	toolCall := llm.ToolCall{Name: name, Arguments: string(args)}
	inv.metricsFor(correlationID).AddToolCalls(1)
	emitToSink(ctx, session.EventTypeToolCallStart, 0, string(katypes.PhaseRCA), map[string]interface{}{
		"tool_name":  name,
		"tool_index": auditIndex,
	})
	result := inv.executeToolWithOptions(callCtx, name, args, correlationID, katypes.PhaseRCA, true)
	inv.emitToolCallAudit(context.WithoutCancel(ctx), toolCall, auditIndex, result, string(katypes.PhaseRCA), correlationID)
	emitToSink(ctx, session.EventTypeToolResult, 0, string(katypes.PhaseRCA), map[string]interface{}{
		"tool_name":      name,
		"tool_index":     auditIndex,
		"result_preview": truncatePreview(result, 200),
	})
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := callCtx.Err(); err != nil {
		return "", err
	}
	if toolErr := discoveryToolError(result); toolErr != nil {
		return "", fmt.Errorf("target-scoped discovery tool %q: %w", name, toolErr)
	}
	return result, nil
}

func isInvestigationCancellation(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) ||
		(errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil)
}

func discoveryToolError(result string) error {
	var envelope struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result), &envelope); err == nil && strings.EqualFold(envelope.Status, "error") {
		if envelope.Error == "" {
			return fmt.Errorf("target-scoped discovery tool returned an error")
		}
		return fmt.Errorf("target-scoped discovery tool returned an error: %s", envelope.Error)
	}
	return nil
}

func hasExactAPIResource(payload apiResourceDiscoveryPayload, target katypes.RemediationTarget) bool {
	for _, resource := range payload.Resources {
		if strings.EqualFold(resource.Kind, target.Kind) && resource.APIVersion == target.APIVersion {
			return true
		}
	}
	return false
}

// hasExactResourceListEvidence accepts the structured list response returned
// by the local metadata resolver and the remote kube-mcp-server. The local
// resolver returns a metadata list object, while kube-mcp-server's YAML output
// is a sequence of the listed objects. A successful metadata list or empty
// remote sequence proves that the requested list endpoint accepted the exact
// GVK; non-empty sequences must carry matching per-item GVK evidence.
func hasExactResourceListEvidence(result string, target katypes.RemediationTarget) bool {
	return hasExactResourceListEvidenceForSource(result, target, true)
}

func hasExactResourceListEvidenceForSource(result string, target katypes.RemediationTarget, allowMetadataList bool) bool {
	if strings.EqualFold(strings.TrimSpace(result), "No resources found") {
		return true
	}
	jsonResult, err := sigsyaml.YAMLToJSON([]byte(result))
	if err != nil {
		return false
	}
	trimmed := bytes.TrimSpace(jsonResult)
	if len(trimmed) == 0 {
		return false
	}

	switch trimmed[0] {
	case '[':
		return hasExactResourceListSequence(trimmed, target)
	case '{':
		return hasExactResourceListEnvelope(trimmed, target, allowMetadataList)
	default:
		return false
	}
}

func hasExactResourceListSequence(trimmed []byte, target katypes.RemediationTarget) bool {
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return false
	}
	if len(items) == 0 {
		return items != nil
	}
	return resourceListItemsWellFormed(items, target, true)
}

func hasExactResourceListEnvelope(trimmed []byte, target katypes.RemediationTarget, allowMetadataList bool) bool {
	var payload struct {
		APIVersion string          `json:"apiVersion"`
		Kind       string          `json:"kind"`
		Items      json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return false
	}
	if len(payload.Items) == 0 || validateResourceListEnvelope(payload.APIVersion, payload.Kind, target.APIVersion, target.Kind) != nil {
		return false
	}
	if payload.APIVersion == "meta.k8s.io/v1" && !allowMetadataList {
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(payload.Items, &items); err != nil || items == nil {
		return false
	}
	return resourceListItemsWellFormed(items, target, false)
}

// resourceListItemsWellFormed rejects opaque item values before a list can
// establish exact-GVK evidence. Full remote objects must carry the requested
// apiVersion/kind; local metadata summaries may instead carry a metadata map.
func resourceListItemsWellFormed(items []json.RawMessage, target katypes.RemediationTarget, requireGVK bool) bool {
	for _, rawItem := range items {
		var item map[string]json.RawMessage
		if err := json.Unmarshal(rawItem, &item); err != nil || item == nil {
			return false
		}
		apiVersionRaw, hasAPIVersion := item["apiVersion"]
		kindRaw, hasKind := item["kind"]
		if hasAPIVersion || hasKind {
			var apiVersion, kind string
			if !hasAPIVersion || !hasKind || json.Unmarshal(apiVersionRaw, &apiVersion) != nil ||
				json.Unmarshal(kindRaw, &kind) != nil || apiVersion != target.APIVersion ||
				!strings.EqualFold(kind, target.Kind) {
				return false
			}
			continue
		}
		if requireGVK {
			return false
		}
		metadataRaw, hasMetadata := item["metadata"]
		if !hasMetadata {
			return false
		}
		if _, err := projectResourceMetadata(metadataRaw); err != nil {
			return false
		}
	}
	return true
}

// projectResourceMetadata validates and retains only the bounded metadata
// fields safe for investigator context and audit persistence. Present fields
// must have their declared scalar types; silently dropping malformed values
// would turn invalid cluster evidence into apparently trustworthy evidence.
func projectResourceMetadata(rawMetadata json.RawMessage) (map[string]interface{}, error) {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(rawMetadata, &metadata); err != nil || metadata == nil {
		return nil, fmt.Errorf("resources_list item metadata is malformed")
	}
	safeMetadata := make(map[string]interface{}, 6)
	for _, key := range []string{"name", "namespace", "uid", "resourceVersion"} {
		if rawValue, exists := metadata[key]; exists {
			var value string
			trimmed := bytes.TrimSpace(rawValue)
			if bytes.Equal(trimmed, []byte("null")) || json.Unmarshal(rawValue, &value) != nil {
				return nil, fmt.Errorf("resources_list metadata.%s is not a string", key)
			}
			safeMetadata[key] = value
		}
	}
	if rawLabels, exists := metadata["labels"]; exists {
		trimmed := bytes.TrimSpace(rawLabels)
		var labels map[string]string
		if bytes.Equal(trimmed, []byte("null")) || json.Unmarshal(rawLabels, &labels) != nil || labels == nil {
			return nil, fmt.Errorf("resources_list metadata.labels is not a string map")
		}
		safeMetadata["labels"] = k8s.BoundedResourceLabels(labels)
	}
	return safeMetadata, nil
}

type resourceListCompleteness struct {
	itemCount  int
	totalCount *int
	truncated  bool
}

type resourceListWireMetadata struct {
	itemCount          json.RawMessage
	totalCount         json.RawMessage
	truncated          json.RawMessage
	continueToken      json.RawMessage
	remainingItemCount json.RawMessage
}

// parseResourceListCompleteness preserves server-provided list completeness
// metadata while rejecting malformed scalar values. Missing fields are derived
// from the projected item count, which is exact for an unpaginated response.
func parseResourceListCompleteness(itemCount int, metadata resourceListWireMetadata) (resourceListCompleteness, error) {
	completeness := resourceListCompleteness{itemCount: itemCount}
	parsedItemCount, hasItemCount, err := parseResourceListNonNegativeInt(metadata.itemCount, "item_count")
	if err != nil {
		return resourceListCompleteness{}, err
	}
	if hasItemCount && parsedItemCount != itemCount {
		return resourceListCompleteness{}, fmt.Errorf("resources_list item_count does not match items")
	}
	continuation, err := resourceListContinuation(metadata.continueToken)
	if err != nil {
		return resourceListCompleteness{}, err
	}
	remaining, hasRemaining, err := parseResourceListNonNegativeInt(metadata.remainingItemCount, "remaining item count")
	if err != nil {
		return resourceListCompleteness{}, err
	}
	completeness.truncated, err = resourceListTruncated(metadata.truncated, continuation)
	if err != nil {
		return resourceListCompleteness{}, err
	}
	totalCount, hasTotalCount, err := resourceListTotalCount(completeness.itemCount, metadata.totalCount, completeness.truncated, remaining, hasRemaining)
	if err != nil {
		return resourceListCompleteness{}, err
	}
	if hasTotalCount {
		completeness.totalCount = &totalCount
	}
	return completeness, nil
}

func parseResourceListNonNegativeInt(raw json.RawMessage, field string) (int, bool, error) {
	if len(raw) == 0 {
		return 0, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, false, fmt.Errorf("resources_list %s is malformed", field)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil || value < 0 {
		return 0, false, fmt.Errorf("resources_list %s is malformed", field)
	}
	return value, true, nil
}

func resourceListContinuation(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, fmt.Errorf("resources_list continuation token is malformed")
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil {
		return false, fmt.Errorf("resources_list continuation token is malformed")
	}
	return token != "", nil
}

func resourceListTotalCount(itemCount int, raw json.RawMessage, incomplete bool, remaining int, hasRemaining bool) (int, bool, error) {
	totalCount, hasTotal, err := parseResourceListNonNegativeInt(raw, "total_count")
	if err != nil {
		return 0, false, err
	}
	switch {
	case hasTotal:
		if totalCount < itemCount {
			return 0, false, fmt.Errorf("resources_list total_count is malformed")
		}
		return totalCount, true, nil
	case hasRemaining:
		totalCount = itemCount + remaining
		if totalCount < itemCount {
			return 0, false, fmt.Errorf("resources_list total_count is malformed")
		}
		return totalCount, true, nil
	case !incomplete:
		totalCount = itemCount
		return totalCount, true, nil
	default:
		return 0, false, nil
	}
}

func resourceListTruncated(raw json.RawMessage, continuation bool) (bool, error) {
	if len(raw) == 0 {
		return continuation, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, fmt.Errorf("resources_list truncated flag is malformed")
	}
	var truncated bool
	if err := json.Unmarshal(raw, &truncated); err != nil {
		return false, fmt.Errorf("resources_list truncated flag is malformed")
	}
	if continuation && !truncated {
		return false, fmt.Errorf("resources_list continuation requires truncated=true")
	}
	return truncated, nil
}

// parseResourceListPageHeader reads the comment metadata emitted by
// kube-mcp-server before a YAML list. The opaque token is intentionally not
// retained in the projected payload; its presence only proves the page is
// incomplete, while remainingItemCount can establish a bounded total.
func parseResourceListPageHeader(result string) (resourceListWireMetadata, error) {
	var metadata resourceListWireMetadata
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		field, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(field) {
		case "continue":
			if value == "" {
				return resourceListWireMetadata{}, fmt.Errorf("resources_list continuation token is malformed")
			}
			metadata.continueToken = json.RawMessage(strconv.Quote(value))
		case "remainingItemCount":
			metadata.remainingItemCount = json.RawMessage(value)
		}
	}
	return metadata, nil
}

func validateResourceListEnvelope(apiVersion, kind, targetAPIVersion, targetKind string) error {
	listKind, isList := strings.CutSuffix(kind, "List")
	if !isList || !strings.EqualFold(listKind, targetKind) {
		return fmt.Errorf("resources_list response envelope kind does not match the requested GVK")
	}
	if apiVersion != "meta.k8s.io/v1" && apiVersion != targetAPIVersion {
		return fmt.Errorf("resources_list response envelope apiVersion does not match the requested GVK")
	}
	return nil
}

// projectResourcesListOutput removes resource bodies before a target-scoped
// list reaches alignment, the LLM, or audit persistence. kube-mcp-server's
// YAML mode returns a bare sequence, while the local resolver returns a
// metadata-list object; both are reduced to GVK plus bounded metadata.
func projectResourcesListOutput(result string, args json.RawMessage) (string, error) {
	target, err := parseResourceListTarget(args)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(result), "No resources found") {
		completeness, completenessErr := parseResourceListCompleteness(0, resourceListWireMetadata{})
		if completenessErr != nil {
			return "", completenessErr
		}
		return marshalProjectedResourceList(target.APIVersion, target.Kind+"List", []map[string]interface{}{}, completeness)
	}
	pageMetadata, pageMetadataErr := parseResourceListPageHeader(result)
	if pageMetadataErr != nil {
		return "", pageMetadataErr
	}
	jsonResult, err := sigsyaml.YAMLToJSON([]byte(result))
	if err != nil {
		return "", fmt.Errorf("resources_list response is not structured: %w", err)
	}
	trimmed := bytes.TrimSpace(jsonResult)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("resources_list response is empty")
	}

	switch trimmed[0] {
	case '[':
		return projectResourceListSequence(trimmed, target, pageMetadata)
	case '{':
		return projectResourceListEnvelope(trimmed, target, pageMetadata)
	default:
		return "", fmt.Errorf("resources_list response is neither a list nor an object")
	}
}

func parseResourceListTarget(args json.RawMessage) (katypes.RemediationTarget, error) {
	var target struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(args, &target); err != nil || target.APIVersion == "" || target.Kind == "" {
		return katypes.RemediationTarget{}, fmt.Errorf("resources_list target arguments are malformed")
	}
	return katypes.RemediationTarget{APIVersion: target.APIVersion, Kind: target.Kind}, nil
}

func projectResourceListSequence(trimmed []byte, target katypes.RemediationTarget, pageMetadata resourceListWireMetadata) (string, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil || items == nil {
		return "", fmt.Errorf("resources_list response is not a list")
	}
	safeItems, err := projectResourceListItems(items, target, true)
	if err != nil {
		return "", err
	}
	completeness, err := parseResourceListCompleteness(len(safeItems), pageMetadata)
	if err != nil {
		return "", err
	}
	return marshalProjectedResourceList(target.APIVersion, target.Kind+"List", safeItems, completeness)
}

func projectResourceListEnvelope(trimmed []byte, target katypes.RemediationTarget, pageMetadata resourceListWireMetadata) (string, error) {

	var envelope struct {
		APIVersion string          `json:"apiVersion"`
		Kind       string          `json:"kind"`
		Items      json.RawMessage `json:"items"`
		ItemCount  json.RawMessage `json:"item_count"`
		TotalCount json.RawMessage `json:"total_count"`
		Truncated  json.RawMessage `json:"truncated"`
		Continue   json.RawMessage `json:"continue"`
		Remaining  json.RawMessage `json:"remainingItemCount"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil || len(envelope.Items) == 0 {
		return "", fmt.Errorf("resources_list response envelope is malformed")
	}
	if err := validateResourceListEnvelope(envelope.APIVersion, envelope.Kind, target.APIVersion, target.Kind); err != nil {
		return "", err
	}
	pageMetadata = mergeResourceListWireMetadata(pageMetadata, resourceListWireMetadata{
		itemCount: envelope.ItemCount, totalCount: envelope.TotalCount,
		truncated: envelope.Truncated, continueToken: envelope.Continue,
		remainingItemCount: envelope.Remaining,
	})
	var items []json.RawMessage
	if err := json.Unmarshal(envelope.Items, &items); err != nil || items == nil {
		return "", fmt.Errorf("resources_list response items are malformed")
	}
	safeItems, err := projectResourceListItems(items, target, false)
	if err != nil {
		return "", err
	}
	completeness, err := parseResourceListCompleteness(len(safeItems), pageMetadata)
	if err != nil {
		return "", err
	}
	return marshalProjectedResourceList(envelope.APIVersion, envelope.Kind, safeItems, completeness)
}

func projectResourceListItems(items []json.RawMessage, target katypes.RemediationTarget, requireGVK bool) ([]map[string]interface{}, error) {
	safeItems := make([]map[string]interface{}, 0, len(items))
	for _, rawItem := range items {
		item, err := projectResourceListItem(rawItem, target.APIVersion, target.Kind, requireGVK)
		if err != nil {
			return nil, err
		}
		safeItems = append(safeItems, item)
	}
	return safeItems, nil
}

func mergeResourceListWireMetadata(base, override resourceListWireMetadata) resourceListWireMetadata {
	if len(override.itemCount) > 0 {
		base.itemCount = override.itemCount
	}
	if len(override.totalCount) > 0 {
		base.totalCount = override.totalCount
	}
	if len(override.truncated) > 0 {
		base.truncated = override.truncated
	}
	if len(override.continueToken) > 0 {
		base.continueToken = override.continueToken
	}
	if len(override.remainingItemCount) > 0 {
		base.remainingItemCount = override.remainingItemCount
	}
	return base
}

func projectResourceListItem(rawItem json.RawMessage, targetAPIVersion, targetKind string, requireGVK bool) (map[string]interface{}, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(rawItem, &item); err != nil || item == nil {
		return nil, fmt.Errorf("resources_list item is malformed")
	}
	safe, hasGVK, err := projectResourceItemGVK(item, targetAPIVersion, targetKind, requireGVK)
	if err != nil {
		return nil, err
	}

	if metadataRaw, ok := item["metadata"]; ok {
		safeMetadata, metadataErr := projectResourceMetadata(metadataRaw)
		if metadataErr != nil {
			return nil, metadataErr
		}
		safe["metadata"] = safeMetadata
	} else if !hasGVK {
		return nil, fmt.Errorf("resources_list item has no safe metadata")
	}
	return safe, nil
}

func projectResourceItemGVK(item map[string]json.RawMessage, targetAPIVersion, targetKind string, requireGVK bool) (map[string]interface{}, bool, error) {
	safe := make(map[string]interface{}, 3)
	apiVersionRaw, hasAPIVersion := item["apiVersion"]
	kindRaw, hasKind := item["kind"]
	if !hasAPIVersion && !hasKind {
		if requireGVK {
			return nil, false, fmt.Errorf("resources_list item is missing its GVK")
		}
		return safe, false, nil
	}

	var apiVersion, kind string
	if !hasAPIVersion || !hasKind || json.Unmarshal(apiVersionRaw, &apiVersion) != nil ||
		json.Unmarshal(kindRaw, &kind) != nil || apiVersion != targetAPIVersion ||
		!strings.EqualFold(kind, targetKind) {
		return nil, false, fmt.Errorf("resources_list item does not match the requested GVK")
	}
	safe["apiVersion"] = apiVersion
	safe["kind"] = kind
	return safe, true, nil
}

func marshalProjectedResourceList(apiVersion, kind string, items []map[string]interface{}, completeness resourceListCompleteness) (string, error) {
	projected := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"items":      items,
		"item_count": completeness.itemCount,
		"truncated":  completeness.truncated,
	}
	if completeness.totalCount != nil {
		projected["total_count"] = *completeness.totalCount
	}
	data, err := json.Marshal(projected)
	if err != nil {
		return "", fmt.Errorf("marshal projected resources_list response: %w", err)
	}
	return string(data), nil
}

func verifyAPIResourcesListEvidence(result string, target katypes.RemediationTarget) (bool, error) {
	if toolErr := discoveryToolError(result); toolErr != nil {
		return false, toolErr
	}

	var payload apiResourceDiscoveryPayload
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return false, fmt.Errorf("malformed api_resources_list response: %w", err)
	}
	if payload.Warning != "" {
		return false, fmt.Errorf("api_resources_list returned a partial discovery warning: %s", payload.Warning)
	}
	if !hasExactAPIResource(payload, target) {
		return false, fmt.Errorf("api_resources_list did not return the exact GVK %s/%s", target.APIVersion, target.Kind)
	}
	return true, nil
}

func verifyResourcesListEvidence(result string, target katypes.RemediationTarget, allowMetadataList bool) error {
	if toolErr := discoveryToolError(result); toolErr != nil {
		return toolErr
	}
	if !hasExactResourceListEvidenceForSource(result, target, allowMetadataList) {
		return fmt.Errorf("resources_list did not return structured evidence for the exact GVK %s/%s", target.APIVersion, target.Kind)
	}
	return nil
}

func (inv *Investigator) verifyExactTargetAPIVersion(ctx context.Context, target katypes.RemediationTarget, correlationID string) apiVersionVerification {
	if target.APIVersion == "" {
		return apiVersionVerification{reason: "api_version is required"}
	}
	if !isValidAPIVersion(target.APIVersion) {
		return apiVersionVerification{reason: fmt.Sprintf("api_version %q is malformed", target.APIVersion)}
	}

	args, _ := json.Marshal(map[string]string{"kind": target.Kind})
	apiResult, apiErr := inv.executeTargetScopedDiscoveryTool(ctx, "api_resources_list", args, correlationID, 0)
	if apiErr == nil {
		validated, verificationErr := verifyAPIResourcesListEvidence(apiResult, target)
		if validated {
			return apiVersionVerification{validated: true, source: "api_resources_list"}
		}
		apiErr = verificationErr
	}
	if apiErr != nil && isInvestigationCancellation(ctx, apiErr) {
		return apiVersionVerification{cancelled: true, reason: "exact GVK verification cancelled: " + apiErr.Error()}
	}
	if apiErr != nil && !strings.Contains(apiErr.Error(), "did not return the exact GVK") {
		return apiVersionVerification{reason: "exact GVK could not be independently verified: " + apiErr.Error()}
	}

	// api_resources_list intentionally exposes preferred versions only. A
	// successful exact resources_list call is independent positive evidence for
	// a served non-preferred version and does not require the named object to
	// exist. It is never used to infer absence from an empty list.
	listArgs, _ := json.Marshal(map[string]interface{}{
		"apiVersion": target.APIVersion,
		"kind":       target.Kind,
		"namespace":  target.Namespace,
		"limit":      100,
	})
	listResult, listErr := inv.executeTargetScopedDiscoveryTool(ctx, "resources_list", listArgs, correlationID, 1)
	if listErr == nil {
		_, remoteTarget := FleetOverlayFromContext(ctx)
		listErr = verifyResourcesListEvidence(listResult, target, !remoteTarget)
		if listErr == nil {
			return apiVersionVerification{validated: true, source: "resources_list"}
		}
	}
	if listErr != nil && isInvestigationCancellation(ctx, listErr) {
		return apiVersionVerification{cancelled: true, reason: "exact GVK verification cancelled: " + listErr.Error()}
	}

	reason := "exact GVK could not be independently verified"
	if apiErr != nil {
		reason += ": " + apiErr.Error()
	}
	if listErr != nil {
		reason += "; resources_list: " + listErr.Error()
	}
	return apiVersionVerification{reason: reason}
}

func markExactTargetAPIVersionValidated(ctx context.Context, result *katypes.InvestigationResult, source string) {
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	target := result.RemediationTarget
	target.APIVersionValidated = true
	result.RemediationTarget = target
	result.APIVersionValidation = &katypes.APIVersionValidation{
		ClusterID: clusterID, Kind: target.Kind, Name: target.Name,
		Namespace: target.Namespace, APIVersion: target.APIVersion, Source: source,
	}
}

func cloneAPIVersionValidation(validation *katypes.APIVersionValidation) *katypes.APIVersionValidation {
	if validation == nil {
		return nil
	}
	clone := *validation
	return &clone
}

// TargetAPIVersionValidationMatches verifies the complete server-owned
// provenance binding before a result crosses an investigation or interactive
// handoff boundary. clusterID is the authoritative target-cluster identity for
// that boundary; an empty value denotes the local hub investigation.
func TargetAPIVersionValidationMatches(result *katypes.InvestigationResult, clusterID string) bool {
	if result == nil || !result.RemediationTarget.APIVersionValidated || result.APIVersionValidation == nil {
		return false
	}
	validation := result.APIVersionValidation
	target := result.RemediationTarget
	return validation.ClusterID == clusterID &&
		strings.EqualFold(validation.Kind, target.Kind) &&
		validation.Name == target.Name &&
		validation.Namespace == target.Namespace &&
		validation.APIVersion == target.APIVersion &&
		validation.Source != ""
}

func completeAPIVersionValidation(result *katypes.InvestigationResult) bool {
	if result == nil || result.APIVersionValidation == nil {
		return false
	}
	return TargetAPIVersionValidationMatches(result, result.APIVersionValidation.ClusterID)
}

func clearTargetAPIVersionValidation(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.RemediationTarget.APIVersionValidated = false
	result.APIVersionValidation = nil
}

func sameCorrectionTarget(expected, actual katypes.RemediationTarget) bool {
	return strings.EqualFold(expected.Kind, actual.Kind) &&
		expected.Name == actual.Name && expected.Namespace == actual.Namespace
}

func (inv *Investigator) correctionToolDefinition(ctx context.Context, name string) (llm.ToolDefinition, bool) {
	if overlay, ok := FleetOverlayFromContext(ctx); ok {
		tool, found := resolveTool(overlay, name)
		if !found {
			return llm.ToolDefinition{}, false
		}
		return llm.ToolDefinition{Name: tool.Name(), Description: tool.Description(), Parameters: tool.Parameters()}, true
	}
	if inv.registry != nil {
		for _, tool := range inv.registry.ToolsForPhase(katypes.PhaseRCA, inv.phaseTools) {
			if tool.Name() == name {
				return llm.ToolDefinition{Name: tool.Name(), Description: tool.Description(), Parameters: tool.Parameters()}, true
			}
		}
	}
	return llm.ToolDefinition{}, false
}

func (inv *Investigator) targetScopedCorrectionTools(ctx context.Context) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, 3)
	for _, name := range []string{"api_resources_list", "resources_list"} {
		if definition, ok := inv.correctionToolDefinition(ctx, name); ok {
			defs = append(defs, definition)
		}
	}
	defs = append(defs, submitOnlyRCATools()...)
	return defs
}

// resolveTargetScopedCorrectionSubmission lets the model use only the
// target-cluster discovery tools during the bounded correction turn. Tool
// results are fed back to the model before it must resubmit the RCA. The
// harness still independently verifies the submitted GVK afterwards; model
// claims about discovery never establish validation.
type correctionToolCalls struct {
	submitContent string
	otherTools    []llm.ToolCall
	discovery     []llm.ToolCall
	unknown       []llm.ToolCall
}

func classifyCorrectionToolCalls(resp llm.ChatResponse, allowed map[string]struct{}) correctionToolCalls {
	classified := correctionToolCalls{}
	for _, toolCall := range resp.ToolCalls {
		if toolCall.Name == SubmitResultToolName && classified.submitContent == "" {
			classified.submitContent = toolCall.Arguments
			continue
		}
		classified.otherTools = append(classified.otherTools, toolCall)
		if toolCall.Name == SubmitResultToolName {
			classified.unknown = append(classified.unknown, toolCall)
			continue
		}
		if _, ok := allowed[toolCall.Name]; ok {
			classified.discovery = append(classified.discovery, toolCall)
		} else {
			classified.unknown = append(classified.unknown, toolCall)
		}
	}
	return classified
}

func correctionToolAllowlist(tools []llm.ToolDefinition) map[string]struct{} {
	allowed := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		allowed[tool.Name] = struct{}{}
	}
	return allowed
}

func (inv *Investigator) executeCorrectionDiscoveryCalls(
	ctx context.Context,
	llmCtx LLMInvocationContext,
	calls []llm.ToolCall,
	discoveryRounds int,
	gateEvent *audit.AuditEvent,
) ([]llm.Message, gateRetryOutcome, bool) {
	toolMessages := make([]llm.Message, 0, len(calls))
	if len(calls) > 0 && discoveryRounds >= maxAPIVersionCorrectionDiscoveryRounds {
		gateEvent.Data["discovery_tool_error"] = "correction discovery tool-call budget exhausted"
		return nil, gateRetryExhausted, false
	}
	if len(calls) > maxAPIVersionCorrectionDiscoveryCallsPerResponse {
		gateEvent.Data["discovery_tool_error"] = "correction discovery response exceeded the tool-call budget"
		return nil, gateRetryExhausted, false
	}
	inv.metricsFor(llmCtx.CorrelationID).AddToolCalls(len(calls))
	for _, toolCall := range calls {
		emitToSink(ctx, session.EventTypeToolCallStart, 0, string(katypes.PhaseRCA), map[string]interface{}{
			"tool_name": toolCall.Name,
		})
		toolResult := inv.executeToolCall(ctx, toolCall, llmCtx.CorrelationID, katypes.PhaseRCA)
		inv.emitToolCallAudit(context.WithoutCancel(ctx), toolCall, len(toolMessages), toolResult, string(katypes.PhaseRCA), llmCtx.CorrelationID)
		emitToSink(ctx, session.EventTypeToolResult, 0, string(katypes.PhaseRCA), map[string]interface{}{
			"tool_name":      toolCall.Name,
			"result_preview": truncatePreview(toolResult, 200),
		})
		if discoveryErr := discoveryToolError(toolResult); discoveryErr != nil {
			gateEvent.Data["discovery_tool_error"] = sanitization.SanitizeForLog(truncatePreview(discoveryErr.Error(), 500))
			return nil, gateRetryExhausted, false
		}
		calledTools, _ := gateEvent.Data["correction_discovery_tools_called"].([]string)
		gateEvent.Data["correction_discovery_tools_called"] = append(calledTools, toolCall.Name)
		toolMessages = append(toolMessages, llm.Message{
			Role:       "tool",
			Content:    toolResult,
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
		})
	}
	return toolMessages, "", true
}

func appendCorrectionUnknownToolGuidance(
	messages []llm.Message,
	unknown []llm.ToolCall,
	usedReminder bool,
	gateEvent *audit.AuditEvent,
) ([]llm.Message, bool, gateRetryOutcome, bool) {
	if len(unknown) == 0 {
		return messages, usedReminder, "", true
	}
	if usedReminder {
		gateEvent.Data["other_tools_called"] = toolCallNames(unknown)
		return messages, usedReminder, gateRetryOtherToolExhausted, false
	}
	for _, toolCall := range unknown {
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("Tool %q is not available for this correction step.", toolCall.Name),
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
		})
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: "You must use only api_resources_list or resources_list for target-cluster evidence, then call submit_result to resubmit your root cause analysis.",
	})
	return messages, true, "", true
}

func appendCorrectionEvidencePrompt(messages []llm.Message) []llm.Message {
	return append(messages, llm.Message{
		Role:    "user",
		Content: "Use the target-cluster discovery result above to choose the exact api_version, then call submit_result with the complete root cause analysis.",
	})
}

func (inv *Investigator) requestCorrectionFollowup(
	ctx context.Context,
	llmCtx LLMInvocationContext,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	gateEvent *audit.AuditEvent,
) (llm.ChatResponse, error) {
	current, err := llm.ChatWithParams(ctx, llmCtx.Client, llm.ChatRequest{
		Messages: messages,
		Tools:    tools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, llmCtx.RuntimeParams)
	if err != nil {
		gateEvent.Data["discovery_tool_error"] = err.Error()
		return llm.ChatResponse{}, err
	}
	if llmCtx.Tokens != nil {
		llmCtx.Tokens.Add(current.Usage)
	}
	inv.metricsFor(llmCtx.CorrelationID).IncLLMTurns()
	inv.recordTokenUsage(llmCtx.CorrelationID, current.Usage)
	return current, nil
}

func (inv *Investigator) resolveTargetScopedCorrectionSubmission(
	ctx context.Context,
	llmCtx LLMInvocationContext,
	retryMessages []llm.Message,
	resp llm.ChatResponse,
	tools []llm.ToolDefinition,
	gateEvent *audit.AuditEvent,
) (content string, usedReminder bool, outcome gateRetryOutcome, ok bool) {
	allowed := correctionToolAllowlist(tools)
	messages := append([]llm.Message(nil), retryMessages...)
	discoveryRounds := 0
	current := resp

	for {
		classified := classifyCorrectionToolCalls(current, allowed)
		if classified.submitContent != "" {
			if len(classified.otherTools) > 0 {
				gateEvent.Data["other_tools_called"] = toolCallNames(classified.otherTools)
				return "", usedReminder, gateRetryOtherToolExhausted, false
			}
			return classified.submitContent, usedReminder, "", true
		}
		if len(current.ToolCalls) == 0 {
			if current.Message.Content != "" {
				return current.Message.Content, usedReminder, "", true
			}
			return "", usedReminder, gateRetryEmptyResponse, false
		}

		assistantMessage := current.Message
		assistantMessage.ToolCalls = current.ToolCalls
		messages = append(messages, assistantMessage)
		toolMessages, toolOutcome, toolOK := inv.executeCorrectionDiscoveryCalls(ctx, llmCtx, classified.discovery, discoveryRounds, gateEvent)
		if !toolOK {
			return "", usedReminder, toolOutcome, false
		}
		messages = append(messages, toolMessages...)
		var guidanceOutcome gateRetryOutcome
		messages, usedReminder, guidanceOutcome, ok = appendCorrectionUnknownToolGuidance(messages, classified.unknown, usedReminder, gateEvent)
		if !ok {
			return "", usedReminder, guidanceOutcome, false
		}
		if len(classified.unknown) == 0 {
			discoveryRounds++
			messages = appendCorrectionEvidencePrompt(messages)
		}

		var err error
		current, err = inv.requestCorrectionFollowup(ctx, llmCtx, messages, tools, gateEvent)
		if err != nil {
			return "", usedReminder, gateRetryOtherToolExhausted, false
		}
	}
}

func toolCallNames(calls []llm.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return names
}

// apiVersionValidationGate requires the LLM to supply an exact apiVersion and
// independently verifies that GVK against the target-scoped discovery tools.
// The REST mapper is used only for correction-message hints; it never decides
// whether a model-supplied GVK is valid or fills a missing version.
func (inv *Investigator) apiVersionValidationGate(
	ctx context.Context,
	result *katypes.InvestigationResult,
	history []llm.Message,
	llmCtx LLMInvocationContext,
) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	if result == nil {
		return result
	}
	kind := result.RemediationTarget.Kind
	if kind == "" {
		return result
	}
	clearTargetAPIVersionValidation(result)
	auditCtx := context.WithoutCancel(ctx)
	verification := apiVersionVerification{}
	if result.RemediationTarget.APIVersion != "" {
		verification = inv.verifyExactTargetAPIVersion(ctx, result.RemediationTarget, correlationID)
		if verification.cancelled {
			gateEvent := newAPIVersionGateEvent(ctx, correlationID, modelName, "", result.RemediationTarget)
			recordAPIVersionVerification(gateEvent, result.RemediationTarget, verification, false)
			gateEvent.Data["retry_outcome"] = string(gateRetryCancelled)
			gateEvent.EventOutcome = audit.OutcomeFailure
			audit.StoreBestEffort(auditCtx, inv.auditStore, gateEvent, inv.auditLog())
			return markAPIVersionValidationCancelled(result, history, llmCtx.rcaBudget)
		}
		if verification.validated {
			markExactTargetAPIVersionValidated(ctx, result, verification.source)
			gateEvent := newAPIVersionGateEvent(ctx, correlationID, modelName, "", result.RemediationTarget)
			recordAPIVersionVerification(gateEvent, result.RemediationTarget, verification, true)
			audit.StoreBestEffort(auditCtx, inv.auditStore, gateEvent, inv.auditLog())
			return result
		}
		// A discovery transport/tool failure cannot be repaired by changing the
		// model answer. Escalate without allowing an unverified target through.
		if strings.Contains(verification.reason, "unavailable") || strings.Contains(verification.reason, "partial discovery warning") || strings.Contains(verification.reason, "malformed") {
			gateEvent := newAPIVersionGateEvent(ctx, correlationID, modelName, "", result.RemediationTarget)
			recordAPIVersionVerification(gateEvent, result.RemediationTarget, verification, false)
			exhausted := inv.apiVersionGateExhaustion(result, "target-scoped discovery", kind, correlationID, gateEvent, gateRetryExhausted)
			audit.StoreBestEffort(auditCtx, inv.auditStore, gateEvent, inv.auditLog())
			return exhausted
		}
	}

	groupList := ""
	if inv.scopeResolver != nil {
		_, gvrs, err := inv.scopeResolver.IsAmbiguousKind(kind)
		if err != nil {
			inv.logger.Info("apiVersionValidationGate: mapper hint unavailable; continuing with target-scoped discovery",
				"kind", kind, "error", err, "correlation_id", correlationID)
		} else {
			groupList = conflictingGroupList(gvrs)
		}
	}

	inv.logger.Info("apiVersionValidationGate triggered: ambiguous kind missing api_version",
		"kind", kind, "conflicting_groups", groupList, "correlation_id", correlationID)

	// #1777 (BR-AUDIT-005, FedRAMP AU-3): prompt_length/prompt_preview are
	// populated in retryForAPIVersion, once the actual retry prompt exists —
	// see the audit.StoreBestEffort call there for why.
	gateEvent := newAPIVersionGateEvent(ctx, correlationID, modelName, groupList, result.RemediationTarget)

	return inv.retryForAPIVersion(ctx, retryForAPIVersionParams{
		Result: result, History: history, Client: client, RuntimeParams: runtimeParams, Tokens: tokens,
		Kind: kind, GroupList: groupList, CorrelationID: correlationID, GateEvent: gateEvent, RCABudget: llmCtx.rcaBudget,
	})
}

func newAPIVersionGateEvent(ctx context.Context, correlationID, modelName, groupList string, target katypes.RemediationTarget) *audit.AuditEvent {
	event := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	event.EventAction = audit.ActionAPIVersionGate
	event.EventOutcome = audit.OutcomeSuccess
	event.Data["model"] = modelName
	event.Data["ambiguous_kind"] = target.Kind
	event.Data["conflicting_groups"] = groupList
	event.Data["target_kind"] = target.Kind
	event.Data["target_name"] = target.Name
	event.Data["target_namespace"] = target.Namespace
	event.Data["target_api_version"] = target.APIVersion
	clusterID, _ := audit.ClusterIDFromContext(ctx)
	event.Data["target_cluster_id"] = clusterID
	return event
}

func recordAPIVersionVerification(event *audit.AuditEvent, target katypes.RemediationTarget, verification apiVersionVerification, validated bool) {
	if event == nil {
		return
	}
	event.Data["target_kind"] = target.Kind
	event.Data["target_name"] = target.Name
	event.Data["target_namespace"] = target.Namespace
	event.Data["target_api_version"] = target.APIVersion
	event.Data["api_version_validation_status"] = map[bool]string{true: "validated", false: "inconclusive"}[validated]
	if verification.source != "" {
		event.Data["api_version_validation_source"] = verification.source
	}
	if verification.reason != "" {
		event.Data["api_version_validation_reason"] = sanitization.SanitizeForLog(truncatePreview(verification.reason, 500))
	}
	if !validated {
		event.EventOutcome = audit.OutcomeFailure
	}
}

func markAPIVersionValidationCancelled(result *katypes.InvestigationResult, history []llm.Message, budget *rcaLoopBudget) *katypes.InvestigationResult {
	result = markRCAContinuationCancelled(result, history, budget)
	clearTargetAPIVersionValidation(result)
	clearWorkflowSelectionFields(result)
	return result
}

func markAPIVersionRetryCancelled(result *katypes.InvestigationResult, history []llm.Message, budget *rcaLoopBudget, gateEvent *audit.AuditEvent) *katypes.InvestigationResult {
	gateEvent.Data["retry_outcome"] = string(gateRetryCancelled)
	gateEvent.EventOutcome = audit.OutcomeFailure
	return markAPIVersionValidationCancelled(result, history, budget)
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
	RCABudget                      *rcaLoopBudget
}

type apiVersionCorrectionAttempt struct {
	llmCtx   LLMInvocationContext
	messages []llm.Message
	tools    []llm.ToolDefinition
	response llm.ChatResponse
}

func (inv *Investigator) runAPIVersionCorrectionAttempt(ctx context.Context, p retryForAPIVersionParams) (apiVersionCorrectionAttempt, error) {
	correctionMsg := fmt.Sprintf(
		`Your remediation_target.kind is %q, which exists in multiple API groups: %s. `+
			`Without an explicit api_version, the system cannot determine the correct API group `+
			`and may grant RBAC permissions to the wrong resource. `+
			`You MUST re-submit your result with the api_version field set to the correct `+
			`"group/version" (e.g. "operators.coreos.com/v1alpha1"). `+
			`Review your investigation context (kubectl_describe output) to determine which `+
			`API group the target %s/%s belongs to.`,
		p.Kind, p.GroupList, p.Kind, p.Result.RemediationTarget.Name,
	)
	retryMessages := appendCorrectionMessage(p.History, correctionMsg)
	correctionTools := inv.targetScopedCorrectionTools(ctx)
	p.GateEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
	p.GateEvent.Data["prompt_preview"] = lastUserMessage(retryMessages)

	llmCtx := LLMInvocationContext{
		Tokens: p.Tokens, CorrelationID: p.CorrelationID, Client: p.Client,
		RuntimeParams: p.RuntimeParams, rcaBudget: p.RCABudget,
	}
	emitGateRetryKeepalive(ctx, "resolving_api_version_ambiguity")
	resp, err := llm.ChatWithParams(ctx, llmCtx.Client, llm.ChatRequest{
		Messages: retryMessages,
		Tools:    correctionTools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, llmCtx.RuntimeParams)
	if err != nil {
		return apiVersionCorrectionAttempt{}, err
	}
	if llmCtx.Tokens != nil {
		llmCtx.Tokens.Add(resp.Usage)
	}
	// #2387: gate retry is its own LLM turn on top of the loop-counted turns.
	inv.metricsFor(p.CorrelationID).IncLLMTurns()
	inv.recordTokenUsage(p.CorrelationID, resp.Usage)
	return apiVersionCorrectionAttempt{
		llmCtx: llmCtx, messages: retryMessages, tools: correctionTools, response: resp,
	}, nil
}

// retryForAPIVersion re-submits the RCA request with a correction message
// asking the LLM to disambiguate the API group for kind, and parses the
// retry response. Falls back to apiVersionGateExhaustion (forcing human
// review) at every failure point: LLM error, empty response, parse error, or
// a retry result that still lacks api_version.
func (inv *Investigator) retryForAPIVersion(ctx context.Context, p retryForAPIVersionParams) *katypes.InvestigationResult {
	result := p.Result
	kind, groupList, correlationID, gateEvent := p.Kind, p.GroupList, p.CorrelationID, p.GateEvent
	priorReviewNeeded, priorReviewReason := result.HumanReviewNeeded, result.HumanReviewReason
	// #2121 (BR-AI-2121, FedRAMP AU-3): fire once, after the final outcome is
	// known, via defer -- this also fixes a pre-existing bug (#2124) where
	// gateEvent.Data["retry_outcome"] was mutated below AFTER this store
	// call fired, so a production audit store (which serializes Data into
	// an outbound request synchronously at call time) never actually
	// persisted retry_outcome.
	defer audit.StoreBestEffort(context.WithoutCancel(ctx), inv.auditStore, gateEvent, inv.auditLog())

	attempt, retryErr := inv.runAPIVersionCorrectionAttempt(ctx, p)
	if retryErr != nil {
		if isInvestigationCancellation(ctx, retryErr) {
			return markAPIVersionRetryCancelled(result, p.History, p.RCABudget, gateEvent)
		}
		inv.logger.Error(retryErr, "apiVersionValidationGate: retry failed, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryExhausted)
	}
	retryContent, usedReminder, failOutcome, ok := inv.resolveTargetScopedCorrectionSubmission(
		ctx, attempt.llmCtx, attempt.messages, attempt.response, attempt.tools, gateEvent)
	if !ok {
		if ctx.Err() != nil {
			return markAPIVersionRetryCancelled(result, p.History, p.RCABudget, gateEvent)
		}
		inv.logger.Info("apiVersionValidationGate: triggering human review after unresolved retry",
			"kind", kind, "retry_outcome", failOutcome, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, failOutcome)
	}

	retryResult, parseErr := inv.parseRCA(retryContent)
	if parseErr != nil {
		if ctx.Err() != nil {
			return markAPIVersionRetryCancelled(result, p.History, p.RCABudget, gateEvent)
		}
		inv.logger.Error(parseErr, "apiVersionValidationGate: retry parse failed, triggering human review",
			"correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryParseError)
	}

	if !sameCorrectionTarget(result.RemediationTarget, retryResult.RemediationTarget) {
		inv.logger.Info("apiVersionValidationGate: retry changed the remediation target, triggering human review",
			"kind", kind, "correlation_id", correlationID)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryExhausted)
	}
	verification := inv.verifyExactTargetAPIVersion(ctx, retryResult.RemediationTarget, correlationID)
	if verification.cancelled {
		recordAPIVersionVerification(gateEvent, retryResult.RemediationTarget, verification, false)
		gateEvent.Data["retry_outcome"] = string(gateRetryCancelled)
		gateEvent.EventOutcome = audit.OutcomeFailure
		return markAPIVersionValidationCancelled(result, p.History, p.RCABudget)
	}
	if !verification.validated {
		inv.logger.Info("apiVersionValidationGate: retry GVK could not be independently verified, triggering human review",
			"kind", retryResult.RemediationTarget.Kind,
			"api_version", retryResult.RemediationTarget.APIVersion,
			"reason", verification.reason,
			"correlation_id", correlationID)
		recordAPIVersionVerification(gateEvent, retryResult.RemediationTarget, verification, false)
		return inv.apiVersionGateExhaustion(result, groupList, kind, correlationID, gateEvent, gateRetryExhausted)
	}
	markExactTargetAPIVersionValidated(ctx, retryResult, verification.source)
	recordAPIVersionVerification(gateEvent, retryResult.RemediationTarget, verification, true)
	preserveGateConfidence(result, retryResult)

	gateEvent.EventOutcome = audit.OutcomeSuccess
	if usedReminder {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolvedAfterReminder)
	} else {
		gateEvent.Data["retry_outcome"] = string(gateRetryResolved)
	}
	// The gate must not erase an unrelated review decision carried by the
	// accepted result. Normal RCA parsing clears its own provisional review
	// marker before entering this gate; a non-empty reason here is therefore
	// preserved across a successful API-version correction.
	if priorReviewNeeded {
		retryResult.HumanReviewNeeded = true
		retryResult.HumanReviewReason = priorReviewReason
	} else {
		retryResult.HumanReviewNeeded = false
		retryResult.HumanReviewReason = ""
	}
	// The retry is a fresh submission on its own turn: its own Reasoning
	// block (if any) is authoritative for the accepted result, mirroring
	// runRCA's "winning turn" semantics (BR-AI-086 AC6, #1578). Without this,
	// resp.Message.Reasoning from the retry call is silently discarded and
	// the gate-accepted result loses reasoning captured by the LLM client.
	retryResult.Reasoning = toReasoningSummary(attempt.response.Message.Reasoning)
	inv.logger.Info("apiVersionValidationGate: retry provided api_version, accepted",
		"kind", kind,
		"api_version", retryResult.RemediationTarget.APIVersion,
		"correlation_id", correlationID)
	return retryResult
}

// preserveGateConfidence retains the earlier RCA score only when the retry omits it.
func preserveGateConfidence(previous, retry *katypes.InvestigationResult) {
	if !retry.ConfidenceSet && previous.ConfidenceSet {
		retry.Confidence = previous.Confidence
		retry.ConfidenceSet = true
		retry.RCAConfidence = previous.RCAConfidence
	}
}

func (inv *Investigator) apiVersionGateExhaustion(
	result *katypes.InvestigationResult,
	groupList, kind, correlationID string,
	gateEvent *audit.AuditEvent,
	outcome gateRetryOutcome,
) *katypes.InvestigationResult {
	if gateEvent != nil {
		gateEvent.Data["retry_outcome"] = string(outcome)
		gateEvent.EventOutcome = audit.OutcomeFailure
	}
	if !result.HumanReviewNeeded {
		result.HumanReviewNeeded = true
	}
	if result.HumanReviewReason == "" {
		result.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete
	}

	// Clear workflow fields: the workflow was selected based on an ambiguous
	// kind without api_version, so it may target the wrong API group and lead
	// to incorrect RBAC grants. Leaving it populated would cause the API
	// response to include selected_workflow despite human review escalation.
	clearWorkflowSelectionFields(result)
	result.Confidence = 0

	result.Warnings = append(result.Warnings,
		fmt.Sprintf("apiVersionValidationGate: kind %q is ambiguous (API groups: %s) "+
			"but LLM did not provide api_version after retry — human review required to prevent "+
			"incorrect RBAC grants", kind, groupList))
	inv.logger.Info("apiVersionValidationGate: exhausted, human review required",
		"kind", kind, "conflicting_groups", groupList, "retry_outcome", outcome, "correlation_id", correlationID)
	return result
}

func clearWorkflowSelectionFields(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowID = ""
	result.ProposedWorkflowID = ""
	result.Confidence = 0
	result.Parameters = nil
	result.ExecutionEngine = ""
	result.ExecutionBundle = ""
	result.ExecutionBundleDigest = ""
	result.ServiceAccountName = ""
	result.EngineConfig = nil
	result.WorkflowRationale = ""
	result.WorkflowVersion = ""
	result.WorkflowSelectionConfidence = nil
	result.ActionType = ""
	result.WorkflowName = ""
	result.ExecutionClusterID = ""
	result.Dependencies = nil
	result.Resources = nil
	result.DeclaredParameterNames = nil
	result.ParameterSchema = nil
	result.AlternativeWorkflows = nil
	result.HandoffDigest = ""
	result.RepeatJustification = nil
	result.RepeatJustificationRequired = nil
	result.PriorWorkflowMatch = nil
	result.RepeatValidationOutcome = ""
	result.PriorRemediationUIDs = nil
}

func enrichFromCatalog(result *katypes.InvestigationResult, v *parser.Validator) {
	if result == nil || v == nil || result.WorkflowID == "" {
		return
	}
	if !v.IsAllowed(result.WorkflowID) {
		// DD-KA-001 Step 1 (Workflow Existence) is unconditional: a
		// workflow_id that does not resolve against the DS catalog allowlist
		// must never survive as structured data, even when HumanReviewNeeded
		// was already set to true by an earlier signal (e.g.
		// investigation_outcome=inconclusive), which caused
		// Validator.Validate() to short-circuit its own allowlist check
		// before ever reaching this function. Clearing WorkflowID here is
		// sufficient: mapInvestigationResultToResponse only emits
		// selected_workflow at all when WorkflowID != "" (Issue #1711).
		result.WorkflowID = ""
		return
	}

	// meta may be legitimately absent even for an allowed ID: this happens
	// only in test fixtures that populate the allowlist without a matching
	// SetWorkflowMeta call. Production always sets both from the same
	// catalog-fetch loop (dsCatalogFetcher.FetchValidator), so an allowed ID
	// there always has metadata. Treat this as best-effort enrichment rather
	// than a second existence check (Issue #1711 cascade fix-forward).
	meta, ok := v.GetWorkflowMeta(result.WorkflowID)
	if !ok {
		return
	}
	// The workflow catalog owns backend selection; never preserve a conflicting
	// model-supplied engine (BR-WE-014, BR-TESTING-001).
	result.ExecutionEngine = meta.ExecutionEngine
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
	if meta.DeclaredParameterNames != nil {
		result.ParameterSchema = append([]models.WorkflowParameter{}, meta.Parameters...)
	} else {
		result.ParameterSchema = nil
	}
	result.EngineConfig = meta.EngineConfig

	// Issue #1661 Change 12: ActionType/WorkflowName are likewise
	// catalog-authoritative -- always overwrite, same pattern as
	// Dependencies/Resources/DeclaredParameterNames above. This closes the
	// gap where KA never populated either field, breaking
	// workflowexecution.execution.started's audit payload (Change 11f).
	result.ActionType = meta.ActionType
	result.WorkflowName = meta.WorkflowName

	// Issue #2326 (DD-FLEET-008, BR-FLEET-004): ExecutionClusterID is
	// likewise catalog-authoritative -- always overwrite, same pattern as
	// ActionType/WorkflowName above.
	result.ExecutionClusterID = meta.ClusterID
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
