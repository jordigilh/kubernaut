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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Gemini generateContent Endpoint (issue #1157)", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewRouter(registry, false, "")
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-GEMINI-001: POST /v1beta/models/{model}:generateContent", func() {
		It("IT-MOCK-GEMINI-001-001: should return text response for request without tools", func() {
			body := geminiRequest("- Signal Name: OOMKilled\n- Namespace: default", nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates).To(HaveLen(1))
			Expect(result.Candidates[0].Content.Role).To(Equal("model"))
			Expect(result.Candidates[0].FinishReason).To(Equal("STOP"))
			Expect(result.Candidates[0].Content.Parts).To(HaveLen(1))
			Expect(result.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("root_cause_analysis"))
			Expect(result.ModelVersion).To(Equal("mock-model"))
		})

		It("IT-MOCK-GEMINI-001-002: should return function call when tools are provided", func() {
			tools := []response.GeminiToolDecl{
				{FunctionDeclarations: []response.GeminiFunctionDecl{
					{Name: "search_workflow_catalog", Description: "Search workflows"},
				}},
			}
			body := geminiRequestWithTools("- Signal Name: OOMKilled\n- Namespace: default", tools)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates).To(HaveLen(1))
			parts := result.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			Expect(parts[0].FunctionCall.Name).To(Equal("search_workflow_catalog"))
		})

		It("IT-MOCK-GEMINI-001-003: should return text after function response is provided", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "- Signal Name: OOMKilled\n- Namespace: default"}}},
				{Role: "model", Parts: []response.GeminiPart{
					{FunctionCall: &response.GeminiFunctionCall{Name: "search_workflow_catalog", Args: map[string]interface{}{"query": "OOMKilled"}}},
				}},
				{Role: "user", Parts: []response.GeminiPart{
					{FunctionResponse: &response.GeminiFunctionResp{Name: "search_workflow_catalog", Response: map[string]string{"result": "found"}}},
				}},
			}
			tools := []response.GeminiToolDecl{
				{FunctionDeclarations: []response.GeminiFunctionDecl{
					{Name: "search_workflow_catalog", Description: "Search workflows"},
				}},
			}
			body := geminiRequestFull(contents, tools, nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates).To(HaveLen(1))
			parts := result.Candidates[0].Content.Parts
			Expect(parts).To(HaveLen(1))
			// After function results come back, should return text (or submit tool)
			hasText := parts[0].Text != ""
			hasFuncCall := parts[0].FunctionCall != nil
			Expect(hasText || hasFuncCall).To(BeTrue(),
				"expected either text or a follow-up function call after function response")
		})

		It("IT-MOCK-GEMINI-001-004: should return 500 for permanent error keyword", func() {
			body := geminiRequest("mock_rca_permanent_error", nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(500))
		})

		It("IT-MOCK-GEMINI-001-005: should return 405 for non-POST methods", func() {
			resp, err := http.Get(server.URL + "/v1beta/models/gemini-2.0-flash:generateContent")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(405))
		})

		It("IT-MOCK-GEMINI-001-006: should return 404 for paths without :generateContent suffix", func() {
			body := geminiRequest("hello", nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:streamGenerateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(404))
		})

		It("IT-MOCK-GEMINI-001-007: should return 400 for invalid JSON", func() {
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", bytes.NewBufferString("{invalid"))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(400))
		})
	})

	Describe("IT-MOCK-GEMINI-002: Force-text mode for Gemini", func() {
		It("IT-MOCK-GEMINI-002-001: should return text even when tools are provided in force-text mode", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, true, "")
			ftServer := httptest.NewServer(router)
			defer ftServer.Close()

			tools := []response.GeminiToolDecl{
				{FunctionDeclarations: []response.GeminiFunctionDecl{
					{Name: "search_workflow_catalog", Description: "Search workflows"},
				}},
			}
			body := geminiRequestWithTools("- Signal Name: OOMKilled", tools)
			resp, err := http.Post(ftServer.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("root_cause_analysis"))
			Expect(result.Candidates[0].Content.Parts[0].FunctionCall).To(BeNil())
		})
	})

	Describe("IT-MOCK-GEMINI-002b: Interactive mode for Gemini", func() {
		It("IT-MOCK-GEMINI-002b-001: should always return text in interactive mode even with tools", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, "interactive")
			iServer := httptest.NewServer(router)
			defer iServer.Close()

			tools := []response.GeminiToolDecl{
				{FunctionDeclarations: []response.GeminiFunctionDecl{
					{Name: "search_workflow_catalog", Description: "Search workflows"},
				}},
			}
			body := geminiRequestWithTools("- Signal Name: OOMKilled\n- Namespace: default", tools)
			resp, err := http.Post(iServer.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("root_cause_analysis"))
			Expect(result.Candidates[0].Content.Parts[0].FunctionCall).To(BeNil())
		})
	})

	Describe("IT-MOCK-GEMINI-003: Scenario detection via Gemini", func() {
		It("IT-MOCK-GEMINI-003-001: should detect OOMKilled scenario from Gemini content", func() {
			body := geminiRequest("- Signal Name: OOMKilled\n- Namespace: production", nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("memory"))
		})

		It("IT-MOCK-GEMINI-003-002: should detect CrashLoopBackOff scenario from Gemini content", func() {
			body := geminiRequest("- Signal Name: CrashLoopBackOff\n- Namespace: staging", nil)
			resp, err := http.Post(server.URL+"/v1beta/models/gemini-2.0-flash:generateContent", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result response.GeminiResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("root_cause_analysis"))
		})
	})
})

func geminiRequest(userText string, tools []response.GeminiToolDecl) *bytes.Buffer {
	req := response.GeminiRequest{
		Contents: []response.GeminiContent{
			{Role: "user", Parts: []response.GeminiPart{{Text: userText}}},
		},
		Tools: tools,
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}

func geminiRequestWithTools(userText string, tools []response.GeminiToolDecl) *bytes.Buffer {
	return geminiRequest(userText, tools)
}

func geminiRequestFull(contents []response.GeminiContent, tools []response.GeminiToolDecl, sysInst *response.GeminiContent) *bytes.Buffer {
	req := response.GeminiRequest{
		Contents:          contents,
		SystemInstruction: sysInst,
		Tools:             tools,
	}
	data, _ := json.Marshal(req)
	return bytes.NewBuffer(data)
}
