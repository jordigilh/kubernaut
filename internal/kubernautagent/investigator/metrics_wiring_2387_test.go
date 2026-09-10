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

package investigator

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// White-box (package investigator, not investigator_test) because RED-phase
// assertions target the exact production dispatch path (Investigate ->
// runLLMLoop -> processToolCalls) with a scripted llm.Client, mirroring
// tool_call_timeout_1949_test.go. Business behavior under test:
// BR-KA-OBSERVABILITY-001 (call-level observability in the structured
// decision payload) and FedRAMP SI-10 (counts are server-computed, never
// LLM-supplied).

// m2387MockClient returns pre-scripted responses in call order, driving
// Investigate through RCA (1 real tool + submit) and workflow selection
// (explicit no-workflow decline, avoiding catalog validation).
type m2387MockClient struct {
	responses []llm.ChatResponse
	callIdx   int
}

func (m *m2387MockClient) Close() error { return nil }

func (m *m2387MockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	// Fail-safe: terminate any unexpected loop turn without catalog contact.
	return llm.ChatResponse{
		ToolCalls: []llm.ToolCall{{ID: "tc-fallback", Name: SubmitResultNoWorkflowToolName, Arguments: `{}`}},
	}, nil
}

func (m *m2387MockClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

func newM2387Investigator(client llm.Client) *Investigator {
	builder, _ := prompt.NewBuilder()
	return New(Config{
		Client:       client,
		Builder:      builder,
		ResultParser: parser.NewResultParser(),
		AuditStore:   audit.NopAuditStore{},
		Logger:       logr.Discard(),
		MaxTurns:     5,
		PhaseTools:   DefaultPhaseToolMap(),
	})
}

// m2387RCAJSON mirrors the live-cluster case in #2387: rich RCA content
// (causal narrative for Deployment/worker) that deliberately carries NO
// total_* fields — the LLM is never told to supply them (#2073/#2074).
const m2387RCAJSON = `{"root_cause_analysis":{"summary":"OOMKill caused by memory leak in worker","severity":"critical","signal_name":"CrashLoopBackOff","contributing_factors":["memory leak in data-processor"],"remediation_target":{"kind":"Deployment","name":"worker","namespace":"demo-checkout","api_version":"apps/v1"}},"confidence":0.98}`

func m2387Signal(rrID string) katypes.SignalContext {
	return katypes.SignalContext{
		Name:          "CrashLoopBackOff",
		Namespace:     "demo-checkout",
		Severity:      "critical",
		Message:       "Back-off restarting failed container",
		ResourceKind:  "Pod",
		ResourceName:  "worker",
		RemediationID: rrID,
	}
}

// m2387Script programs 3 LLM turns / 1 real tool call: RCA turn 1 dispatches
// kubectl_describe (cross-kind RCA: Pod signal -> Deployment target, so the
// same-kind gate stays out of the way), RCA turn 2 submits the RCA, and the
// workflow-selection turn explicitly declines via submit_result_no_workflow.
// Every turn carries token usage so token-reporting specs can assert
// cumulative TokenUsage (10/5/15 per turn).
func m2387Script() []llm.ChatResponse {
	usage := llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	return []llm.ChatResponse{
		{
			Message: llm.Message{Role: "assistant", Content: "Investigating..."},
			ToolCalls: []llm.ToolCall{
				{ID: "tc-1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"worker","namespace":"demo-checkout"}`},
			},
			Usage: usage,
		},
		{
			Message: llm.Message{Role: "assistant", Content: "RCA complete"},
			ToolCalls: []llm.ToolCall{
				{ID: "tc-2", Name: SubmitResultToolName, Arguments: m2387RCAJSON},
			},
			Usage: usage,
		},
		{
			Message: llm.Message{Role: "assistant", Content: "No matching workflow"},
			ToolCalls: []llm.ToolCall{
				{ID: "tc-3", Name: SubmitResultNoWorkflowToolName, Arguments: `{}`},
			},
			Usage: usage,
		},
	}
}

var _ = Describe("InvestigationMetrics wiring — #2387", func() {

	Describe("UT-KA-2387-001: Investigate populates totals from actual loop activity (BR-KA-OBSERVABILITY-001)", func() {
		It("sets TotalLLMTurns/TotalToolCalls from real turns and dispatches, excluding sentinel submits", func() {
			inv := newM2387Investigator(&m2387MockClient{responses: m2387Script()})
			result, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-001"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.TotalLLMTurns).To(Equal(3),
				"UT-KA-2387-001: 2 RCA turns + 1 workflow-selection turn must be counted (BR-KA-OBSERVABILITY-001)")
			Expect(result.TotalToolCalls).To(Equal(1),
				"UT-KA-2387-001: only the kubectl_describe dispatch counts — submit_result/submit_result_no_workflow sentinels are consumed, never executed")
		})
	})

	Describe("UT-KA-2387-002: counts are server-computed, never LLM-supplied (FedRAMP SI-10)", func() {
		It("reports real counts even though the submit payload carries no totals", func() {
			var submitted map[string]interface{}
			Expect(json.Unmarshal([]byte(m2387RCAJSON), &submitted)).To(Succeed())
			Expect(submitted).NotTo(HaveKey("total_llm_turns"),
				"contract: the LLM is never told to supply total_llm_turns (#2073/#2074)")
			Expect(submitted).NotTo(HaveKey("total_tool_calls"))

			inv := newM2387Investigator(&m2387MockClient{responses: m2387Script()})
			result, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-002"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.TotalLLMTurns).To(Equal(3),
				"UT-KA-2387-002: totals must be injected server-side before MarshalRCASubset, not parsed from LLM output")
			Expect(result.TotalToolCalls).To(Equal(1))
		})
	})

	Describe("UT-KA-2387-005: per-investigation isolation on the shared singleton", func() {
		It("does not leak counts across RemediationIDs", func() {
			doubled := append(m2387Script(), m2387Script()...)
			inv := newM2387Investigator(&m2387MockClient{responses: doubled})
			first, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-005a"))
			Expect(err).NotTo(HaveOccurred())
			second, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-005b"))
			Expect(err).NotTo(HaveOccurred())
			Expect(first.TotalLLMTurns).To(Equal(3))
			Expect(first.TotalToolCalls).To(Equal(1))
			Expect(second.TotalLLMTurns).To(Equal(3),
				"UT-KA-2387-005: the second investigation must start from zero, not accumulate the first (per-correlationID scope, cf. #1892)")
			Expect(second.TotalToolCalls).To(Equal(1))
		})
	})

	Describe("UT-KA-2387-006: cumulative per-RR totals across legs (message + extraction + discovery)", func() {
		It("accumulates every leg without reset or double-count", func() {
			// One mock, one Investigator, one rrID, three legs in order:
			// 2 interactive turns (1 real tool) + 1 extraction turn + 1
			// discovery turn = 4 turns / 1 tool call, all under rr-2387-006.
			script := []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "Investigating..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-i1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"worker","namespace":"demo-checkout"}`},
					},
				},
				{
					Message: llm.Message{Role: "assistant", Content: "still looking"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-i2", Name: SubmitResultToolName, Arguments: m2387RCAJSON},
					},
				},
				{
					Message: llm.Message{Role: "assistant", Content: "RCA extracted"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-e1", Name: SubmitResultToolName, Arguments: m2387RCAJSON},
					},
				},
				{
					Message: llm.Message{Role: "assistant", Content: "No matching workflow"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-d1", Name: SubmitResultNoWorkflowToolName, Arguments: `{}`},
					},
				},
			}
			inv := newM2387Investigator(&m2387MockClient{responses: script})

			_, err := inv.RunInteractiveTurn(context.Background(), []llm.Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "Investigate: critical CrashLoopBackOff in demo-checkout"},
			}, "rr-2387-006")
			Expect(err).NotTo(HaveOccurred())

			rcaResult, err := inv.RunRCAExtractionFromConversation(context.Background(), []llm.Message{
				{Role: "user", Content: "what did you find?"},
			}, "rr-2387-006")
			Expect(err).NotTo(HaveOccurred())
			Expect(rcaResult).NotTo(BeNil())

			result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), m2387Signal("rr-2387-006"), rcaResult, nil, "rr-2387-006")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.TotalLLMTurns).To(Equal(4),
				"UT-KA-2387-006: 2 interactive + 1 extraction + 1 discovery turns accumulate per-RR (no reset, no double-count)")
			Expect(result.TotalToolCalls).To(Equal(1))
		})
	})

	Describe("UT-KA-2387-007: extraction records its turn into the per-RR scope, not onto the result", func() {
		It("leaves result totals zero so later legs sum without double-count", func() {
			client := &m2387MockClient{responses: []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "RCA extracted"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-e1", Name: SubmitResultToolName, Arguments: m2387RCAJSON},
					},
				},
			}}
			inv := newM2387Investigator(client)
			rcaResult, err := inv.RunRCAExtractionFromConversation(context.Background(), []llm.Message{
				{Role: "user", Content: "what did you find?"},
			}, "rr-2387-007")
			Expect(err).NotTo(HaveOccurred())
			Expect(rcaResult.TotalLLMTurns).To(Equal(0),
				"UT-KA-2387-007: scope is the single source of truth — results never pre-carry counts")
			Expect(inv.metricsFor("rr-2387-007").LLMTurns()).To(Equal(1))
		})
	})

	Describe("UT-KA-2387-008: completed investigations report cumulative token usage (BR-KA-OBSERVABILITY-001)", func() {
		It("sets TokenUsage from all turns, not just audit", func() {
			inv := newM2387Investigator(&m2387MockClient{responses: m2387Script()})
			result, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-008"))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.TokenUsage).NotTo(BeNil(),
				"UT-KA-2387-008: success paths must report tokens like cancelled paths already do")
			Expect(result.TokenUsage.PromptTokens).To(Equal(30))
			Expect(result.TokenUsage.CompletionTokens).To(Equal(15))
			Expect(result.TokenUsage.TotalTokens).To(Equal(45))
		})
	})

	Describe("IT-KA-2387-010: complete-event payload carries real counts (KA→AF handoff, BR-AUDIT-005 AU-3)", func() {
		It("MarshalRCASubset of the Investigate result contains non-zero totals", func() {
			inv := newM2387Investigator(&m2387MockClient{responses: m2387Script()})
			result, err := inv.Investigate(context.Background(), m2387Signal("rr-2387-010"))
			Expect(err).NotTo(HaveOccurred())

			data := session.MarshalRCASubset(result)
			Expect(data).NotTo(BeNil())
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed).To(HaveKeyWithValue("total_llm_turns", BeNumerically("==", 3)),
				"IT-KA-2387-010: AF phase_guard canonicalGroundedRCA can only rename what KA actually emits")
			Expect(parsed).To(HaveKeyWithValue("total_tool_calls", BeNumerically("==", 1)))
		})
	})
})
