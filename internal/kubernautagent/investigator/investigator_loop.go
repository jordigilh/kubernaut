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
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext) (LoopResult, error) {
	decisionLoopID, err := decisionLoopIDForInvocation(phase, llmCtx.decisionLoopID, llmCtx.ModelName, messages)
	if err != nil {
		return nil, fmt.Errorf("derive decision loop identity: %w", err)
	}
	llmCtx.decisionLoopID = decisionLoopID

	state := &loopTurnState{loopStart: time.Now()}
	budget := llmCtx.rcaBudget
	if budget == nil {
		budget = &rcaLoopBudget{}
	}

	for budget.used < inv.maxTurns {
		turn := budget.used
		budget.used++
		result, newMessages, done, err := inv.runLoopTurn(ctx, state, messages, phase, llmCtx, turn)
		messages = newMessages
		if err != nil {
			return nil, err
		}
		if done {
			return result, nil
		}
	}

	exhausted := &ExhaustedResult{Reason: "max turns exhausted"}
	result := loopDecisionSnapshot(exhausted, phase)
	inv.emitInvestigationDecision(ctx, investigationDecisionParams{
		CorrelationID:      llmCtx.CorrelationID,
		Phase:              phase,
		TurnKind:           investigationDecisionTurnTerminal,
		TurnID:             decisionLoopPrefix(phase, llmCtx.decisionLoopID) + ":exhausted",
		Model:              llmCtx.ModelName,
		Tokens:             llmCtx.Tokens,
		Result:             result,
		FailureClass:       "turn_budget_exhausted",
		ContinuationReason: "turn_budget_exhausted",
	})
	return exhausted, nil
}

// loopTurnState carries the mutable state that persists across turns of
// runLLMLoop: the loop start time (for duration audit fields) and the
// truncation-retry-once tracking (retried flag + escalated MaxTokens).
type loopTurnState struct {
	loopStart         time.Time
	truncationRetried bool
	maxTokens         int
}

type loopDecisionEmission struct {
	Phase              katypes.Phase
	LLMContext         LLMInvocationContext
	Turn               int
	TurnKind           string
	TurnIDSuffix       string
	Result             LoopResult
	FailureClass       string
	ContinuationReason string
	RetryAttempt       int
}

func (inv *Investigator) emitLoopDecision(ctx context.Context, p loopDecisionEmission) {
	inv.emitInvestigationDecision(ctx, investigationDecisionParams{
		CorrelationID:      p.LLMContext.CorrelationID,
		Phase:              p.Phase,
		TurnKind:           p.TurnKind,
		TurnID:             fmt.Sprintf("%s:turn:%d%s", decisionLoopPrefix(p.Phase, p.LLMContext.decisionLoopID), p.Turn, p.TurnIDSuffix),
		Model:              p.LLMContext.ModelName,
		Tokens:             p.LLMContext.Tokens,
		Result:             loopDecisionSnapshot(p.Result, p.Phase),
		FailureClass:       p.FailureClass,
		ContinuationReason: p.ContinuationReason,
		RetryAttempt:       p.RetryAttempt,
	})
}

func decisionLoopPrefix(phase katypes.Phase, loopID string) string {
	if loopID == "" {
		return string(phase)
	}
	return string(phase) + ":" + loopID
}

// decisionLoopIDForInvocation gives each independent LLM invocation its own
// deterministic namespace while preserving retry/reconciliation deduplication
// for the same phase, model, loop kind, and starting conversation. The
// conversation is hashed rather than copied into the audit identity so prompt
// content cannot leak through the event ID seed (DD-KA-2520-2521).
func decisionLoopIDForInvocation(phase katypes.Phase, loopID, modelName string, messages []llm.Message) (string, error) {
	if loopID == "" {
		loopID = string(phase)
	}
	identity := struct {
		Phase    katypes.Phase
		LoopID   string
		Model    string
		Messages []llm.Message
	}{
		Phase:    phase,
		LoopID:   loopID,
		Model:    modelName,
		Messages: messages,
	}
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("marshal decision loop identity: %w", err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%s:%s", loopID, hex.EncodeToString(digest[:8])), nil
}

type loopResponseParams struct {
	State         *loopTurnState
	Messages      []llm.Message
	Phase         katypes.Phase
	LLMContext    LLMInvocationContext
	Turn          int
	Response      llm.ChatResponse
	CorrelationID string
}

func (inv *Investigator) processLoopResponse(ctx context.Context, p loopResponseParams) (LoopResult, []llm.Message, bool, error) {
	if len(p.Response.ToolCalls) > 0 {
		return inv.processToolTurn(ctx, p)
	}
	if p.Response.FinishReason == llm.FinishReasonLength {
		return inv.processTruncatedTurn(ctx, p)
	}
	inv.emitLoopDecision(ctx, loopDecisionEmission{
		Phase:              p.Phase,
		LLMContext:         p.LLMContext,
		Turn:               p.Turn,
		TurnKind:           investigationDecisionTurnTerminal,
		TurnIDSuffix:       ":text",
		ContinuationReason: "text_response",
	})
	return &TextResult{Content: p.Response.Message.Content, Reasoning: p.Response.Message.Reasoning, Messages: p.Messages}, p.Messages, true, nil
}

func (inv *Investigator) processToolTurn(ctx context.Context, p loopResponseParams) (LoopResult, []llm.Message, bool, error) {
	toolMessages, sentinel, budgetExhausted := inv.processToolCalls(ctx, p.Messages, p.Response, p.Turn, string(p.Phase), p.CorrelationID)
	if sentinel != nil {
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              p.Phase,
			LLMContext:         p.LLMContext,
			Turn:               p.Turn,
			TurnKind:           investigationDecisionTurnTerminal,
			TurnIDSuffix:       ":sentinel",
			Result:             sentinel,
			ContinuationReason: "sentinel_submit",
		})
		return sentinel, toolMessages, true, nil
	}
	if budgetExhausted {
		exhausted := &ExhaustedResult{Reason: ReasonToolBudgetExhausted}
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              p.Phase,
			LLMContext:         p.LLMContext,
			Turn:               p.Turn,
			TurnKind:           investigationDecisionTurnTerminal,
			TurnIDSuffix:       ":tool_budget",
			Result:             exhausted,
			FailureClass:       ReasonToolBudgetExhausted,
			ContinuationReason: "tool_budget_exhausted",
		})
		return exhausted, toolMessages, true, nil
	}
	inv.emitLoopDecision(ctx, loopDecisionEmission{
		Phase:              p.Phase,
		LLMContext:         p.LLMContext,
		Turn:               p.Turn,
		TurnKind:           investigationDecisionTurnTool,
		ContinuationReason: "tool_turn_complete",
	})
	return nil, toolMessages, false, nil
}

