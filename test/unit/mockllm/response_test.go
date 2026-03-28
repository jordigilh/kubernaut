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
	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Response Builders", func() {

	var cfg scenarios.MockScenarioConfig

	BeforeEach(func() {
		cfg = scenarios.MockScenarioConfig{
			ScenarioName:  "oomkilled",
			SignalName:    "OOMKilled",
			Severity:      "critical",
			WorkflowName:  "oomkill-increase-memory-v1",
			WorkflowID:    uuid.DeterministicUUID("oomkill-increase-memory-v1"),
			WorkflowTitle: "OOMKill Recovery - Increase Memory Limits",
			Confidence:    0.95,
			RootCause:     "Container exceeded memory limits due to traffic spike",
			ResourceKind:  "Deployment",
			ResourceNS:    "production",
			ResourceName:  "api-server",
			Parameters:    map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		}
	})

	Describe("UT-MOCK-001: OpenAI tool call response builder", func() {
		It("UT-MOCK-001-001: should produce valid tool call response with finish_reason=tool_calls", func() {
			resp := response.BuildToolCallResponse("mock-model", "search_workflow_catalog", cfg)
			Expect(resp.Object).To(Equal("chat.completion"))
			Expect(resp.Model).To(Equal("mock-model"))
			Expect(resp.Choices).To(HaveLen(1))
			Expect(resp.Choices[0].FinishReason).To(Equal("tool_calls"))
			Expect(resp.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(resp.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal("search_workflow_catalog"))
		})

		It("UT-MOCK-001-002: should include valid JSON in tool call arguments", func() {
			resp := response.BuildToolCallResponse("mock-model", "search_workflow_catalog", cfg)
			argsJSON := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
			var args map[string]interface{}
			Expect(json.Unmarshal([]byte(argsJSON), &args)).To(Succeed())
		})

		It("UT-MOCK-001-003: should produce chatcmpl- prefixed ID", func() {
			resp := response.BuildToolCallResponse("mock-model", "search_workflow_catalog", cfg)
			Expect(resp.ID).To(HavePrefix("chatcmpl-"))
		})

		It("UT-MOCK-001-004: should produce tool call with call_ prefixed ID", func() {
			resp := response.BuildToolCallResponse("mock-model", "search_workflow_catalog", cfg)
			Expect(resp.Choices[0].Message.ToolCalls[0].ID).To(HavePrefix("call_"))
		})
	})

	Describe("UT-MOCK-001: OpenAI text response builder", func() {
		It("UT-MOCK-001-005: should produce text response with finish_reason=stop", func() {
			resp := response.BuildTextResponse("mock-model", cfg)
			Expect(resp.Choices).To(HaveLen(1))
			Expect(resp.Choices[0].FinishReason).To(Equal("stop"))
			Expect(resp.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*resp.Choices[0].Message.Content).To(ContainSubstring("Root Cause Analysis"))
			Expect(resp.Choices[0].Message.ToolCalls).To(BeEmpty())
		})
	})

	Describe("UT-MOCK-002: Ollama response builder", func() {
		It("UT-MOCK-002-001: should produce Ollama response with done=true", func() {
			resp := response.BuildOllamaResponse("mock-model", cfg)
			Expect(resp.Done).To(BeTrue())
			Expect(resp.Model).To(Equal("mock-model"))
			Expect(resp.Response).To(ContainSubstring("Root Cause Analysis"))
			Expect(resp.TotalDuration).To(BeNumerically(">", 0))
		})

		It("UT-MOCK-002-002: should include created_at timestamp", func() {
			resp := response.BuildOllamaResponse("mock-model", cfg)
			Expect(resp.CreatedAt).NotTo(BeEmpty())
		})
	})

	Describe("UT-MOCK-001: Error response builder", func() {
		It("should produce error response matching Python format", func() {
			resp := response.BuildErrorResponse("Mock permanent LLM error for testing")
			data, err := json.Marshal(resp)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			errObj := raw["error"].(map[string]interface{})
			Expect(errObj["message"]).To(Equal("Mock permanent LLM error for testing"))
			Expect(errObj["type"]).To(Equal("server_error"))
			Expect(errObj["code"]).To(Equal("internal_error"))
		})
	})
})

// Helper to check that a response can round-trip through JSON matching Python shape
var _ openai.ChatCompletionResponse
