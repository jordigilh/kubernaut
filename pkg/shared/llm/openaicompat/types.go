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

// Package openaicompat implements the wire protocol shared by every
// OpenAI-Chat-Completions-compatible provider (OpenAI, Azure OpenAI, Ollama,
// vLLM, LlamaStack, Mistral, HuggingFace TGI, DeepSeek, and Bedrock's
// OpenAI-compatible endpoints).
//
// Deliberately independent of both genai.Content (used by the AI Frontend's
// ADK-based launcher) and llm.Message (used by Kubernaut Agent) — DD-HAPI-019
// Framework Isolation — so this package can be shared by both consumers
// without either depending on the other's types. Each consumer owns a thin
// translation layer at its boundary.
//
// Authority: BR-AI-086, DD-LLM-005.
package openaicompat

import "encoding/json"

// Message is the shared, protocol-neutral chat message shape.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
	// Reasoning holds this message's captured chain-of-thought
	// (reasoning_content), when the provider returned one on this turn.
	// Whether it is replayed to the provider on a later request is governed
	// by Request.ReasoningMode, not by this field's mere presence — see
	// buildMessages.
	Reasoning string
}

// ToolCall represents the LLM requesting execution of a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// TokenUsage tracks token consumption for a single call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Request contains everything needed to build one Chat Completions call.
type Request struct {
	Model          string
	Messages       []Message
	Tools          []ToolDefinition
	Temperature    *float64
	TopP           *float64
	MaxTokens      int
	StopSequences  []string
	ResponseSchema json.RawMessage
	// ReasoningMode governs whether/how a captured Message.Reasoning value
	// is replayed back to the provider on this call. Resolved once via
	// DetectReasoningMode at client-construction time — never left as the
	// zero value ("none") by accident for a model that needs a different
	// mode, and never guessed per-call from business logic (DD-HAPI-019).
	ReasoningMode ReasoningMode
	// Effort is the canonical, provider-agnostic reasoning-depth value
	// ("", "none", "minimal", "low", "medium", "high", "xhigh" — #1604).
	// Distinct from ReasoningMode/Message.Reasoning: this asks the
	// provider to think harder or less; it never controls whether
	// already-captured reasoning text is replayed.
	Effort string
	// EffortDialect governs how Effort is mapped onto the wire. Resolved
	// once via DetectEffortDialect at client-construction time, same
	// pattern as ReasoningMode.
	EffortDialect EffortDialect
}

// Response is the mapped, non-streaming reply from a Chat Completions call.
type Response struct {
	Message      Message
	FinishReason string
	Usage        TokenUsage
}

// Wire-protocol finish_reason values, as defined by the OpenAI Chat
// Completions API and mirrored by every compatible provider. Response.
// FinishReason is passed through verbatim from the wire (unnormalized) so
// each consumer (AF, KA) maps it to its own canonical type; these constants
// are the single source of truth for the literal values both consumers
// switch on, replacing what were previously two independently-typed copies
// of the same magic strings.
const (
	FinishReasonStop          = "stop"
	FinishReasonLength        = "length"
	FinishReasonToolCalls     = "tool_calls"
	FinishReasonContentFilter = "content_filter"
)

// StreamEvent represents a single unit of streamed output. Delta carries an
// incremental text fragment; Done marks the terminal event, at which point
// Final holds the fully-accumulated Response (mirroring the non-streaming
// shape, including resolved tool calls).
type StreamEvent struct {
	Delta         string
	ToolCallDelta *PartialToolCall
	Done          bool
	Final         *Response
}

// PartialToolCall represents an incremental fragment of a tool call
// received during streaming.
type PartialToolCall struct {
	Index          int
	ID             string
	Name           string
	ArgumentsDelta string
}