func (inv *Investigator) processTruncatedTurn(ctx context.Context, p loopResponseParams) (LoopResult, []llm.Message, bool, error) {
	if !p.State.truncationRetried {
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              p.Phase,
			LLMContext:         p.LLMContext,
			Turn:               p.Turn,
			TurnKind:           investigationDecisionTurnTool,
			TurnIDSuffix:       ":truncation",
			ContinuationReason: "response_truncated_retry",
			RetryAttempt:       1,
		})
		return nil, inv.handleTruncation(ctx, p.State, p.Messages, p.Response, p.Phase, p.CorrelationID), false, nil
	}

	// #1614: the escalated retry is also truncated. Returning it as text would
	// silently ship a partial answer downstream, so classify it as exhausted.
	exhausted := &ExhaustedResult{Reason: "output truncated after retry"}
	inv.emitLoopDecision(ctx, loopDecisionEmission{
		Phase:              p.Phase,
		LLMContext:         p.LLMContext,
		Turn:               p.Turn,
		TurnKind:           investigationDecisionTurnTerminal,
		TurnIDSuffix:       ":truncated",
		Result:             exhausted,
		FailureClass:       "output_truncated",
		ContinuationReason: "output_truncated_after_retry",
		RetryAttempt:       1,
	})
	return exhausted, p.Messages, true, nil
}

// runLoopTurn executes a single turn of the investigation LLM loop: the
// cancellation check, LLM call, response audit/sink emission, and outcome
// classification (tool calls / truncation retry / final text). done=true
// tells the caller to return result immediately (err is set only for
// non-cancellation failures); done=false means the caller should continue
// to the next turn with newMessages.
func (inv *Investigator) runLoopTurn(ctx context.Context, state *loopTurnState, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext, turn int) (result LoopResult, newMessages []llm.Message, done bool, err error) {
	tokens, correlationID := llmCtx.Tokens, llmCtx.CorrelationID
	toolDefs := inv.toolDefinitionsForPhase(ctx, phase)

	if ctx.Err() != nil {
		emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              phase,
			LLMContext:         llmCtx,
			Turn:               turn,
			TurnKind:           investigationDecisionTurnTerminal,
			TurnIDSuffix:       ":cancelled",
			Result:             &CancelledResult{Turn: turn, Phase: string(phase)},
			ContinuationReason: "context_cancelled",
		})
		// nolint:nilerr // intentional: cancellation is a documented normal
		// outcome for this function (see doc comment above), not a failure
		// -- ctx.Err() is converted into a typed cancelled result, not
		// propagated as err (Issue #1546 Tier 3).
		return buildCancelledResult(messages, turn, string(phase), tokens), messages, true, nil
	}
	if phase == katypes.PhaseWorkflowDiscovery {
		if budgetErr := validateWorkflowSelectionContextBudget(messages, inv.structuredContext.budget); budgetErr != nil {
			inv.logger.Error(budgetErr, "workflow selection context exceeded structured Phase 3 budget",
				"correlation_id", llmCtx.CorrelationID, "turn", turn)
			return &ExhaustedResult{Reason: structuredPhase3BudgetExhaustionReason(messages)}, messages, true, nil
		}
	}

	inv.emitLLMRequestAudit(ctx, correlationID, llmCtx.ModelName, phase, messages, toolDefs, llmCtx.WorkflowDiscovery)

	resp, cancelled, callErr := inv.doLLMCall(ctx, state, messages, phase, llmCtx, turn, toolDefs)
	if callErr != nil {
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              phase,
			LLMContext:         llmCtx,
			Turn:               turn,
			TurnKind:           investigationDecisionTurnTerminal,
			TurnIDSuffix:       ":failure",
			Result:             &ExhaustedResult{Reason: "llm_call_failed"},
			FailureClass:       "llm_call_failed",
			ContinuationReason: "llm_call_failed",
		})
		return nil, messages, true, callErr
	}
	if cancelled != nil {
		inv.emitLoopDecision(ctx, loopDecisionEmission{
			Phase:              phase,
			LLMContext:         llmCtx,
			Turn:               turn,
			TurnKind:           investigationDecisionTurnTerminal,
			TurnIDSuffix:       ":cancelled",
			Result:             cancelled,
			ContinuationReason: "context_cancelled",
		})
		return cancelled, messages, true, nil
	}

	// #2387 (BR-KA-OBSERVABILITY-001): one successful provider round-trip is
	// one LLM turn — tool-call, sentinel-submit, truncation, and plain-text
	// turns all count. Failures and cancellations above do not. Token usage
	// joins the cumulative per-RR scope alongside the audit accumulator.
	inv.metricsFor(correlationID).IncLLMTurns()
	inv.recordTokenUsage(correlationID, resp.Usage)

	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	inv.emitLLMResponseAudit(ctx, correlationID, resp)

	// #1634: field name must be "text" — this is what AF's FormatEventForUser
	// (pkg/apifrontend/tools/ka_investigate_bridge.go) reads. A prior
	// "content_preview" key here meant AF silently dropped every one of
	// these events (extractJSONField returned "", emitEventToA2A no-opped).
	emitToSink(ctx, session.EventTypeReasoningDelta, turn, string(phase), map[string]interface{}{
		"text":            truncatePreview(resp.Message.Content, 200),
		"tool_call_count": len(resp.ToolCalls),
	})
	// #1635 / BR-AI-086 AC10: live-stream captured reasoning content via a
	// dedicated event type, distinct from the orchestration-narration event
	// above (DD-LLM-009). #2006: pass the untruncated narration text (not
	// the 200-char preview above) so the dedup guard compares the full text.
	emitReasoningContentEvent(ctx, resp.Message.Reasoning, resp.Message.Content, turn, string(phase))

	return inv.processLoopResponse(ctx, loopResponseParams{
		State:         state,
		Messages:      messages,
		Phase:         phase,
		LLMContext:    llmCtx,
		Turn:          turn,
		Response:      resp,
		CorrelationID: correlationID,
	})
}

