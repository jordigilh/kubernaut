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
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// UT-KA-AUDIT-002: ResultToAuditJSON maps InvestigationResult.Reasoning into
// the response_data audit-log map, omitting the key entirely when nil
// (BR-AI-086 AC6).
var _ = Describe("UT-KA-AUDIT-002: ResultToAuditJSON reasoning mapping", func() {
	It("should omit the reasoning key when Reasoning is nil", func() {
		r := &katypes.InvestigationResult{RCASummary: "no reasoning"}

		m := investigator.ResultToAuditJSON(r)

		Expect(m).NotTo(HaveKey("reasoning"))
	})

	It("should include reasoning text and redacted=false when reasoning was captured", func() {
		r := &katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning: &katypes.ReasoningSummary{
				Text: "Considered a spike vs a leak; sustained climb over 6h points to a leak.",
			},
		}

		m := investigator.ResultToAuditJSON(r)

		Expect(m).To(HaveKey("reasoning"))
		reasoning, ok := m["reasoning"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(reasoning["text"]).To(Equal(r.Reasoning.Text))
		Expect(reasoning["redacted"]).To(Equal(false))
	})

	It("should include redacted=true with empty text when reasoning was redacted by the provider", func() {
		r := &katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning:  &katypes.ReasoningSummary{Redacted: true},
		}

		m := investigator.ResultToAuditJSON(r)

		Expect(m).To(HaveKey("reasoning"))
		reasoning, ok := m["reasoning"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(reasoning["text"]).To(BeEmpty())
		Expect(reasoning["redacted"]).To(Equal(true))
	})
})

// UT-KA-AUDIT-005: the RCA loop wiring — when the LLM's winning turn (the
// one whose tool call or text became the parsed InvestigationResult) carried
// a Reasoning block, InvestigationResult.Reasoning must be populated from
// it. Proves the LLM-client-layer Reasoning capture (BR-AI-086 AC1-3,
// milestones 1-4) actually reaches the RCA-completion result, not just the
// per-turn audit event (BR-AI-086 AC6).
var _ = Describe("UT-KA-AUDIT-005: RCA result carries the winning turn's reasoning", func() {
	var (
		capStore *capturingAuditStore
		builder  *prompt.Builder
		signal   katypes.SignalContext
	)

	BeforeEach(func() {
		capStore = &capturingAuditStore{}
		var err error
		builder, err = prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		signal = katypes.SignalContext{
			Name:          "OOMKilled",
			Namespace:     "production",
			Severity:      "critical",
			Message:       "Container payment-svc exceeded memory limit",
			RemediationID: "rem-audit-001-test",
			ResourceKind:  "Pod",
			ResourceName:  "payment-svc-abc123",
			IncidentID:    "inc-audit-001-test",
		}
	})

	It("should populate InvestigationResult.Reasoning from the submit_result turn's visible thinking text", func() {
		rcaJSON := `{"rca_summary":"OOM due to memory leak","severity":"critical"}`
		mockLLM := newScriptedLLM(llm.ChatResponse{
			Message: llm.Message{
				Role:    "assistant",
				Content: rcaJSON,
				Reasoning: &llm.ReasoningBlock{
					Text: "Sustained memory climb over 6h rules out a transient spike.",
				},
			},
			Usage: llm.TokenUsage{TotalTokens: 100},
		})

		inv := investigator.New(investigator.Config{
			Client:       mockLLM,
			Builder:      builder,
			ResultParser: parser.NewResultParser(),
			AuditStore:   capStore,
			Logger:       logr.Discard(),
			MaxTurns:     5,
			ModelName:    "test-model",
		})

		result, err := inv.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		Expect(result.Reasoning).NotTo(BeNil(), "InvestigationResult.Reasoning must be populated from the winning turn")
		Expect(result.Reasoning.Text).To(Equal("Sustained memory climb over 6h rules out a transient spike."))
		Expect(result.Reasoning.Redacted).To(BeFalse())

		completeEvents := capStore.eventsByType(audit.EventTypeRCAComplete)
		Expect(completeEvents).To(HaveLen(1), "should emit exactly one aiagent.rca.complete event")

		responseDataStr, ok := completeEvents[0].Data["response_data"].(string)
		Expect(ok).To(BeTrue())
		var responseData map[string]interface{}
		Expect(json.Unmarshal([]byte(responseDataStr), &responseData)).To(Succeed())
		Expect(responseData).To(HaveKey("reasoning"),
			"aiagent.rca.complete's response_data must carry the reasoning captured during RCA (BR-AI-086 AC6)")
	})

	It("should leave InvestigationResult.Reasoning nil when the LLM response carried no reasoning block", func() {
		rcaJSON := `{"rca_summary":"OOM due to memory leak","severity":"critical"}`
		mockLLM := newScriptedLLM(llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: rcaJSON},
			Usage:   llm.TokenUsage{TotalTokens: 100},
		})

		inv := investigator.New(investigator.Config{
			Client:       mockLLM,
			Builder:      builder,
			ResultParser: parser.NewResultParser(),
			AuditStore:   capStore,
			Logger:       logr.Discard(),
			MaxTurns:     5,
			ModelName:    "test-model",
		})

		result, err := inv.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Reasoning).To(BeNil(), "no reasoning was returned by the LLM, so InvestigationResult.Reasoning must stay nil")
	})
})

