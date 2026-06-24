// Package openai provides an in-house adapter implementing the ADK model.LLM
// interface for OpenAI-compatible API endpoints (OpenAI, LlamaStack, vLLM, Ollama).
//
// The adapter uses net/http directly and accepts a custom *http.Client for
// transport chain injection (TLS CA, OAuth2, custom headers, circuit breaker)
// per Issue #1342.
package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Model implements model.LLM for OpenAI-compatible endpoints.
type Model struct {
	name       string
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// Option configures the Model.
type Option func(*Model)

// WithHTTPClient injects a custom HTTP client for transport chain support.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Model) {
		m.httpClient = c
	}
}

// NewModel creates a new OpenAI-compatible model adapter.
func NewModel(modelName, endpoint, apiKey string, opts ...Option) *Model {
	m := &Model{
		name:     modelName,
		endpoint: strings.TrimSuffix(endpoint, "/"),
		apiKey:   apiKey,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.httpClient == nil {
		m.httpClient = http.DefaultClient
	}
	return m
}

// Name returns the model name.
func (m *Model) Name() string {
	return m.name
}

// GenerateContent implements model.LLM. It converts the ADK LLMRequest to an
// OpenAI chat completion request, sends it, and maps the response back.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		body := m.buildRequestBody(req, stream)

		jsonBody, err := json.Marshal(body)
		if err != nil {
			yield(nil, fmt.Errorf("marshal request: %w", err))
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			m.endpoint+"/chat/completions", strings.NewReader(string(jsonBody)))
		if err != nil {
			yield(nil, fmt.Errorf("build request: %w", err))
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if m.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
		}

		resp, err := m.httpClient.Do(httpReq)
		if err != nil {
			yield(nil, fmt.Errorf("send request: %w", err))
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			yield(nil, fmt.Errorf("OpenAI API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes)))
			return
		}

		if stream {
			m.handleStreamingResponse(resp.Body, yield)
		} else {
			m.handleNonStreamingResponse(resp.Body, yield)
		}
	}
}

func (m *Model) buildRequestBody(req *model.LLMRequest, stream bool) map[string]any {
	body := map[string]any{
		"model":  m.name,
		"stream": stream,
	}

	var messages []map[string]any

	if req.Config != nil && req.Config.SystemInstruction != nil {
		for _, part := range req.Config.SystemInstruction.Parts {
			if part.Text != "" {
				messages = append(messages, map[string]any{
					"role":    "system",
					"content": part.Text,
				})
			}
		}
	}

	for _, content := range req.Contents {
		msg := m.convertContent(content)
		if msg != nil {
			messages = append(messages, msg)
		}
	}

	body["messages"] = messages

	if req.Config != nil {
		m.applyGenerationConfig(body, req.Config)
	}

	return body
}

func (m *Model) convertContent(content *genai.Content) map[string]any {
	if content == nil || len(content.Parts) == 0 {
		return nil
	}

	role := content.Role
	if role == "" {
		role = "user"
	}
	if role == "model" {
		role = "assistant"
	}

	var textParts []string
	var toolCalls []map[string]any
	var toolCallID string

	for _, part := range content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, map[string]any{
				"id":   part.FunctionCall.ID,
				"type": "function",
				"function": map[string]any{
					"name":      part.FunctionCall.Name,
					"arguments": string(argsJSON),
				},
			})
		}
		if part.FunctionResponse != nil {
			role = "tool"
			toolCallID = part.FunctionResponse.ID
			respJSON, _ := json.Marshal(part.FunctionResponse.Response)
			textParts = append(textParts, string(respJSON))
		}
	}

	msg := map[string]any{"role": role}

	if len(textParts) > 0 {
		msg["content"] = strings.Join(textParts, "")
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	if toolCallID != "" {
		msg["tool_call_id"] = toolCallID
	}

	return msg
}

func (m *Model) applyGenerationConfig(body map[string]any, cfg *genai.GenerateContentConfig) {
	if cfg.Temperature != nil {
		body["temperature"] = *cfg.Temperature
	}
	if cfg.TopP != nil {
		body["top_p"] = *cfg.TopP
	}
	if cfg.MaxOutputTokens != 0 {
		body["max_tokens"] = cfg.MaxOutputTokens
	}
	if len(cfg.StopSequences) > 0 {
		body["stop"] = cfg.StopSequences
	}

	if len(cfg.Tools) > 0 {
		var tools []map[string]any
		for _, tool := range cfg.Tools {
			for _, fn := range tool.FunctionDeclarations {
				t := map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":        fn.Name,
						"description": fn.Description,
					},
				}
				if fn.Parameters != nil {
					t["function"].(map[string]any)["parameters"] = convertSchema(fn.Parameters)
				}
				tools = append(tools, t)
			}
		}
		body["tools"] = tools
	}

	if cfg.ResponseSchema != nil {
		body["response_format"] = map[string]any{
			"type":        "json_schema",
			"json_schema": convertSchema(cfg.ResponseSchema),
		}
	}
}

