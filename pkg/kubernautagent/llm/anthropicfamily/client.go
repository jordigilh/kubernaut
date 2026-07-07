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

// Package anthropicfamily implements llm.Client for Claude models across
// the Anthropic model family's auth modes, using the official Anthropic Go
// SDK: Google Vertex AI (New) and the native Anthropic API (NewWithAPIKey).
// Bedrock auth mode is planned separately (issue #1582, DD-LLM-006).
//
// The SDK's vertex package handles all Vertex-specific protocol differences
// automatically: anthropic_version in the request body, model removal from
// the body, URL rewriting to rawPredict, and global/multi-region endpoints.
//
// Structured output (output_config) is NOT supported on Vertex AI per
// official Anthropic docs — this adapter does not attempt to set it.
//
// Model-aware reasoning/thinking token support (BR-AI-086) is shared across
// all auth modes: buildParams/mapResponse/convertAssistantMessage are auth
// mode-agnostic. Thinking-tier detection (adaptive vs. manual-budget-only)
// reuses adk-anthropic-go/converters.ThinkingConfigToAnthropic — the same
// logic the API Frontend uses — rather than an independently-maintained
// second copy (DD-LLM-005).
//
// Reference: https://docs.anthropic.com/en/api/claude-on-vertex-ai
// Reference: https://github.com/anthropics/anthropic-sdk-go
package anthropicfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Alcova-AI/adk-anthropic-go/converters"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Option configures the Client.
type Option func(*clientOpts)

type clientOpts struct {
	extraSDKOpts  []option.RequestOption
	logger        logr.Logger
	httpTimeout   time.Duration
	baseTransport http.RoundTripper
}

// WithSDKOptions injects additional Anthropic SDK request options (e.g. base URL
// override for testing). Production code should not need this.
func WithSDKOptions(opts ...option.RequestOption) Option {
	return func(o *clientOpts) { o.extraSDKOpts = append(o.extraSDKOpts, opts...) }
}

// WithLogger injects a logr.Logger for diagnostic messages (e.g., malformed tool schemas).
// If not provided, logging is silently discarded.
func WithLogger(l logr.Logger) Option {
	return func(o *clientOpts) { o.logger = l }
}

// WithHTTPTimeout sets an explicit timeout on the underlying HTTP client used
// by the Anthropic SDK, preventing unbounded network calls (#956).
func WithHTTPTimeout(d time.Duration) Option {
	return func(o *clientOpts) { o.httpTimeout = d }
}

// WithBaseTransport sets a custom base RoundTripper (e.g. mTLS, circuit breaker,
// custom headers) that will be wrapped with GCP OAuth2 authentication.
// The SDK's vertex middleware (URL rewriting, anthropic_version) is preserved;
// only the HTTP client is replaced with one that layers OAuth2 on top of this
// transport. Issue #1342: enterprise mTLS for LLM gateways.
func WithBaseTransport(rt http.RoundTripper) Option {
	return func(o *clientOpts) { o.baseTransport = rt }
}

// Client implements llm.Client for Claude on Vertex AI using the official
// Anthropic Go SDK with the vertex middleware.
type Client struct {
	sdk    anthropic.Client
	model  string
	logger logr.Logger
}

// New creates a Client for Claude on Vertex AI.
//
// credentialsJSON holds the GCP service account or authorized_user JSON,
// resolved at runtime from the Helm-mounted credentials directory.
// If empty, ambient Application Default Credentials (ADC) are used.
//
// The SDK's vertex package automatically handles:
//   - anthropic_version: "vertex-2023-10-16" in the request body
//   - model removed from body (placed in the rawPredict URL)
//   - global/us/eu multi-region endpoint routing
//   - GCP OAuth2 Bearer token transport
func New(ctx context.Context, model string, credentialsJSON []byte, project, location string, opts ...Option) (*Client, error) {
	if project == "" {
		return nil, fmt.Errorf("anthropicfamily: project is required (vertex_project config)")
	}
	if location == "" {
		location = "us-central1"
	}

	o := newClientOpts(opts...)

	vertexOpt, tokenSource, err := resolveVertexAuth(ctx, credentialsJSON, project, location)
	if err != nil {
		return nil, err
	}

	sdk := anthropic.NewClient(buildSDKOptions(o, vertexOpt, tokenSource)...)
	return &Client{sdk: sdk, model: model, logger: o.logger}, nil
}