// UT-KA-AUDIT-006: ResultToAuditJSON caps oversized reasoning text at the
// audit size guard (BR-AI-086 AC6 REFACTOR: bounds worst-case audit storage
// for a runaway/misconfigured extended-thinking budget).
var _ = Describe("UT-KA-AUDIT-006: reasoning audit size guard", func() {
	It("should truncate reasoning text exceeding the audit cap and append a truncation marker", func() {
		oversized := strings.Repeat("a", 45000)
		r := &katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning:  &katypes.ReasoningSummary{Text: oversized},
		}

		m := investigator.ResultToAuditJSON(r)

		reasoning, ok := m["reasoning"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		text, ok := reasoning["text"].(string)
		Expect(ok).To(BeTrue())
		Expect(len(text)).To(BeNumerically("<", len(oversized)),
			"oversized reasoning text must be truncated for audit storage")
		Expect(text).To(ContainSubstring("truncated"))
	})

	It("should NOT truncate reasoning text within the audit cap", func() {
		withinCap := strings.Repeat("b", 1000)
		r := &katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning:  &katypes.ReasoningSummary{Text: withinCap},
		}

		m := investigator.ResultToAuditJSON(r)

		reasoning := m["reasoning"].(map[string]interface{})
		Expect(reasoning["text"]).To(Equal(withinCap))
	})
})

// UT-KA-AUDIT-007: emitLLMResponseAudit surfaces has_reasoning (mirrors the
// has_analysis pattern) so captured-vs-omitted reasoning is observable
// per-turn without special-casing key presence (BR-AI-086 AC6 REFACTOR).
var _ = Describe("UT-KA-AUDIT-007: has_reasoning observability field", func() {
	It("should set has_reasoning=true on the LLM response audit event when reasoning was captured", func() {
		capStore := &capturingAuditStore{}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		mockLLM := newScriptedLLM(llm.ChatResponse{
			Message: llm.Message{
				Role:      "assistant",
				Content:   `{"rca_summary":"OOM","severity":"critical"}`,
				Reasoning: &llm.ReasoningBlock{Text: "considered leak vs spike"},
			},
			Usage: llm.TokenUsage{TotalTokens: 50},
		})
		inv := investigator.New(investigator.Config{
			Client: mockLLM, Builder: builder, ResultParser: parser.NewResultParser(),
			AuditStore: capStore, Logger: logr.Discard(), MaxTurns: 5, ModelName: "test-model",
		})

		_, err = inv.Investigate(context.Background(), katypes.SignalContext{
			Name: "sig", Namespace: "default", RemediationID: "rem-audit-007", IncidentID: "inc-audit-007",
		})
		Expect(err).NotTo(HaveOccurred())

		respEvents := capStore.eventsByType(audit.EventTypeLLMResponse)
		Expect(respEvents).NotTo(BeEmpty())
		Expect(respEvents[0].Data["has_reasoning"]).To(Equal(true))
	})

	It("should set has_reasoning=false on the LLM response audit event when no reasoning was captured", func() {
		capStore := &capturingAuditStore{}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		mockLLM := newScriptedLLM(llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOM","severity":"critical"}`},
			Usage:   llm.TokenUsage{TotalTokens: 50},
		})
		inv := investigator.New(investigator.Config{
			Client: mockLLM, Builder: builder, ResultParser: parser.NewResultParser(),
			AuditStore: capStore, Logger: logr.Discard(), MaxTurns: 5, ModelName: "test-model",
		})

		_, err = inv.Investigate(context.Background(), katypes.SignalContext{
			Name: "sig", Namespace: "default", RemediationID: "rem-audit-007b", IncidentID: "inc-audit-007b",
		})
		Expect(err).NotTo(HaveOccurred())

		respEvents := capStore.eventsByType(audit.EventTypeLLMResponse)
		Expect(respEvents).NotTo(BeEmpty())
		Expect(respEvents[0].Data["has_reasoning"]).To(Equal(false))
	})
})

