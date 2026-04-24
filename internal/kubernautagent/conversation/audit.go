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
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// AuditReader abstracts DataStorage audit event querying (G5).
type AuditReader interface {
	QueryAuditEvents(ctx context.Context, correlationID string) ([]AuditEvent, error)
}

// AuditEvent represents a stored audit event returned by the reader.
type AuditEvent struct {
	EventType string
	Data      map[string]interface{}
}

// AuditChainFetcher reconstructs the investigation LLM history from audit events.
type AuditChainFetcher struct {
	reader     AuditReader
	maxRetries int
}

// NewAuditChainFetcher creates a fetcher backed by the given reader.
func NewAuditChainFetcher(reader AuditReader) *AuditChainFetcher {
	return &AuditChainFetcher{
		reader:     reader,
		maxRetries: 6,
	}
}

const (
	initialBackoff = 100 * time.Millisecond
	maxTokenBudget = 120000
)

// FetchInvestigationHistory retrieves audit events for the given correlation ID
// and reconstructs them as an LLM message history for conversation seeding.
// Retries with exponential backoff when the chain is empty (R1 mitigation).
func (f *AuditChainFetcher) FetchInvestigationHistory(ctx context.Context, correlationID string) ([]llm.Message, error) {
	var events []AuditEvent
	backoff := initialBackoff

	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		var err error
		events, err = f.reader.QueryAuditEvents(ctx, correlationID)
		if err != nil {
			return nil, fmt.Errorf("querying audit events: %w", err)
		}
		if len(events) > 0 {
			break
		}
		if attempt < f.maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("audit chain empty for correlation_id %s after %d retries", correlationID, f.maxRetries)
	}

	messages := EventsToMessages(events)
	return fitTokenBudget(messages), nil
}

// EventsToMessages converts a slice of audit events to LLM messages.
// Mapping: aiagent.llm.request → system+user from messages array,
// aiagent.llm.response → assistant, aiagent.llm.tool_call → tool.
func EventsToMessages(events []AuditEvent) []llm.Message {
	var messages []llm.Message

	for _, e := range events {
		switch e.EventType {
		case "aiagent.llm.request":
			messages = append(messages, extractMessagesFromRequest(e)...)
		case "aiagent.llm.response":
			content, _ := e.Data["analysis_content"].(string)
			if content != "" {
				messages = append(messages, llm.Message{Role: "assistant", Content: content})
			}
		case "aiagent.llm.tool_call":
			toolName, _ := e.Data["tool_name"].(string)
			toolResult, _ := e.Data["tool_result"].(string)
			if toolResult != "" {
				messages = append(messages, llm.Message{
					Role:     "tool",
					Content:  toolResult,
					ToolName: toolName,
				})
			}
		case "aiagent.conversation.turn":
			q, _ := e.Data["question"].(string)
			a, _ := e.Data["answer"].(string)
			if q != "" {
				messages = append(messages, llm.Message{Role: "user", Content: q})
			}
			if a != "" {
				messages = append(messages, llm.Message{Role: "assistant", Content: a})
			}
		case "aiagent.response.complete":
			// Terminal event; the response_data is captured in the system prompt context
		}
	}

	return messages
}

func extractMessagesFromRequest(e AuditEvent) []llm.Message {
	raw, ok := e.Data["messages"]
	if !ok {
		return nil
	}
	msgSlice, ok := raw.([]map[string]interface{})
	if !ok {
		return nil
	}
	var out []llm.Message
	for _, m := range msgSlice {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if role == "" {
			continue
		}
		msg := llm.Message{Role: role, Content: content}
		if tcID, ok := m["tool_call_id"].(string); ok {
			msg.ToolCallID = tcID
		}
		if name, ok := m["name"].(string); ok {
			msg.ToolName = name
		}
		out = append(out, msg)
	}
	return out
}

func fitTokenBudget(messages []llm.Message) []llm.Message {
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
	}
	if totalLen <= maxTokenBudget {
		return messages
	}

	// Keep first (system prompt) and last N messages; summarize middle
	if len(messages) <= 2 {
		return messages
	}

	keepTail := len(messages) / 2
	if keepTail < 2 {
		keepTail = 2
	}

	summary := "Previous investigation context (summarized): "
	for _, m := range messages[1 : len(messages)-keepTail] {
		summary += m.Role + ": " + truncate(m.Content, 200) + " | "
	}

	result := []llm.Message{messages[0]}
	result = append(result, llm.Message{Role: "system", Content: summary})
	result = append(result, messages[len(messages)-keepTail:]...)
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
