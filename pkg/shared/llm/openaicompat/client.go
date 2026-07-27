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

package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client sends Chat Completions requests to any OpenAI-protocol-compatible
// endpoint (OpenAI itself, Azure OpenAI, Ollama, vLLM, LlamaStack, Mistral,
// HuggingFace TGI, DeepSeek, Bedrock's OpenAI-compatible surface, or an
// arbitrary self-hosted/custom endpoint).
type Client struct {
	model      string
	endpoint   string
	apiKey     string
	httpClient *http.Client
	// azureAPIVersion, when non-empty, switches do() from the flat
	// /chat/completions path + Bearer auth every other provider in this
	// package's doc comment uses, to Azure OpenAI's own deployment-scoped
	// URL + api-key header (#1600). Resolved once via WithAzureAPIVersion at
	// client-construction time, the same pattern as ReasoningMode/
	// EffortDialect.
	azureAPIVersion string
}

// Option configures the Client.
type Option func(*Client)

// WithHTTPClient injects a custom HTTP client for transport chain support
// (TLS CA, OAuth2, custom headers, circuit breaker — issue #1342).
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithAzureAPIVersion switches this client into Azure OpenAI mode (#1600):
// requests go to Azure's deployment-scoped URL (using the model name as the
// deployment ID, per Azure's own convention — this package has no separate
// deployment-ID concept) with the given api-version query parameter, and
// authenticate via the api-key header instead of Authorization: Bearer.
func WithAzureAPIVersion(apiVersion string) Option {
	return func(cl *Client) { cl.azureAPIVersion = apiVersion }
}

// New creates a Client for the given model and Chat-Completions-compatible
// endpoint. apiKey may be empty for endpoints that don't require auth
// (many local self-hosted deployments).
func New(model, endpoint, apiKey string, opts ...Option) *Client {
	c := &Client{
		model:    model,
		endpoint: strings.TrimSuffix(endpoint, "/"),
		apiKey:   apiKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}
	return c
}

// Chat sends a non-streaming Chat Completions request and maps the response.
func (c *Client) Chat(ctx context.Context, req Request) (*Response, error) {
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var wire chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("openaicompat: decode response: %w", err)
	}
	return mapResponse(&wire), nil
}

// StreamChat sends a streaming Chat Completions request, invoking yield for
// each text delta and once more with the final accumulated Response.
// Returning false from yield stops consuming the stream.
func (c *Client) StreamChat(ctx context.Context, req Request, yield func(StreamEvent) bool) error {
	resp, err := c.do(ctx, req, true)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return streamResponse(resp.Body, yield)
}

// do builds and sends the HTTP request, returning the raw response after
// validating the status code.
func (c *Client) do(ctx context.Context, req Request, stream bool) (*http.Response, error) {
	body := buildRequestBody(c.model, req, stream)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}

	reqURL, authHeader, authValue := c.requestTarget()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if authValue != "" {
		httpReq.Header.Set(authHeader, authValue)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}
	return resp, nil
}

// APIError represents a non-2xx HTTP response from the compat endpoint. It
// carries the status code programmatically so callers can classify
// retryable (429, 5xx) vs. non-retryable (400, 401, 403, 404) failures
// instead of parsing the error string (kubernaut#1585).
type APIError struct {
	StatusCode int
	Body       string
}

// Error preserves the exact message format the bare fmt.Errorf previously
// produced, so existing logs/tests that only inspect the string are
// unaffected by this type's introduction.
func (e *APIError) Error() string {
	return fmt.Sprintf("openaicompat: API error (HTTP %d): %s", e.StatusCode, e.Body)
}