// NewWithAPIKey creates a Client for Claude via the native Anthropic API
// (api.anthropic.com), authenticating with a static API key rather than
// GCP Vertex AI credentials. Distinct from New (Vertex-only) so that
// Vertex's required project/location parameters are never mistakenly
// requested for this auth mode.
func NewWithAPIKey(apiKey, model string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropicfamily: apiKey is required")
	}

	o := newClientOpts(opts...)
	sdkOpts := applyHTTPOptions([]option.RequestOption{option.WithAPIKey(apiKey)}, o, nil)
	sdkOpts = append(sdkOpts, o.extraSDKOpts...)

	sdk := anthropic.NewClient(sdkOpts...)
	return &Client{sdk: sdk, model: model, logger: o.logger}, nil
}

// newClientOpts applies functional Options over a clientOpts pre-populated
// with defaults, shared by every auth-mode constructor (New, NewWithAPIKey,
// and — per DD-LLM-006 — the planned Bedrock constructor).
func newClientOpts(opts ...Option) *clientOpts {
	o := &clientOpts{logger: logr.Discard()}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

// resolveVertexAuth resolves the Vertex AI SDK request option and OAuth2
// token source: explicit credentialsJSON when non-empty, otherwise ambient
// Application Default Credentials (ADC).
func resolveVertexAuth(ctx context.Context, credentialsJSON []byte, project, location string) (option.RequestOption, oauth2.TokenSource, error) {
	trimmed := bytes.TrimSpace(credentialsJSON)
	if len(trimmed) == 0 {
		return resolveADCAuth(ctx, project, location)
	}

	credType, err := validateCredentialType(trimmed)
	if err != nil {
		return nil, nil, err
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, trimmed, credType,
		"https://www.googleapis.com/auth/cloud-platform",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("anthropicfamily: invalid credentials JSON: %w", err)
	}
	return vertex.WithCredentials(ctx, location, project, creds), creds.TokenSource, nil
}

// resolveADCAuth resolves ambient Application Default Credentials for
// Vertex AI. The token source is best-effort: a FindDefaultCredentials
// failure does not abort New, it simply leaves tokenSource nil.
func resolveADCAuth(ctx context.Context, project, location string) (option.RequestOption, oauth2.TokenSource, error) {
	vertexOpt, err := safeWithGoogleAuth(ctx, location, project)
	if err != nil {
		return nil, nil, err
	}
	var tokenSource oauth2.TokenSource
	if adcCreds, credErr := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/cloud-platform"); credErr == nil {
		tokenSource = adcCreds.TokenSource
	}
	return vertexOpt, tokenSource, nil
}

// buildSDKOptions assembles the Anthropic SDK request options: the Vertex
// auth option, an optional request timeout, an optional OAuth2-wrapped
// custom transport (#1342 enterprise mTLS), and any extra caller-supplied
// SDK options.
func buildSDKOptions(o *clientOpts, vertexOpt option.RequestOption, tokenSource oauth2.TokenSource) []option.RequestOption {
	sdkOpts := applyHTTPOptions([]option.RequestOption{vertexOpt}, o, tokenSource)
	return append(sdkOpts, o.extraSDKOpts...)
}

