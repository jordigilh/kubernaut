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
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Full Conversation Flows", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewRouter(registry, false)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-011: Legacy two-turn conversation", func() {
		It("should complete: tool_call -> final_analysis", func() {
			// Turn 1: Initial request → tool call
			turn1Body := marshalReq(map[string]interface{}{
				"model":    "mock-model",
				"messages": []map[string]string{{"role": "user", "content": "- Signal Name: OOMKilled\n- Namespace: default"}},
				"tools": []map[string]interface{}{
					{"type": "function", "function": map[string]interface{}{"name": "search_workflow_catalog", "parameters": map[string]interface{}{}}},
				},
			})
			resp1 := postJSON(server.URL+"/v1/chat/completions", turn1Body)
			var r1 openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp1.Body).Decode(&r1)).To(Succeed())
			resp1.Body.Close()
			Expect(r1.Choices[0].FinishReason).To(Equal("tool_calls"))

			// Turn 2: Include tool result → final analysis
			turn2Body := marshalReq(map[string]interface{}{
				"model": "mock-model",
				"messages": []interface{}{
					map[string]string{"role": "user", "content": "- Signal Name: OOMKilled\n- Namespace: default"},
					map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": r1.Choices[0].Message.ToolCalls},
					map[string]string{"role": "tool", "content": `{"workflows": [{"id": "test-wf"}]}`},
				},
				"tools": []map[string]interface{}{
					{"type": "function", "function": map[string]interface{}{"name": "search_workflow_catalog", "parameters": map[string]interface{}{}}},
				},
			})
			resp2 := postJSON(server.URL+"/v1/chat/completions", turn2Body)
			var r2 openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp2.Body).Decode(&r2)).To(Succeed())
			resp2.Body.Close()
			Expect(r2.Choices[0].FinishReason).To(Equal("stop"))
			Expect(r2.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*r2.Choices[0].Message.Content).To(ContainSubstring("Root Cause"))
		})
	})

	Describe("IT-MOCK-012: Three-step conversation flow", func() {
		It("should complete: list_available_actions -> list_workflows -> get_workflow -> final_analysis", func() {
			threeStepTools := []map[string]interface{}{
				{"type": "function", "function": map[string]interface{}{"name": "list_available_actions", "parameters": map[string]interface{}{}}},
				{"type": "function", "function": map[string]interface{}{"name": "list_workflows", "parameters": map[string]interface{}{}}},
				{"type": "function", "function": map[string]interface{}{"name": "get_workflow", "parameters": map[string]interface{}{}}},
			}

			messages := []interface{}{
				map[string]string{"role": "user", "content": "- Signal Name: CrashLoopBackOff\n- Namespace: staging"},
			}

			expectedTools := []string{"list_available_actions", "list_workflows", "get_workflow"}
			for i, expectedTool := range expectedTools {
				body := marshalReq(map[string]interface{}{
					"model": "mock-model", "messages": messages, "tools": threeStepTools,
				})
				resp := postJSON(server.URL+"/v1/chat/completions", body)
				var result openai.ChatCompletionResponse
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				resp.Body.Close()

				Expect(result.Choices[0].FinishReason).To(Equal("tool_calls"),
					"step %d should return tool_calls", i)
				Expect(result.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(expectedTool),
					"step %d should call %s", i, expectedTool)

				// Append assistant + tool result for next turn
				messages = append(messages,
					map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": result.Choices[0].Message.ToolCalls},
					map[string]string{"role": "tool", "content": `{"result": "ok"}`},
				)
			}

			// Final turn → analysis
			finalBody := marshalReq(map[string]interface{}{
				"model": "mock-model", "messages": messages, "tools": threeStepTools,
			})
			resp := postJSON(server.URL+"/v1/chat/completions", finalBody)
			var finalResult openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&finalResult)).To(Succeed())
			resp.Body.Close()
			Expect(finalResult.Choices[0].FinishReason).To(Equal("stop"))
			Expect(*finalResult.Choices[0].Message.Content).To(ContainSubstring("Root Cause"))
		})
	})

	Describe("IT-MOCK-013: Four-step flow with get_resource_context (ADR-056)", func() {
		It("should insert get_resource_context as first tool call", func() {
			fourStepTools := []map[string]interface{}{
				{"type": "function", "function": map[string]interface{}{"name": "get_resource_context", "parameters": map[string]interface{}{}}},
				{"type": "function", "function": map[string]interface{}{"name": "list_available_actions", "parameters": map[string]interface{}{}}},
				{"type": "function", "function": map[string]interface{}{"name": "list_workflows", "parameters": map[string]interface{}{}}},
				{"type": "function", "function": map[string]interface{}{"name": "get_workflow", "parameters": map[string]interface{}{}}},
			}

			body := marshalReq(map[string]interface{}{
				"model":    "mock-model",
				"messages": []map[string]string{{"role": "user", "content": "- Signal Name: OOMKilled"}},
				"tools":    fourStepTools,
			})
			resp := postJSON(server.URL+"/v1/chat/completions", body)
			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			resp.Body.Close()

			Expect(result.Choices[0].FinishReason).To(Equal("tool_calls"))
			Expect(result.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("get_resource_context"))
		})
	})
})

func marshalReq(v interface{}) *bytes.Buffer {
	data, _ := json.Marshal(v)
	return bytes.NewBuffer(data)
}

func postJSON(url string, body *bytes.Buffer) *http.Response {
	resp, err := http.Post(url, "application/json", body)
	Expect(err).NotTo(HaveOccurred())
	return resp
}