func convertSchema(s *genai.Schema) map[string]any {
	if s == nil {
		return nil
	}
	result := map[string]any{
		"type": strings.ToLower(string(s.Type)),
	}
	if s.Description != "" {
		result["description"] = s.Description
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any)
		for name, prop := range s.Properties {
			props[name] = convertSchema(prop)
		}
		result["properties"] = props
	}
	if len(s.Required) > 0 {
		result["required"] = s.Required
	}
	if s.Items != nil {
		result["items"] = convertSchema(s.Items)
	}
	if len(s.Enum) > 0 {
		result["enum"] = s.Enum
	}
	return result
}

// chatCompletionResponse represents the OpenAI chat completion response.
type chatCompletionResponse struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Model   string                  `json:"model"`
	Choices []chatCompletionChoice  `json:"choices"`
	Usage   *chatCompletionUsage    `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                    `json:"index"`
	Message      chatCompletionMessage  `json:"message"`
	FinishReason *string                `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role      string              `json:"role"`
	Content   *string             `json:"content"`
	ToolCalls []chatToolCall      `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function chatToolCallFn  `json:"function"`
}

type chatToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (m *Model) handleNonStreamingResponse(body io.Reader, yield func(*model.LLMResponse, error) bool) {
	var resp chatCompletionResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		yield(nil, fmt.Errorf("decode response: %w", err))
		return
	}

	llmResp := m.convertResponse(&resp)
	yield(llmResp, nil)
}

func (m *Model) convertResponse(resp *chatCompletionResponse) *model.LLMResponse {
	llmResp := &model.LLMResponse{
		Content: &genai.Content{
			Role: "model",
		},
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		if choice.Message.Content != nil && *choice.Message.Content != "" {
			llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{
				Text: *choice.Message.Content,
			})
		}

		for _, tc := range choice.Message.ToolCalls {
			llmResp.Content.Parts = append(llmResp.Content.Parts,
				parseFunctionCall(tc.ID, tc.Function.Name, tc.Function.Arguments))
		}

		llmResp.FinishReason = mapFinishReason(choice.FinishReason)
	}

	if resp.Usage != nil {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(resp.Usage.PromptTokens),
			CandidatesTokenCount: int32(resp.Usage.CompletionTokens),
			TotalTokenCount:      int32(resp.Usage.TotalTokens),
		}
	}

	return llmResp
}

// Streaming types
type chatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Model   string                      `json:"model"`
	Choices []chatCompletionChunkChoice `json:"choices"`
}

type chatCompletionChunkChoice struct {
	Index        int                       `json:"index"`
	Delta        chatCompletionChunkDelta  `json:"delta"`
	FinishReason *string                   `json:"finish_reason"`
}

type chatCompletionChunkDelta struct {
	Role      string               `json:"role,omitempty"`
	Content   string               `json:"content,omitempty"`
	ToolCalls []chatChunkToolCall  `json:"tool_calls,omitempty"`
}

type chatChunkToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function chatChunkToolFn  `json:"function"`
}

type chatChunkToolFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// toolCallAccumulator accumulates streaming tool call fragments.
type toolCallAccumulator struct {
	id   string
	name string
	args strings.Builder
}

func (m *Model) handleStreamingResponse(body io.Reader, yield func(*model.LLMResponse, error) bool) {
	accumulators := make(map[int]*toolCallAccumulator)
	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			resp := &model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: choice.Delta.Content}},
				},
			}
			if !yield(resp, nil) {
				return
			}
		}

		for _, tc := range choice.Delta.ToolCalls {
			acc, exists := accumulators[tc.Index]
			if !exists {
				acc = &toolCallAccumulator{}
				accumulators[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.args.WriteString(tc.Function.Arguments)
		}

		if choice.FinishReason != nil {
			finishReason := mapFinishReason(choice.FinishReason)

			resp := &model.LLMResponse{
				Content: &genai.Content{
					Role: "model",
				},
				FinishReason: finishReason,
				TurnComplete: true,
			}

			for _, acc := range accumulators {
				resp.Content.Parts = append(resp.Content.Parts,
					parseFunctionCall(acc.id, acc.name, acc.args.String()))
			}

			if !yield(resp, nil) {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		yield(nil, fmt.Errorf("read SSE stream: %w", err))
	}
}

func parseFunctionCall(id, name, argsJSON string) *genai.Part {
	var args map[string]any
	_ = json.Unmarshal([]byte(argsJSON), &args)
	return &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   id,
			Name: name,
			Args: args,
		},
	}
}

func mapFinishReason(reason *string) genai.FinishReason {
	if reason == nil {
		return genai.FinishReasonUnspecified
	}
	switch *reason {
	case "stop":
		return genai.FinishReasonStop
	case "length":
		return genai.FinishReasonMaxTokens
	case "tool_calls":
		return genai.FinishReasonStop
	case "content_filter":
		return genai.FinishReasonSafety
	default:
		return genai.FinishReasonUnspecified
	}
}