// loopDecisionSnapshot projects a loop-level terminal state into the bounded
// typed contract without retaining the loop's raw reason or response content.
func loopDecisionSnapshot(result LoopResult, phase katypes.Phase) *katypes.InvestigationResult {
	snapshot := &katypes.InvestigationResult{}
	switch result.(type) {
	case *CancelledResult:
		snapshot.Cancelled = true
		snapshot.CancelledPhase = string(phase)
		snapshot.TerminalOutcome = sharedtypes.TerminalOutcomeCancelled
	case *SubmitNoWorkflowResult:
		markWorkflowDiscoveryNoMatch(snapshot)
	case *ExhaustedResult:
		if phase == katypes.PhaseWorkflowDiscovery {
			markWorkflowDiscoveryFailed(snapshot)
		} else {
			markRCAIncomplete(snapshot)
		}
	}
	if phase == katypes.PhaseWorkflowDiscovery && snapshot.WorkflowDiscoveryStatus == "" {
		snapshot.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusStarted
	}
	return snapshot
}

// doLLMCall builds the per-turn chat request (using state.maxTokens for any
// escalated truncation retry) and dispatches to callLLMTurn.
func (inv *Investigator) doLLMCall(ctx context.Context, state *loopTurnState, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext, turn int, toolDefs []llm.ToolDefinition) (llm.ChatResponse, LoopResult, error) {
	chatReq := llm.ChatRequest{
		Messages: messages,
		Tools:    toolDefs,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: submitResultSchemaForPhase(phase), MaxTokens: state.maxTokens},
	}
	return inv.callLLMTurn(ctx, llmTurnCallParams{
		client:        llmCtx.Client,
		chatReq:       chatReq,
		messages:      messages,
		turn:          turn,
		phase:         string(phase),
		correlationID: llmCtx.CorrelationID,
		runtimeParams: llmCtx.RuntimeParams,
		tokens:        llmCtx.Tokens,
		loopStart:     state.loopStart,
	})
}

// handleTruncation marks the turn as retried, escalates state.maxTokens, and
// returns the message history with the truncation-retry messages appended
// for runLoopTurn to continue with on the next turn.
func (inv *Investigator) handleTruncation(ctx context.Context, state *loopTurnState, messages []llm.Message, resp llm.ChatResponse, phase katypes.Phase, correlationID string) []llm.Message {
	state.truncationRetried = true
	state.maxTokens = escalateMaxTokens(resp.Usage.CompletionTokens)
	inv.logger.Info("LLM response truncated, retrying with escalated MaxTokens",
		"phase", string(phase),
		"escalated_max_tokens", state.maxTokens,
		"correlation_id", correlationID)

	return append(messages, inv.buildTruncationRetryMessages(ctx, resp, correlationID, state.maxTokens)...)
}

// buildCancelledResult constructs the CancelledResult snapshot returned from
// every context-cancellation exit point in runLLMLoop, deduplicating the two
// identical struct literals that previously existed (pre-timeout check and
// post-LLM-call-error check).
func buildCancelledResult(messages []llm.Message, turn int, phase string, tokens *TokenAccumulator) *CancelledResult {
	return &CancelledResult{
		Messages: messages,
		Turn:     turn,
		Phase:    phase,
		Tokens:   tokens,
	}
}

// llmTurnCallParams groups the per-turn values needed by callLLMTurn. Kept as
// a config struct (rather than individual parameters) per the Go
// Anti-Pattern Checklist's 8+-parameter rule.
type llmTurnCallParams struct {
	client        llm.Client
	chatReq       llm.ChatRequest
	messages      []llm.Message
	turn          int
	phase         string
	correlationID string
	runtimeParams llm.RuntimeParams
	tokens        *TokenAccumulator
	loopStart     time.Time
}

