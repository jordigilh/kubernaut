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
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Force-Text Mode", func() {

	Describe("UT-MOCK-004-001: Force-text response ignores tools and returns text", func() {
		It("should produce text-only response even when tools are provided", func() {
			cfg := scenarios.MockScenarioConfig{
				ScenarioName: "oomkilled",
				WorkflowID:   "test-wf-id",
				RootCause:    "test root cause",
				Confidence:   0.95,
				Severity:     "critical",
			}
			tools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "search_workflow_catalog"}},
			}

			resp := response.BuildForceTextResponse("mock-model", cfg, tools)
			Expect(resp.Choices).To(HaveLen(1))
			Expect(resp.Choices[0].FinishReason).To(Equal("stop"))
			Expect(resp.Choices[0].Message.Content).NotTo(BeNil())
			Expect(resp.Choices[0].Message.ToolCalls).To(BeEmpty())
		})
	})

	Describe("UT-MOCK-004-002: Force-text response includes same analysis content as normal text", func() {
		It("should contain RCA text", func() {
			cfg := scenarios.MockScenarioConfig{
				ScenarioName: "crashloop",
				WorkflowID:   "some-wf",
				RootCause:    "Container failing due to missing configuration",
				Confidence:   0.88,
				Severity:     "high",
			}
			resp := response.BuildForceTextResponse("mock-model", cfg, nil)
			Expect(*resp.Choices[0].Message.Content).To(ContainSubstring("Root Cause Analysis"))
			Expect(*resp.Choices[0].Message.Content).To(ContainSubstring("Container failing"))
		})
	})
})
