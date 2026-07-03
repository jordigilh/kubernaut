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

package investigator_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// This file characterizes retryWorkflowSubmit / retryRCASubmit branches left
// uncovered by the existing suite (verified via `go tool cover -func`): the
// ctx-cancelled-mid-retry early return, the tool-call success classification
// branches (submit_result_no_workflow / submit_result_with_workflow /
// submit_result), and the parse-failure-continue branch on both functions.
// Wave 5 (GO-ANTIPATTERN-AUDIT 2026-07-01) Phase 0c — these tests must pass
// unchanged after Phase 2 extracts the shared emitRetryAudit + tool-call
// classifier helpers. Reuses scriptedMockClient/loopCharTestInvestigator
// (investigator_loop_characterization_test.go) and cancelAwareMockClient/
// cancelTestInvestigator (cancel_test.go), already defined in this package.

var _ = Describe("Kubernaut Agent Investigator — retryWorkflowSubmit / retryRCASubmit characterization (Wave 5 RED)", func() {

	Describe("UT-KA-WAVE5-R01: retryWorkflowSubmit — submit_result_no_workflow succeeds on first attempt", func() {
		It("classifies as no_matching_workflows immediately when the retry calls submit_result_no_workflow", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}}},
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "I cannot decide on a workflow"}}},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_retry", Name: "submit_result_no_workflow", Arguments: `{}`}},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
			Expect(result.Reason).To(ContainSubstring("submit_result_no_workflow after retry"))
			Expect(mockClient.callIdx).To(Equal(3))
		})
	})

	Describe("UT-KA-WAVE5-R02: retryWorkflowSubmit — submit_result_with_workflow parse-fail-continue then succeeds", func() {
		It("appends the failed tool response and retries, succeeding once valid JSON is submitted (maxParseRetries=2)", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}}},
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "I cannot decide on a workflow"}}},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_retry1", Name: "submit_result_with_workflow", Arguments: `not-valid-json`}},
					}},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_retry2", Name: "submit_result_with_workflow", Arguments: `{"workflow_id":"wf-1","confidence":0.9}`}},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("wf-1"))
			Expect(mockClient.callIdx).To(Equal(4), "both parse-level retry attempts must have fired")
		})
	})

	Describe("UT-KA-WAVE5-R03: retryWorkflowSubmit — context cancelled before the first retry attempt", func() {
		It("returns nil immediately without an LLM call, surfacing as a cancelled workflow-discovery result", func() {
			ctx, cancel := context.WithCancel(context.Background())
			mockClient := &cancelAwareMockClient{
				cancelAfter: 2,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{Message: llm.Message{Role: "assistant", Content: "I cannot decide on a workflow"}},
				},
			}
			inv := cancelTestInvestigator(mockClient)

			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue())
			Expect(result.CancelledPhase).To(Equal(string(katypes.PhaseWorkflowDiscovery)))
			Expect(mockClient.calls).To(HaveLen(2), "retryWorkflowSubmit must return before making its own LLM call once ctx is cancelled")
		})
	})

	Describe("UT-KA-WAVE5-R07: retryWorkflowSubmit — the retry's own LLM call errors on attempt 1, then succeeds on attempt 2", func() {
		It("logs the error, continues to the next attempt, and succeeds once a valid submit_result_with_workflow arrives", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}}},
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "I cannot decide on a workflow"}}},
					{err: errors.New("wf retry upstream 500")},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_retry2", Name: "submit_result_with_workflow", Arguments: `{"workflow_id":"wf-2","confidence":0.9}`}},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("wf-2"))
			Expect(mockClient.callIdx).To(Equal(4), "the errored attempt must not stop retryWorkflowSubmit from trying again")
		})
	})

	Describe("UT-KA-WAVE5-R04: retryRCASubmit — submit_result tool-call succeeds on the single retry attempt", func() {
		It("parses the retry's tool-call arguments and returns the corrected result (maxRCAParseRetries=1)", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "not valid json at all"}}},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca_retry", Name: "submit_result", Arguments: `{"rca_summary":"OOMKilled (corrected)","confidence":0.9}`}},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			sig := testSignal
			sig.Interactive = true // stay isolated to the RCA phase

			result, err := inv.Investigate(context.Background(), sig)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("OOMKilled (corrected)"))
			Expect(mockClient.callIdx).To(Equal(2))
		})
	})

	Describe("UT-KA-WAVE5-R05: retryRCASubmit — submit_result tool-call still fails to parse, single retry exhausts", func() {
		It("appends the failed retry response and falls back to treating the original content as the summary", func() {
			const originalContent = "not valid json at all"
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: originalContent}}},
					{resp: llm.ChatResponse{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_rca_retry", Name: "submit_result", Arguments: `still-not-valid-json`}},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			sig := testSignal
			sig.Interactive = true

			result, err := inv.Investigate(context.Background(), sig)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal(originalContent))
			Expect(result.HumanReviewNeeded).To(BeFalse())
			Expect(mockClient.callIdx).To(Equal(2), "the single RCA parse-retry attempt (maxRCAParseRetries=1) must have fired")
		})
	})

	Describe("UT-KA-WAVE5-R06: retryRCASubmit — the retry's own LLM call errors", func() {
		It("logs the error, exhausts the single retry attempt, and falls back to treating the original content as the summary", func() {
			const originalContent = "not valid json at all"
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: originalContent}}},
					{err: errors.New("retry upstream 503")},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			sig := testSignal
			sig.Interactive = true

			result, err := inv.Investigate(context.Background(), sig)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal(originalContent))
			Expect(mockClient.callIdx).To(Equal(2))
		})
	})
})