// callLLMTurn invokes the LLM for one loop turn and classifies the outcome:
// a context-cancellation error becomes a CancelledResult snapshot (cancelled
// non-nil, err nil); any other error is recorded via a ResponseFailed audit
// event and returned wrapped (err non-nil); success returns the response
// with both cancelled and err nil.
func (inv *Investigator) callLLMTurn(ctx context.Context, p llmTurnCallParams) (llm.ChatResponse, LoopResult, error) {
	resp, err := inv.chatOrStream(ctx, p.client, p.chatReq, p.turn, p.phase, p.runtimeParams)
	if err == nil {
		return resp, nil, nil
	}
	// A DeadlineExceeded only means cancellation when the OUTER (parent)
	// context is the one that's done — chatOrStream/ChatWithParams create
	// their own per-attempt child context via context.WithTimeout, so that
	// child expiring on its own (backend too slow, ctx.Err() on the parent
	// still nil) is a backend failure, not an operator/system cancellation
	// (#1612). A literal context.Canceled is always treated as
	// cancellation, matching mocks that call the real cancelFn and return
	// ctx.Err() verbatim.
	if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
		emitToSink(ctx, session.EventTypeCancelled, p.turn, p.phase, nil)
		// nolint:nilerr // intentional: cancellation (see #1612 rationale
		// above) is converted into a typed cancelled result, not
		// propagated as err (Issue #1546 Tier 3).
		return llm.ChatResponse{}, buildCancelledResult(p.messages, p.turn, p.phase, p.tokens), nil
	}
	failEvent := audit.NewEvent(audit.EventTypeResponseFailed, p.correlationID)
	failEvent.EventAction = audit.ActionResponseFailed
	failEvent.EventOutcome = audit.OutcomeFailure
	failEvent.Data["error_message"] = err.Error()
	errorCode := "ERR_UPSTREAM_FAILURE"
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
		errorCode = "ERR_UPSTREAM_TIMEOUT"
	}
	// BR-AUDIT-005 Gap #7: retain machine-readable failure classification and
	// retry guidance for SOC2 reconstruction of failed LLM turns.
	failEvent.Data["error_details"] = sharedaudit.NewErrorDetails(
		"kubernautagent", errorCode, err.Error(), llm.IsRetryable(err),
	)
	failEvent.Data["phase"] = p.phase
	failEvent.Data["duration_seconds"] = time.Since(p.loopStart).Seconds()
	audit.StoreBestEffort(ctx, inv.auditStore, failEvent, inv.auditLog())
	// session.EventTypeError already has a live consumer (API Frontend's SSE
	// bridge, ka_investigate_bridge.go) that extracts Data["error"] and
	// surfaces it to the operator — it just had no producer for a generic
	// (non-cancellation) LLM turn failure until now. This gives the observer
	// a distinguishable "backend unavailable" signal in real time instead of
	// silence until the whole investigation fails (#1612 gap).
	emitToSink(ctx, session.EventTypeError, p.turn, p.phase, map[string]interface{}{
		"error": err.Error(),
	})
	return llm.ChatResponse{}, nil, fmt.Errorf("%s LLM call turn %d: %w", p.phase, p.turn, err)
}

// emitLLMRequestAudit records the per-turn LLM request audit event (AU-3:
// model, prompt length/preview, enabled toolsets, full message history).
func (inv *Investigator) emitLLMRequestAudit(ctx context.Context, correlationID, modelName string, phase katypes.Phase, messages []llm.Message, toolDefs []llm.ToolDefinition, workflowContext *workflowDiscoveryAuditContext) {
	reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	reqEvent.EventAction = audit.ActionLLMRequest
	reqEvent.EventOutcome = audit.OutcomeSuccess
	reqEvent.Data["model"] = modelName
	reqEvent.Data["phase"] = string(phase)
	reqEvent.Data["prompt_length"] = totalPromptLength(messages)
	reqEvent.Data["prompt_preview"] = lastUserMessage(messages)
	reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
	reqEvent.Data["messages"] = messagesToAuditFormat(messages)
	if workflowContext != nil {
		reqEvent.Data["workflow_discovery_enrichment_labels_present"] = workflowContext.EnrichmentLabelsPresent
		reqEvent.Data["workflow_discovery_signal_labels_present"] = workflowContext.SignalLabelsPresent
		reqEvent.Data["workflow_discovery_prompt_labels_present"] = workflowContext.PromptLabelsPresent
		reqEvent.Data["workflow_discovery_detected_labels"] = workflowContext.DetectedLabels
	}
	audit.StoreBestEffort(ctx, inv.auditStore, reqEvent, inv.auditLog())
}

// emitLLMResponseAudit records the per-turn LLM response audit event (AU-3:
// token usage, analysis content/preview, tool-call count, finish reason).
func (inv *Investigator) emitLLMResponseAudit(ctx context.Context, correlationID string, resp llm.ChatResponse) {
	respEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
	respEvent.EventAction = audit.ActionLLMResponse
	respEvent.EventOutcome = audit.OutcomeSuccess
	respEvent.Data["prompt_tokens"] = resp.Usage.PromptTokens
	respEvent.Data["completion_tokens"] = resp.Usage.CompletionTokens
	respEvent.Data["total_tokens"] = resp.Usage.TotalTokens
	respEvent.Data["has_analysis"] = resp.Message.Content != ""
	respEvent.Data["analysis_length"] = len(resp.Message.Content)
	respEvent.Data["analysis_preview"] = truncatePreview(resp.Message.Content, 500)
	respEvent.Data["analysis_full"] = resp.Message.Content
	respEvent.Data["analysis_content"] = resp.Message.Content
	respEvent.Data["tool_call_count"] = len(resp.ToolCalls)
	respEvent.Data["finish_reason"] = resp.FinishReason
	// has_reasoning mirrors the has_analysis observability pattern above:
	// an explicit boolean lets dashboards/alerts track the captured-vs-
	// omitted reasoning rate without needing to special-case key presence
	// (BR-AI-086 AC6).
	respEvent.Data["has_reasoning"] = resp.Message.Reasoning != nil
	if resp.Message.Reasoning != nil {
		respEvent.Data["reasoning_text"] = truncateReasoning(resp.Message.Reasoning.Text)
		respEvent.Data["reasoning_redacted"] = resp.Message.Reasoning.Redacted
	}
	audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.auditLog())
}

