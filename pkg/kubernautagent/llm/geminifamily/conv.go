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

package geminifamily

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	gemini_schema "github.com/cloudwego/eino/schema/gemini"
	einojsonschema "github.com/eino-contrib/jsonschema"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// thoughtSignatureExtraKey MUST stay byte-for-byte identical to
// eino-ext/components/model/agenticgemini's own unexported
// content_block_extra.go:thoughtSignatureExtraKey constant. That accessor
// (getThoughtSignature/setThoughtSignature) is unexported with no public
// equivalent anywhere in the module (confirmed by source inspection and
// `grep -rn ThoughtSignature`, agenticgemini v0.2.2, the latest available
// version per `go list -m -versions`) — see DD-LLM-010's "Discovery Spike
// Findings" section for the full investigation. Reproducing this exact map
// key is the only way to attach/read Gemini's ThoughtSignature bytes
// through the public schema.ContentBlock.Extra map without forking the
// library. This is a known coupling to an unversioned internal contract of
// a third-party dependency, not a Kubernaut design choice; a follow-up
// issue tracks requesting a public accessor upstream. Round-trip coverage
// lives in conv_test.go.
const thoughtSignatureExtraKey = "_eino_ext_agentic_gemini_thought_signature"

// attachThoughtSignature stores raw thought-signature bytes on a content
// block's Extra map using agenticgemini's own private key (see
// thoughtSignatureExtraKey doc comment). agenticgemini's conv.go reads this
// key when converting an outgoing content block to a genai.Part.
func attachThoughtSignature(cb *schema.ContentBlock, sig []byte) {
	if cb == nil || len(sig) == 0 {
		return
	}
	if cb.Extra == nil {
		cb.Extra = make(map[string]any, 1)
	}
	cb.Extra[thoughtSignatureExtraKey] = sig
}

// extractThoughtSignature reads raw thought-signature bytes previously
// stashed by agenticgemini on an incoming (model-returned) content block.
func extractThoughtSignature(cb *schema.ContentBlock) []byte {
	if cb == nil || cb.Extra == nil {
		return nil
	}
	if v, ok := cb.Extra[thoughtSignatureExtraKey].([]byte); ok {
		return v
	}
	return nil
}

// selfGeneratedMarker is a minimal non-nil schema.AgenticResponseMeta that
// satisfies agenticgemini's isSelfGeneratedMessage check
// (ResponseMeta.GeminiExtension != nil). Without it, agenticgemini's
// non-self-generated whitelist silently drops ContentBlockTypeReasoning
// blocks on replay (FunctionToolCall/FunctionToolResult are whitelisted
// regardless and need no marker) — see DD-LLM-010's spike findings.
func selfGeneratedMarker() *schema.AgenticResponseMeta {
	return &schema.AgenticResponseMeta{GeminiExtension: &gemini_schema.ResponseMetaExtension{}}
}

// toEinoMessages translates Kubernaut's role-tagged message history into
// eino's schema.AgenticMessage list. A leading "system" message becomes a
// dedicated schema.AgenticRoleTypeSystem message — agenticgemini's
// genInputAndConf extracts it into Gemini's SystemInstruction only when it
// is message index 0, matching how callers already order KA's Messages
// (mirroring anthropicfamily's convertMessagesToAnthropic system handling).
func toEinoMessages(messages []llm.Message) ([]*schema.AgenticMessage, error) {
	out := make([]*schema.AgenticMessage, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			out = append(out, &schema.AgenticMessage{
				Role:          schema.AgenticRoleTypeSystem,
				ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: m.Content})},
			})
		case "user":
			out = append(out, &schema.AgenticMessage{
				Role:          schema.AgenticRoleTypeUser,
				ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: m.Content})},
			})
		case "assistant":
			if am := convertAssistantMessage(m); am != nil {
				out = append(out, am)
			}
		case "tool":
			out = append(out, &schema.AgenticMessage{
				Role: schema.AgenticRoleTypeUser,
				ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolResult{
					CallID: m.ToolCallID,
					Name:   m.ToolName,
					Content: []*schema.FunctionToolResultContentBlock{{
						Type: schema.FunctionToolResultContentBlockTypeText,
						Text: &schema.UserInputText{Text: m.Content},
					}},
				})},
			})
		default:
			return nil, fmt.Errorf("geminifamily: unsupported message role %q", m.Role)
		}
	}
	return out, nil
}

