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

// Package vertexanthropic implements llm.Client for Claude models hosted on
// Google Vertex AI using the official Anthropic Go SDK.
//
// The SDK's vertex package handles all Vertex-specific protocol differences
// automatically: anthropic_version in the request body, model removal from
// the body, URL rewriting to rawPredict, and global/multi-region endpoints.
//
// Structured output (output_config) is NOT supported on Vertex AI per
// official Anthropic docs — this adapter does not attempt to set it.
//
// Reference: https://docs.anthropic.com/en/api/claude-on-vertex-ai
// Reference: https://github.com/anthropics/anthropic-sdk-go
package vertexanthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"golang.org/x/oauth2/google"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Option configures the Client.
type Option func(*clientOpts)

type clientOpts struct {
	extraSDKOpts []option.RequestOption
}

// WithSDKOptions injects additional Anthropic SDK request options (e.g. base URL
// override for testing). Production code should not need this.
func WithSDKOptions(opts ...option.RequestOption) Option {
	return func(o *clientOpts) { o.extraSDKOpts = append(o.extraSDKOpts, opts...) }
}

// Client implements llm.Client for Claude on Vertex AI using the official
// Anthropic Go SDK with the vertex middleware.
type Client struct {
	sdk   anthropic.Client
	model string
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
		return nil, fmt.Errorf("vertexanthropic: project is required (vertex_project config)")
	}
	if location == "" {
		location = "us-central1"
	}

	o := &clientOpts{}
	for _, fn := range opts {
		fn(o)
	}

	var vertexOpt option.RequestOption

	trimmed := bytes.TrimSpace(credentialsJSON)
	if len(trimmed) > 0 {
		credType, err := validateCredentialType(trimmed)
		if err != nil {
			return nil, err
		}
		creds, err := google.CredentialsFromJSONWithType(ctx, trimmed, credType,
			"https://www.googleapis.com/auth/cloud-platform",
		)
		if err != nil {
			return nil, fmt.Errorf("vertexanthropic: invalid credentials JSON: %w", err)
		}
		vertexOpt = vertex.WithCredentials(ctx, location, project, creds)
	} else {
		var adcErr error
		vertexOpt, adcErr = safeWithGoogleAuth(ctx, location, project)
		if adcErr != nil {
			return nil, adcErr
		}
	}

	sdkOpts := []option.RequestOption{vertexOpt}
	sdkOpts = append(sdkOpts, o.extraSDKOpts...)
	sdk := anthropic.NewClient(sdkOpts...)

	return &Client{sdk: sdk, model: model}, nil
}

// Chat translates a Kubernaut ChatRequest to the Anthropic Messages API,
// calls the SDK, and maps the response back.
func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	params := c.buildParams(req)

	msg, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("vertexanthropic: %w", err)
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
	if req.Options.Temperature > 0 {
		params.Temperature = anthropic.Float(req.Options.Temperature)
	}

	var pendingToolResults []anthropic.ContentBlockParamUnion
	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			params.Messages = append(params.Messages,
				anthropic.NewUserMessage(pendingToolResults...))
			pendingToolResults = nil
		}
	}

	for _, m := range req.Messages {
		if m.Role != "tool" {
			flushToolResults()
		}
		switch m.Role {
		case "system":
			params.System = []anthropic.TextBlockParam{
				{Text: m.Content},
			}
		case "user":
			params.Messages = append(params.Messages,
				anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var parts []anthropic.ContentBlockParamUnion
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
				params.Messages = append(params.Messages,
					anthropic.NewAssistantMessage(parts...))
			} else {
				params.Messages = append(params.Messages,
					anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
			}
		case "tool":
			pendingToolResults = append(pendingToolResults,
				anthropic.NewToolResultBlock(m.ToolCallID, m.Content, false))
		}
	}
	flushToolResults()

	if len(req.Tools) > 0 {
		var tools []anthropic.ToolUnionParam
		for _, td := range req.Tools {
			schema := parseInputSchema(td.Parameters)
			tools = append(tools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        td.Name,
					Description: anthropic.String(td.Description),
					InputSchema: schema,
				},
			})
		}
		params.Tools = tools
	}

	return params
}

func parseInputSchema(raw json.RawMessage) anthropic.ToolInputSchemaParam {
	var s struct {
		Properties any      `json:"properties"`
		Required   []string `json:"required"`
	}
	_ = json.Unmarshal(raw, &s)
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
		return "", fmt.Errorf("vertexanthropic: invalid credentials JSON: %w", err)
	}
	ct := google.CredentialsType(f.Type)
	if !allowedCredentialTypes[ct] {
		return "", fmt.Errorf("vertexanthropic: unsupported credential type %q; only service_account and authorized_user are accepted", f.Type)
	}
	return ct, nil
}

// safeWithGoogleAuth wraps vertex.WithGoogleAuth with panic recovery because
// the SDK panics (rather than returning an error) when GCP Application Default
// Credentials are unavailable.
func safeWithGoogleAuth(ctx context.Context, location, project string) (opt option.RequestOption, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("vertexanthropic: GCP ADC unavailable — set GOOGLE_APPLICATION_CREDENTIALS or provide explicit credentials JSON: %v", r)
		}
	}()
	opt = vertex.WithGoogleAuth(ctx, location, project,
		"https://www.googleapis.com/auth/cloud-platform")
	return opt, nil
}

// Close is a no-op for the Anthropic SDK client which has no closeable
// resources. Satisfies llm.Client.
// StreamChat uses the Anthropic SDK's Messages.NewStreaming to deliver text
// deltas incrementally. The final ChatResponse is built from the accumulated
// message, reusing the existing mapResponse path.
func (c *Client) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	params := c.buildParams(req)
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	acc := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		acc.Accumulate(event)
		if delta, ok := extractTextDelta(event); ok && delta != "" {
			if err := callback(llm.ChatStreamEvent{Delta: delta}); err != nil {
				return llm.ChatResponse{}, err
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("vertexanthropic: stream error: %w", err)
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

func (c *Client) Close() error { return nil }

var _ llm.Client = (*Client)(nil)
