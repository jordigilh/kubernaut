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

package llm

import (
	"context"
	"encoding/json"
)

// Client abstracts the LLM provider behind a Kubernaut-owned interface.
// Business logic never imports the underlying framework (LangChainGo, Eino, etc.).
// Authority: DD-HAPI-019 — Framework Isolation Pattern
//
// Close releases resources held by the client (gRPC connections, HTTP idle
// pools). Callers must call Close when the client is no longer needed.
// Implementations where no cleanup is required should return nil.
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Close() error
}

// ChatRequest contains the messages and tool definitions for an LLM call.
type ChatRequest struct {
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
	Options  ChatOptions      `json:"options,omitempty"`
}

// FinishReason constants normalized across all LLM providers. Adapters must
// map provider-specific stop reasons to one of these values.
const (
	FinishReasonStop      = "stop"
	FinishReasonLength    = "length"
	FinishReasonToolCalls = "tool_calls"
)

// ChatResponse contains the LLM's reply and any tool call requests.
type ChatResponse struct {
	Message      Message    `json:"message"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        TokenUsage `json:"usage,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// Message represents a single conversation message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents the LLM requesting execution of a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TokenUsage tracks token consumption for a single LLM call.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatOptions holds optional parameters for the LLM call.
type ChatOptions struct {
	Temperature  float64         `json:"temperature,omitempty"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
	JSONMode     bool            `json:"json_mode,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}
