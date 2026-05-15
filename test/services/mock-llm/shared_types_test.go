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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

var _ = Describe("Shared Types", func() {

	Describe("UT-MOCK-060-001: ChatCompletionResponse marshals to JSON with all required fields", func() {
		It("should produce JSON with id, object, created, model, choices, usage keys", func() {
			resp := openai.ChatCompletionResponse{
				ID:      "chatcmpl-abc12345",
				Object:  "chat.completion",
				Created: 1701388800,
				Model:   "mock-model",
				Choices: []openai.Choice{
					{
						Index: 0,
						Message: openai.Message{
							Role:    "assistant",
							Content: stringPtr("Hello"),
						},
						FinishReason: "stop",
					},
				},
				Usage: openai.Usage{
					PromptTokens:     100,
					CompletionTokens: 50,
					TotalTokens:      150,
				},
			}

			data, err := json.Marshal(resp)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())

			Expect(raw).To(HaveKey("id"))
			Expect(raw).To(HaveKey("object"))
			Expect(raw).To(HaveKey("created"))
			Expect(raw).To(HaveKey("model"))
			Expect(raw).To(HaveKey("choices"))
			Expect(raw).To(HaveKey("usage"))

			Expect(raw["id"]).To(Equal("chatcmpl-abc12345"))
			Expect(raw["object"]).To(Equal("chat.completion"))
			Expect(raw["created"]).To(BeNumerically("==", 1701388800))
			Expect(raw["model"]).To(Equal("mock-model"))

			choices := raw["choices"].([]interface{})
			Expect(choices).To(HaveLen(1))
			choice := choices[0].(map[string]interface{})
			Expect(choice["finish_reason"]).To(Equal("stop"))
			msg := choice["message"].(map[string]interface{})
			Expect(msg["role"]).To(Equal("assistant"))
			Expect(msg["content"]).To(Equal("Hello"))

			usage := raw["usage"].(map[string]interface{})
			Expect(usage["prompt_tokens"]).To(BeNumerically("==", 100))
			Expect(usage["completion_tokens"]).To(BeNumerically("==", 50))
			Expect(usage["total_tokens"]).To(BeNumerically("==", 150))
		})
	})

	Describe("UT-MOCK-061-001: Tool name constants match Python string values exactly", func() {
		It("should define tool name constants matching Python's string values", func() {
			Expect(openai.ToolSearchWorkflowCatalog).To(Equal("search_workflow_catalog"))
			Expect(openai.ToolListAvailableActions).To(Equal("list_available_actions"))
			Expect(openai.ToolListWorkflows).To(Equal("list_workflows"))
			Expect(openai.ToolGetWorkflow).To(Equal("get_workflow"))
			Expect(openai.ToolGetResourceContext).To(Equal("get_resource_context"))
			Expect(openai.ToolFetchKubernetesResourceYaml).To(Equal("fetchKubernetesResourceYaml"))
			Expect(openai.ToolListNamespacedEvents).To(Equal("listNamespacedEvents"))
		})
	})

	Describe("UT-MOCK-062-001: OllamaChatResponse marshals to JSON with required fields", func() {
		It("should produce JSON with done, created_at, total_duration, message keys", func() {
			resp := openai.OllamaChatResponse{
				Model:           "mock-model",
				CreatedAt:       "2025-11-30T00:00:00Z",
				Response:        "test content",
				Done:            true,
				Context:         []int{},
				TotalDuration:   1000000000,
				LoadDuration:    100000000,
				PromptEvalCount: 100,
				EvalCount:       50,
			}

			data, err := json.Marshal(resp)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())

			Expect(raw).To(HaveKey("model"))
			Expect(raw).To(HaveKey("created_at"))
			Expect(raw).To(HaveKey("response"))
			Expect(raw).To(HaveKey("done"))
			Expect(raw).To(HaveKey("context"))
			Expect(raw).To(HaveKey("total_duration"))
			Expect(raw).To(HaveKey("load_duration"))
			Expect(raw).To(HaveKey("prompt_eval_count"))
			Expect(raw).To(HaveKey("eval_count"))

			Expect(raw["model"]).To(Equal("mock-model"))
			Expect(raw["created_at"]).To(Equal("2025-11-30T00:00:00Z"))
			Expect(raw["done"]).To(BeTrue())
			Expect(raw["total_duration"]).To(BeNumerically("==", 1000000000))
		})
	})
})

func stringPtr(s string) *string {
	return &s
}