// processToolCalls handles one turn's tool-call batch: sentinel detection
// (submit_result and friends short-circuit the loop), parallel tool
// execution via errgroup, per-tool-call audit emission, and appending the
// assistant + tool-result messages. Returns the updated message history, a
// non-nil sentinel LoopResult when the LLM called a sentinel tool (the
// caller must return it immediately), and whether the anomaly detector's
// total tool-call budget is now exhausted.
func (inv *Investigator) processToolCalls(ctx context.Context, messages []llm.Message, resp llm.ChatResponse, turn int, phase string, correlationID string) (newMessages []llm.Message, sentinel LoopResult, budgetExhausted bool) {
	if sentinel = inv.findSentinelTool(ctx, messages, resp, phase, correlationID); sentinel != nil {
		return messages, sentinel, false
	}

	assistantMsg := resp.Message
	assistantMsg.ToolCalls = resp.ToolCalls
	messages = append(messages, assistantMsg)
	toolResults := inv.executeToolBatch(ctx, resp, turn, phase, correlationID)

	// #2387 (BR-KA-OBSERVABILITY-001): batch-count the dispatched tools.
	// Sentinel submits (submit_result*) return before this point, so they are
	// consumed, never executed, and never counted here.
	inv.metricsFor(correlationID).AddToolCalls(len(resp.ToolCalls))

	for i, tc := range resp.ToolCalls {
		emitToSink(ctx, session.EventTypeToolResult, turn, phase, map[string]interface{}{
			"tool_name":      tc.Name,
			"tool_index":     i,
			"result_preview": truncatePreview(toolResults[i], 200),
		})

		inv.emitToolCallAudit(ctx, tc, i, toolResults[i], phase, correlationID)
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    toolResults[i],
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
		})
	}

	return messages, nil, inv.anomalyDetectorFor(correlationID).TotalExceeded()
}

func (inv *Investigator) findSentinelTool(ctx context.Context, messages []llm.Message, resp llm.ChatResponse, phase, correlationID string) LoopResult {
	for _, tc := range resp.ToolCalls {
		if !inv.toolAllowedInPhase(ctx, katypes.Phase(phase), tc.Name) {
			continue
		}
		if sr := sentinelResult(tc, resp.Message.Reasoning, messages); sr != nil {
			// DD-KA-2496: an allowed submit must not hide a rejected call in
			// the same batch. Preserve sentinel short-circuit semantics for
			// permitted tools while making off-phase attempts auditable.
			for i, call := range resp.ToolCalls {
				if !inv.toolAllowedInPhase(ctx, katypes.Phase(phase), call.Name) {
					result := inv.phaseToolError(call.Name, katypes.Phase(phase), correlationID)
					inv.emitToolCallAudit(ctx, call, i, result, phase, correlationID)
				}
			}
			inv.logger.Info("sentinel detected",
				"tool", tc.Name,
				"phase", phase,
				"correlation_id", correlationID)
			return sr
		}
	}
	return nil
}

func (inv *Investigator) executeToolBatch(ctx context.Context, resp llm.ChatResponse, turn int, phase, correlationID string) []string {
	toolResults := make([]string, len(resp.ToolCalls))
	var g errgroup.Group
	for i, tc := range resp.ToolCalls {
		emitToSink(ctx, session.EventTypeToolCallStart, turn, phase, map[string]interface{}{
			"tool_name":  tc.Name,
			"tool_index": i,
		})
		g.Go(func() error {
			toolResults[i] = inv.executeToolCall(ctx, tc, correlationID, katypes.Phase(phase))
			return nil
		})
	}
	_ = g.Wait()
	return toolResults
}

func (inv *Investigator) executeToolCall(ctx context.Context, tc llm.ToolCall, correlationID string, phase katypes.Phase) string {
	// BR-KA-267, #1949: bound each tool call so a stuck dependency (e.g. a K8s
	// informer-cache read or DataStorage call) cannot hang the batch forever.
	toolCtx, cancel := context.WithTimeout(ctx, inv.toolCallTimeout)
	defer cancel()
	parentScope, hasParentScope := katypes.EvidenceStatusScopeFromContext(ctx)
	if !hasParentScope {
		return inv.executeTool(toolCtx, tc.Name, json.RawMessage(tc.Arguments), correlationID, phase)
	}

	// Each concurrent tool call gets an isolated child accumulator. The parent
	// is merged after the call regardless of success, typed failure, or
	// cancellation, so source status never rides in the bounded tool result.
	childScope := katypes.NewEvidenceStatusScope()
	toolCtx = katypes.WithEvidenceStatusScope(toolCtx, childScope)
	result := inv.executeTool(toolCtx, tc.Name, json.RawMessage(tc.Arguments), correlationID, phase)
	parentScope.Merge(childScope.Snapshot())
	return result
}

