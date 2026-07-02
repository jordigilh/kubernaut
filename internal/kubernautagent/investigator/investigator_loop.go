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
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	toolDefs := inv.toolDefinitionsForPhase(phase)
	loopStart := time.Now()
	truncationRetried := false
	maxTokens := 0

	for turn := 0; turn < inv.maxTurns; turn++ {
		if ctx.Err() != nil {
			emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
			return &CancelledResult{
				Messages: messages,
				Turn:     turn,
				Phase:    string(phase),
				Tokens:   tokens,
			}, nil
		}

		reqEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		reqEvent.EventAction = audit.ActionLLMRequest
		reqEvent.EventOutcome = audit.OutcomeSuccess
		reqEvent.Data["model"] = modelName
		reqEvent.Data["prompt_length"] = totalPromptLength(messages)
		reqEvent.Data["prompt_preview"] = lastUserMessage(messages, 500)
		reqEvent.Data["toolsets_enabled"] = toolNames(toolDefs)
		reqEvent.Data["messages"] = messagesToAuditFormat(messages)
		audit.StoreBestEffort(ctx, inv.auditStore, reqEvent, inv.auditLog())

		chatReq := llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: submitResultSchemaForPhase(phase), MaxTokens: maxTokens},
		}
		resp, err := inv.chatOrStream(ctx, client, chatReq, turn, string(phase), modelName, runtimeParams)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				emitToSink(ctx, session.EventTypeCancelled, turn, string(phase), nil)
				return &CancelledResult{
					Messages: messages,
					Turn:     turn,
					Phase:    string(phase),
					Tokens:   tokens,
				}, nil
			}
			failEvent := audit.NewEvent(audit.EventTypeResponseFailed, correlationID)
			failEvent.EventAction = audit.ActionResponseFailed
			failEvent.EventOutcome = audit.OutcomeFailure
			failEvent.Data["error_message"] = err.Error()
			failEvent.Data["phase"] = string(phase)
			failEvent.Data["duration_seconds"] = time.Since(loopStart).Seconds()
			audit.StoreBestEffort(ctx, inv.auditStore, failEvent, inv.auditLog())
			return nil, fmt.Errorf("%s LLM call turn %d: %w", phase, turn, err)
		}

		if tokens != nil {
			tokens.Add(resp.Usage)
		}

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
		audit.StoreBestEffort(ctx, inv.auditStore, respEvent, inv.auditLog())

		emitToSink(ctx, session.EventTypeReasoningDelta, turn, string(phase), map[string]interface{}{
			"content_preview": truncatePreview(resp.Message.Content, 200),
			"tool_call_count": len(resp.ToolCalls),
		})

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				if sr := sentinelResult(tc); sr != nil {
					inv.logger.Info("sentinel detected",
						"tool", tc.Name,
						"phase", string(phase),
						"correlation_id", correlationID)
					return sr, nil
				}
			}

			assistantMsg := resp.Message
			assistantMsg.ToolCalls = resp.ToolCalls
			messages = append(messages, assistantMsg)

			toolResults := make([]string, len(resp.ToolCalls))
			var g errgroup.Group
			for i, tc := range resp.ToolCalls {
				emitToSink(ctx, session.EventTypeToolCallStart, turn, string(phase), map[string]interface{}{
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
				emitToSink(ctx, session.EventTypeToolResult, turn, string(phase), map[string]interface{}{
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
			if inv.pipeline.AnomalyDetector.TotalExceeded() {
				return &ExhaustedResult{Reason: "tool budget exhausted"}, nil
			}
			continue
		}

		if resp.FinishReason == llm.FinishReasonLength && !truncationRetried {
			truncationRetried = true
			maxTokens = escalateMaxTokens(resp.Usage.CompletionTokens)
			inv.logger.Info("LLM response truncated, retrying with escalated MaxTokens",
				"phase", string(phase),
				"escalated_max_tokens", maxTokens,
				"correlation_id", correlationID)

			truncEvent := audit.NewEvent(audit.EventTypeLLMResponse, correlationID)
			truncEvent.EventAction = "truncation_detected"
			truncEvent.EventOutcome = audit.OutcomeFailure
			truncEvent.Data["has_analysis"] = resp.Message.Content != ""
			truncEvent.Data["analysis_length"] = len(resp.Message.Content)
			truncEvent.Data["analysis_preview"] = truncatePreview(resp.Message.Content, 500)
			truncEvent.Data["finish_reason"] = resp.FinishReason
			truncEvent.Data["escalated_max_tokens"] = maxTokens
			truncEvent.Data["truncated_content_length"] = len(resp.Message.Content)
			audit.StoreBestEffort(ctx, inv.auditStore, truncEvent, inv.auditLog())

			messages = append(messages, resp.Message)
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: "Your previous response was truncated (output token limit reached). Please provide your complete response. Use the submit_result tool to deliver your structured result.",
			})
			continue
		}

		return &TextResult{Content: resp.Message.Content}, nil
	}

	return &ExhaustedResult{Reason: "max turns exhausted"}, nil
}

// chatOrStream dispatches to either streaming or non-streaming chat
// depending on whether an event sink is present in ctx.
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

	callCtx := ctx
	var cancel context.CancelFunc
	if runtimeParams.TimeoutSeconds > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(runtimeParams.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	return client.StreamChat(callCtx, req, func(evt llm.ChatStreamEvent) error {
		if evt.Delta != "" {
			emitToSink(ctx, session.EventTypeTokenDelta, turn, phase, map[string]interface{}{
				"delta": evt.Delta,
			})
		}
		return nil
	})
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