// IT-KA-1578-002: phase-handoff regression — Phase 1's RCA reasoning must
// survive into the final InvestigationResult after Phase 3 (workflow
// discovery/selection) builds its own result, WITHOUT leaking the raw
// reasoning text into the Phase 3 LLM prompt (phase-isolation invariant;
// mirrors the #847 CausalChain/DueDiligence propagation precedent in
// phase1_propagation_test.go, but merged directly in
// mergeAndFinalizeWorkflowResult rather than via Phase1Data/prompt
// templates, since RCA reasoning is Phase-1-specific forensic metadata, not
// Phase-3 prompt input).
var _ = Describe("IT-KA-1578-002: Phase 1 reasoning survives Phase 3 workflow-result merge", func() {
	It("should carry Phase 1's RCA reasoning into the Phase 3 workflow-selection result without adding it to the Phase 3 LLM prompt", func() {
		logger := logr.Discard()
		store := &gateRecordingAuditStore{}
		client := &gateMockLLMClient{
			responses: []llm.ChatResponse{
				gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9}`),
			},
		}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)

		inv := investigator.New(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher: enricher, AuditStore: store, Logger: logger,
			MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
		})

		reasoningText := "Sustained memory climb over 6h rules out a transient spike."
		rcaResult := &katypes.InvestigationResult{
			RCASummary: "OOM due to memory leak",
			Reasoning:  &katypes.ReasoningSummary{Text: reasoningText},
			RemediationTarget: katypes.RemediationTarget{
				Kind: "Pod", Name: "web-pod", Namespace: "default",
			},
		}
		signal := katypes.SignalContext{Name: "sig", Namespace: "default", ResourceKind: "Pod", ResourceName: "web-pod"}

		result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-1578-002")

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Reasoning).NotTo(BeNil(),
			"IT-KA-1578-002: Phase 1's RCA reasoning must survive into the Phase 3 result")
		Expect(result.Reasoning.Text).To(Equal(reasoningText))

		Expect(client.calls).To(HaveLen(1))
		for _, msg := range client.calls[0].Messages {
			Expect(msg.Content).NotTo(ContainSubstring(reasoningText),
				"IT-KA-1578-002 (phase-isolation invariant): Phase 1's raw reasoning text must not leak into the Phase 3 LLM prompt")
		}
	})
})

// UT-KA-AUDIT-004: messagesToAuditFormat (feeding InvestigationResult's
// AccumulatedMessages forensic JSON blob, BR-AUDIT-070) includes each
// assistant message's reasoning text/redacted flag, not just role/content/
// tool_calls. Exercised via the cancellation path since AccumulatedMessages
// is only populated on CancelledResult snapshots.
var _ = Describe("UT-KA-AUDIT-004: AccumulatedMessages carries per-message reasoning", func() {
	It("should include a reasoning key on cancelled-investigation messages that carried a ReasoningBlock", func() {
		ctx, cancel := context.WithCancel(context.Background())
		spy := &capturingAuditStore{}
		mockClient := &cancelAwareMockClient{
			cancelAfter: 1,
			cancelFn:    cancel,
			responses: []llm.ChatResponse{
				{
					Message: llm.Message{
						Role:    "assistant",
						Content: "investigating...",
						Reasoning: &llm.ReasoningBlock{
							Text: "Checking pod events before concluding.",
						},
					},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
					},
					Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
				},
			},
		}

		inv := cancelTestInvestigatorWithAudit(mockClient, spy)
		result, err := inv.Investigate(ctx, testSignal)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Cancelled).To(BeTrue())
		Expect(result.AccumulatedMessages).NotTo(BeEmpty())

		foundReasoning := false
		for _, msg := range result.AccumulatedMessages {
			if r, ok := msg["reasoning"]; ok {
				reasoning, ok := r.(map[string]interface{})
				Expect(ok).To(BeTrue(), "reasoning entry must be a map with text/redacted")
				Expect(reasoning["text"]).To(Equal("Checking pod events before concluding."))
				foundReasoning = true
			}
		}
		Expect(foundReasoning).To(BeTrue(), "at least one accumulated message must carry the reasoning block for forensic reconstruction (SOC2 CC8.1)")
	})
})
