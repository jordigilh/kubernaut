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

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

const (
	listActionsTool   = "list_available_actions"
	listWorkflowsTool = "list_workflows"
	getWorkflowTool   = "get_workflow"
)

var _ = Describe("Provider-neutral workflow discovery planner", func() {
	const workflowID = "workflow-target"

	Describe("UT-MOCK-2442-001: initial discovery", func() {
		It("calls list_available_actions first", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: conversation.DiscoveryTranscript{
					AdvertisedTools: []string{listActionsTool, listWorkflowsTool, getWorkflowTool},
				},
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal(listActionsTool))
		})

		It("calls get_resource_context first when that tool is enabled", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: conversation.DiscoveryTranscript{
					AdvertisedTools: []string{"get_resource_context", listActionsTool, listWorkflowsTool, getWorkflowTool},
				},
				ExpectedWorkflowID: workflowID,
				HasResourceContext: true,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal("get_resource_context"))
		})
	})

	Describe("UT-MOCK-2442-002: discovery membership", func() {
		It("calls get_workflow only after the target appears in list_workflows", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"workflow-target"}]}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal(getWorkflowTool))
			Expect(plan.Arguments).To(HaveKeyWithValue("workflow_id", workflowID))
		})

		It("does not grant membership from get_workflow alone", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(getWorkflowTool), result(getWorkflowTool, `{"workflowId":"workflow-target"}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryUnresolved))
			Expect(plan.ToolName).NotTo(Equal(getWorkflowTool))
		})
	})

	Describe("UT-MOCK-2442-027 [BR-MOCK-012, AC-4, AC-6, ASVS V4.1.3/V4.1.5]: advertised-tool enforcement", func() {
		It("fails closed instead of authorizing list_workflows when only list_available_actions is advertised", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: conversation.DiscoveryTranscript{
					AdvertisedTools: []string{listActionsTool},
					Events: []conversation.DiscoveryEvent{
						result(listActionsTool, `{}`),
					},
				},
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryUnresolved))
			Expect(plan.ToolName).NotTo(Equal(listWorkflowsTool))
			Expect(plan.Reason).To(ContainSubstring("not advertised"))
		})
	})

	Describe("UT-MOCK-2442-003: pagination", func() {
		It("requests the next page when the target is absent and a cursor exists", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[],"pagination":{"hasNext":true,"nextCursor":"cursor-1"}}`),
				),
				ExpectedWorkflowID: workflowID,
				ActionType:         "remediation",
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal(listWorkflowsTool))
			Expect(plan.Arguments).To(HaveKeyWithValue("page", "next"))
			Expect(plan.Arguments).To(HaveKeyWithValue("cursor", "cursor-1"))
		})

		It("accumulates membership across pages", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"other"}],"pagination":{"hasNext":true,"nextCursor":"cursor-1"}}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"workflow-target"}]}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal(getWorkflowTool))
		})
	})

	Describe("UT-MOCK-2442-004: exhausted discovery", func() {
		It("returns unresolved without get_workflow or workflow submission", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[],"pagination":{"hasNext":false}}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryUnresolved))
			Expect(plan.ToolName).NotTo(Equal(getWorkflowTool))
			Expect(plan.ToolName).NotTo(Equal("submit_result_with_workflow"))
			Expect(plan.Reason).To(ContainSubstring("not returned"))
		})

		It("fails closed on malformed list_workflows content", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `not-json`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryUnresolved))
			Expect(plan.Reason).To(ContainSubstring("invalid"))
		})
	})

	Describe("UT-MOCK-2442-005: completion and unrelated tools", func() {
		It("completes only after the discovered workflow result returns", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"workflow-target"}]}`),
					call(getWorkflowTool), result(getWorkflowTool, `{"workflowId":"workflow-target"}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryComplete))
		})

		It("ignores parallel non-discovery results when discovery is incomplete", func() {
			plan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"workflow-target"}]}`),
					call("kubectl_logs"), result("kubectl_logs", `{"logs":"ok"}`),
				),
				ExpectedWorkflowID: workflowID,
			})

			Expect(plan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(plan.ToolName).To(Equal(getWorkflowTool))
		})
	})

	Describe("UT-MOCK-2442-009: selection context isolation", func() {
		It("does not carry discovery membership into a fresh transcript", func() {
			firstPlan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript: transcript(
					call(listActionsTool), result(listActionsTool, `{}`),
					call(listWorkflowsTool), result(listWorkflowsTool, `{"workflows":[{"workflowId":"workflow-first"}]}`),
					call(getWorkflowTool), result(getWorkflowTool, `{"workflowId":"workflow-first"}`),
				),
				ExpectedWorkflowID: "workflow-first",
			})
			Expect(firstPlan.Kind).To(Equal(conversation.DiscoveryComplete))

			secondPlan := conversation.PlanDiscovery(conversation.DiscoveryPlannerInput{
				Transcript:         transcript(),
				ExpectedWorkflowID: "workflow-second",
			})
			Expect(secondPlan.Kind).To(Equal(conversation.DiscoveryCallTool))
			Expect(secondPlan.ToolName).To(Equal(listActionsTool))
		})
	})
})

func transcript(events ...conversation.DiscoveryEvent) conversation.DiscoveryTranscript {
	return conversation.DiscoveryTranscript{
		AdvertisedTools: []string{listActionsTool, listWorkflowsTool, getWorkflowTool},
		Events:          events,
	}
}

func call(name string) conversation.DiscoveryEvent {
	return conversation.DiscoveryEvent{Kind: conversation.DiscoveryToolCallEvent, ToolName: name}
}

func result(name, payload string) conversation.DiscoveryEvent {
	return conversation.DiscoveryEvent{Kind: conversation.DiscoveryToolResultEvent, ToolName: name, Payload: payload}
}
