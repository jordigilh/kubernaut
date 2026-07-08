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

// Package openai implements llm.Client for OpenAI-protocol-compatible
// endpoints (OpenAI, Azure OpenAI, Ollama, vLLM, LlamaStack, Mistral,
// HuggingFace TGI, DeepSeek, Bedrock's OpenAI-compatible surface).
//
// This is a thin translation layer: all wire-protocol work is delegated to
// pkg/shared/llm/openaicompat, the same client the AI Frontend's launcher
// wraps (DD-LLM-005). This file only translates between Kubernaut's
// llm.Message/ChatRequest/ChatResponse and the shared package's neutral
// types (DD-HAPI-019 Framework Isolation).
package openai

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Client implements llm.Client for OpenAI-protocol-compatible endpoints.
type Client struct {
	client           *openaicompat.Client
	reasoningMode    openaicompat.ReasoningMode
	effortDialect    openaicompat.EffortDialect
	defaultReasoning *llm.ReasoningRequest
}

// Option configures the Client.
type Option func(*clientOpts)

type clientOpts struct {
	httpClient         *http.Client
	capabilityOverride string
	azureAPIVersion    string
	defaultReasoning   *llm.ReasoningRequest
}

// WithHTTPClient injects a custom HTTP client for transport chain support
// (TLS CA, OAuth2, custom headers, circuit breaker — issue #1342).
func WithHTTPClient(c *http.Client) Option {
	return func(o *clientOpts) { o.httpClient = c }
}

// WithHTTPTimeout sets a request timeout on a default HTTP client. Ignored
// if WithHTTPClient is also supplied (that client's own timeout applies).
func WithHTTPTimeout(d time.Duration) Option {
	return func(o *clientOpts) {
		if o.httpClient == nil {
			o.httpClient = &http.Client{Timeout: d}
		}
	}
}

// WithCapabilityOverride short-circuits model-name-based reasoning-mode
// auto-detection (shared/types.LLMReasoningConfig.CapabilityOverride) — the
// escape hatch for self-hosted/custom models that can't be reliably
// identified by name pattern alone (BR-AI-086 AC5).
func WithCapabilityOverride(override string) Option {
	return func(o *clientOpts) { o.capabilityOverride = override }
}

// WithAzureAPIVersion switches this client into Azure OpenAI mode (#1600):
// the underlying openaicompat.Client uses Azure's deployment-scoped URL
// (model doubles as deployment ID) and api-key auth instead of the flat
// OpenAI path and Bearer auth. See openaicompat.WithAzureAPIVersion.
func WithAzureAPIVersion(apiVersion string) Option {
	return func(o *clientOpts) { o.azureAPIVersion = apiVersion }
}

// WithReasoning sets the construction-time default reasoning/effort request,
// applied whenever a per-call req.Options.Reasoning is nil (#1604, mirroring
// anthropicfamily.WithReasoning for cross-family symmetry). Unlike the
// Anthropic family, this controls only the request-side Effort knob
// ("reasoning_effort") — reasoning-content capture for this family remains
// unconditional and model-driven (BR-AI-086 AC3), independent of this
// option.
func WithReasoning(r llm.ReasoningRequest) Option {
	return func(o *clientOpts) { o.defaultReasoning = &r }
}

// New creates a Client for the given model and OpenAI-Chat-Completions-
// compatible endpoint. The reasoning round-trip mode is auto-detected from
// model (BR-AI-086, DD-LLM-005) unless overridden.
func New(model, endpoint, apiKey string, opts ...Option) *Client {
	o := &clientOpts{}
	for _, fn := range opts {
		fn(o)
	}

	var compatOpts []openaicompat.Option
	if o.httpClient != nil {
		compatOpts = append(compatOpts, openaicompat.WithHTTPClient(o.httpClient))
	}
	if o.azureAPIVersion != "" {
		compatOpts = append(compatOpts, openaicompat.WithAzureAPIVersion(o.azureAPIVersion))
	}

	return &Client{
		client:           openaicompat.New(model, endpoint, apiKey, compatOpts...),
		reasoningMode:    openaicompat.DetectReasoningMode(model, o.capabilityOverride),
		effortDialect:    openaicompat.DetectEffortDialect(model),
		defaultReasoning: o.defaultReasoning,
	}
}

// Chat translates a Kubernaut ChatRequest to the shared Request, calls the
// shared client, and maps the response back.
func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	resp, err := c.client.Chat(ctx, c.buildRequest(req))
	if err != nil {
		return llm.ChatResponse{}, classifyErr(err)
	}
	return convertResponse(resp), nil
}

