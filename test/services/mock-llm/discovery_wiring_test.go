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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Mock LLM discovery planner wiring", func() {
	It("IT-MOCK-2442-001: routes OpenAI discovery by workflow membership", func() {
		registry := scenarios.DefaultRegistry()
		scenario, ok := registry.Get("oomkilled")
		Expect(ok).To(BeTrue())
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		expectedWorkflowID := configured.Config().WorkflowID

		testServer := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
		defer testServer.Close()

		messages := []openai.Message{{Role: "user", Content: stringPtr("Signal Name: OOMKilled\nNamespace: production")}}
		tools := openAIDiscoveryTools()

		first := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		Expect(first.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolListAvailableActions))
		appendOpenAIToolResult(&messages, first, `{"actions":[]}`)

		second := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		Expect(second.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolListWorkflows))
		appendOpenAIToolResult(&messages, second, `{"workflows":[{"workflow_id":"`+expectedWorkflowID+`"}]}`)

		third := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		Expect(third.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolGetWorkflow))
		Expect(third.Choices[0].Message.ToolCalls[0].Function.Arguments).To(ContainSubstring(expectedWorkflowID))
	})

	It("IT-MOCK-2442-002: routes Gemini discovery by workflow membership", func() {
		registry := scenarios.DefaultRegistry()
		scenario, ok := registry.Get("oomkilled")
		Expect(ok).To(BeTrue())
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		expectedWorkflowID := configured.Config().WorkflowID

		testServer := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
		defer testServer.Close()

		contents := []response.GeminiContent{{Role: "user", Parts: []response.GeminiPart{{Text: "Signal Name: OOMKilled\nNamespace: production"}}}}
		tools := geminiDiscoveryTools()

		first := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		Expect(first.Candidates[0].Content.Parts[0].FunctionCall.Name).To(Equal(openai.ToolListAvailableActions))
		appendGeminiFunctionResult(&contents, first, openai.ToolListAvailableActions, map[string]interface{}{"actions": []interface{}{}})

		second := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		Expect(second.Candidates[0].Content.Parts[0].FunctionCall.Name).To(Equal(openai.ToolListWorkflows))
		appendGeminiFunctionResult(&contents, second, openai.ToolListWorkflows, map[string]interface{}{"workflows": []interface{}{map[string]interface{}{"workflow_id": expectedWorkflowID}}})

		third := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		Expect(third.Candidates[0].Content.Parts[0].FunctionCall.Name).To(Equal(openai.ToolGetWorkflow))
		Expect(third.Candidates[0].Content.Parts[0].FunctionCall.Args).To(HaveKeyWithValue("workflow_id", expectedWorkflowID))
	})

	It("IT-MOCK-2442-015: global force-text does not suppress advertised discovery", func() {
		testServer := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistry(), true, config.ModeFull))
		defer testServer.Close()

		result := postOpenAI(testServer.URL, openai.ChatCompletionRequest{
			Model:    "mock",
			Messages: []openai.Message{{Role: "user", Content: stringPtr("an unclassified alert")}},
			Tools:    openAIDiscoveryTools(),
		})

		Expect(result.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolListAvailableActions))
	})

	It("IT-MOCK-2442-013: OpenAI refuses an undiscovered workflow", func() {
		testServer := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistry(), false, config.ModeFull))
		defer testServer.Close()

		messages := []openai.Message{{Role: "user", Content: stringPtr("Signal Name: OOMKilled")}}
		tools := openAIDiscoveryTools()
		first := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		appendOpenAIToolResult(&messages, first, `{"actions":[]}`)
		second := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		appendOpenAIToolResult(&messages, second, `{"workflows":[{"workflow_id":"other-workflow"}]}`)

		third := postOpenAI(testServer.URL, openai.ChatCompletionRequest{Model: "mock", Messages: messages, Tools: tools})
		Expect(third.Choices[0].Message.ToolCalls).To(BeEmpty())
		Expect(third.Choices[0].Message.Content).NotTo(BeNil())
	})

	It("IT-MOCK-2442-014: Gemini refuses an undiscovered workflow", func() {
		testServer := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistry(), false, config.ModeFull))
		defer testServer.Close()

		contents := []response.GeminiContent{{Role: "user", Parts: []response.GeminiPart{{Text: "Signal Name: OOMKilled"}}}}
		tools := geminiDiscoveryTools()
		first := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		appendGeminiFunctionResult(&contents, first, openai.ToolListAvailableActions, map[string]interface{}{"actions": []interface{}{}})
		second := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		appendGeminiFunctionResult(&contents, second, openai.ToolListWorkflows, map[string]interface{}{"workflows": []interface{}{map[string]interface{}{"workflow_id": "other-workflow"}}})

		third := postGemini(testServer.URL, response.GeminiRequest{Contents: contents, Tools: tools})
		Expect(third.Candidates[0].Content.Parts[0].FunctionCall).To(BeNil())
		Expect(third.Candidates[0].Content.Parts[0].Text).NotTo(BeEmpty())
	})
})

func openAIDiscoveryTools() []openai.Tool {
	return []openai.Tool{
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListAvailableActions}},
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListWorkflows}},
		{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolGetWorkflow}},
	}
}

func geminiDiscoveryTools() []response.GeminiToolDecl {
	return []response.GeminiToolDecl{{FunctionDeclarations: []response.GeminiFunctionDecl{
		{Name: openai.ToolListAvailableActions},
		{Name: openai.ToolListWorkflows},
		{Name: openai.ToolGetWorkflow},
	}}}
}

func postOpenAI(serverURL string, request openai.ChatCompletionRequest) openai.ChatCompletionResponse {
	body, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	responseBody, err := http.Post(serverURL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	defer responseBody.Body.Close()
	Expect(responseBody.StatusCode).To(Equal(http.StatusOK))

	var result openai.ChatCompletionResponse
	Expect(json.NewDecoder(responseBody.Body).Decode(&result)).To(Succeed())
	return result
}

func postGemini(serverURL string, request response.GeminiRequest) response.GeminiResponse {
	body, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	responseBody, err := http.Post(serverURL+"/v1beta/models/gemini-mock:generateContent", "application/json", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	defer responseBody.Body.Close()
	Expect(responseBody.StatusCode).To(Equal(http.StatusOK))

	var result response.GeminiResponse
	Expect(json.NewDecoder(responseBody.Body).Decode(&result)).To(Succeed())
	return result
}

func appendOpenAIToolResult(messages *[]openai.Message, result openai.ChatCompletionResponse, payload string) {
	assistant := result.Choices[0].Message
	*messages = append(*messages, assistant, openai.Message{
		Role:       "tool",
		ToolCallID: assistant.ToolCalls[0].ID,
		Content:    stringPtr(payload),
	})
}

func appendGeminiFunctionResult(contents *[]response.GeminiContent, result response.GeminiResponse, name string, payload map[string]interface{}) {
	*contents = append(*contents,
		result.Candidates[0].Content,
		response.GeminiContent{Role: "user", Parts: []response.GeminiPart{{FunctionResponse: &response.GeminiFunctionResp{
			Name: name, Response: payload,
		}}}},
	)
}