// convertAssistantMessage builds the eino assistant message for a single
// Kubernaut assistant-role message (reasoning, text, and/or tool-call
// blocks). Returns nil when there is nothing to emit.
//
// A captured reasoning signature is replayed on whichever content block
// Google's docs say actually carries it: the first tool-call block when
// present (mandatory for Gemini 3 function calling — a missing signature
// there is a hard 400, see DD-LLM-010), otherwise the reasoning block
// itself. When a reasoning block is emitted, the message is marked
// self-generated (selfGeneratedMarker) so agenticgemini's non-self-generated
// whitelist does not silently drop it.
func convertAssistantMessage(m llm.Message) *schema.AgenticMessage {
	var blocks []*schema.ContentBlock
	var reasoningBlock *schema.ContentBlock

	if m.Reasoning != nil && m.Reasoning.Text != "" {
		reasoningBlock = schema.NewContentBlock(&schema.Reasoning{Text: m.Reasoning.Text})
		blocks = append(blocks, reasoningBlock)
	}
	if m.Content != "" {
		blocks = append(blocks, schema.NewContentBlock(&schema.AssistantGenText{Text: m.Content}))
	}

	var firstToolCallBlock *schema.ContentBlock
	for _, tc := range m.ToolCalls {
		args := tc.Arguments
		if args == "" {
			args = "{}"
		}
		block := schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    tc.ID,
			Name:      tc.Name,
			Arguments: args,
		})
		if firstToolCallBlock == nil {
			firstToolCallBlock = block
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return nil
	}

	if m.Reasoning != nil && m.Reasoning.Signature != "" {
		if sig, err := base64.StdEncoding.DecodeString(m.Reasoning.Signature); err == nil {
			switch {
			case firstToolCallBlock != nil:
				attachThoughtSignature(firstToolCallBlock, sig)
			case reasoningBlock != nil:
				attachThoughtSignature(reasoningBlock, sig)
			}
		}
	}

	msg := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: blocks}
	if reasoningBlock != nil {
		msg.ResponseMeta = selfGeneratedMarker()
	}
	return msg
}

// toEinoTools translates Kubernaut's provider-agnostic tool definitions
// into eino's schema.ToolInfo list, tolerating malformed parameter schemas
// (logged, falls back to an empty schema) rather than failing the whole
// request — mirroring anthropicfamily's buildAnthropicTools/parseInputSchema
// resilience, including the log-on-malformed-schema observability (Fail-Open
// Safety: no silent failures).
func toEinoTools(toolDefs []llm.ToolDefinition, logger logr.Logger) []*schema.ToolInfo {
	tools := make([]*schema.ToolInfo, 0, len(toolDefs))
	for _, td := range toolDefs {
		s := &einojsonschema.Schema{}
		if len(td.Parameters) > 0 {
			if err := json.Unmarshal(td.Parameters, s); err != nil {
				logger.Info("geminifamily: malformed tool parameter schema, using empty schema",
					"tool", td.Name, "error", err.Error())
			}
		}
		tools = append(tools, &schema.ToolInfo{
			Name:        td.Name,
			Desc:        td.Description,
			ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
		})
	}
	return tools
}

// fromEinoMessage maps an eino schema.AgenticMessage (the final, fully
// concatenated response — either Generate's direct return or the result of
// schema.ConcatAgenticMessages over a stream) back into a Kubernaut
// llm.ChatResponse.
//
// The thought signature is captured from the FIRST content block that
// carries one, regardless of block type — Google's docs place it on the
// first function-call part when a tool call is present, or the last part
// otherwise (see DD-LLM-010) — and stored as an opaque, base64-encoded
// string on Message.Reasoning.Signature, reusing KA's existing
// verbatim-replay contract (llm.ReasoningBlock doc comment) rather than
// adding a Gemini-specific field.
func fromEinoMessage(msg *schema.AgenticMessage) llm.ChatResponse {
	resp := llm.ChatResponse{Message: llm.Message{Role: "assistant"}}

	acc := accumulateContentBlocks(msg.ContentBlocks)
	if acc.hasReasoning || len(acc.signature) > 0 {
		resp.Message.Reasoning = &llm.ReasoningBlock{
			Text:      acc.reasoningText,
			Signature: base64.StdEncoding.EncodeToString(acc.signature),
		}
	}
	resp.Message.Content = strings.Join(acc.textParts, "")
	resp.ToolCalls = acc.toolCalls
	resp.Message.ToolCalls = acc.toolCalls

	resp.Usage, resp.FinishReason = mapResponseMeta(msg.ResponseMeta)
	// Gemini reports FinishReason "STOP" even when the turn ends in a
	// function call (no dedicated "tool_calls" finish reason on the wire),
	// so tool-call presence always takes priority — mirroring how
	// anthropicfamily's stop_reason "tool_use" is the authoritative signal
	// there.
	if len(resp.ToolCalls) > 0 {
		resp.FinishReason = llm.FinishReasonToolCalls
	}

	return resp
}