const omittedWorkflowSchemaAuditResult = `{"result_omitted":true,"reason":"workflow_schema_not_persisted"}`
const maxAuditToolResultBytes = 8192
const structuredPhase3BudgetExhaustedReason = "structured Phase 3 context budget exceeded"
const structuredPhase3BudgetExhaustedAfterDiscoveryReason = "structured Phase 3 context budget exceeded after discovery started"

var auditSummaryDigestState struct {
	sync.Once
	key []byte
	err error
}

// auditSafeToolResult keeps the Step 3 parameter schema in the LLM's message
// history but excludes it from the generic tool-call audit record. Bulk
// Kubernetes summaries are reduced to count/truncation/digest/reference data;
// the selected workflow ID remains available in the corresponding arguments.
func auditSafeToolResult(toolName, toolResult string) string {
	result, _ := auditToolResultProjection(toolName, toolResult, "")
	return result
}

// auditToolResultProjection returns the durable, allow-listed representation of
// a tool result and its structured audit summary when one is available.
func auditToolResultProjection(toolName, toolResult, reference string) (string, map[string]interface{}) {
	if toolName == "get_workflow" {
		return omittedWorkflowSchemaAuditResult, nil
	}
	if toolName == "" {
		return toolResult, nil
	}
	if isBulkKubernetesSummaryTool(toolName) {
		if auditToolErrorDetails(toolResult) != nil {
			return toolResult, nil
		}
		if projected, summary, ok := auditBulkToolResult(toolName, toolResult, reference); ok {
			return projected, summary
		}
		projected, summary, _ := auditOmittedToolResult(toolName, toolResult, reference, "bulk_summary_missing")
		return projected, summary
	}
	if projected, summary, ok := auditOmittedOversizedToolResult(toolName, toolResult, reference); ok {
		return projected, summary
	}
	return toolResult, nil
}

// auditBulkToolResult projects a Kubernetes list summary without retaining its
// item bodies or arbitrary fields.
func auditBulkToolResult(toolName, toolResult, reference string) (string, map[string]interface{}, bool) {
	var listSummary struct {
		ItemCount  int  `json:"item_count"`
		TotalCount *int `json:"total_count"`
		Truncated  bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(toolResult), &listSummary); err != nil ||
		(!strings.Contains(toolResult, `"item_count"`) && !strings.Contains(toolResult, `"total_count"`)) {
		return auditOmittedToolResult(toolName, toolResult, reference, "bulk_summary_missing")
	}
	summary := map[string]interface{}{
		"tool_name":  toolName,
		"item_count": listSummary.ItemCount,
		"truncated":  listSummary.Truncated,
	}
	if listSummary.TotalCount != nil {
		summary["total_count"] = *listSummary.TotalCount
	}
	return encodeAuditBulkSummary(reference, summary)
}

func encodeAuditBulkSummary(reference string, summary map[string]interface{}) (string, map[string]interface{}, bool) {
	canonical, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return "", nil, false
	}
	digest, digestErr := auditSummaryDigest(canonical)
	if digestErr != nil {
		return "", nil, false
	}
	summary["digest"] = digest
	if reference != "" {
		summary["reference"] = reference
	}
	encoded, marshalErr := json.Marshal(summary)
	if marshalErr == nil {
		return string(encoded), summary, true
	}
	return "", nil, false
}

// auditSummaryDigest creates an opaque process-local integrity fingerprint for
// the allow-listed summary. The key prevents the value from being a fast
// password hash while keeping repeated audit projections in one process stable.
func auditSummaryDigest(input []byte) (string, error) {
	auditSummaryDigestState.Do(func() {
		auditSummaryDigestState.key = make([]byte, 32)
		if _, err := cryptorand.Read(auditSummaryDigestState.key); err != nil {
			auditSummaryDigestState.err = fmt.Errorf("generate audit summary digest key: %w", err)
		}
	})
	if auditSummaryDigestState.err != nil {
		return "", auditSummaryDigestState.err
	}
	digest := hmac.New(sha256.New, auditSummaryDigestState.key)
	if _, err := digest.Write(input); err != nil {
		return "", fmt.Errorf("hash audit summary: %w", err)
	}
	return fmt.Sprintf("hmac-sha256:%x", digest.Sum(nil)), nil
}

// auditOmittedOversizedToolResult applies the generic audit result size guard.
func auditOmittedOversizedToolResult(toolName, toolResult, reference string) (string, map[string]interface{}, bool) {
	if len(toolResult) <= maxAuditToolResultBytes {
		return "", nil, false
	}
	return auditOmittedToolResult(toolName, toolResult, reference, "size_limit")
}

// auditOmittedToolResult records only bounded omission metadata for a result.
func auditOmittedToolResult(toolName, toolResult, reference, reason string) (string, map[string]interface{}, bool) {
	summary := map[string]interface{}{
		"tool_name":      toolName,
		"result_omitted": true,
		"reason":         reason,
		"bytes":          len(toolResult),
	}
	if reference != "" {
		summary["reference"] = reference
	}
	encoded, marshalErr := json.Marshal(summary)
	if marshalErr == nil {
		return string(encoded), summary, true
	}
	return "", nil, false
}

// isBulkKubernetesSummaryTool identifies tools whose audit result must be an
// allow-listed metadata/count projection rather than arbitrary JSON.
func isBulkKubernetesSummaryTool(toolName string) bool {
	return strings.HasPrefix(toolName, "kubectl_get_by_kind_") ||
		toolName == "kubectl_find_resource" || toolName == "kubernetes_jq_query" || toolName == "kubernetes_count"
}

