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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("OpenAI + Ollama Endpoints", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewRouter(registry, false)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-001: OpenAI /v1/chat/completions", func() {
		It("IT-MOCK-001-001: should return tool call for first request with tools (legacy)", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", []string{"search_workflow_catalog"})
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices).To(HaveLen(1))
			Expect(result.Choices[0].FinishReason).To(Equal("tool_calls"))
			Expect(result.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(result.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("search_workflow_catalog"))
		})

		It("IT-MOCK-001-002: should return text response when no tools provided", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices[0].FinishReason).To(Equal("stop"))
			Expect(result.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*result.Choices[0].Message.Content).To(ContainSubstring("Root Cause"))
		})

		It("IT-MOCK-001-003: should return HTTP 500 for permanent error keyword", func() {
			body := chatRequest("analyze: mock_rca_permanent_error", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(500))
		})
	})

	Describe("IT-MOCK-002: Ollama /api/chat", func() {
		It("IT-MOCK-002-001: should return Ollama response with done=true", func() {
			body := ollamaChatRequest("- Signal Name: CrashLoopBackOff\n- Namespace: staging")
			resp, err := http.Post(server.URL+"/api/chat", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.OllamaChatResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Done).To(BeTrue())
			Expect(result.Response).To(ContainSubstring("Root Cause"))
		})

		It("IT-MOCK-002-002: should handle /api/generate endpoint", func() {
			body := ollamaGenerateRequest("- Signal Name: OOMKilled")
			resp, err := http.Post(server.URL+"/api/generate", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.OllamaChatResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Done).To(BeTrue())
		})

		It("IT-MOCK-002-003: should return 500 for Ollama permanent error", func() {
			body := ollamaChatRequest("mock_rca_permanent_error")
			resp, err := http.Post(server.URL+"/api/chat", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(500))
		})
	})

	Describe("IT-MOCK-004-001: Force-text mode over HTTP", func() {
		It("should return text even when tools are provided in force-text mode", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, true)
			ftServer := httptest.NewServer(router)
			defer ftServer.Close()

			body := chatRequest("- Signal Name: OOMKilled", []string{"search_workflow_catalog"})
			resp, err := http.Post(ftServer.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices[0].FinishReason).To(Equal("stop"))
			Expect(result.Choices[0].Message.ToolCalls).To(BeEmpty())
		})
	})
})

func chatRequest(content string, toolNames []string) *bytes.Buffer {
	req := map[string]interface{}{
		"model": "mock-model",
		"messages": []map[string]string{
			{"role": "user", "content": content},
		},
	}
	if len(toolNames) > 0 {
		tools := make([]map[string]interface{}, len(toolNames))
		for i, name := range toolNames {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        name,
					"description": "test",
					"parameters":  map[string]interface{}{},
				},
			}
		}
		req["tools"] = tools
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}

func ollamaChatRequest(content string) *bytes.Buffer {
	c := strings.ToLower(content)
	_ = c
	req := map[string]interface{}{
		"model": "mock-model",
		"messages": []map[string]string{
			{"role": "user", "content": content},
		},
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}

func ollamaGenerateRequest(prompt string) *bytes.Buffer {
	req := map[string]interface{}{
		"model":  "mock-model",
		"prompt": prompt,
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}