// contentBlockAccumulation holds the running state built up while walking
// an AgenticMessage's ContentBlocks — extracted from fromEinoMessage to
// keep its cognitive complexity within the AGENTS.md Go anti-pattern
// budget.
type contentBlockAccumulation struct {
	textParts     []string
	reasoningText string
	hasReasoning  bool
	signature     []byte
	toolCalls     []llm.ToolCall
}

// accumulateContentBlocks walks a response's content blocks once, collecting
// assistant text, the first captured thought signature (Google's docs place
// it on the first function-call part when present, or the last part
// otherwise — see DD-LLM-010), reasoning text, and tool calls.
func accumulateContentBlocks(blocks []*schema.ContentBlock) contentBlockAccumulation {
	var acc contentBlockAccumulation
	for _, block := range blocks {
		if block == nil {
			continue
		}
		if len(acc.signature) == 0 {
			if sig := extractThoughtSignature(block); len(sig) > 0 {
				acc.signature = sig
			}
		}
		switch block.Type {
		case schema.ContentBlockTypeReasoning:
			if block.Reasoning != nil {
				acc.reasoningText = block.Reasoning.Text
				acc.hasReasoning = true
			}
		case schema.ContentBlockTypeAssistantGenText:
			if block.AssistantGenText != nil {
				acc.textParts = append(acc.textParts, block.AssistantGenText.Text)
			}
		case schema.ContentBlockTypeFunctionToolCall:
			if block.FunctionToolCall != nil {
				acc.toolCalls = append(acc.toolCalls, llm.ToolCall{
					ID:        block.FunctionToolCall.CallID,
					Name:      block.FunctionToolCall.Name,
					Arguments: block.FunctionToolCall.Arguments,
				})
			}
		}
	}
	return acc
}

// mapResponseMeta translates eino's AgenticResponseMeta (token usage +
// Gemini's own finish-reason extension) into KA's TokenUsage/FinishReason
// pair. Returns zero values when meta is nil (e.g. a partial stream chunk).
func mapResponseMeta(meta *schema.AgenticResponseMeta) (llm.TokenUsage, string) {
	if meta == nil {
		return llm.TokenUsage{}, ""
	}
	var usage llm.TokenUsage
	if meta.TokenUsage != nil {
		usage = llm.TokenUsage{
			PromptTokens:     meta.TokenUsage.PromptTokens,
			CompletionTokens: meta.TokenUsage.CompletionTokens,
			TotalTokens:      meta.TokenUsage.TotalTokens,
		}
	}
	var finishReason string
	if meta.GeminiExtension != nil {
		finishReason = normalizeGeminiFinishReason(meta.GeminiExtension.FinishReason)
	}
	return usage, finishReason
}

// normalizeGeminiFinishReason maps Gemini's finish-reason wire values to
// our canonical FinishReason constants.
func normalizeGeminiFinishReason(raw string) string {
	switch raw {
	case "STOP":
		return llm.FinishReasonStop
	case "MAX_TOKENS":
		return llm.FinishReasonLength
	default:
		if raw != "" {
			return raw
		}
		return llm.FinishReasonStop
	}
}

// extractStreamTextDelta concatenates the incremental text carried by one
// streamed AgenticMessage chunk — assistant text and/or reasoning text —
// for forwarding as a single llm.ChatStreamEvent.Delta, mirroring
// anthropicfamily's extractTextDelta (#1775 interleaved-thinking parity).
func extractStreamTextDelta(chunk *schema.AgenticMessage) string {
	if chunk == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range chunk.ContentBlocks {
		if block == nil {
			continue
		}
		switch block.Type {
		case schema.ContentBlockTypeAssistantGenText:
			if block.AssistantGenText != nil {
				sb.WriteString(block.AssistantGenText.Text)
			}
		case schema.ContentBlockTypeReasoning:
			if block.Reasoning != nil {
				sb.WriteString(block.Reasoning.Text)
			}
		}
	}
	return sb.String()
}
