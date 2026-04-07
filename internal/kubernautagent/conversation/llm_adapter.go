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

package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// ErrMaxToolTurnsExceeded indicates the tool-call loop hit its iteration limit.
// The SSE error event has already been emitted; callers should skip success auditing.
var ErrMaxToolTurnsExceeded = errors.New("max tool turns exceeded")

// LLMAdapterDeps bundles dependencies for NewLLMAdapter.
type LLMAdapterDeps struct {
	Client       llm.Client
	Sessions     *SessionManager
	ToolRegistry *registry.Registry
	AuditStore   audit.AuditStore
	Logger       *slog.Logger
	MaxToolTurns int
	ModelName    string
}

// LLMAdapter bridges llm.Client to ConversationLLM with a bounded tool-call loop (DD-CONV-001).
type LLMAdapter struct {
	client       llm.Client
	sessions     *SessionManager
	toolRegistry *registry.Registry
	auditStore   audit.AuditStore
	logger       *slog.Logger
	maxToolTurns int
	modelName    string
}

// NewLLMAdapter creates an LLMAdapter from the given deps.
func NewLLMAdapter(deps LLMAdapterDeps) *LLMAdapter {
	maxTurns := deps.MaxToolTurns
	if maxTurns < 0 {
		maxTurns = 0
	}
	return &LLMAdapter{
		client:       deps.Client,
		sessions:     deps.Sessions,
		toolRegistry: deps.ToolRegistry,
		auditStore:   deps.AuditStore,
		logger:       deps.Logger,
		maxToolTurns: maxTurns,
		modelName:    deps.ModelName,
	}
}

func toolToDefinition(t tools.Tool) llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Parameters(),
	}
}

