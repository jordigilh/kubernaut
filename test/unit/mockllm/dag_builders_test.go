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

var _ = Describe("DAG Builders", func() {

	Describe("UT-MOCK-560-001: LegacyDAG produces tool_call then text", func() {
		It("should return search_workflow_catalog on first call (0 tool results)", func() {
			dag := conversation.LegacyDAG()
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: strPtr("- Signal Name: OOMKilled")},
			})
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result).NotTo(BeNil())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepToolCall))
			Expect(result.Result.ToolName).To(Equal(openai.ToolSearchWorkflowCatalog))
		})

		It("should return final_analysis when tool results >= 1", func() {
			dag := conversation.LegacyDAG()
			ctx := conversation.NewContext([]openai.Message{
				{Role: "user", Content: strPtr("- Signal Name: OOMKilled")},
				{Role: "tool", Content: strPtr(`{"result":"ok"}`)},
			})
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result).NotTo(BeNil())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepFinalAnalysis))
			Expect(result.Result.ToolName).To(BeEmpty())
		})
	})

	Describe("UT-MOCK-560-002: ThreeStepDAG produces correct 4-step sequence", func() {
		It("should return list_available_actions on step 0", func() {
			dag := conversation.ThreeStepDAG(false)
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepToolCall))
			Expect(result.Result.ToolName).To(Equal(openai.ToolListAvailableActions))
		})

		It("should return list_workflows on step 1", func() {
			dag := conversation.ThreeStepDAG(false)
			ctx := ctxWithToolResults(1)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepToolCall))
			Expect(result.Result.ToolName).To(Equal(openai.ToolListWorkflows))
		})

		It("should return get_workflow on step 2", func() {
			dag := conversation.ThreeStepDAG(false)
			ctx := ctxWithToolResults(2)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepToolCall))
			Expect(result.Result.ToolName).To(Equal(openai.ToolGetWorkflow))
		})

		It("should return final_analysis on step 3+", func() {
			dag := conversation.ThreeStepDAG(false)
			ctx := ctxWithToolResults(3)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepFinalAnalysis))
		})
	})

	Describe("UT-MOCK-560-003: ThreeStepDAG with resource context adds get_resource_context", func() {
		It("should return get_resource_context on step 0 when hasResourceContext=true", func() {
			dag := conversation.ThreeStepDAG(true)
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepToolCall))
			Expect(result.Result.ToolName).To(Equal(openai.ToolGetResourceContext))
		})

		It("should return list_available_actions on step 1", func() {
			dag := conversation.ThreeStepDAG(true)
			ctx := ctxWithToolResults(1)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ToolName).To(Equal(openai.ToolListAvailableActions))
		})

		It("should return final_analysis on step 4+", func() {
			dag := conversation.ThreeStepDAG(true)
			ctx := ctxWithToolResults(4)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ResponseType).To(Equal(conversation.StepFinalAnalysis))
		})
	})

	Describe("UT-MOCK-560-004: SelectDAG returns correct DAG for tools", func() {
		It("should return legacy DAG when no three-step tools present", func() {
			dag := conversation.SelectDAG([]openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSearchWorkflowCatalog}},
			})
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ToolName).To(Equal(openai.ToolSearchWorkflowCatalog))
		})

		It("should return three-step DAG when list_available_actions present", func() {
			dag := conversation.SelectDAG([]openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListAvailableActions}},
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListWorkflows}},
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolGetWorkflow}},
			})
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ToolName).To(Equal(openai.ToolListAvailableActions))
		})

		It("should return four-step DAG when get_resource_context present", func() {
			dag := conversation.SelectDAG([]openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolListAvailableActions}},
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolGetResourceContext}},
			})
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Result.ToolName).To(Equal(openai.ToolGetResourceContext))
		})
	})

	Describe("UT-MOCK-560-005: DAG path records full traversal", func() {
		It("should include dispatch and terminal node in path for legacy DAG", func() {
			dag := conversation.LegacyDAG()
			ctx := ctxWithToolResults(0)
			result, err := dag.Execute(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Path).To(HaveLen(2))
			Expect(result.Path[0]).To(Equal("dispatch"))
			Expect(result.Path[1]).To(Equal("search_workflow_catalog"))
		})
	})
})

func strPtr(s string) *string { return &s }

func ctxWithToolResults(count int) *conversation.Context {
	msgs := []openai.Message{
		{Role: "user", Content: strPtr("- Signal Name: OOMKilled")},
	}
	for i := 0; i < count; i++ {
		msgs = append(msgs, openai.Message{
			Role:    "tool",
			Content: strPtr(`{"result":"ok"}`),
		})
	}
	return conversation.NewContext(msgs)
}