// applyHTTPOptions appends the request-timeout and custom-transport SDK
// options shared across every anthropicfamily auth-mode constructor.
//
// When tokenSource is non-nil (Vertex auth), a caller-supplied baseTransport
// is wrapped with OAuth2 Bearer token injection (#1342 enterprise mTLS).
// When tokenSource is nil (native API-key auth, and — per DD-LLM-006 — the
// planned Bedrock SigV4 auth), baseTransport is used as-is: authentication
// for those modes is carried by a request header or signature, not a
// Vertex-style bearer-token transport.
func applyHTTPOptions(sdkOpts []option.RequestOption, o *clientOpts, tokenSource oauth2.TokenSource) []option.RequestOption {
	if o.httpTimeout > 0 {
		sdkOpts = append(sdkOpts, option.WithRequestTimeout(o.httpTimeout))
	}
	if o.baseTransport == nil {
		return sdkOpts
	}
	transport := o.baseTransport
	if tokenSource != nil {
		transport = &oauth2.Transport{Base: o.baseTransport, Source: tokenSource}
	}
	return append(sdkOpts, option.WithHTTPClient(&http.Client{
		Transport: transport,
		Timeout:   o.httpTimeout,
	}))
}

// Chat translates a Kubernaut ChatRequest to the Anthropic Messages API,
// calls the SDK, and maps the response back.
func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	params := c.buildParams(req)

	msg, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("anthropicfamily: %w", err)
	}

	return c.mapResponse(msg), nil
}

func (c *Client) buildParams(req llm.ChatRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(4096),
	}

	if req.Options.MaxTokens > 0 {
		params.MaxTokens = int64(req.Options.MaxTokens)
	}
	if req.Options.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Options.Temperature)
	}

	system, messages := convertMessagesToAnthropic(req.Messages)
	params.System = system
	params.Messages = messages

	if len(req.Tools) > 0 {
		params.Tools = buildAnthropicTools(req.Tools, c.logger)
	}

	if req.Options.Reasoning != nil && req.Options.Reasoning.Enabled {
		params.Thinking = resolveThinkingParam(req.Options.Reasoning, anthropic.Model(c.model))
	}

	return params
}

// resolveThinkingParam maps a KA ReasoningRequest to the Anthropic thinking
// parameter, delegating model-tier detection (adaptive-capable vs
// manual-budget-only) to adk-anthropic-go/converters — the same logic AF
// uses, avoiding a second, independently-maintained tier-detection table
// (DD-LLM-005). An explicit BudgetTokens always wins with a manual budget
// regardless of tier; omitting it lets the per-tier default apply (adaptive
// on adaptive-capable models, a high-effort manual budget otherwise).
func resolveThinkingParam(r *llm.ReasoningRequest, model anthropic.Model) anthropic.ThinkingConfigParamUnion {
	cfg := &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelHigh}
	if r.BudgetTokens > 0 {
		budget := int32(r.BudgetTokens)
		cfg = &genai.ThinkingConfig{ThinkingBudget: &budget}
	}
	return converters.ThinkingConfigToAnthropic(cfg, model).Thinking
}

// convertMessagesToAnthropic translates Kubernaut's role-tagged message
// history into the Anthropic Messages API's system block + message list.
// Consecutive "tool" messages are buffered and flushed as a single user
// message (Anthropic requires all tool_result blocks for one assistant turn
// to be grouped into one user message), flushing whenever a non-tool message
// is encountered or at the end of the history.
func convertMessagesToAnthropic(messages []llm.Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var system []anthropic.TextBlockParam
	var msgs []anthropic.MessageParam

	var pendingToolResults []anthropic.ContentBlockParamUnion
	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			msgs = append(msgs, anthropic.NewUserMessage(pendingToolResults...))
			pendingToolResults = nil
		}
	}

	for _, m := range messages {
		if m.Role != "tool" {
			flushToolResults()
		}
		switch m.Role {
		case "system":
			system = []anthropic.TextBlockParam{
				{Text: m.Content},
			}
		case "user":
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			if am, ok := convertAssistantMessage(m); ok {
				msgs = append(msgs, am)
			}
		case "tool":
			pendingToolResults = append(pendingToolResults,
				anthropic.NewToolResultBlock(m.ToolCallID, m.Content, false))
		}
	}
	flushToolResults()

	return system, msgs
}

