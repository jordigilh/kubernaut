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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/bedrock"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/googleai/vertex"
	"github.com/tmc/langchaingo/llms/huggingface"
	"github.com/tmc/langchaingo/llms/mistral"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

// Option configures provider-specific settings for the LangChainGo adapter.
type Option func(*options)

type options struct {
	azureAPIVersion string
	vertexProject   string
	vertexLocation  string
	bedrockRegion   string
	httpClient      *http.Client
	closeFn         func() error
}

// WithAzureAPIVersion sets the Azure OpenAI API version (required for "azure" provider).
func WithAzureAPIVersion(v string) Option {
	return func(o *options) { o.azureAPIVersion = v }
}

// WithVertexProject sets the GCP project (required for "vertex" provider).
func WithVertexProject(p string) Option {
	return func(o *options) { o.vertexProject = p }
}

// WithVertexLocation sets the GCP location (defaults to "us-central1" if empty).
func WithVertexLocation(l string) Option {
	return func(o *options) { o.vertexLocation = l }
}

func WithBedrockRegion(r string) Option {
	return func(o *options) { o.bedrockRegion = r }
}

// WithHTTPClient sets a custom HTTP client for providers that support it
// (Anthropic and vertex_ai). Used to chain transports for structured output
// injection and auth header passthrough. For vertex_ai, GCP OAuth2 auth is
// layered on top of the provided transport internally by the shim.
func WithHTTPClient(c *http.Client) Option {
	return func(o *options) { o.httpClient = c }
}

// WithCloser sets an optional cleanup function called by Close. Use this to
// release HTTP idle connections from a custom transport chain.
func WithCloser(fn func() error) Option {
	return func(o *options) { o.closeFn = fn }
}

// Adapter implements llm.Client by delegating to LangChainGo.
// Authority: DD-HAPI-019-001 — Framework Isolation Pattern
type Adapter struct {
	model   llms.Model
	closeFn func() error
}

// New creates a new LangChainGo adapter for the given provider.
// Supported providers: "openai", "ollama", "azure", "vertex", "vertex_ai", "anthropic", "bedrock", "huggingface", "mistral".
func New(provider, endpoint, model, apiKey string, opts ...Option) (*Adapter, error) {
	o := &options{vertexLocation: "us-central1"}
	for _, fn := range opts {
		fn(o)
	}
	m, err := newModel(provider, endpoint, model, apiKey, o)
	if err != nil {
		return nil, fmt.Errorf("langchaingo: %w", err)
	}
	return &Adapter{model: m, closeFn: o.closeFn}, nil
}

func newModel(provider, endpoint, model, apiKey string, o *options) (llms.Model, error) {
	switch provider {
	case "openai":
		return openai.New(
			openai.WithBaseURL(endpoint+"/v1"),
			openai.WithModel(model),
			openai.WithToken(apiKey),
		)
	case "ollama":
		return ollama.New(
			ollama.WithServerURL(endpoint),
			ollama.WithModel(model),
		)
	case "azure":
		if o.azureAPIVersion == "" {
			return nil, fmt.Errorf("azure provider requires api_version (use WithAzureAPIVersion)")
		}
		return openai.New(
			openai.WithAPIType(openai.APITypeAzure),
			openai.WithBaseURL(endpoint),
			openai.WithModel(model),
			openai.WithToken(apiKey),
			openai.WithAPIVersion(o.azureAPIVersion),
		)
	case "vertex":
		if o.vertexProject == "" {
			return nil, fmt.Errorf("vertex provider requires project (use WithVertexProject)")
		}
		vopts := []googleai.Option{
			googleai.WithCloudProject(o.vertexProject),
			googleai.WithCloudLocation(o.vertexLocation),
			googleai.WithDefaultModel(model),
		}
		if apiKey != "" {
			vopts = append(vopts, googleai.WithCredentialsJSON([]byte(apiKey)))
		}
		return vertex.New(context.Background(), vopts...)
	case "anthropic":
		aopts := []anthropic.Option{anthropic.WithModel(model), anthropic.WithToken(apiKey)}
		if endpoint != "" {
			aopts = append(aopts, anthropic.WithBaseURL(endpoint))
		}
		if o.httpClient != nil {
			aopts = append(aopts, anthropic.WithHTTPClient(o.httpClient))
		}
		return anthropic.New(aopts...)
	case "bedrock":
		bopts := []bedrock.Option{bedrock.WithModel(model)}
		if o.bedrockRegion != "" {
			awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
				awsconfig.WithRegion(o.bedrockRegion),
			)
			if err != nil {
				return nil, fmt.Errorf("bedrock: loading AWS config for region %q: %w", o.bedrockRegion, err)
			}
			bopts = append(bopts, bedrock.WithClient(bedrockruntime.NewFromConfig(awsCfg)))
		}
		return bedrock.New(bopts...)
	case "huggingface":
		return huggingface.New(huggingface.WithToken(apiKey), huggingface.WithModel(model))
	case "mistral":
		mopts := []mistral.Option{mistral.WithAPIKey(apiKey), mistral.WithModel(model)}
		if endpoint != "" {
			mopts = append(mopts, mistral.WithEndpoint(endpoint))
		}
		return mistral.New(mopts...)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", provider)
	}
}

// Chat translates a Kubernaut ChatRequest into LangChainGo's MessageContent
// format, calls GenerateContent, and maps the response back.
// Per-session OutputSchema is propagated to the HTTP transport via context,
// enabling phase-specific structured output for Anthropic (see #700).
func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if len(req.Options.OutputSchema) > 0 {
		ctx = transport.WithOutputSchema(ctx, req.Options.OutputSchema)
	}

	msgs := toMessages(req.Messages)
	opts := buildCallOptions(req)

	cr, err := a.model.GenerateContent(ctx, msgs, opts...)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("langchaingo chat: %w", err)
	}

	return fromContentResponse(cr), nil
}