// workflowDiscoveryHasStarted reports whether the provider has already
// requested a discovery tool, which changes the typed outcome of a later
// structured-context budget exhaustion.
func workflowDiscoveryHasStarted(messages []llm.Message) bool {
	for _, message := range messages {
		for _, toolCall := range message.ToolCalls {
			if isWorkflowDiscoveryTool(toolCall.Name) {
				return true
			}
		}
	}
	return false
}

// isWorkflowDiscoveryTool excludes shared planning tools from discovery-state
// classification while recognizing the two catalog-discovery calls.
func isWorkflowDiscoveryTool(name string) bool {
	return name == "list_available_actions" || name == "list_workflows"
}

// structuredPhase3BudgetExhaustionReason preserves whether discovery already
// ran when a later provider turn exceeds the structured context budget.
func structuredPhase3BudgetExhaustionReason(messages []llm.Message) string {
	if workflowDiscoveryHasStarted(messages) {
		return structuredPhase3BudgetExhaustedAfterDiscoveryReason
	}
	return structuredPhase3BudgetExhaustedReason
}

// buildTruncationRetryMessages emits the truncation-detected audit event and
// returns the two messages (the truncated assistant reply plus a correction
// prompt) that runLLMLoop must append before retrying with an escalated
// MaxTokens. truncationRetried/maxTokens mutation stays loop-local state in
// the caller — this helper is a pure audit-emit + message-builder.
func (inv *Investigator) buildTruncationRetryMessages(ctx context.Context, resp llm.ChatResponse, correlationID string, escalatedMaxTokens int) []llm.Message {
	truncEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
	truncEvent.EventAction = "truncation_detected"
	truncEvent.EventOutcome = audit.OutcomeFailure
	truncEvent.Data["has_analysis"] = resp.Message.Content != ""
	truncEvent.Data["analysis_length"] = len(resp.Message.Content)
	truncEvent.Data["analysis_preview"] = truncatePreview(resp.Message.Content, 500)
	truncEvent.Data["finish_reason"] = resp.FinishReason
	truncEvent.Data["escalated_max_tokens"] = escalatedMaxTokens
	truncEvent.Data["truncated_content_length"] = len(resp.Message.Content)
	audit.StoreBestEffort(ctx, inv.auditStore, truncEvent, inv.auditLog())

	return []llm.Message{
		resp.Message,
		{
			Role:    "user",
			Content: "Your previous response was truncated (output token limit reached). Please provide your complete response. Use the submit_result tool to deliver your structured result.",
		},
	}
}

// chatOrStream dispatches to either streaming or non-streaming chat
// depending on whether an event sink is present in ctx. The streaming
// branch retries with backoff exactly like ChatWithParams (#1612), sharing
// the same llm.RetryWithBackoff machinery and RuntimeParams-driven policy
// (MaxRetries, RetryBackoff) — previously RuntimeParams.MaxRetries was
// silently dead here, since StreamChat was called exactly once with no
// retry at all. The retry-attempt telemetry (RecordLLMCallRetry) is
// deliberately labeled by phase/outcome only, not model (DD-LLM-009), so
// this takes no modelName -- callers already emit the model on the
// request/gate audit events before invoking this.
func (inv *Investigator) chatOrStream(ctx context.Context, client llm.Client, req llm.ChatRequest, turn int, phase string, runtimeParams llm.RuntimeParams) (llm.ChatResponse, error) {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil {
		inv.logger.Info("chatOrStream: sink is nil, falling back to non-streaming Chat",
			"turn", turn, "phase", phase,
			"session_id", session.SessionIDFromContext(ctx))
		return llm.ChatWithParams(ctx, client, req, runtimeParams)
	}
	inv.logger.Info("chatOrStream: sink active, using streaming",
		"turn", turn, "phase", phase,
		"session_id", session.SessionIDFromContext(ctx),
		"sink_ptr", fmt.Sprintf("%p", sink),
		"diag_sent", diagSendOK.Load(),
		"diag_dropped", diagSendDrop.Load(),
		"diag_nil", diagSinkNil.Load())

	// #1749: shared with ChatWithParams's non-streaming path — see
	// llm.ApplyTemperature's doc comment for why this was pulled out into
	// one function.
	llm.ApplyTemperature(&req.Options, runtimeParams)

	bo := llm.ResolveRetryBackoff(runtimeParams)
	maxAttempts := llm.ResolveMaxAttempts(runtimeParams)

	var attempts int
	resp, err := llm.RetryWithBackoff(ctx, maxAttempts, bo, func(int) llm.AttemptResult[llm.ChatResponse] {
		attempts++
		// BR-KA-267, #1949: a timeout is applied unconditionally, never the
		// bare (potentially unbounded) parent ctx — RuntimeParams.TimeoutSeconds
		// <= 0 (unset/misconfigured) previously left this streaming call with
		// no deadline at all, so a stalled connection hung the goroutine
		// running it indefinitely. DefaultLLMCallTimeout mirrors the
		// production default configured via llm.DefaultRuntimeParams.
		timeout := time.Duration(runtimeParams.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = DefaultLLMCallTimeout
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)

		// eventSent tracks whether this attempt's callback has already
		// forwarded a token delta to the sink. Once true, a retry is never
		// safe regardless of error classification — the operator has
		// already observed partial output from this attempt, and
		// re-running the call from scratch would duplicate/interleave
		// stream events (#1612).
		var eventSent bool
		resp, err := client.StreamChat(callCtx, req, func(evt llm.ChatStreamEvent) error {
			eventSent = emitStreamEvent(ctx, turn, phase, evt) || eventSent
			return nil
		})

		cancel()

		return llm.AttemptResult[llm.ChatResponse]{Value: resp, Err: err, SafeToRetry: !eventSent && llm.IsRetryable(err)}
	})

	// Retry-attempt telemetry (#1612 gap remediation): only record when a
	// retry actually happened (attempts > 1) — a first-try success or
	// first-try non-retryable failure isn't a "retry" event and would just
	// add noise to the metric.
	if attempts > 1 {
		outcome := "succeeded"
		if err != nil {
			outcome = "exhausted"
			if !llm.IsRetryable(err) {
				outcome = "non_retryable"
			}
		}
		inv.metrics.RecordLLMCallRetry(phase, outcome)
	}

	return resp, err
}

