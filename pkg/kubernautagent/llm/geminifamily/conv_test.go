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

	"github.com/cloudwego/eino/schema"
	gemini_schema "github.com/cloudwego/eino/schema/gemini"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// UT-GM-1778: toEinoMessages maps Kubernaut's role-tagged history to eino's
// schema.AgenticMessage list (BR-AI-087).
var _ = Describe("toEinoMessages — #1778 BR-AI-087", func() {
	It("UT-GM-1778-001: maps a leading system message to AgenticRoleTypeSystem", func() {
		msgs, err := toEinoMessages([]llm.Message{
			{Role: "system", Content: "You are a Kubernetes investigator."},
			{Role: "user", Content: "Why is the pod crashing?"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(msgs).To(HaveLen(2))
		Expect(msgs[0].Role).To(Equal(schema.AgenticRoleTypeSystem))
		Expect(msgs[1].Role).To(Equal(schema.AgenticRoleTypeUser))
	})

	It("UT-GM-1778-002: maps a tool message to a FunctionToolResult content block", func() {
		msgs, err := toEinoMessages([]llm.Message{
			{Role: "tool", Content: `{"restartCount":5}`, ToolCallID: "toolu_001", ToolName: "kubectl_describe"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(msgs).To(HaveLen(1))
		Expect(msgs[0].Role).To(Equal(schema.AgenticRoleTypeUser))
		Expect(msgs[0].ContentBlocks).To(HaveLen(1))
		block := msgs[0].ContentBlocks[0]
		Expect(block.Type).To(Equal(schema.ContentBlockTypeFunctionToolResult))
		Expect(block.FunctionToolResult.CallID).To(Equal("toolu_001"))
		Expect(block.FunctionToolResult.Name).To(Equal("kubectl_describe"))
	})

	It("UT-GM-1778-003: rejects unsupported message roles", func() {
		_, err := toEinoMessages([]llm.Message{{Role: "bogus", Content: "x"}})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported message role"))
	})

	It("UT-GM-1778-004: skips assistant messages with no content, reasoning, or tool calls", func() {
		msgs, err := toEinoMessages([]llm.Message{{Role: "assistant", Content: ""}})
		Expect(err).NotTo(HaveOccurred())
		Expect(msgs).To(BeEmpty())
	})
})

// UT-GM-1778: convertAssistantMessage covers reasoning + tool-call + thought
// signature replay — the core Gemini-3 multi-turn function-calling contract
// discovered in DD-LLM-010's spike.
var _ = Describe("convertAssistantMessage — #1778 BR-AI-087", func() {
	It("UT-GM-1778-010: maps plain assistant text with no reasoning or tool calls", func() {
		am := convertAssistantMessage(llm.Message{Role: "assistant", Content: "The pod is OOMKilled."})
		Expect(am).NotTo(BeNil())
		Expect(am.Role).To(Equal(schema.AgenticRoleTypeAssistant))
		Expect(am.ContentBlocks).To(HaveLen(1))
		Expect(am.ContentBlocks[0].Type).To(Equal(schema.ContentBlockTypeAssistantGenText))
		Expect(am.ContentBlocks[0].AssistantGenText.Text).To(Equal("The pod is OOMKilled."))
		Expect(am.ResponseMeta).To(BeNil(), "no reasoning present, so no self-generated marker is needed")
	})

	It("UT-GM-1778-011: emits a reasoning block and sets the self-generated marker", func() {
		am := convertAssistantMessage(llm.Message{
			Role:      "assistant",
			Content:   "Investigating.",
			Reasoning: &llm.ReasoningBlock{Text: "checking restart count first"},
		})
		Expect(am).NotTo(BeNil())
		Expect(am.ContentBlocks).To(HaveLen(2))
		Expect(am.ContentBlocks[0].Type).To(Equal(schema.ContentBlockTypeReasoning))
		Expect(am.ContentBlocks[0].Reasoning.Text).To(Equal("checking restart count first"))
		Expect(am.ResponseMeta).NotTo(BeNil())
		Expect(am.ResponseMeta.GeminiExtension).NotTo(BeNil(), "must satisfy agenticgemini's isSelfGeneratedMessage whitelist")
	})

	It("UT-GM-1778-012: replays the thought signature on the first tool-call block when tool calls are present", func() {
		sig := []byte("opaque-signature-bytes")
		am := convertAssistantMessage(llm.Message{
			Role:      "assistant",
			Reasoning: &llm.ReasoningBlock{Text: "let me check", Signature: base64.StdEncoding.EncodeToString(sig)},
			ToolCalls: []llm.ToolCall{
				{ID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`},
				{ID: "toolu_002", Name: "kubectl_logs", Arguments: `{"name":"pod-a"}`},
			},
		})
		Expect(am).NotTo(BeNil())

		var toolCallBlocks []*schema.ContentBlock
		var reasoningBlock *schema.ContentBlock
		for _, b := range am.ContentBlocks {
			switch b.Type {
			case schema.ContentBlockTypeFunctionToolCall:
				toolCallBlocks = append(toolCallBlocks, b)
			case schema.ContentBlockTypeReasoning:
				reasoningBlock = b
			}
		}
		Expect(toolCallBlocks).To(HaveLen(2))
		Expect(extractThoughtSignature(toolCallBlocks[0])).To(Equal(sig), "signature must attach to the FIRST tool-call block, mandatory for Gemini 3 multi-turn function calling")
		Expect(extractThoughtSignature(toolCallBlocks[1])).To(BeEmpty(), "signature must not be duplicated onto subsequent tool-call blocks")
		Expect(extractThoughtSignature(reasoningBlock)).To(BeEmpty())
	})

	It("UT-GM-1778-013: replays the thought signature on the reasoning block when there are no tool calls", func() {
		sig := []byte("reasoning-only-signature")
		am := convertAssistantMessage(llm.Message{
			Role:      "assistant",
			Content:   "Done.",
			Reasoning: &llm.ReasoningBlock{Text: "no tools needed", Signature: base64.StdEncoding.EncodeToString(sig)},
		})
		Expect(am).NotTo(BeNil())
		Expect(extractThoughtSignature(am.ContentBlocks[0])).To(Equal(sig))
	})

	It("UT-GM-1778-014: defaults empty tool-call arguments to an empty JSON object", func() {
		am := convertAssistantMessage(llm.Message{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "toolu_001", Name: "noop", Arguments: ""}},
		})
		Expect(am).NotTo(BeNil())
		Expect(am.ContentBlocks[0].FunctionToolCall.Arguments).To(Equal("{}"))
	})

	It("UT-GM-1778-015: malformed base64 signature is silently ignored rather than failing the request", func() {
		am := convertAssistantMessage(llm.Message{
			Role:      "assistant",
			Content:   "ok",
			Reasoning: &llm.ReasoningBlock{Text: "x", Signature: "not-valid-base64!!"},
		})
		Expect(am).NotTo(BeNil())
	})
})

var _ = Describe("attachThoughtSignature / extractThoughtSignature round-trip — #1778", func() {
	It("UT-GM-1778-020: round-trips signature bytes through ContentBlock.Extra", func() {
		cb := &schema.ContentBlock{}
		attachThoughtSignature(cb, []byte("abc123"))
		Expect(extractThoughtSignature(cb)).To(Equal([]byte("abc123")))
	})

	It("UT-GM-1778-021: no-ops on a nil block or empty signature", func() {
		attachThoughtSignature(nil, []byte("x")) // must not panic
		cb := &schema.ContentBlock{}
		attachThoughtSignature(cb, nil)
		Expect(cb.Extra).To(BeNil())
	})

	It("UT-GM-1778-022: extractThoughtSignature returns nil for a block with no Extra", func() {
		Expect(extractThoughtSignature(&schema.ContentBlock{})).To(BeNil())
		Expect(extractThoughtSignature(nil)).To(BeNil())
	})
})

var _ = Describe("toEinoTools — #1778 BR-AI-087", func() {
	It("UT-GM-1778-030: translates tool definitions including JSON schema parameters", func() {
		tools := toEinoTools([]llm.ToolDefinition{
			{
				Name:        "kubectl_describe",
				Description: "Describe a Kubernetes resource",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}},"required":["kind"]}`),
			},
		})
		Expect(tools).To(HaveLen(1))
		Expect(tools[0].Name).To(Equal("kubectl_describe"))
		Expect(tools[0].Desc).To(Equal("Describe a Kubernetes resource"))
		Expect(tools[0].ParamsOneOf).NotTo(BeNil())
	})

	It("UT-GM-1778-031: tolerates malformed parameter schemas rather than failing", func() {
		tools := toEinoTools([]llm.ToolDefinition{
			{Name: "broken", Description: "x", Parameters: json.RawMessage(`not-json`)},
		})
		Expect(tools).To(HaveLen(1))
		Expect(tools[0].Name).To(Equal("broken"))
	})

	It("UT-GM-1778-032: empty tool list yields an empty (non-nil) slice", func() {
		tools := toEinoTools(nil)
		Expect(tools).To(BeEmpty())
	})
})

var _ = Describe("fromEinoMessage — #1778 BR-AI-087", func() {
	It("UT-GM-1778-040: maps text content and token usage", func() {
		msg := &schema.AgenticMessage{
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.AssistantGenText{Text: "The pod is OOMKilled."}),
			},
			ResponseMeta: &schema.AgenticResponseMeta{
				TokenUsage: &schema.TokenUsage{PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60},
			},
		}
		resp := fromEinoMessage(msg)
		Expect(resp.Message.Role).To(Equal("assistant"))
		Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
		Expect(resp.Usage.PromptTokens).To(Equal(50))
		Expect(resp.Usage.CompletionTokens).To(Equal(10))
		Expect(resp.Usage.TotalTokens).To(Equal(60))
		Expect(resp.Message.Reasoning).To(BeNil())
	})

	It("UT-GM-1778-041: maps tool calls into both resp.ToolCalls and resp.Message.ToolCalls, forcing FinishReasonToolCalls", func() {
		msg := &schema.AgenticMessage{
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolCall{CallID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`}),
			},
			ResponseMeta: &schema.AgenticResponseMeta{
				GeminiExtension: &gemini_schema.ResponseMetaExtension{FinishReason: "STOP"},
			},
		}
		resp := fromEinoMessage(msg)
		Expect(resp.ToolCalls).To(HaveLen(1))
		Expect(resp.ToolCalls[0].ID).To(Equal("toolu_001"))
		Expect(resp.Message.ToolCalls).To(HaveLen(1))
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonToolCalls), "tool-call presence must always win over Gemini's STOP finish reason")
	})

	It("UT-GM-1778-042: maps reasoning text and base64-encodes the captured thought signature", func() {
		reasoningBlock := schema.NewContentBlock(&schema.Reasoning{Text: "checking restart count"})
		attachThoughtSignature(reasoningBlock, []byte("sig-bytes"))
		msg := &schema.AgenticMessage{ContentBlocks: []*schema.ContentBlock{reasoningBlock}}

		resp := fromEinoMessage(msg)
		Expect(resp.Message.Reasoning).NotTo(BeNil())
		Expect(resp.Message.Reasoning.Text).To(Equal("checking restart count"))
		decoded, err := base64.StdEncoding.DecodeString(resp.Message.Reasoning.Signature)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal([]byte("sig-bytes")))
	})

	It("UT-GM-1778-043: captures the thought signature from the first block that carries one", func() {
		toolBlock := schema.NewContentBlock(&schema.FunctionToolCall{CallID: "toolu_001", Name: "f", Arguments: "{}"})
		attachThoughtSignature(toolBlock, []byte("first-sig"))
		msg := &schema.AgenticMessage{ContentBlocks: []*schema.ContentBlock{toolBlock}}

		resp := fromEinoMessage(msg)
		Expect(resp.Message.Reasoning).NotTo(BeNil(), "a captured signature alone must produce a ReasoningBlock even without reasoning text")
		decoded, err := base64.StdEncoding.DecodeString(resp.Message.Reasoning.Signature)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded).To(Equal([]byte("first-sig")))
	})

	It("UT-GM-1778-044: maps MAX_TOKENS finish reason to FinishReasonLength", func() {
		msg := &schema.AgenticMessage{
			ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: "partial"})},
			ResponseMeta:  &schema.AgenticResponseMeta{GeminiExtension: &gemini_schema.ResponseMetaExtension{FinishReason: "MAX_TOKENS"}},
		}
		resp := fromEinoMessage(msg)
		Expect(resp.FinishReason).To(Equal(llm.FinishReasonLength))
	})
})

var _ = Describe("normalizeGeminiFinishReason — #1778", func() {
	It("UT-GM-1778-050: maps STOP to FinishReasonStop", func() {
		Expect(normalizeGeminiFinishReason("STOP")).To(Equal(llm.FinishReasonStop))
	})
	It("UT-GM-1778-051: maps MAX_TOKENS to FinishReasonLength", func() {
		Expect(normalizeGeminiFinishReason("MAX_TOKENS")).To(Equal(llm.FinishReasonLength))
	})
	It("UT-GM-1778-052: passes through unrecognized non-empty raw values verbatim", func() {
		Expect(normalizeGeminiFinishReason("SAFETY")).To(Equal("SAFETY"))
	})
	It("UT-GM-1778-053: defaults empty raw value to FinishReasonStop", func() {
		Expect(normalizeGeminiFinishReason("")).To(Equal(llm.FinishReasonStop))
	})
})

var _ = Describe("extractStreamTextDelta — #1778", func() {
	It("UT-GM-1778-060: concatenates assistant text and reasoning text from one chunk", func() {
		chunk := &schema.AgenticMessage{
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.Reasoning{Text: "thinking... "}),
				schema.NewContentBlock(&schema.AssistantGenText{Text: "answer"}),
			},
		}
		Expect(extractStreamTextDelta(chunk)).To(Equal("thinking... answer"))
	})

	It("UT-GM-1778-061: returns empty string for a nil chunk", func() {
		Expect(extractStreamTextDelta(nil)).To(Equal(""))
	})

	It("UT-GM-1778-062: ignores tool-call blocks (no text to surface)", func() {
		chunk := &schema.AgenticMessage{
			ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlock(&schema.FunctionToolCall{CallID: "x", Name: "f", Arguments: "{}"}),
			},
		}
		Expect(extractStreamTextDelta(chunk)).To(Equal(""))
	})
})

var _ = Describe("selfGeneratedMarker — #1778", func() {
	It("UT-GM-1778-070: returns a non-nil GeminiExtension satisfying agenticgemini's whitelist check", func() {
		marker := selfGeneratedMarker()
		Expect(marker).NotTo(BeNil())
		Expect(marker.GeminiExtension).NotTo(BeNil())
	})
})
