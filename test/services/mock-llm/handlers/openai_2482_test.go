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

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

func issue2482Request(toolMessage string) *http.Request {
	body := `{"model":"mock-model","messages":[{"role":"user","content":"MOCK_PARALLEL_TOOLS"},{"role":"assistant","tool_calls":[{"id":"call_empty","type":"function","function":{"name":"kubectl_logs","arguments":"{}"}}],"content":null},` + toolMessage + `],"tools":[{"type":"function","function":{"name":"kubectl_logs","parameters":{"type":"object"}}},{"type":"function","function":{"name":"submit_result","parameters":{"type":"object"}}}]}`
	return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
}

func issue2482Response(toolMessage string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handlers.NewRouter(scenarios.DefaultRegistry(), false, "full").ServeHTTP(response, issue2482Request(toolMessage))
	return response
}

var _ = Describe("OpenAI-compatible provider tool-result contract — #2482", func() {
	It("UT-MOCK-2482-001 [BR-AI-086, SI-10]: rejects an omitted tool content field", func() {
		response := issue2482Response(`{"role":"tool","tool_call_id":"call_empty"}`)

		Expect(response.Code).To(Equal(http.StatusBadRequest))
	})

	It("UT-MOCK-2482-002 [BR-AI-086, SI-10]: rejects a null tool content field", func() {
		response := issue2482Response(`{"role":"tool","tool_call_id":"call_empty","content":null}`)

		Expect(response.Code).To(Equal(http.StatusBadRequest))
	})

	It("UT-MOCK-2482-003 [BR-AI-086, SI-10]: accepts an empty tool content string", func() {
		response := issue2482Response(`{"role":"tool","tool_call_id":"call_empty","content":""}`)

		Expect(response.Code).NotTo(Equal(http.StatusBadRequest))
	})

	It("IT-MOCK-2482-004 [BR-KA-263, BR-AI-086, SI-10]: replays an empty Kubernetes tool result through the shared client and reaches the next tool call", func() {
		server := httptest.NewServer(handlers.NewRouter(scenarios.DefaultRegistry(), false, "full"))
		defer server.Close()

		client := openaicompat.New("mock-model", server.URL, "")
		toolDefinitions := []openaicompat.ToolDefinition{
			{Name: "kubectl_logs", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "submit_result", Parameters: json.RawMessage(`{"type":"object"}`)},
		}

		first, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "MOCK_EMPTY_TOOL_RESULT_REPLAY"}},
			Tools:    toolDefinitions,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Message.ToolCalls).To(HaveLen(6))
		Expect(first.Message.ToolCalls[4].Name).To(Equal("kubectl_logs_all_containers"))

		messages := []openaicompat.Message{
			{Role: "user", Content: "MOCK_EMPTY_TOOL_RESULT_REPLAY"},
			{Role: "assistant", ToolCalls: first.Message.ToolCalls},
		}
		for i, toolCall := range first.Message.ToolCalls {
			content := "result"
			if i == 4 {
				content = ""
			}
			messages = append(messages, openaicompat.Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    content,
			})
		}
		second, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: messages,
			Tools:    toolDefinitions,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Message.ToolCalls).To(HaveLen(1))
		Expect(second.Message.ToolCalls[0].Name).To(Equal("submit_result"))
	})
})
