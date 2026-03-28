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
)

var _ = Describe("Conversation Modes", func() {

	Describe("UT-MOCK-011: Legacy two-phase flow", func() {
		It("UT-MOCK-011-001: should return tool call when no tool results yet", func() {
			mode := conversation.LegacyMode()
			Expect(mode).NotTo(BeNil())
			Expect(mode.Name()).To(Equal("legacy"))
			Expect(mode.TotalSteps()).To(Equal(2))
		})

		It("UT-MOCK-011-002: should determine step 0 is tool_call, step 1 is final_analysis", func() {
			mode := conversation.LegacyMode()
			Expect(mode.StepType(0)).To(Equal(conversation.StepToolCall))
			Expect(mode.StepType(1)).To(Equal(conversation.StepFinalAnalysis))
		})

		It("UT-MOCK-011-003: should return tool name for legacy step 0", func() {
			mode := conversation.LegacyMode()
			Expect(mode.ToolNameForStep(0)).To(Equal(openai.ToolSearchWorkflowCatalog))
		})
	})

	Describe("UT-MOCK-012: Three-step discovery flow (without resource context)", func() {
		It("UT-MOCK-012-001: should have 3 steps plus final analysis", func() {
			mode := conversation.ThreeStepMode(false)
			Expect(mode.Name()).To(Equal("three_step"))
			Expect(mode.TotalSteps()).To(Equal(4))
		})

		It("UT-MOCK-012-002: should map steps to correct tool names", func() {
			mode := conversation.ThreeStepMode(false)
			Expect(mode.StepType(0)).To(Equal(conversation.StepToolCall))
			Expect(mode.ToolNameForStep(0)).To(Equal(openai.ToolListAvailableActions))
			Expect(mode.StepType(1)).To(Equal(conversation.StepToolCall))
			Expect(mode.ToolNameForStep(1)).To(Equal(openai.ToolListWorkflows))
			Expect(mode.StepType(2)).To(Equal(conversation.StepToolCall))
			Expect(mode.ToolNameForStep(2)).To(Equal(openai.ToolGetWorkflow))
			Expect(mode.StepType(3)).To(Equal(conversation.StepFinalAnalysis))
		})
	})

	Describe("UT-MOCK-013: Four-step flow with resource context (ADR-056)", func() {
		It("UT-MOCK-013-001: should have 4 tool steps plus final analysis", func() {
			mode := conversation.ThreeStepMode(true)
			Expect(mode.Name()).To(Equal("three_step_rc"))
			Expect(mode.TotalSteps()).To(Equal(5))
		})

		It("UT-MOCK-013-002: should insert get_resource_context as step 0", func() {
			mode := conversation.ThreeStepMode(true)
			Expect(mode.ToolNameForStep(0)).To(Equal(openai.ToolGetResourceContext))
			Expect(mode.ToolNameForStep(1)).To(Equal(openai.ToolListAvailableActions))
			Expect(mode.ToolNameForStep(2)).To(Equal(openai.ToolListWorkflows))
			Expect(mode.ToolNameForStep(3)).To(Equal(openai.ToolGetWorkflow))
			Expect(mode.StepType(4)).To(Equal(conversation.StepFinalAnalysis))
		})

		It("UT-MOCK-013-003: should select mode from tools list", func() {
			threeStepTools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "list_available_actions"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "list_workflows"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "get_workflow"}},
			}
			mode := conversation.SelectMode(threeStepTools)
			Expect(mode.Name()).To(Equal("three_step"))

			legacyTools := []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: "search_workflow_catalog"}},
			}
			mode = conversation.SelectMode(legacyTools)
			Expect(mode.Name()).To(Equal("legacy"))

			rcTools := append(threeStepTools, openai.Tool{
				Type: "function", Function: openai.ToolDefinition{Name: "get_resource_context"},
			})
			mode = conversation.SelectMode(rcTools)
			Expect(mode.Name()).To(Equal("three_step_rc"))
		})
	})
})
