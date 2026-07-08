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

// Issue #1614: a response truncated (FinishReasonLength) on BOTH the
// initial RCA attempt and the escalated retry was silently returned as a
// normal (still-truncated) result instead of flagging it for human review.
// These IT tests drive the real Investigate() production entry point end to
// end — through runLoopTurn -> classifyRCALoopResult -> checkRCAEarlyReturn
// -> finalizeAndEmitRCAOnlyResult -> audit persistence — proving both the
// business outcome (BR-HAPI-197) and the FedRAMP/SOC2 control objectives
// (AC-6, AU-2, AU-3, AU-12, SOC2 CC7.2/CC8.1) that depend on this wiring,
// mirroring the paired UT+IT pattern used for the analogous #1044
// apiVersion-gate fix (apiversion_gate_integration_test.go) and the
// AU-tagged assertions in investigator_1430_skip_discovery_test.go.
var _ = Describe("TP-1614: Double max_tokens truncation Integration — Full Investigate() Pipeline", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{}}
		enricher = enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		Name:          "api-server-pod",
		Namespace:     "production",
		ResourceKind:  "Pod",
		ResourceName:  "api-server-pod",
		Severity:      "critical",
		Environment:   "Production",
		Priority:      "P1",
		Message:       "OOMKilled",
		RemediationID: "rr-1614-double-truncation",
	}

	Describe("IT-KA-1614-001: Full pipeline — double truncation stops before workflow discovery (BR-HAPI-197, AC-6)", func() {
		It("should set HumanReviewNeeded=true and never invoke workflow discovery once the retry is also truncated", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:      llm.Message{Role: "assistant", Content: "partial RCA analysis, ran out of tokens..."},
						FinishReason: llm.FinishReasonLength,
						Usage:        llm.TokenUsage{CompletionTokens: 4096},
					},
					{
						Message:      llm.Message{Role: "assistant", Content: "still truncated even after escalated retry..."},
						FinishReason: llm.FinishReasonLength,
						Usage:        llm.TokenUsage{CompletionTokens: 8192},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"IT-KA-1614-001 / BR-HAPI-197: a response still truncated after the escalated retry is an unreliable AI result and must require human review")
			Expect(result.Reason).To(ContainSubstring("output truncated after retry"),
				"IT-KA-1614-001: reason must identify truncation, not a generic exhaustion")
			Expect(mockClient.calls).To(HaveLen(2),
				"IT-KA-1614-001 / AC-6: exactly 2 LLM calls (initial + escalated retry) — workflow discovery must never run on an unreliable RCA result")
		})
	})

	Describe("IT-KA-1614-002: Audit event for double-truncation exhaustion persisted (AU-2, AU-3, AU-12, SOC2 CC7.2/CC8.1)", func() {
		It("should persist a response_complete audit event with needs_human_review and the truncation reason", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:      llm.Message{Role: "assistant", Content: "partial RCA analysis, ran out of tokens..."},
						FinishReason: llm.FinishReasonLength,
						Usage:        llm.TokenUsage{CompletionTokens: 4096},
					},
					{
						Message:      llm.Message{Role: "assistant", Content: "still truncated even after escalated retry..."},
						FinishReason: llm.FinishReasonLength,
						Usage:        llm.TokenUsage{CompletionTokens: 8192},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())

			completeEvents := filterEvents(auditStore.events, audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1),
				"IT-KA-1614-002 / AU-12: exactly one response_complete audit event must be generated on this early-return path")
			Expect(completeEvents[0].CorrelationID).To(Equal(signal.RemediationID),
				"IT-KA-1614-002 / AU-3, SOC2 CC8.1: audit event must be reconstructable by correlation_id")

			responseDataStr, ok := completeEvents[0].Data["response_data"].(string)
			Expect(ok).To(BeTrue(), "response_data must be serialized as a JSON string")

			var responseData map[string]interface{}
			Expect(json.Unmarshal([]byte(responseDataStr), &responseData)).To(Succeed())
			Expect(responseData).To(HaveKeyWithValue("needs_human_review", true),
				"IT-KA-1614-002 / AU-3, SOC2 CC7.2: persisted decision must record needs_human_review=true")
			Expect(responseData["reason"]).To(ContainSubstring("output truncated after retry"),
				"IT-KA-1614-002 / AU-3: persisted reason must identify the truncation failure")
		})
	})
})
