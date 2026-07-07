// Package openai provides an in-house adapter implementing the ADK model.LLM
// interface for OpenAI-compatible API endpoints (OpenAI, LlamaStack, vLLM, Ollama).
//
// This is a thin translation layer: all wire-protocol work (HTTP transport,
// SSE streaming, tool-call accumulation, reasoning-content round-trip rules)
// is delegated to pkg/shared/llm/openaicompat, shared with Kubernaut Agent's
// equivalent wrapper (DD-LLM-005). This file only translates between ADK's
// genai.Content/model.LLMRequest and the shared package's neutral types.
package openai

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Model implements model.LLM for OpenAI-compatible endpoints.
type Model struct {
	name          string
	client        *openaicompat.Client
	reasoningMode openaicompat.ReasoningMode
	effortDialect openaicompat.EffortDialect
	effort        string
}

// Option configures the Model.
type Option func(*modelOpts)

type modelOpts struct {
	httpClient *http.Client
	effort     string
}

// WithHTTPClient injects a custom HTTP client for transport chain support.
func WithHTTPClient(c *http.Client) Option {
	return func(o *modelOpts) {
		o.httpClient = c
	}
}

// WithReasoningEffort sets the construction-time reasoning-depth value
// (#1604's unified Effort knob — one of "", "none", "minimal", "low",
// "medium", "high", "xhigh"). Unlike KA's kaopenai.WithReasoning, there is
// no per-call override here: ADK's model.LLMRequest carries no reasoning
// field, so this is the only knob (DD-LLM-005 addendum). An empty value
// (the default) sends no effort parameter at all — the provider's own
// vendor default applies, matching KA's zero-regression behavior.
func WithReasoningEffort(effort string) Option {
	return func(o *modelOpts) {
		o.effort = effort
	}
}

// NewModel creates a new OpenAI-compatible model adapter. The reasoning
// round-trip mode is auto-detected from modelName (BR-AI-086, DD-LLM-005) —
// unrecognized models default to no reasoning capture/replay, preserving
// today's behavior exactly for every currently-configured model. The effort
// wire dialect is likewise auto-detected (#1604); WithReasoningEffort's
// value is only ever sent for a recognized dialect (see applyEffort in the
// shared openaicompat package).
func NewModel(modelName, endpoint, apiKey string, opts ...Option) *Model {
	o := &modelOpts{}
	for _, opt := range opts {
		opt(o)
	}

	var clientOpts []openaicompat.Option
	if o.httpClient != nil {
		clientOpts = append(clientOpts, openaicompat.WithHTTPClient(o.httpClient))
	}

	return &Model{
		name:          modelName,
		client:        openaicompat.New(modelName, endpoint, apiKey, clientOpts...),
		reasoningMode: openaicompat.DetectReasoningMode(modelName, ""),
		effortDialect: openaicompat.DetectEffortDialect(modelName),
		effort:        o.effort,
	}
}

// Name returns the model name.
func (m *Model) Name() string {
	return m.name
}

// GenerateContent implements model.LLM. It converts the ADK LLMRequest to the
// shared package's Request, delegates to openaicompat.Client, and maps the
// response back.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		compatReq := m.buildRequest(req)

		if stream {
			_ = m.client.StreamChat(ctx, compatReq, func(ev openaicompat.StreamEvent) bool {
				return yieldStreamEvent(ev, yield)
			})
			return
		}

		resp, err := m.client.Chat(ctx, compatReq)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(convertResponse(resp), nil)
	}
}

func (m *Model) buildRequest(req *model.LLMRequest) openaicompat.Request {
	compatReq := openaicompat.Request{
		Model:         m.name,
		ReasoningMode: m.reasoningMode,
	}
	if m.effort != "" {
		compatReq.Effort = m.effort
		compatReq.EffortDialect = m.effortDialect
	}

	if req.Config != nil && req.Config.SystemInstruction != nil {
		for _, part := range req.Config.SystemInstruction.Parts {
			if part.Text != "" {
				compatReq.Messages = append(compatReq.Messages, openaicompat.Message{
					Role: "system", Content: part.Text,
				})
			}
		}
	}

	for _, content := range req.Contents {
		if msg, ok := convertContent(content); ok {
			compatReq.Messages = append(compatReq.Messages, msg)
		}
	}

	if req.Config != nil {
		applyGenerationConfig(&compatReq, req.Config)
	}

	return compatReq
}

