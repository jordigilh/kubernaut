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
	"fmt"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/v2/model"
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
	httpClient         *http.Client
	azureAPIVersion    string
	effort             string
	capabilityOverride string
}

// WithHTTPClient injects a custom HTTP client for transport chain support.
func WithHTTPClient(c *http.Client) Option {
	return func(o *modelOpts) {
		o.httpClient = c
	}
}

// WithAzureAPIVersion switches this model into Azure OpenAI mode (#1600):
// the underlying openaicompat.Client uses Azure's deployment-scoped URL
// (modelName doubles as deployment ID) and api-key auth instead of the flat
// OpenAI path and Bearer auth. Net-new for AF — added for parity with KA's
// equivalent option, see openaicompat.WithAzureAPIVersion.
func WithAzureAPIVersion(apiVersion string) Option {
	return func(o *modelOpts) {
		o.azureAPIVersion = apiVersion
	}
}

// WithReasoningEffort sets the construction-time reasoning-depth value
// (#1604's unified Effort knob — one of "", "none", "minimal", "low",
// "medium", "high", "xhigh", "max"). Unlike KA's kaopenai.WithReasoning, there is
// no per-call override here: ADK's model.LLMRequest carries no reasoning
// field, so this is the only knob (DD-LLM-005 addendum). An empty value
// (the default) sends no effort parameter at all — the provider's own
// vendor default applies, matching KA's zero-regression behavior.
func WithReasoningEffort(effort string) Option {
	return func(o *modelOpts) {
		o.effort = effort
	}
}

// WithCapabilityOverride opts a custom OpenAI-compatible endpoint into or out
// of model-name reasoning detection, including the request-side effort dialect.
func WithCapabilityOverride(override string) Option {
	return func(o *modelOpts) {
		o.capabilityOverride = override
	}
}

// NewModel creates a new OpenAI-compatible model adapter. The reasoning
// round-trip mode is auto-detected from modelName (BR-AI-086, DD-LLM-005) —
// unrecognized models default to no reasoning capture/replay, preserving
// today's behavior exactly for every currently-configured model. The effort
// wire dialect is likewise auto-detected (#1604); WithReasoningEffort's
// value is sent only for a recognized dialect or an explicit capability
// override (see applyEffort in the shared openaicompat package).
func NewModel(modelName, endpoint, apiKey string, opts ...Option) *Model {
	o := &modelOpts{}
	for _, opt := range opts {
		opt(o)
	}

	var clientOpts []openaicompat.Option
	if o.httpClient != nil {
		clientOpts = append(clientOpts, openaicompat.WithHTTPClient(o.httpClient))
	}
	if o.azureAPIVersion != "" {
		clientOpts = append(clientOpts, openaicompat.WithAzureAPIVersion(o.azureAPIVersion))
	}

	return &Model{
		name:          modelName,
		client:        openaicompat.New(modelName, endpoint, apiKey, clientOpts...),
		reasoningMode: openaicompat.DetectReasoningMode(modelName, o.capabilityOverride),
		effortDialect: openaicompat.DetectEffortDialectWithOverride(modelName, o.capabilityOverride),
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
			var mappingErr error
			streamErr := m.client.StreamChat(ctx, compatReq, func(ev openaicompat.StreamEvent) bool {
				continueStream, err := yieldStreamEvent(ev, yield)
				if err != nil {
					mappingErr = err
					return false
				}
				return continueStream
			})
			if mappingErr != nil {
				yield(nil, mappingErr)
				return
			}
			if streamErr != nil {
				yield(nil, streamErr)
			}
			return
		}

		resp, err := m.client.Chat(ctx, compatReq)
		if err != nil {
			yield(nil, err)
			return
		}
		converted, err := convertResponse(resp)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(converted, nil)
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
			if raw, err := marshalFunctionParameters(fn); err == nil {
				td.Parameters = raw
			}
			req.Tools = append(req.Tools, td)
		}
	}
	applyToolChoice(req, cfg.ToolConfig)

	if cfg.ResponseSchema != nil {
		if raw, err := json.Marshal(convertSchema(cfg.ResponseSchema)); err == nil {
			req.ResponseSchema = raw
		}
	}
}

func marshalFunctionParameters(fn *genai.FunctionDeclaration) ([]byte, error) {
	if fn.Parameters != nil {
		return json.Marshal(convertSchema(fn.Parameters))
	}
	if fn.ParametersJsonSchema != nil {
		// ADK v2 functiontool declarations use the raw JSON Schema field.
		return json.Marshal(fn.ParametersJsonSchema)
	}
	return nil, nil
}

func applyToolChoice(req *openaicompat.Request, config *genai.ToolConfig) {
	if config == nil || config.FunctionCallingConfig == nil {
		return
	}
	fcc := config.FunctionCallingConfig
	if fcc.Mode != genai.FunctionCallingConfigModeAny || len(fcc.AllowedFunctionNames) != 1 {
		return
	}
	req.ToolChoice = &openaicompat.ToolChoice{Name: fcc.AllowedFunctionNames[0]}
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
func convertResponse(resp *openaicompat.Response) (*model.LLMResponse, error) {
	llmResp := &model.LLMResponse{
		Content: &genai.Content{Role: "model"},
	}

	if resp.Message.Content != "" {
		llmResp.Content.Parts = append(llmResp.Content.Parts, &genai.Part{Text: resp.Message.Content})
	}
	for _, tc := range resp.Message.ToolCalls {
		part, err := parseFunctionCall(tc)
		if err != nil {
			return nil, err
		}
		llmResp.Content.Parts = append(llmResp.Content.Parts, part)
	}
	llmResp.FinishReason = mapFinishReason(resp.FinishReason)

	if resp.Usage != (openaicompat.TokenUsage{}) {
		llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(resp.Usage.PromptTokens),
			CandidatesTokenCount: int32(resp.Usage.CompletionTokens),
			TotalTokenCount:      int32(resp.Usage.TotalTokens),
		}
	}
	return llmResp, nil
}

// yieldStreamEvent maps one shared StreamEvent to the ADK per-chunk callback
// contract: a partial LLMResponse for each text delta, plus the accumulated
// final LLMResponse (tool calls, finish reason) on Done. Returns false if
// the ADK-side yield requested the stream to stop.
func yieldStreamEvent(ev openaicompat.StreamEvent, yield func(*model.LLMResponse, error) bool) (bool, error) {
	if ev.Delta != "" {
		if !yield(&model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: ev.Delta}}},
		}, nil) {
			return false, nil
		}
	}
	if ev.Done && ev.Final != nil {
		resp, err := convertResponse(ev.Final)
		if err != nil {
			return false, err
		}
		resp.TurnComplete = true
		return yield(resp, nil), nil
	}
	return true, nil
}

func parseFunctionCall(tc openaicompat.ToolCall) (*genai.Part, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		return nil, fmt.Errorf("decode tool call arguments: %w", err)
	}
	if args == nil {
		return nil, fmt.Errorf("decode tool call arguments: expected JSON object")
	}
	return &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   tc.ID,
			Name: tc.Name,
			Args: args,
		},
	}, nil
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