// convertAssistantMessage builds the Anthropic assistant message for a
// single Kubernaut assistant-role message (thinking, text, and/or tool_use
// blocks). Returns ok=false when there is nothing to emit (no content, no
// tool calls, no reasoning).
//
// A reasoning block, when present, is always placed FIRST in the content
// array — the Anthropic API requires the thinking/redacted_thinking block
// that preceded a tool_use to be replayed before it on the next turn (same
// failure class as issue #1299: orphaned content blocks on replay).
func convertAssistantMessage(m llm.Message) (anthropic.MessageParam, bool) {
	var parts []anthropic.ContentBlockParamUnion
	if m.Reasoning != nil {
		parts = append(parts, reasoningToContentBlock(m.Reasoning))
	}
	if len(m.ToolCalls) > 0 {
		if m.Content != "" {
			parts = append(parts, anthropic.NewTextBlock(m.Content))
		}
		for _, tc := range m.ToolCalls {
			var input any
			if tc.Arguments != "" {
				input = json.RawMessage(tc.Arguments)
			} else {
				input = json.RawMessage("{}")
			}
			parts = append(parts, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
		}
		return anthropic.NewAssistantMessage(parts...), true
	}
	if m.Content != "" {
		parts = append(parts, anthropic.NewTextBlock(m.Content))
		return anthropic.NewAssistantMessage(parts...), true
	}
	if len(parts) > 0 {
		return anthropic.NewAssistantMessage(parts...), true
	}
	return anthropic.MessageParam{}, false
}

// reasoningToContentBlock converts a captured ReasoningBlock back into the
// Anthropic content block union used for replay: a visible ThinkingBlockParam
// when not redacted, or an opaque RedactedThinkingBlockParam otherwise. Both
// must be replayed byte-for-byte — never inspected or modified.
func reasoningToContentBlock(r *llm.ReasoningBlock) anthropic.ContentBlockParamUnion {
	if r.Redacted {
		return anthropic.ContentBlockParamUnion{
			OfRedactedThinking: &anthropic.RedactedThinkingBlockParam{Data: r.Signature},
		}
	}
	return anthropic.ContentBlockParamUnion{
		OfThinking: &anthropic.ThinkingBlockParam{
			Signature: r.Signature,
			Thinking:  r.Text,
		},
	}
}

// buildAnthropicTools translates Kubernaut's provider-agnostic tool
// definitions into Anthropic's ToolUnionParam list, tolerating malformed
// parameter schemas (logged, falls back to an empty schema) rather than
// failing the whole request.
func buildAnthropicTools(toolDefs []llm.ToolDefinition, logger logr.Logger) []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(toolDefs))
	for _, td := range toolDefs {
		schema := parseInputSchema(td.Parameters, logger)
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        td.Name,
				Description: anthropic.String(td.Description),
				InputSchema: schema,
			},
		})
	}
	return tools
}

func parseInputSchema(raw json.RawMessage, logger logr.Logger) anthropic.ToolInputSchemaParam {
	var s struct {
		Properties any      `json:"properties"`
		Required   []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		logger.Info("anthropicfamily: malformed tool parameter schema, using empty schema",
			"error", err.Error())
	}
	return anthropic.ToolInputSchemaParam{
		Properties: s.Properties,
		Required:   s.Required,
	}
}

func (c *Client) mapResponse(msg *anthropic.Message) llm.ChatResponse {
	resp := llm.ChatResponse{
		Message: llm.Message{
			Role: "assistant",
		},
		Usage: llm.TokenUsage{
			PromptTokens:     int(msg.Usage.InputTokens),
			CompletionTokens: int(msg.Usage.OutputTokens),
			TotalTokens:      int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
		},
		FinishReason: normalizeAnthropicStopReason(string(msg.StopReason)),
	}

	var textParts []string
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			tu := block.AsToolUse()
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:        tu.ID,
				Name:      tu.Name,
				Arguments: string(tu.Input),
			})
		case "thinking":
			tb := block.AsThinking()
			resp.Message.Reasoning = &llm.ReasoningBlock{
				Text:      tb.Thinking,
				Signature: tb.Signature,
			}
		case "redacted_thinking":
			rtb := block.AsRedactedThinking()
			resp.Message.Reasoning = &llm.ReasoningBlock{
				Signature: rtb.Data,
				Redacted:  true,
			}
		}
	}
	resp.Message.Content = strings.Join(textParts, "")
	resp.Message.ToolCalls = resp.ToolCalls

	return resp
}

