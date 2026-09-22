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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
)

var _ = Describe("Mock LLM provider adapters", func() {
	Describe("UT-MOCK-2442-007: OpenAI and Gemini semantic parity", func() {
		It("correlates OpenAI parallel tool results by tool call ID", func() {
			transcript := handlers.NormalizeOpenAITranscript(openai.ChatCompletionRequest{
				Tools: []openai.Tool{
					{Function: openai.ToolDefinition{Name: "list_workflows"}},
					{Function: openai.ToolDefinition{Name: "kubectl_logs"}},
				},
				Messages: []openai.Message{
					{Role: "assistant", ToolCalls: []openai.ToolCall{
						{ID: "call-workflows", Function: openai.FunctionCall{Name: "list_workflows"}},
						{ID: "call-logs", Function: openai.FunctionCall{Name: "kubectl_logs"}},
					}},
					{Role: "tool", ToolCallID: "call-logs", Content: stringPtr(`{"logs":"ok"}`)},
					{Role: "tool", ToolCallID: "call-workflows", Content: stringPtr(`{"workflows":[]}`)},
				},
			})

			Expect(transcript.Events).To(HaveLen(4))
			Expect(transcript.Events[2].ToolName).To(Equal("kubectl_logs"))
			Expect(transcript.Events[3].ToolName).To(Equal("list_workflows"))
		})

		It("does not infer a result name when an unknown tool call ID is supplied", func() {
			transcript := handlers.NormalizeOpenAITranscript(openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{Role: "assistant", ToolCalls: []openai.ToolCall{
						{ID: "call-workflows", Function: openai.FunctionCall{Name: "list_workflows"}},
					}},
					{Role: "tool", ToolCallID: "unknown-call", Content: stringPtr(`{"workflows":[]}`)},
				},
			})

			Expect(transcript.Events).To(HaveLen(1))
			Expect(transcript.Events[0].Kind).To(Equal(conversation.DiscoveryToolCallEvent))
		})

		It("normalizes equivalent Gemini function calls and responses", func() {
			transcript, err := handlers.NormalizeGeminiTranscript(
				[]response.GeminiContent{
					{Role: "model", Parts: []response.GeminiPart{{FunctionCall: &response.GeminiFunctionCall{Name: "list_workflows"}}}},
					{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
						Name: "list_workflows", Response: map[string]interface{}{"workflows": []interface{}{}},
					}}}},
				},
				[]response.GeminiToolDecl{{FunctionDeclarations: []response.GeminiFunctionDecl{{Name: "list_workflows"}}}},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(transcript.AdvertisedTools).To(ConsistOf("list_workflows"))
			Expect(transcript.Events).To(HaveLen(2))
			Expect(transcript.Events[0].Kind).To(Equal(conversation.DiscoveryToolCallEvent))
			Expect(transcript.Events[1].ToolName).To(Equal("list_workflows"))
			Expect(transcript.Events[1].Payload).To(ContainSubstring("workflows"))
		})
	})
})
