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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #1936 (main port of #1935 root cause #2, workflow-discovery extension
// #1945; BR-AUDIT-005 v2.0, FedRAMP AU-3/CC7.2/CC8.1): runWorkflowSelection's
// local `messages` variable is never reassigned from runLLMLoop's result,
// so retryWorkflowSubmit and the self-correction closure both operate on a
// stale [system, user]-only history regardless of how many workflow-
// discovery tools (list_available_actions, list_workflows) the LLM actually
// called. These specs prove the wiring fix end to end through the real
// Investigate() production path, reusing the gateMockLLMClient/
// gateRecordingAuditStore/gateK8sClient/gateDSClient/stubCatalogFetcher
// helpers already established on main (apiversion_gate_test.go,
// wiring_test.go).
var _ = Describe("#1936: workflow-discovery retries must include real tool-call history", func() {

	var (
		logger     logr.Logger
		auditStore *gateRecordingAuditStore
		mockClient *gateMockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
		signal     katypes.SignalContext
	)

	BeforeEach(func() {
		logger = logr.Discard()
		auditStore = &gateRecordingAuditStore{}
		mockClient = &gateMockLLMClient{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		enricher = enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
		signal = katypes.SignalContext{
			ResourceKind: "Pod",
			ResourceName: "api-server-xyz",
			Name:         "api-server-xyz",
			Namespace:    "production",
			Severity:     "critical",
			Message:      "Pod OOMKilled repeatedly",
		}
	})

	// wfToolCallMessages inspects a captured llm.ChatRequest for tool-call/
	// tool-result turns matching the given tool name (mirrors
	// gate_history_propagation_1936_test.go's toolCallMessages helper).
	wfToolCallMessages := func(req llm.ChatRequest, toolName string) (sawCall, sawResult bool) {
		for _, msg := range req.Messages {
			for _, tc := range msg.ToolCalls {
				if tc.Name == toolName {
					sawCall = true
				}
			}
			if msg.Role == "tool" && msg.ToolName == toolName {
				sawResult = true
			}
		}
		return sawCall, sawResult
	}

	Describe("IT-KA-1936-007: retryWorkflowSubmit retry includes prior tool-call turns", func() {
		It("sends the earlier list_available_actions tool_use/tool_result pair as part of the retry request", func() {
			mockClient.responses = []llm.ChatResponse{
				// Turn 0 (RCA phase): parseable, no tool calls needed.
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Pod OOMKilled","confidence":0.85}`}},
				// Turn 1 (workflow-discovery, top-level loop turn 0): real tool call.
				{
					Message: llm.Message{Role: "assistant", Content: "Checking available actions..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`},
					},
				},
				// Turn 2 (workflow-discovery, top-level loop turn 1): unparseable
				// plain text final response — triggers retryWorkflowSubmit.
				{Message: llm.Message{Role: "assistant", Content: "I could not determine a matching workflow from the available actions."}},
				// Turn 3: the actual parse-retry LLM call under test.
				gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9}`),
			}

			// investigator.New (not newTestInvestigator): a nil
			// CatalogFetcher makes the final selection fail closed after
			// the retry under test has already been captured, isolating
			// this spec to the retryWorkflowSubmit history-propagation
			// behavior (matches #1945's precedent).
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 4),
				"IT-KA-1936-007: expected RCA + WD tool-call turn + WD final text turn + parse-retry LLM calls")
			retryReq := mockClient.calls[3]

			sawCall, sawResult := wfToolCallMessages(retryReq, "list_available_actions")
			Expect(sawCall).To(BeTrue(),
				"IT-KA-1936-007: workflow-discovery parse-retry must include the earlier list_available_actions tool_use turn (#1936)")
			Expect(sawResult).To(BeTrue(),
				"IT-KA-1936-007: workflow-discovery parse-retry must include the paired tool_result turn (#1936)")
		})
	})

	Describe("IT-KA-1936-008: self-correction second attempt includes first attempt's nested tool-call turn", func() {
		It("sends the first correction attempt's own list_workflows tool_use/tool_result pair as part of the second attempt's request", func() {
			mockClient.responses = []llm.ChatResponse{
				// Turn 0 (RCA phase): parseable, no tool calls needed.
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Pod OOMKilled","confidence":0.85}`}},
				// Turn 1 (workflow-discovery, top-level loop turn 0): LLM
				// immediately submits a hallucinated workflow_id — parses
				// fine but fails catalog validation, triggering self-correction
				// attempt 1.
				gateWfToolResp(`{"workflow_id":"invalid-wf","confidence":0.7}`),
				// Turn 2 (self-correction attempt 1, nested loop turn 0): real
				// tool call made DURING the correction attempt itself.
				{
					Message: llm.Message{Role: "assistant", Content: "Let me check the workflow catalog again..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_wf_list", Name: "list_workflows", Arguments: `{}`},
					},
				},
				// Turn 3 (self-correction attempt 1, nested loop turn 1): still
				// invalid — attempt 1 fails validation too, triggering attempt 2.
				gateWfToolResp(`{"workflow_id":"invalid-wf-2","confidence":0.7}`),
				// Turn 4: self-correction attempt 2's request — the one under
				// test. Resolves to a valid workflow so SelfCorrect terminates.
				gateWfToolResp(`{"workflow_id":"valid-wf","confidence":0.9}`),
			}

			validator := parser.NewValidator([]string{"valid-wf"})
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher: &stubCatalogFetcher{validator: validator},
				},
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("valid-wf"), "self-correction must converge on the valid workflow")

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 5),
				"IT-KA-1936-008: expected RCA + top-level submit + 2 self-correction attempts (1 nested tool-call turn + 3 submit turns)")
			secondAttemptReq := mockClient.calls[4]

			sawCall, sawResult := wfToolCallMessages(secondAttemptReq, "list_workflows")
			Expect(sawCall).To(BeTrue(),
				"IT-KA-1936-008: self-correction attempt 2's request must include attempt 1's own list_workflows tool_use turn (#1936), "+
					"not just the top-level [system, user] + correction messages")
			Expect(sawResult).To(BeTrue(),
				"IT-KA-1936-008: self-correction attempt 2's request must include the paired tool_result turn (#1936)")
		})
	})
})