// normalizeAnthropicStopReason maps Anthropic's stop_reason values to our
// canonical FinishReason constants.
func normalizeAnthropicStopReason(raw string) string {
	switch raw {
	case "end_turn", "stop_sequence":
		return llm.FinishReasonStop
	case "max_tokens":
		return llm.FinishReasonLength
	case "tool_use":
		return llm.FinishReasonToolCalls
	default:
		if raw != "" {
			return raw
		}
		return llm.FinishReasonStop
	}
}

// allowedCredentialTypes lists the GCP credential types that Kubernaut accepts.
// external_account and similar types are rejected to prevent loading credentials
// with attacker-controlled token endpoints (SA1019 mitigation).
var allowedCredentialTypes = map[google.CredentialsType]bool{
	google.ServiceAccount: true,
	google.AuthorizedUser: true,
}

// validateCredentialType parses the "type" field from the credential JSON and
// rejects any type not in the allow-list. This replaces the deprecated
// google.CredentialsFromJSON which loaded any credential type without
// validation (staticcheck SA1019).
func validateCredentialType(jsonData []byte) (google.CredentialsType, error) {
	var f struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(jsonData, &f); err != nil {
		return "", fmt.Errorf("anthropicfamily: invalid credentials JSON: %w", err)
	}
	ct := google.CredentialsType(f.Type)
	if !allowedCredentialTypes[ct] {
		return "", fmt.Errorf("anthropicfamily: unsupported credential type %q; only service_account and authorized_user are accepted", f.Type)
	}
	return ct, nil
}

// safeWithGoogleAuth wraps vertex.WithGoogleAuth with panic recovery because
// the SDK panics (rather than returning an error) when GCP Application Default
// Credentials are unavailable.
func safeWithGoogleAuth(ctx context.Context, location, project string) (opt option.RequestOption, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("anthropicfamily: GCP ADC unavailable — set GOOGLE_APPLICATION_CREDENTIALS or provide explicit credentials JSON: %v", r)
		}
	}()
	opt = vertex.WithGoogleAuth(ctx, location, project,
		"https://www.googleapis.com/auth/cloud-platform")
	return opt, nil
}

// StreamChat uses the Anthropic SDK's Messages.NewStreaming to deliver text
// deltas incrementally. The final ChatResponse is built from the accumulated
// message, reusing the existing mapResponse path.
func (c *Client) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	params := c.buildParams(req)
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	acc := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := acc.Accumulate(event); err != nil {
			return llm.ChatResponse{}, fmt.Errorf("anthropicfamily: accumulate error: %w", err)
		}
		if delta, ok := extractTextDelta(event); ok && delta != "" {
			if err := callback(llm.ChatStreamEvent{Delta: delta}); err != nil {
				return llm.ChatResponse{}, err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("anthropicfamily: stream error: %w", err)
	}
	_ = callback(llm.ChatStreamEvent{Done: true})
	return c.mapResponse(&acc), nil
}

// extractTextDelta extracts the text delta from a content_block_delta event.
// Returns ("", false) for non-delta events or non-text deltas (e.g., tool input).
func extractTextDelta(event anthropic.MessageStreamEventUnion) (string, bool) {
	if event.Type != "content_block_delta" {
		return "", false
	}
	if event.Delta.Text != "" {
		return event.Delta.Text, true
	}
	return "", false
}

// Close is a no-op for the Anthropic SDK client which has no closeable
// resources. Satisfies llm.Client.
func (c *Client) Close() error { return nil }

var _ llm.Client = (*Client)(nil)
