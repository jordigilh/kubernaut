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

package llm_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func floatPtr(v float64) *float64 { return &v }

var _ = Describe("LLM Types — #433", func() {

	Describe("UT-KA-433-004: ChatRequest/ChatResponse round-trip serialization", func() {
		It("should preserve all fields through JSON marshal/unmarshal", func() {
			original := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: "You are a K8s investigator."},
					{Role: "user", Content: "Investigate OOMKilled in namespace prod"},
					{Role: "assistant", Content: "I'll check the pods.", ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-abc","namespace":"prod"}`},
					}},
					{Role: "tool", Content: `{"status":"Running"}`, ToolCallID: "tc_1"},
				},
				Tools: []llm.ToolDefinition{
					{
						Name:        "kubectl_describe",
						Description: "Describe a K8s resource",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`),
					},
				},
			Options: llm.ChatOptions{
				Temperature: floatPtr(0.1),
				MaxTokens:   4096,
				JSONMode:    true,
			},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored llm.ChatRequest
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Messages).To(HaveLen(4))
			Expect(restored.Messages[0].Role).To(Equal("system"))
			Expect(restored.Messages[2].ToolCalls).To(HaveLen(1))
			Expect(restored.Messages[2].ToolCalls[0].Name).To(Equal("kubectl_describe"))
			Expect(restored.Messages[3].ToolCallID).To(Equal("tc_1"))
			Expect(restored.Tools).To(HaveLen(1))
			Expect(restored.Tools[0].Name).To(Equal("kubectl_describe"))
			Expect(restored.Options.Temperature).NotTo(BeNil())
			Expect(*restored.Options.Temperature).To(BeNumerically("~", 0.1, 0.001))
			Expect(restored.Options.MaxTokens).To(Equal(4096))
			Expect(restored.Options.JSONMode).To(BeTrue())

			respOriginal := llm.ChatResponse{
				Message: llm.Message{Role: "assistant", Content: "Investigation complete."},
				ToolCalls: []llm.ToolCall{
					{ID: "tc_2", Name: "list_workflows", Arguments: `{"action_type":"IncreaseMemory"}`},
				},
				Usage: llm.TokenUsage{
					PromptTokens:     1500,
					CompletionTokens: 300,
					TotalTokens:      1800,
				},
			}

			respData, err := json.Marshal(respOriginal)
			Expect(err).NotTo(HaveOccurred())

			var respRestored llm.ChatResponse
			err = json.Unmarshal(respData, &respRestored)
			Expect(err).NotTo(HaveOccurred())

			Expect(respRestored.Message.Role).To(Equal("assistant"))
			Expect(respRestored.ToolCalls).To(HaveLen(1))
			Expect(respRestored.ToolCalls[0].Name).To(Equal("list_workflows"))
			Expect(respRestored.Usage.TotalTokens).To(Equal(1800))
		})
	})

	Describe("UT-KA-1578-001: provider-agnostic reasoning fields — BR-AI-086 AC1/AC2", func() {
		It("should default Message.Reasoning and ChatOptions.Reasoning to nil and omit them from JSON", func() {
			msg := llm.Message{Role: "assistant", Content: "plain conclusion, no deliberation captured"}
			opts := llm.ChatOptions{MaxTokens: 100}

			Expect(msg.Reasoning).To(BeNil())
			Expect(opts.Reasoning).To(BeNil())

			msgData, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(msgData)).NotTo(ContainSubstring("reasoning"))

			optsData, err := json.Marshal(opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(optsData)).NotTo(ContainSubstring("reasoning"))
		})

		It("should round-trip a populated ReasoningBlock on Message, including opaque signature and redaction", func() {
			original := llm.Message{
				Role:    "assistant",
				Content: "The pod is OOMKilled because of a memory leak.",
				Reasoning: &llm.ReasoningBlock{
					Text:      "Let me check the memory usage pattern over time...",
					Signature: "opaque-provider-signature-bytes",
					Redacted:  false,
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored llm.Message
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Reasoning).NotTo(BeNil())
			Expect(restored.Reasoning.Text).To(Equal("Let me check the memory usage pattern over time..."))
			Expect(restored.Reasoning.Signature).To(Equal("opaque-provider-signature-bytes"))
			Expect(restored.Reasoning.Redacted).To(BeFalse())
		})

		It("should round-trip a redacted ReasoningBlock with empty visible text (Anthropic redacted_thinking)", func() {
			original := llm.Message{
				Role:    "assistant",
				Content: "conclusion only",
				Reasoning: &llm.ReasoningBlock{
					Signature: "encrypted-redacted-payload",
					Redacted:  true,
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored llm.Message
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Reasoning).NotTo(BeNil())
			Expect(restored.Reasoning.Text).To(BeEmpty())
			Expect(restored.Reasoning.Redacted).To(BeTrue())
		})

		It("should round-trip a ReasoningRequest on ChatOptions with Enabled and BudgetTokens", func() {
			original := llm.ChatOptions{
				MaxTokens: 4096,
				Reasoning: &llm.ReasoningRequest{
					Enabled:      true,
					BudgetTokens: 2048,
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored llm.ChatOptions
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Reasoning).NotTo(BeNil())
			Expect(restored.Reasoning.Enabled).To(BeTrue())
			Expect(restored.Reasoning.BudgetTokens).To(Equal(2048))
		})

		It("should round-trip a ReasoningRequest's Effort field (#1604)", func() {
			original := llm.ChatOptions{
				Reasoning: &llm.ReasoningRequest{
					Enabled: true,
					Effort:  "high",
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored llm.ChatOptions
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Reasoning).NotTo(BeNil())
			Expect(restored.Reasoning.Effort).To(Equal("high"))
		})
	})
})