// classifyErr marks err non-retryable (kubernaut#1585) when it unwraps to
// the shared openaicompat client's typed *openaicompat.APIError with a
// permanent (400/401/403/404-class) StatusCode. Classification happens
// here, at this package's own translation boundary, rather than in the
// shared llm package, so the generic retry machinery never needs to
// import a provider-specific error shape (DD-HAPI-019 Framework
// Isolation).
func classifyErr(err error) error {
	var apiErr *openaicompat.APIError
	if errors.As(err, &apiErr) && llm.IsNonRetryableHTTPStatus(apiErr.StatusCode) {
		return llm.MarkNonRetryable(err)
	}
	return err
}

// StreamChat streams the response, forwarding text deltas via callback and
// building the final ChatResponse from the accumulated result.
func (c *Client) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	var final llm.ChatResponse
	var callbackErr error

	err := c.client.StreamChat(ctx, c.buildRequest(req), func(ev openaicompat.StreamEvent) bool {
		if ev.Delta != "" {
			if callbackErr = callback(llm.ChatStreamEvent{Delta: ev.Delta}); callbackErr != nil {
				return false
			}
		}
		if ev.Done && ev.Final != nil {
			final = convertResponse(ev.Final)
			_ = callback(llm.ChatStreamEvent{Done: true})
		}
		return true
	})
	if callbackErr != nil {
		return llm.ChatResponse{}, callbackErr
	}
	if err != nil {
		return llm.ChatResponse{}, classifyErr(err)
	}
	return final, nil
}

// Close is a no-op: the underlying openaicompat.Client holds no closeable
// resources beyond the standard library's HTTP connection pooling.
// Satisfies llm.Client.
func (c *Client) Close() error { return nil }

var _ llm.Client = (*Client)(nil)

// buildRequest translates a Kubernaut ChatRequest into the shared Request.
func (c *Client) buildRequest(req llm.ChatRequest) openaicompat.Request {
	out := openaicompat.Request{
		ReasoningMode: c.reasoningMode,
		MaxTokens:     req.Options.MaxTokens,
	}
	if req.Options.Temperature != nil {
		out.Temperature = req.Options.Temperature
	}
	out.Messages = make([]openaicompat.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, convertMessage(m))
	}
	for _, td := range req.Tools {
		out.Tools = append(out.Tools, openaicompat.ToolDefinition{
			Name:        td.Name,
			Description: td.Description,
			Parameters:  td.Parameters,
		})
	}
	if len(req.Options.OutputSchema) > 0 {
		out.ResponseSchema = req.Options.OutputSchema
	}

	reasoning := req.Options.Reasoning
	if reasoning == nil {
		reasoning = c.defaultReasoning
	}
	if reasoning != nil && reasoning.Enabled {
		out.Effort = reasoning.Effort
		out.EffortDialect = c.effortDialect
	}
	return out
}

// convertMessage translates a Kubernaut Message into the shared Message,
// flattening ReasoningBlock down to the shared package's plain-text
// Reasoning field. Redacted reasoning has no OpenAI-protocol equivalent
// (only Anthropic's API distinguishes visible vs. opaque blocks), so a
// redacted block's Signature — the only field it carries — is passed
// through as-is; providers reached via this client never produce redacted
// blocks in the first place.
func convertMessage(m llm.Message) openaicompat.Message {
	out := openaicompat.Message{
		Role:       m.Role,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
	}
	for _, tc := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, openaicompat.ToolCall{
			ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments,
		})
	}
	if m.Reasoning != nil {
		if m.Reasoning.Redacted {
			out.Reasoning = m.Reasoning.Signature
		} else {
			out.Reasoning = m.Reasoning.Text
		}
	}
	return out
}

// convertResponse translates a shared Response into a Kubernaut ChatResponse.
func convertResponse(resp *openaicompat.Response) llm.ChatResponse {
	out := llm.ChatResponse{
		Message: llm.Message{
			Role:      "assistant",
			Content:   resp.Message.Content,
			ToolCalls: convertToolCalls(resp.Message.ToolCalls),
		},
		ToolCalls: convertToolCalls(resp.Message.ToolCalls),
		Usage: llm.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		FinishReason: normalizeFinishReason(resp.FinishReason),
	}
	if resp.Message.Reasoning != "" {
		out.Message.Reasoning = &llm.ReasoningBlock{Text: resp.Message.Reasoning}
	}
	return out
}

func convertToolCalls(calls []openaicompat.ToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for _, tc := range calls {
		out = append(out, llm.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

// normalizeFinishReason maps the shared client's raw OpenAI-protocol
// finish_reason string to Kubernaut's canonical FinishReason constants.
func normalizeFinishReason(raw string) string {
	switch raw {
	case openaicompat.FinishReasonStop:
		return llm.FinishReasonStop
	case openaicompat.FinishReasonLength:
		return llm.FinishReasonLength
	case openaicompat.FinishReasonToolCalls:
		return llm.FinishReasonToolCalls
	default:
		if raw != "" {
			return raw
		}
		return llm.FinishReasonStop
	}
}
