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

package langchaingo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// vertexAnthropicModel implements llms.Model for Claude models hosted on
// Google Vertex AI Model Garden. It translates LangChainGo MessageContent
// to Anthropic Messages API format, sends the request to the Vertex AI
// rawPredict endpoint, and parses the Anthropic response back.
//
// Auth is handled internally: the constructor accepts credentialsJSON (from the
// resolved GOOGLE_APPLICATION_CREDENTIALS file) and layers a GCP OAuth2 Bearer
// token transport on top of any base transport provided by httpClient (which
// may carry StructuredOutputTransport, custom headers, etc.).
//
// Endpoint: {baseURL}/v1/projects/{project}/locations/{location}/publishers/anthropic/models/{model}:rawPredict
type vertexAnthropicModel struct {
	project  string
	location string
	model    string
	baseURL  string
	client   *http.Client
}

func newVertexAnthropicModel(project, location, model, baseURL string, credentialsJSON []byte, httpClient *http.Client) (*vertexAnthropicModel, error) {
	if project == "" {
		return nil, fmt.Errorf("vertex_ai provider requires project (use WithVertexProject)")
	}
	if location == "" {
		location = "us-central1"
	}
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com", location) // pre-commit:allow-sensitive (GCP endpoint pattern)
	}

	var base http.RoundTripper = http.DefaultTransport
	if httpClient != nil && httpClient.Transport != nil {
		base = httpClient.Transport
	}

	trimmedCreds := bytes.TrimSpace(credentialsJSON)
	if len(trimmedCreds) > 0 {
		creds, err := google.CredentialsFromJSON(
			context.Background(), trimmedCreds,
			"https://www.googleapis.com/auth/cloud-platform",
		)
		if err != nil {
			return nil, fmt.Errorf("vertex_ai: invalid credentials JSON: %w", err)
		}
		base = &oauth2.Transport{Source: creds.TokenSource, Base: base}
	}

	return &vertexAnthropicModel{
		project:  project,
		location: location,
		model:    model,
		baseURL:  strings.TrimRight(baseURL, "/"),
		client:   &http.Client{Transport: base},
	}, nil
}

func (m *vertexAnthropicModel) rawPredictURL() string {
	return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		m.baseURL, m.project, m.location, m.model)
}

// GenerateContent implements llms.Model by translating to Anthropic Messages API.
func (m *vertexAnthropicModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	co := llms.CallOptions{}
	for _, o := range opts {
		o(&co)
	}

	reqBody := m.buildAnthropicRequest(messages, &co)

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.rawPredictURL(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("vertex_ai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai: HTTP request: %w", err)
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("vertex_ai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		body := respBytes
		const maxErrBody = 512
		if len(body) > maxErrBody {
			body = body[:maxErrBody]
		}
		return nil, fmt.Errorf("vertex_ai: HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	return m.parseAnthropicResponse(respBytes)
}

type anthropicRequest struct {
	Model       string                `json:"model"`
	MaxTokens   int                   `json:"max_tokens"`
	System      string                `json:"system,omitempty"`
	Messages    []anthropicMessage    `json:"messages"`
	Tools       []anthropicTool       `json:"tools,omitempty"`
	Temperature *float64              `json:"temperature,omitempty"`
	Stream      bool                  `json:"stream"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

func (m *vertexAnthropicModel) buildAnthropicRequest(messages []llms.MessageContent, co *llms.CallOptions) *anthropicRequest {
	req := &anthropicRequest{
		Model:     m.model,
		MaxTokens: 4096,
		Stream:    false,
	}

	if co.MaxTokens > 0 {
		req.MaxTokens = co.MaxTokens
	}
	if co.Temperature > 0 {
		t := co.Temperature
		req.Temperature = &t
	}

	for _, msg := range messages {
		switch msg.Role {
		case llms.ChatMessageTypeSystem:
			for _, part := range msg.Parts {
				if tc, ok := part.(llms.TextContent); ok {
					req.System = tc.Text
				}
			}
		case llms.ChatMessageTypeHuman:
			content := m.partsToAnthropicContent(msg.Parts)
			req.Messages = append(req.Messages, anthropicMessage{Role: "user", Content: content})
		case llms.ChatMessageTypeAI:
			content := m.partsToAnthropicContent(msg.Parts)
			req.Messages = append(req.Messages, anthropicMessage{Role: "assistant", Content: content})
		case llms.ChatMessageTypeTool:
			content := m.toolResponseToAnthropicContent(msg.Parts)
			req.Messages = append(req.Messages, anthropicMessage{Role: "user", Content: content})
		}
	}

	for _, tool := range co.Tools {
		if tool.Function != nil {
			req.Tools = append(req.Tools, anthropicTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: tool.Function.Parameters,
			})
		}
	}

	return req
}

func (m *vertexAnthropicModel) partsToAnthropicContent(parts []llms.ContentPart) interface{} {
	if len(parts) == 1 {
		if tc, ok := parts[0].(llms.TextContent); ok {
			return tc.Text
		}
	}

	var blocks []map[string]interface{}
	for _, part := range parts {
		switch p := part.(type) {
		case llms.TextContent:
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": p.Text,
			})
		case llms.ToolCall:
			var input interface{}
			if p.FunctionCall != nil {
				_ = json.Unmarshal([]byte(p.FunctionCall.Arguments), &input)
			}
			blocks = append(blocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    p.ID,
				"name":  p.FunctionCall.Name,
				"input": input,
			})
		}
	}
	return blocks
}

func (m *vertexAnthropicModel) toolResponseToAnthropicContent(parts []llms.ContentPart) interface{} {
	var blocks []map[string]interface{}
	for _, part := range parts {
		if tr, ok := part.(llms.ToolCallResponse); ok {
			blocks = append(blocks, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": tr.ToolCallID,
				"content":     tr.Content,
			})
		}
	}
	return blocks
}

type anthropicResponse struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Role       string                   `json:"role"`
	Content    []anthropicContentBlock  `json:"content"`
	StopReason string                   `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func (m *vertexAnthropicModel) parseAnthropicResponse(data []byte) (*llms.ContentResponse, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("vertex_ai: parse response: %w", err)
	}

	choice := &llms.ContentChoice{
		StopReason: resp.StopReason,
		GenerationInfo: map[string]any{
			"PromptTokens":     resp.Usage.InputTokens,
			"CompletionTokens": resp.Usage.OutputTokens,
			"TotalTokens":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			args := string(block.Input)
			choice.ToolCalls = append(choice.ToolCalls, llms.ToolCall{
				ID:   block.ID,
				Type: "function",
				FunctionCall: &llms.FunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}
	choice.Content = strings.Join(textParts, "")

	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{choice},
	}, nil
}

// Call implements llms.Model for the simpler text-only interface.
func (m *vertexAnthropicModel) Call(ctx context.Context, prompt string, opts ...llms.CallOption) (string, error) {
	msgs := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}
	resp, err := m.GenerateContent(ctx, msgs, opts...)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Content, nil
}

var _ llms.Model = (*vertexAnthropicModel)(nil)
