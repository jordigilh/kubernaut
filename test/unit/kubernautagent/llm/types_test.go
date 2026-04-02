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
					Temperature: 0.1,
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
			Expect(restored.Options.Temperature).To(BeNumerically("~", 0.1, 0.001))
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
})