func emitStreamEvent(ctx context.Context, turn int, phase string, evt llm.ChatStreamEvent) bool {
	if evt.Delta != "" {
		emitToSink(ctx, session.EventTypeTokenDelta, turn, phase, map[string]interface{}{
			"delta": evt.Delta,
		})
	}
	if evt.ToolCallDelta != nil {
		data := map[string]interface{}{
			"index":           evt.ToolCallDelta.Index,
			"arguments_delta": evt.ToolCallDelta.ArgumentsDelta,
		}
		if evt.ToolCallDelta.ID != "" {
			data["id"] = evt.ToolCallDelta.ID
		}
		if evt.ToolCallDelta.Name != "" {
			data["name"] = evt.ToolCallDelta.Name
		}
		emitToSink(ctx, session.EventTypeToolCallDelta, turn, phase, data)
	}
	return evt.Delta != "" || evt.ToolCallDelta != nil
}

// emitToSink sends an InvestigationEvent to the context-carried event sink
// using non-blocking send semantics. If the sink is nil (no subscriber yet),
// the event is buffered for replay on the next Subscribe (#1811) rather than
// lost; if a channel is attached but its buffer is full, the event is
// dropped. Either way, the investigation loop is never blocked by a slow or
// absent SSE consumer.
func emitToSink(ctx context.Context, eventType string, turn int, phase string, data map[string]interface{}) {
	sinkPresent := session.EventSinkFromContext(ctx) != nil
	var raw json.RawMessage
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return
		}
	}
	event := session.InvestigationEvent{
		Type:  eventType,
		Turn:  turn,
		Phase: phase,
		Data:  raw,
	}
	switch {
	case session.EmitEvent(ctx, event):
		diagSendOK.Add(1)
	case !sinkPresent:
		diagSinkNil.Add(1)
	default:
		diagSendDrop.Add(1)
	}
}

// emitGateRetryKeepalive emits an EventTypeToolCallStart keepalive around a
// silent, non-streamed llm.ChatWithParams call (#2088, main port of #2086:
// sameKindValidationGate, apiVersionValidationGate,
// RunRCAExtractionFromConversation). Turn 0 is a sentinel -- these calls
// happen outside runLLMLoop's normal turn counting. EventTypeToolCallStart is
// used (not EventTypeReasoningDelta/TokenDelta) specifically because
// bridgeEventsCollectSummary only accumulates the latter two into the RCA
// summary returned to the driving agent; using a text-accumulating event type
// would splice placeholder status text directly into the summary
// (spike-discovered regression, see UT-AF-2086-006).
func emitGateRetryKeepalive(ctx context.Context, toolName string) {
	emitToSink(ctx, session.EventTypeToolCallStart, 0, string(katypes.PhaseRCA), map[string]interface{}{
		"tool_name": toolName,
	})
}

// emitCancellationAudit emits an investigation-level cancellation event
// carrying the phase, turn, token usage, and accumulated messages at the point
// of cancellation. Enriched per COR-2 (token cost attribution), AUD-4
// (session cross-reference), AUD-6 (messages for forensic RAG), and SEC-1
// (content cap at 64KB). The context may already be cancelled so we use
// context.Background() for the audit store call (fire-and-forget per ADR-038).
func (inv *Investigator) emitCancellationAudit(ctx context.Context, result *katypes.InvestigationResult, correlationID string) {
	event := audit.NewEvent(audit.EventTypeInvestigationCancelled, correlationID, audit.WithSessionID(session.SessionIDFromContext(ctx)))
	event.EventAction = audit.ActionInvestigationCancelled
	event.EventOutcome = audit.OutcomeFailure
	event.Data["cancelled_phase"] = result.CancelledPhase
	event.Data["cancelled_at_turn"] = result.CancelledAtTurn
	if result.TokenUsage != nil {
		event.Data["total_prompt_tokens"] = result.TokenUsage.PromptTokens
		event.Data["total_completion_tokens"] = result.TokenUsage.CompletionTokens
		event.Data["total_tokens"] = result.TokenUsage.TotalTokens
	}
	if len(result.AccumulatedMessages) > 0 {
		if b, err := json.Marshal(result.AccumulatedMessages); err == nil {
			s := string(b)
			if len(s) > maxForensicPayloadBytes {
				s = s[:maxForensicPayloadBytes]
			}
			event.Data["accumulated_messages"] = s
		}
	}
	auditCtx := context.WithoutCancel(ctx)
	audit.StoreBestEffort(auditCtx, inv.auditStore, event, inv.logger) // audit.StoreBestEffort is intentionally detached so cancellation cannot drop the event
	inv.emitInvestigationDecision(auditCtx, investigationDecisionParams{
		CorrelationID:      correlationID,
		SessionID:          session.SessionIDFromContext(ctx),
		Phase:              katypes.Phase(result.CancelledPhase),
		TurnKind:           investigationDecisionTurnTerminal,
		TurnID:             "investigation:cancelled",
		Result:             result,
		ContinuationReason: "investigation_cancelled",
	})
}