func toMessages(msgs []llm.Message) []llms.MessageContent {
	out := make([]llms.MessageContent, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageContent(m))
	}
	return out
}

func toMessageContent(m llm.Message) llms.MessageContent {
	switch m.Role {
	case "system":
		return llms.TextParts(llms.ChatMessageTypeSystem, m.Content)
	case "user":
		return llms.TextParts(llms.ChatMessageTypeHuman, m.Content)
	case "assistant":
		mc := llms.MessageContent{Role: llms.ChatMessageTypeAI}
		if m.Content != "" {
			mc.Parts = append(mc.Parts, llms.TextContent{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			mc.Parts = append(mc.Parts, llms.ToolCall{
				ID:   tc.ID,
				Type: "function",
				FunctionCall: &llms.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		return mc
	case "tool":
		return llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: m.ToolCallID,
					Name:       m.ToolName,
					Content:    m.Content,
				},
			},
		}
	default:
		return llms.TextParts(llms.ChatMessageTypeGeneric, m.Content)
	}
}

func buildCallOptions(req llm.ChatRequest) []llms.CallOption {
	var opts []llms.CallOption
	if len(req.Tools) > 0 {
		tools := make([]llms.Tool, 0, len(req.Tools))
		for _, td := range req.Tools {
			var params any
			if len(td.Parameters) > 0 {
				_ = json.Unmarshal(td.Parameters, &params)
			}
			tools = append(tools, llms.Tool{
				Type: "function",
				Function: &llms.FunctionDefinition{
					Name:        td.Name,
					Description: td.Description,
					Parameters:  params,
				},
			})
		}
		opts = append(opts, llms.WithTools(tools))
	}
	if req.Options.Temperature > 0 {
		opts = append(opts, llms.WithTemperature(req.Options.Temperature))
	}
	if req.Options.MaxTokens > 0 {
		opts = append(opts, llms.WithMaxTokens(req.Options.MaxTokens))
	}
	if req.Options.JSONMode {
		opts = append(opts, llms.WithJSONMode())
	}
	return opts
}

func fromContentResponse(cr *llms.ContentResponse) llm.ChatResponse {
	if cr == nil || len(cr.Choices) == 0 {
		return llm.ChatResponse{}
	}
	choice := cr.Choices[0]

	resp := llm.ChatResponse{
		Message: llm.Message{
			Role:    "assistant",
			Content: choice.Content,
		},
	}

	for _, tc := range choice.ToolCalls {
		name := ""
		args := ""
		if tc.FunctionCall != nil {
			name = tc.FunctionCall.Name
			args = tc.FunctionCall.Arguments
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      name,
			Arguments: args,
		})
	}

	resp.FinishReason = normalizeStopReason(choice.StopReason)

	if gi := choice.GenerationInfo; gi != nil {
		if pt, ok := gi["PromptTokens"].(int); ok {
			resp.Usage.PromptTokens = pt
		}
		if ct, ok := gi["CompletionTokens"].(int); ok {
			resp.Usage.CompletionTokens = ct
		}
		if tt, ok := gi["TotalTokens"].(int); ok {
			resp.Usage.TotalTokens = tt
		}
	}

	return resp
}

// normalizeStopReason maps provider-specific stop reasons to our canonical
// FinishReason constants. LangChainGo exposes the raw provider value as a
// string, so we normalize across OpenAI ("stop"/"length"/"tool_calls"),
// Anthropic ("end_turn"/"max_tokens"/"tool_use"), and Gemini
// ("FinishReasonStop"/"FinishReasonMaxTokens").
func normalizeStopReason(raw string) string {
	switch strings.ToLower(raw) {
	case "stop", "end_turn", "stop_sequence", "finishreasonstop":
		return llm.FinishReasonStop
	case "length", "max_tokens", "finishreasonmaxtokens":
		return llm.FinishReasonLength
	case "tool_calls", "tool_use":
		return llm.FinishReasonToolCalls
	default:
		if raw != "" {
			return raw
		}
		return llm.FinishReasonStop
	}
}

// Close releases resources held by the adapter. For providers with gRPC
// connections (e.g. vertex via genai.Client), it calls the model's Close
// method. The optional closeFn is called to release HTTP idle connections
// StreamChat uses LangChainGo's WithStreamingFunc to forward text deltas
// to the callback incrementally. The final ChatResponse is built from the
// complete ContentResponse return value.
func (a *Adapter) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	if len(req.Options.OutputSchema) > 0 {
		ctx = transport.WithOutputSchema(ctx, req.Options.OutputSchema)
	}
	msgs := toMessages(req.Messages)
	opts := buildCallOptions(req)
	opts = append(opts, llms.WithStreamingFunc(func(_ context.Context, chunk []byte) error {
		return callback(llm.ChatStreamEvent{Delta: string(chunk)})
	}))
	resp, err := a.model.GenerateContent(ctx, msgs, opts...)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	_ = callback(llm.ChatStreamEvent{Done: true})
	return fromContentResponse(resp), nil
}

// from the custom transport chain.
func (a *Adapter) Close() error {
	var firstErr error
	if c, ok := a.model.(interface{ Close() error }); ok {
		firstErr = c.Close()
	}
	if a.closeFn != nil {
		if err := a.closeFn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// compile-time interface check
var _ llm.Client = (*Adapter)(nil)