// Respond implements ConversationLLM with a bounded tool-call loop.
func (a *LLMAdapter) Respond(ctx context.Context, sessionID, message string, emit func(ConversationEvent)) error {
	session, err := a.sessions.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session lookup: %w", err)
	}

	systemPrompt, err := session.SystemPrompt()
	if err != nil {
		return fmt.Errorf("render system prompt: %w", err)
	}

	priorMsgs := session.GetMessages()
	userMsg := llm.Message{Role: "user", Content: message}
	messages := make([]llm.Message, 0, 1+len(priorMsgs)+1)
	messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, priorMsgs...)
	messages = append(messages, userMsg)

	newMsgStart := 1 + len(priorMsgs)

	// Build tool definitions from guardrails-filtered registry + per-session todoWrite
	var toolDefs []llm.ToolDefinition
	if a.toolRegistry != nil {
		filtered := session.Guardrails.FilterTools(a.toolRegistry.All())
		for _, t := range filtered {
			toolDefs = append(toolDefs, toolToDefinition(t))
		}
	}
	if tw := session.TodoWrite(); tw != nil {
		toolDefs = append(toolDefs, toolToDefinition(tw))
	}

	for iteration := 0; iteration <= a.maxToolTurns; iteration++ {
		a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMRequest, audit.ActionLLMRequest, audit.OutcomePending, map[string]interface{}{
			"model":            a.modelName,
			"prompt_length":    totalPromptLength(messages),
			"prompt_preview":   lastUserMessage(messages, 500),
			"toolsets_enabled": toolNames(toolDefs),
		})

		resp, chatErr := a.client.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
		})
		if chatErr != nil {
			a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMResponse, audit.ActionLLMResponse, audit.OutcomeFailure, map[string]interface{}{
				"error":      chatErr.Error(),
				"session_id": sessionID,
			})
			return fmt.Errorf("LLM chat: %w", chatErr)
		}

		a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMResponse, audit.ActionLLMResponse, audit.OutcomeSuccess, map[string]interface{}{
			"has_analysis":    resp.Message.Content != "",
			"analysis_length": len(resp.Message.Content),
			"analysis_preview": truncatePreview(resp.Message.Content, 500),
			"total_tokens":    resp.Usage.TotalTokens,
			"tool_call_count": len(resp.ToolCalls),
		})

		if len(resp.ToolCalls) == 0 {
			messages = append(messages, resp.Message)
			session.AppendMessages(messages[newMsgStart:]...)
			emit(ConversationEvent{Type: "message", Data: mustMarshal(map[string]string{"content": resp.Message.Content})})
			return nil
		}

		if iteration == a.maxToolTurns {
			emit(ConversationEvent{
				Type: "error",
				Data: mustMarshal(map[string]string{"error": fmt.Sprintf("max tool turns (%d) exceeded", a.maxToolTurns)}),
			})
			session.AppendMessages(messages[newMsgStart:]...)
			return ErrMaxToolTurnsExceeded
		}

		// Append the assistant message with tool calls
		assistantMsg := resp.Message
		assistantMsg.ToolCalls = resp.ToolCalls
		messages = append(messages, assistantMsg)

		for i, tc := range resp.ToolCalls {
			emit(ConversationEvent{
				Type: "tool_call",
				Data: mustMarshal(map[string]string{"name": tc.Name, "args": tc.Arguments}),
			})

			var argsMap map[string]interface{}
			if unmarshalErr := json.Unmarshal([]byte(tc.Arguments), &argsMap); unmarshalErr != nil {
				errMsg := fmt.Sprintf("malformed tool arguments: %s", unmarshalErr.Error())
				emit(ConversationEvent{
					Type: "tool_error",
					Data: mustMarshal(map[string]string{"name": tc.Name, "error": errMsg}),
				})
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    errMsg,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
				a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMToolCall, audit.ActionToolExecution, audit.OutcomeFailure, map[string]interface{}{
					"tool_call_index": i,
					"tool_name":       tc.Name,
					"tool_arguments":  tc.Arguments,
					"error":           errMsg,
				})
				continue
			}

			// Route todo_write to per-session tool before guardrails (internal agent tool).
			var result string
			var execErr error
			isTodoWrite := tc.Name == "todo_write" && session.TodoWrite() != nil

			if isTodoWrite {
				result, execErr = session.TodoWrite().Execute(ctx, json.RawMessage(tc.Arguments))
			} else {
				if valErr := session.Guardrails.ValidateToolCall(tc.Name, argsMap); valErr != nil {
					errMsg := valErr.Error()
					emit(ConversationEvent{
						Type: "tool_error",
						Data: mustMarshal(map[string]string{"name": tc.Name, "error": errMsg}),
					})
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    errMsg,
						ToolCallID: tc.ID,
						ToolName:   tc.Name,
					})
					a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMToolCall, audit.ActionToolExecution, audit.OutcomeFailure, map[string]interface{}{
						"tool_call_index": i,
						"tool_name":       tc.Name,
						"tool_arguments":  tc.Arguments,
						"error":           errMsg,
					})
					continue
				}

				if a.toolRegistry != nil {
					result, execErr = a.toolRegistry.Execute(ctx, tc.Name, json.RawMessage(tc.Arguments))
				} else {
					execErr = fmt.Errorf("tool %q not available", tc.Name)
				}
			}

			if execErr != nil {
				errMsg := execErr.Error()
				emit(ConversationEvent{
					Type: "tool_error",
					Data: mustMarshal(map[string]string{"name": tc.Name, "error": errMsg}),
				})
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    errMsg,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
				})
				a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMToolCall, audit.ActionToolExecution, audit.OutcomeFailure, map[string]interface{}{
					"tool_call_index": i,
					"tool_name":       tc.Name,
					"tool_arguments":  tc.Arguments,
					"error":           errMsg,
				})
				continue
			}

			emit(ConversationEvent{
				Type: "tool_result",
				Data: mustMarshal(map[string]string{"name": tc.Name, "result": result}),
			})
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
			})

			a.emitAudit(ctx, session.CorrelationID, audit.EventTypeLLMToolCall, audit.ActionToolExecution, audit.OutcomeSuccess, map[string]interface{}{
				"tool_call_index":    i,
				"tool_name":          tc.Name,
				"tool_arguments":     tc.Arguments,
				"tool_result":        result,
				"tool_result_preview": truncatePreview(result, 500),
			})
		}
	}

	return nil
}

func totalPromptLength(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total
}

func lastUserMessage(messages []llm.Message, maxLen int) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return truncatePreview(messages[i].Content, maxLen)
		}
	}
	return ""
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func toolNames(defs []llm.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func (a *LLMAdapter) emitAudit(ctx context.Context, correlationID, eventType, action, outcome string, data map[string]interface{}) {
	if a.auditStore == nil {
		return
	}
	event := audit.NewEvent(eventType, correlationID)
	event.EventAction = action
	event.EventOutcome = outcome
	for k, v := range data {
		event.Data[k] = v
	}
	audit.StoreBestEffort(ctx, a.auditStore, event, a.logger)
}