// requestTarget returns the request URL and auth header name/value for this
// client's configured mode. Azure mode (#1600) uses a deployment-scoped
// path — the model name doubles as the deployment ID, Azure's own
// convention — plus a mandatory api-version query parameter, and an
// api-key header; every other provider uses the flat /chat/completions
// path and Authorization: Bearer. In both modes, an empty apiKey yields an
// empty authValue, so no auth header is sent at all — the same "no header
// rather than an empty one" behavior this package already had for Bearer,
// preserved here so an operator using a transport-layer auth wrapper
// (e.g. Azure AD/Entra Bearer tokens, out of scope for this package — see
// #1600's issue) never has this client's header collide with theirs.
func (c *Client) requestTarget() (reqURL, authHeader, authValue string) {
	if c.azureAPIVersion != "" {
		u := fmt.Sprintf("%s/openai/deployments/%s/chat/completions",
			c.endpoint, url.PathEscape(c.model))
		q := url.Values{"api-version": {c.azureAPIVersion}}
		return u + "?" + q.Encode(), "api-key", c.apiKey
	}
	if c.apiKey == "" {
		return c.endpoint + "/chat/completions", "Authorization", ""
	}
	return c.endpoint + "/chat/completions", "Authorization", "Bearer " + c.apiKey
}

// buildRequestBody assembles the JSON request body, applying generation
// config and reasoning-replay rules (per req.ReasoningMode) to the message
// history.
func buildRequestBody(model string, req Request, stream bool) map[string]any {
	body := map[string]any{
		"model":  model,
		"stream": stream,
	}
	body["messages"] = buildMessages(req.Messages, req.ReasoningMode)

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.StopSequences) > 0 {
		body["stop"] = req.StopSequences
	}
	if len(req.Tools) > 0 {
		body["tools"] = buildTools(req.Tools)
	}
	if len(req.ResponseSchema) > 0 {
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": req.ResponseSchema,
		}
	}
	applyEffort(body, req.Effort, req.EffortDialect)
	return body
}

// applyEffort maps the canonical, provider-agnostic Effort value onto the
// wire dialect the model actually speaks (#1604). A no-op — no field is
// added at all — when Effort is empty (the provider's own vendor default
// applies) or EffortDialect is EffortDialectNone (the model has no
// recognized effort knob; never send a speculative field a bare-bones
// server might reject, the same compatibility-floor principle as
// DetectReasoningMode).
func applyEffort(body map[string]any, effort string, dialect EffortDialect) {
	if effort == "" {
		return
	}
	switch dialect {
	case EffortDialectOpenAI:
		body["reasoning_effort"] = effort
	case EffortDialectDeepSeek:
		applyDeepSeekEffort(body, effort)
	}
}

// deepSeekEffortTiers downscales the canonical 6-value vocabulary onto
// DeepSeek's own 2-tier dialect, per DeepSeek's published compatibility
// mapping (https://api-docs.deepseek.com/guides/thinking_mode): low/medium
// map up to high (DeepSeek's floor), xhigh maps to max (DeepSeek's
// ceiling). "none" is handled separately below — it disables thinking
// entirely rather than mapping to a low tier.
var deepSeekEffortTiers = map[string]string{
	"minimal": "high",
	"low":     "high",
	"medium":  "high",
	"high":    "high",
	"xhigh":   "max",
}

func applyDeepSeekEffort(body map[string]any, effort string) {
	if effort == "none" {
		body["thinking"] = map[string]any{"type": "disabled"}
		return
	}
	if wireEffort, ok := deepSeekEffortTiers[effort]; ok {
		body["reasoning_effort"] = wireEffort
		body["thinking"] = map[string]any{"type": "enabled"}
	}
}

// buildMessages translates the shared Message list into the OpenAI wire
// shape, replaying each message's captured Reasoning only when
// shouldReplayReasoning approves it for that message's ReasoningMode and
// tool-call state (BR-AI-086 req 3 / #1578).
func buildMessages(messages []Message, mode ReasoningMode) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		msg := map[string]any{"role": m.Role}
		if m.Content != "" {
			msg["content"] = m.Content
		}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = buildOutboundToolCalls(m.ToolCalls)
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if m.Reasoning != "" && shouldReplayReasoning(mode, len(m.ToolCalls) > 0) {
			msg["reasoning_content"] = m.Reasoning
		}
		out = append(out, msg)
	}
	return out
}

func buildOutboundToolCalls(calls []ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, tc := range calls {
		out = append(out, map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}
	return out
}

func buildTools(defs []ToolDefinition) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
	for _, td := range defs {
		fn := map[string]any{
			"name":        td.Name,
			"description": td.Description,
		}
		if len(td.Parameters) > 0 {
			fn["parameters"] = td.Parameters
		}
		tools = append(tools, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	return tools
}
