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

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Gemini Response Builders (issue #1157)", func() {

	var cfg scenarios.MockScenarioConfig

	BeforeEach(func() {
		cfg = scenarios.MockScenarioConfig{
			ScenarioName:         "oomkilled",
			SignalName:           "OOMKilled",
			Severity:             "critical",
			WorkflowName:         "oomkill-increase-memory-v1",
			WorkflowID:           uuid.DeterministicUUID("oomkill-increase-memory-v1"),
			WorkflowTitle:        "OOMKill Recovery - Increase Memory Limits",
			Confidence:           0.95,
			RootCause:            "Container exceeded memory limits due to traffic spike",
			ResourceKind:         "Deployment",
			ResourceNS:           "production",
			ResourceName:         "api-server",
			InvestigationOutcome: "actionable",
			IsActionable:         scenarios.BoolPtr(true),
			Parameters:           map[string]string{"MEMORY_LIMIT_NEW": "512Mi"},
		}
	})

	Describe("UT-MOCK-GEMINI-001: Gemini text response builder", func() {
		It("UT-MOCK-GEMINI-001-001: should produce valid response with role=model and finishReason=STOP", func() {
			resp := response.BuildGeminiTextResponse(cfg)
			Expect(resp.Candidates).To(HaveLen(1))
			Expect(resp.Candidates[0].Content.Role).To(Equal("model"))
			Expect(resp.Candidates[0].FinishReason).To(Equal("STOP"))
			Expect(resp.ModelVersion).To(Equal("mock-model"))
		})

		It("UT-MOCK-GEMINI-001-002: should include analysis text in parts with no function call", func() {
			resp := response.BuildGeminiTextResponse(cfg)
			Expect(resp.Candidates[0].Content.Parts).To(HaveLen(1))
			Expect(resp.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("root_cause_analysis"))
			Expect(resp.Candidates[0].Content.Parts[0].FunctionCall).To(BeNil())
		})

		It("UT-MOCK-GEMINI-001-003: should include root cause in response text", func() {
			resp := response.BuildGeminiTextResponse(cfg)
			Expect(resp.Candidates[0].Content.Parts[0].Text).To(ContainSubstring("Container exceeded memory limits"))
		})
	})

	Describe("UT-MOCK-GEMINI-002: Gemini tool call response builder", func() {
		It("UT-MOCK-GEMINI-002-001: should produce function call with correct tool name", func() {
			resp := response.BuildGeminiToolCallResponse("search_workflow_catalog", cfg)
			Expect(resp.Candidates).To(HaveLen(1))
			Expect(resp.Candidates[0].Content.Role).To(Equal("model"))
			Expect(resp.Candidates[0].Content.Parts).To(HaveLen(1))
			Expect(resp.Candidates[0].Content.Parts[0].FunctionCall.Name).To(Equal("search_workflow_catalog"))
		})

		It("UT-MOCK-GEMINI-002-002: should include query arg in function call arguments", func() {
			resp := response.BuildGeminiToolCallResponse("search_workflow_catalog", cfg)
			args := resp.Candidates[0].Content.Parts[0].FunctionCall.Args
			Expect(args).To(HaveKey("query"))
		})

		It("UT-MOCK-GEMINI-002-003: should have finishReason=STOP", func() {
			resp := response.BuildGeminiToolCallResponse("search_workflow_catalog", cfg)
			Expect(resp.Candidates[0].FinishReason).To(Equal("STOP"))
		})

		It("UT-MOCK-GEMINI-002-004: should have no text part when function call is present", func() {
			resp := response.BuildGeminiToolCallResponse("search_workflow_catalog", cfg)
			Expect(resp.Candidates[0].Content.Parts[0].Text).To(BeEmpty())
		})
	})

	Describe("UT-MOCK-GEMINI-003: Gemini multi-tool call response builder", func() {
		It("UT-MOCK-GEMINI-003-001: should produce multiple function calls in parts", func() {
			entries := []scenarios.MultiToolCallEntry{
				{Name: "kubectl_get_yaml", Arguments: map[string]string{"kind": "Pod", "namespace": "default"}},
				{Name: "get_resource_context", Arguments: map[string]string{"kind": "Deployment", "name": "api"}},
			}
			resp := response.BuildGeminiMultiToolCallResponse(entries)
			Expect(resp.Candidates).To(HaveLen(1))
			Expect(resp.Candidates[0].Content.Parts).To(HaveLen(2))
			Expect(resp.Candidates[0].Content.Parts[0].FunctionCall.Name).To(Equal("kubectl_get_yaml"))
			Expect(resp.Candidates[0].Content.Parts[1].FunctionCall.Name).To(Equal("get_resource_context"))
		})

		It("UT-MOCK-GEMINI-003-002: should pass arguments correctly for each tool call", func() {
			entries := []scenarios.MultiToolCallEntry{
				{Name: "kubectl_get_yaml", Arguments: map[string]string{"kind": "Pod"}},
			}
			resp := response.BuildGeminiMultiToolCallResponse(entries)
			args := resp.Candidates[0].Content.Parts[0].FunctionCall.Args
			Expect(args).To(HaveKeyWithValue("kind", "Pod"))
		})
	})

	Describe("UT-MOCK-GEMINI-004: Gemini content text extraction", func() {
		It("UT-MOCK-GEMINI-004-001: should extract last user text from contents", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "first question"}}},
				{Role: "model", Parts: []response.GeminiPart{{Text: "model reply"}}},
				{Role: "user", Parts: []response.GeminiPart{{Text: "second question about OOMKilled"}}},
			}
			lastUser, allText := response.ExtractTextFromContents(contents, nil)
			Expect(lastUser).To(Equal("second question about OOMKilled"))
			Expect(allText).To(ContainSubstring("first question"))
			Expect(allText).To(ContainSubstring("model reply"))
			Expect(allText).To(ContainSubstring("second question about OOMKilled"))
		})

		It("UT-MOCK-GEMINI-004-002: should include system instruction in allText", func() {
			sysInst := &response.GeminiContent{
				Role:  "system",
				Parts: []response.GeminiPart{{Text: "You are a remediation agent"}},
			}
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "help me"}}},
			}
			lastUser, allText := response.ExtractTextFromContents(contents, sysInst)
			Expect(lastUser).To(Equal("help me"))
			Expect(allText).To(ContainSubstring("You are a remediation agent"))
			Expect(allText).To(ContainSubstring("help me"))
		})

		It("UT-MOCK-GEMINI-004-003: should include function call names and args in allText", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "investigate pods"}}},
				{Role: "model", Parts: []response.GeminiPart{
					{FunctionCall: &response.GeminiFunctionCall{Name: "kubectl_get_yaml", Args: map[string]interface{}{"kind": "Pod"}}},
				}},
			}
			_, allText := response.ExtractTextFromContents(contents, nil)
			Expect(allText).To(ContainSubstring("kubectl_get_yaml"))
			Expect(allText).To(ContainSubstring("Pod"))
		})

		It("UT-MOCK-GEMINI-004-004: should include function response data in allText", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{
					{FunctionResponse: &response.GeminiFunctionResp{
						Name:     "kubectl_get_yaml",
						Response: map[string]string{"result": "pod-data"},
					}},
				}},
			}
			_, allText := response.ExtractTextFromContents(contents, nil)
			Expect(allText).To(ContainSubstring("kubectl_get_yaml"))
			Expect(allText).To(ContainSubstring("pod-data"))
		})
	})

	Describe("UT-MOCK-GEMINI-005: HasFunctionResponse detection", func() {
		It("UT-MOCK-GEMINI-005-001: should return false when no function responses present", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{{Text: "hello"}}},
				{Role: "model", Parts: []response.GeminiPart{{Text: "hi"}}},
			}
			Expect(response.HasFunctionResponse(contents)).To(BeFalse())
		})

		It("UT-MOCK-GEMINI-005-002: should return true when function response is present", func() {
			contents := []response.GeminiContent{
				{Role: "user", Parts: []response.GeminiPart{
					{FunctionResponse: &response.GeminiFunctionResp{Name: "tool1", Response: "ok"}},
				}},
			}
			Expect(response.HasFunctionResponse(contents)).To(BeTrue())
		})
	})

	Describe("UT-MOCK-GEMINI-006: Gemini error response builder", func() {
		It("UT-MOCK-GEMINI-006-001: should produce error with correct structure", func() {
			resp := response.BuildGeminiErrorResponse("something went wrong")
			errObj, ok := resp["error"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(errObj["message"]).To(Equal("something went wrong"))
			Expect(errObj["status"]).To(Equal("INTERNAL"))
			Expect(errObj["code"]).To(BeEquivalentTo(500))
		})
	})
})
