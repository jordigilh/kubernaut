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

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Adapter implements llm.Client by delegating to LangChainGo.
// Authority: DD-HAPI-019-001 — Framework Isolation Pattern
type Adapter struct {
	model llms.Model
}

// New creates a new LangChainGo adapter for the given provider.
// Supported providers: "openai", "ollama".
func New(provider, endpoint, model, apiKey string) (*Adapter, error) {
	m, err := newModel(provider, endpoint, model, apiKey)
	if err != nil {
		return nil, fmt.Errorf("langchaingo: %w", err)
	}
	return &Adapter{model: m}, nil
}

func newModel(provider, endpoint, model, apiKey string) (llms.Model, error) {
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
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %q", provider)
	}
}

// Chat translates a Kubernaut ChatRequest into LangChainGo's MessageContent
// format, calls GenerateContent, and maps the response back.
func (a *Adapter) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
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

// compile-time interface check
var _ llm.Client = (*Adapter)(nil)
