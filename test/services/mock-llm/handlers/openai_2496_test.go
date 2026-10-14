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
	"encoding/json"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("History-aware external provider fixtures — BR-KA-016", func() {
	It("IT-MOCK-2498-001: skips optional RCA context, forces off-phase calls once, then resumes real catalog discovery [BR-KA-017]", func() {
		registry := scenarios.DefaultRegistry()
		scenario, found := registry.Get("phase_isolation_2498")
		Expect(found).To(BeTrue(), "the deployed phase-isolation provider fixture must be registered")
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		workflowID := configured.Config().WorkflowID
		server := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
		DeferCleanup(server.Close)
		messages := []openai.Message{{Role: "user", Content: issue2482StringPtr("Signal Name: phaseisolation2498")}}
		rca := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{
			Model: "mock-model", Messages: messages,
			Tools: []openai.Tool{
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSubmitResult}},
				{Type: "function", Function: openai.ToolDefinition{Name: "get_namespaced_resource_context"}},
				{Type: "function", Function: openai.ToolDefinition{Name: "get_cluster_resource_context"}},
				{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolKubectlGetByName}},
			},
		})
		Expect(rca.Choices[0].Message.ToolCalls).To(HaveLen(1))
		Expect(rca.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolSubmitResult), "RCA must not depend on calling optional context tools")
		tools := issue2482DiscoveryTools()
		forced := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
		Expect(forced.Choices[0].Message.ToolCalls).To(HaveLen(3))
		forcedNames := make([]string, 0, 3)
		messages = append(messages, forced.Choices[0].Message)
		for _, call := range forced.Choices[0].Message.ToolCalls {
			forcedNames = append(forcedNames, call.Function.Name)
			var arguments map[string]interface{}
			Expect(json.Unmarshal([]byte(call.Function.Arguments), &arguments)).To(Succeed())
			Expect(arguments).To(HaveKeyWithValue("phase", "rca"), "provider-authored phase must not authorize the real dispatcher")
			messages = append(messages, openai.Message{Role: "tool", ToolCallID: call.ID,
				Content: issue2482StringPtr(`{"status":"error","error":"tool not allowed in phase workflow_discovery"}`)})
		}
		Expect(forcedNames).To(ConsistOf("get_namespaced_resource_context", "get_cluster_resource_context", openai.ToolKubectlGetByName))
		for _, step := range []struct{ tool, result string }{
			{openai.ToolListAvailableActions, `{"actions":[]}`},
			{openai.ToolListWorkflows, `{"workflows":[{"workflow_id":"` + workflowID + `"}]}`},
			{openai.ToolGetWorkflow, `{"workflow_id":"` + workflowID + `"}`},
		} {
			response := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
			Expect(response.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(response.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(step.tool), "rejected off-phase calls must not satisfy catalog membership")
			appendIssue2482ToolResult(&messages, response, step.result)
		}
		final := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
		Expect(final.Choices[0].Message.ToolCalls).To(HaveLen(1))
		Expect(final.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(openai.ToolSubmitResultWithWorkflow))
	})

	DescribeTable("IT-MOCK-2496-002: real endpoint preserves independent RCA confidence and catalog calls before selection/decline",
		func(name, finalTool string, confidence float64, justified bool) {
			registry := scenarios.DefaultRegistry()
			scenario, found := registry.Get(name)
			Expect(found).To(BeTrue())
			configured, ok := scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			cfg := configured.Config()
			server := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
			DeferCleanup(server.Close)
			messages := []openai.Message{{Role: "user", Content: issue2482StringPtr("Signal Name: " + cfg.SignalName)}}
			rca := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{
				Model: "mock-model", Messages: messages,
				Tools: []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSubmitResult}}},
			})
			Expect(rca.Choices[0].Message.ToolCalls).To(HaveLen(1))
			var payload map[string]interface{}
			Expect(json.Unmarshal([]byte(rca.Choices[0].Message.ToolCalls[0].Function.Arguments), &payload)).To(Succeed())
			Expect(payload).To(HaveKeyWithValue("confidence", .99), "RCA confidence must not replace current workflow-selection confidence")

			tools := issue2482DiscoveryTools()
			for _, step := range []struct{ tool, result string }{
				{openai.ToolListAvailableActions, `{"actions":[]}`},
				{openai.ToolListWorkflows, `{"workflows":[{"workflow_id":"` + cfg.WorkflowID + `"}]}`},
				{openai.ToolGetWorkflow, `{"workflow_id":"` + cfg.WorkflowID + `"}`},
			} {
				response := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
				Expect(response.Choices[0].Message.ToolCalls).To(HaveLen(1))
				Expect(response.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(step.tool))
				appendIssue2482ToolResult(&messages, response, step.result)
			}
			final := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
			Expect(final.Choices[0].Message.ToolCalls).To(HaveLen(1))
			Expect(final.Choices[0].Message.ToolCalls[0].Function.Name).To(Equal(finalTool))
			payload = nil
			Expect(json.Unmarshal([]byte(final.Choices[0].Message.ToolCalls[0].Function.Arguments), &payload)).To(Succeed())
			if finalTool == openai.ToolSubmitResultNoWorkflow {
				Expect(payload).NotTo(HaveKey("selected_workflow"))
				return
			}
			selected, ok := payload["selected_workflow"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(selected).To(HaveKeyWithValue("confidence", confidence))
			if justified {
				Expect(selected).To(HaveKey("repeat_justification"))
			} else {
				Expect(selected).NotTo(HaveKey("repeat_justification"))
			}
		},
		Entry("accepted", "history_repeat_accepted_2496", openai.ToolSubmitResultWithWorkflow, .84, true),
		Entry("below floor", "history_repeat_low_2496", openai.ToolSubmitResultWithWorkflow, .42, true),
		Entry("approval band", "history_repeat_approval_2496", openai.ToolSubmitResultWithWorkflow, .75, true),
		Entry("different workflow, same family", "history_different_2496", openai.ToolSubmitResultWithWorkflow, .96, false),
		Entry("declines available catalog", "history_no_workflow_2496", openai.ToolSubmitResultNoWorkflow, 0., false),
	)
})
