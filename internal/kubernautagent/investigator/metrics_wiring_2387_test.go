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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	toolregistry "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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

type m2387DiscoveryTool struct {
	name string
	err  error
}

func (m m2387DiscoveryTool) Name() string      { return m.name }
func (m2387DiscoveryTool) Description() string { return "target-cluster discovery fixture" }
func (m2387DiscoveryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (m m2387DiscoveryTool) Execute(context.Context, json.RawMessage) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.name != "api_resources_list" {
		return `{"status":"success"}`, nil
	}
	return `{"resources":[{"apiVersion":"apps/v1","kind":"Deployment"}]}`, nil
}

func newM2387Investigator(client llm.Client) *Investigator {
	builder, _ := prompt.NewBuilder()
	registry := toolregistry.New()
	registry.Register(m2387DiscoveryTool{name: "api_resources_list"})
	registry.Register(m2387DiscoveryTool{name: "kubectl_describe"})
	return newM2387InvestigatorWithRegistry(client, builder, registry)
}

func newM2387InvestigatorWithRegistry(client llm.Client, builder *prompt.Builder, registry *toolregistry.Registry) *Investigator {
	return New(Config{
		Client:       client,
		Builder:      builder,
		ResultParser: parser.NewResultParser(),
		AuditStore:   audit.NopAuditStore{},
		Logger:       logr.Discard(),
		MaxTurns:     5,
		PhaseTools:   DefaultPhaseToolMap(),
		Registry:     registry,
	})
}

// m2387RCAJSON mirrors the live-cluster case in #2387: rich RCA content
// (causal narrative for Deployment/worker) that deliberately carries NO
// total_* fields — the LLM is never told to supply them (#2073/#2074).
const m2387RCAJSON = `{"root_cause_analysis":{"summary":"OOMKill caused by memory leak in worker","severity":"critical","signal_name":"CrashLoopBackOff","contributing_factors":["memory leak in data-processor"],"remediation_target":{"kind":"Deployment","name":"worker","namespace":"demo-checkout","api_version":"apps/v1"}},"confidence":0.98,"actionable":true,"investigation_outcome":"actionable"}`

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

// m2387Script programs 3 LLM turns / 1 model-dispatched tool call: RCA turn 1
// dispatches kubectl_describe (cross-kind RCA: Pod signal -> Deployment target,
// so the same-kind gate stays out of the way), RCA turn 2 submits the RCA, and
// the workflow-selection turn explicitly declines via submit_result_no_workflow.
// Exact target-GVK verification adds one server-owned discovery call.
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
			Expect(result.TotalToolCalls).To(Equal(2),
				"UT-KA-2387-001: kubectl_describe and server-owned exact-GVK verification count; submit_result/submit_result_no_workflow sentinels are consumed, never executed")
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
			Expect(result.TotalToolCalls).To(Equal(2))
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
			Expect(first.TotalToolCalls).To(Equal(2))
			Expect(second.TotalLLMTurns).To(Equal(3),
				"UT-KA-2387-005: the second investigation must start from zero, not accumulate the first (per-correlationID scope, cf. #1892)")
			Expect(second.TotalToolCalls).To(Equal(2))
		})
	})

	Describe("UT-KA-2387-006: cumulative per-RR totals across legs (message + extraction + discovery)", func() {
		It("accumulates every leg without reset or double-count", func() {
			// One mock, one Investigator, one rrID, three legs in order:
			// 2 interactive turns (1 model-dispatched tool) + 1 extraction turn +
			// 1 discovery turn = 4 turns / 2 total tool calls (including the
			// server-owned exact-GVK verification), all under rr-2387-006.
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
			Expect(result.TotalToolCalls).To(Equal(2))
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

	Describe("UT-KA-2387-009: early discovery returns preserve cumulative totals (BR-KA-OBSERVABILITY-001)", func() {
		It("stamps scoped totals on pre-existing review and gate-failure returns", func() {
			seedTotals := func(inv *Investigator, correlationID string) {
				inv.metricsFor(correlationID).IncLLMTurns()
				inv.metricsFor(correlationID).AddToolCalls(2)
				inv.recordTokenUsage(correlationID, llm.TokenUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18})
			}

			preExistingReviewID := "rr-2387-009-review"
			preExistingInv := newM2387Investigator(&m2387MockClient{})
			seedTotals(preExistingInv, preExistingReviewID)
			preExisting, err := preExistingInv.RunWorkflowDiscoveryFromRCA(
				context.Background(), m2387Signal(preExistingReviewID),
				&katypes.InvestigationResult{
					HumanReviewNeeded: true,
					TotalLLMTurns:     99,
					TotalToolCalls:    99,
				}, nil, preExistingReviewID,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(preExisting.TotalLLMTurns).To(Equal(1))
			Expect(preExisting.TotalToolCalls).To(Equal(2))
			Expect(preExisting.TokenUsage).To(Equal(&katypes.TokenUsageSummary{
				PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18,
			}))

			gateFailureID := "rr-2387-009-gate"
			failedRegistry := toolregistry.New()
			failedRegistry.Register(m2387DiscoveryTool{
				name: "api_resources_list",
				err:  errors.New("target discovery unavailable"),
			})
			builder, _ := prompt.NewBuilder()
			gateFailureInv := newM2387InvestigatorWithRegistry(&m2387MockClient{}, builder, failedRegistry)
			seedTotals(gateFailureInv, gateFailureID)
			gateFailure, err := gateFailureInv.RunWorkflowDiscoveryFromRCA(
				context.Background(), m2387Signal(gateFailureID),
				&katypes.InvestigationResult{
					InvestigationOutcome:    katypes.InvestigationOutcomeProblemResolved,
					TerminalOutcome:         sharedtypes.TerminalOutcomeProblemResolved,
					WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch,
					RemediationTarget: katypes.RemediationTarget{
						Kind: "Deployment", Name: "worker", Namespace: "demo-checkout", APIVersion: "apps/v1",
					},
					TotalLLMTurns:  99,
					TotalToolCalls: 99,
				}, nil, gateFailureID,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(gateFailure.HumanReviewNeeded).To(BeTrue())
			Expect(gateFailure.TerminalOutcome).To(Equal(sharedtypes.TerminalOutcomeRCAIncomplete),
				"target verification failure must replace a stale problem_resolved outcome")
			Expect(gateFailure.WorkflowDiscoveryStatus).To(Equal(sharedtypes.WorkflowDiscoveryStatusNotStarted),
				"target verification failure must not claim workflow discovery completed")
			Expect(gateFailure.TotalLLMTurns).To(Equal(1))
			Expect(gateFailure.TotalToolCalls).To(Equal(3),
				"the failed exact-GVK discovery call must remain in cumulative totals")
			Expect(gateFailure.TokenUsage).To(Equal(&katypes.TokenUsageSummary{
				PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18,
			}))
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
			Expect(parsed).To(HaveKeyWithValue("total_tool_calls", BeNumerically("==", 2)))
		})
	})
})