func convertContent(content *genai.Content) (openaicompat.Message, bool) {
	if content == nil || len(content.Parts) == 0 {
		return openaicompat.Message{}, false
	}

	role := content.Role
	if role == "" {
		role = "user"
	}
	if role == "model" {
		role = "assistant"
	}

	msg := openaicompat.Message{}
	var textParts []string

	for _, part := range content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			msg.ToolCalls = append(msg.ToolCalls, openaicompat.ToolCall{
				ID:        part.FunctionCall.ID,
				Name:      part.FunctionCall.Name,
				Arguments: string(argsJSON),
			})
		}
		if part.FunctionResponse != nil {
			role = "tool"
			msg.ToolCallID = part.FunctionResponse.ID
			respJSON, _ := json.Marshal(part.FunctionResponse.Response)
			textParts = append(textParts, string(respJSON))
		}
	}

	msg.Role = role
	msg.Content = strings.Join(textParts, "")
	return msg, true
}

func applyGenerationConfig(req *openaicompat.Request, cfg *genai.GenerateContentConfig) {
	if cfg.Temperature != nil {
		t := float64(*cfg.Temperature)
		req.Temperature = &t
	}
	if cfg.TopP != nil {
		p := float64(*cfg.TopP)
		req.TopP = &p
	}
	if cfg.MaxOutputTokens != 0 {
		req.MaxTokens = int(cfg.MaxOutputTokens)
	}
	if len(cfg.StopSequences) > 0 {
		req.StopSequences = cfg.StopSequences
	}

	for _, tool := range cfg.Tools {
		for _, fn := range tool.FunctionDeclarations {
			td := openaicompat.ToolDefinition{
				Name:        fn.Name,
				Description: fn.Description,
			}
			if fn.Parameters != nil {
				if raw, err := json.Marshal(convertSchema(fn.Parameters)); err == nil {
					td.Parameters = raw
				}
			}
			req.Tools = append(req.Tools, td)
		}
	}

	if cfg.ResponseSchema != nil {
		if raw, err := json.Marshal(convertSchema(cfg.ResponseSchema)); err == nil {
			req.ResponseSchema = raw
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

// convertResponse translates a shared Response into the ADK LLMResponse.
func convertResponse(resp *openaicompat.Response) *model.LLMResponse {
	llmResp := &model.LLMResponse{
		Content: &genai.Content{Role: "model"},
	}

	if resp.Message.Content != "" {
		llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{Text: resp.Message.Content})
	}
	for _, tc := range resp.Message.ToolCalls {
		llmResp.Content.Parts = append(llmResp.Content.Parts, parseFunctionCall(tc))
	}
	llmResp.FinishReason = mapFinishReason(resp.FinishReason)

	if resp.Usage != (openaicompat.TokenUsage{}) {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(resp.Usage.PromptTokens),
			CandidatesTokenCount: int32(resp.Usage.CompletionTokens),
			TotalTokenCount:      int32(resp.Usage.TotalTokens),
		}
	}
	return llmResp
}

// yieldStreamEvent maps one shared StreamEvent to the ADK per-chunk callback
// contract: a partial LLMResponse for each text delta, plus the accumulated
// final LLMResponse (tool calls, finish reason) on Done. Returns false if
// the ADK-side yield requested the stream to stop.
func yieldStreamEvent(ev openaicompat.StreamEvent, yield func(*model.LLMResponse, error) bool) bool {
	if ev.Delta != "" {
		if !yield(&model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: ev.Delta}}},
		}, nil) {
			return false
		}
	}
	if ev.Done && ev.Final != nil {
		resp := convertResponse(ev.Final)
		resp.TurnComplete = true
		return yield(resp, nil)
	}
	return true
}

func parseFunctionCall(tc openaicompat.ToolCall) *genai.Part {
	var args map[string]any
	_ = json.Unmarshal([]byte(tc.Arguments), &args)
	return &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: args,
		},
	}
}

func mapFinishReason(reason string) genai.FinishReason {
	switch reason {
	case openaicompat.FinishReasonStop:
		return genai.FinishReasonStop
	case openaicompat.FinishReasonLength:
		return genai.FinishReasonMaxTokens
	case openaicompat.FinishReasonToolCalls:
		return genai.FinishReasonStop
	case openaicompat.FinishReasonContentFilter:
		return genai.FinishReasonSafety
	case "":
		return genai.FinishReasonUnspecified
	default:
		return genai.FinishReasonUnspecified
	}
}
