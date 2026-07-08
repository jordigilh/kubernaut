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
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func (inv *Investigator) runLLMLoop(ctx context.Context, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext) (LoopResult, error) {
	state := &loopTurnState{loopStart: time.Now()}

	for turn := 0; turn < inv.maxTurns; turn++ {
		result, newMessages, done, err := inv.runLoopTurn(ctx, state, messages, phase, llmCtx, turn)
		messages = newMessages
		if err != nil {
			return nil, err
		}
		if done {
			return result, nil
		}
	}

	return &ExhaustedResult{Reason: "max turns exhausted"}, nil
}

// loopTurnState carries the mutable state that persists across turns of
// runLLMLoop: the loop start time (for duration audit fields) and the
// truncation-retry-once tracking (retried flag + escalated MaxTokens).
type loopTurnState struct {
	loopStart         time.Time
	truncationRetried bool
	maxTokens         int
}

// runLoopTurn executes a single turn of the investigation LLM loop: the
// cancellation check, LLM call, response audit/sink emission, and outcome
// classification (tool calls / truncation retry / final text). done=true
// tells the caller to return result immediately (err is set only for
// non-cancellation failures); done=false means the caller should continue
// to the next turn with newMessages.
func (inv *Investigator) runLoopTurn(ctx context.Context, state *loopTurnState, messages []llm.Message, phase katypes.Phase, llmCtx LLMInvocationContext, turn int) (result LoopResult, newMessages []llm.Message, done bool, err error) {
	tokens, correlationID := llmCtx.Tokens, llmCtx.CorrelationID
	toolDefs := inv.toolDefinitionsForPhase(phase)

	if ctx.Err() != nil {
		emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
		return buildCancelledResult(messages, turn, string(phase), tokens), messages, true, nil
	}

	inv.emitLLMRequestAudit(ctx, correlationID, llmCtx.ModelName, messages, toolDefs)

	resp, cancelled, callErr := inv.doLLMCall(ctx, state, messages, phase, llmCtx, turn, toolDefs)
	if callErr != nil {
		return nil, messages, true, callErr
	}
	if cancelled != nil {
		return cancelled, messages, true, nil
	}

	if tokens != nil {
		tokens.Add(resp.Usage)
	}

	inv.emitLLMResponseAudit(ctx, correlationID, resp)

	emitToSink(ctx, session.EventTypeReasoningDelta, turn, string(phase), map[string]interface{}{
		"content_preview": truncatePreview(resp.Message.Content, 200),
		"tool_call_count": len(resp.ToolCalls),
	})

	if len(resp.ToolCalls) > 0 {
		toolMessages, sentinel, budgetExhausted := inv.processToolCalls(ctx, messages, resp, turn, string(phase), correlationID)
		if sentinel != nil {
			return sentinel, toolMessages, true, nil
		}
		if budgetExhausted {
			return &ExhaustedResult{Reason: "tool budget exhausted"}, toolMessages, true, nil
		}
		return nil, toolMessages, false, nil
	}

	if resp.FinishReason == llm.FinishReasonLength && !state.truncationRetried {
		return nil, inv.handleTruncation(ctx, state, messages, resp, phase, correlationID), false, nil
	}

	return &TextResult{Content: resp.Message.Content, Reasoning: resp.Message.Reasoning}, messages, true, nil
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
		modelName:     llmCtx.ModelName,
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
	modelName     string
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
	resp, err := inv.chatOrStream(ctx, p.client, p.chatReq, p.turn, p.phase, p.modelName, p.runtimeParams)
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
		return llm.ChatResponse{}, buildCancelledResult(p.messages, p.turn, p.phase, p.tokens), nil
	}
	failEvent := audit.NewEvent(audit.EventTypeResponseFailed, p.correlationID)
	failEvent.EventAction = audit.ActionResponseFailed
	failEvent.EventOutcome = audit.OutcomeFailure
	failEvent.Data["error_message"] = err.Error()
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
func (inv *Investigator) emitLLMRequestAudit(ctx context.Context, correlationID, modelName string, messages []llm.Message, toolDefs []llm.ToolDefinition) {
	reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
	reqEvent.EventAction = audit.ActionLLMRequest
	reqEvent.EventOutcome = audit.OutcomeSuccess
	reqEvent.Data["model"] = modelName
	reqEvent.Data["prompt_length"] = totalPromptLength(messages)
	reqEvent.Data["prompt_preview"] = lastUserMessage(messages, 500)
	reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
	reqEvent.Data["messages"] = messagesToAuditFormat(messages)
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
	for _, tc := range resp.ToolCalls {
		if sr := sentinelResult(tc, resp.Message.Reasoning); sr != nil {
			inv.logger.Info("sentinel detected",
				"tool", tc.Name,
				"phase", phase,
				"correlation_id", correlationID)
			return messages, sr, false
		}
	}

	assistantMsg := resp.Message
	assistantMsg.ToolCalls = resp.ToolCalls
	messages = append(messages, assistantMsg)

	toolResults := make([]string, len(resp.ToolCalls))
	var g errgroup.Group
	for i, tc := range resp.ToolCalls {
		emitToSink(ctx, session.EventTypeToolCallStart, turn, phase, map[string]interface{}{
			"tool_name":  tc.Name,
			"tool_index": i,
		})
		g.Go(func() error {
			toolResults[i] = inv.executeTool(ctx, tc.Name, json.RawMessage(tc.Arguments))
			return nil
		})
	}
	_ = g.Wait()

	for i, tc := range resp.ToolCalls {
		emitToSink(ctx, session.EventTypeToolResult, turn, phase, map[string]interface{}{
			"tool_name":      tc.Name,
			"tool_index":     i,
			"result_preview": truncatePreview(toolResults[i], 200),
		})

		tcEvent := audit.NewEvent(audit.EventTypeLLMToolCall, correlationID)
		tcEvent.EventAction = audit.ActionToolExecution
		tcEvent.EventOutcome = audit.OutcomeSuccess
		tcEvent.Data["tool_call_index"] = i
		tcEvent.Data["tool_name"] = tc.Name
		tcEvent.Data["tool_arguments"] = tc.Arguments
		tcEvent.Data["tool_result"] = toolResults[i]
		tcEvent.Data["tool_result_preview"] = truncatePreview(toolResults[i], 500)
		audit.StoreBestEffort(ctx, inv.auditStore, tcEvent, inv.auditLog())

		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    toolResults[i],
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
		})
	}

	budgetExhausted = inv.pipeline.AnomalyDetector.TotalExceeded()
	return messages, nil, budgetExhausted
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
// retry at all.
func (inv *Investigator) chatOrStream(ctx context.Context, client llm.Client, req llm.ChatRequest, turn int, phase string, modelName string, runtimeParams llm.RuntimeParams) (llm.ChatResponse, error) {
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

	temp := runtimeParams.Temperature
	req.Options.Temperature = &temp

	bo := llm.ResolveRetryBackoff(runtimeParams)
	maxAttempts := llm.ResolveMaxAttempts(runtimeParams)

	var attempts int
	resp, err := llm.RetryWithBackoff(ctx, maxAttempts, bo, func(int) llm.AttemptResult[llm.ChatResponse] {
		attempts++
		callCtx := ctx
		var cancel context.CancelFunc
		if runtimeParams.TimeoutSeconds > 0 {
			callCtx, cancel = context.WithTimeout(ctx, time.Duration(runtimeParams.TimeoutSeconds)*time.Second)
		}

		// eventSent tracks whether this attempt's callback has already
		// forwarded a token delta to the sink. Once true, a retry is never
		// safe regardless of error classification — the operator has
		// already observed partial output from this attempt, and
		// re-running the call from scratch would duplicate/interleave
		// stream events (#1612).
		var eventSent bool
		resp, err := client.StreamChat(callCtx, req, func(evt llm.ChatStreamEvent) error {
			if evt.Delta != "" {
				eventSent = true
				emitToSink(ctx, session.EventTypeTokenDelta, turn, phase, map[string]interface{}{
					"delta": evt.Delta,
				})
			}
			return nil
		})

		if cancel != nil {
			cancel()
		}

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

// emitToSink sends an InvestigationEvent to the context-carried event sink
// using non-blocking send semantics. If the sink is nil (no subscriber) or
// the channel buffer is full, the event is silently dropped. This ensures
// the investigation loop is never blocked by a slow SSE consumer.
func emitToSink(ctx context.Context, eventType string, turn int, phase string, data map[string]interface{}) {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil {
		diagSinkNil.Add(1)
		return
	}
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
	select {
	case sink <- event:
		diagSendOK.Add(1)
	default:
		diagSendDrop.Add(1)
	}
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
	audit.StoreBestEffort(context.Background(), inv.auditStore, event, inv.logger)
}
