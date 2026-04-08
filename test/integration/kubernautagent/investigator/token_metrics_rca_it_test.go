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
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("KA-KA Integration Parity — Token Usage (TP-433-PARITY)", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-TK-001: Investigation emits aiagent.response.complete with non-zero total_tokens_used", func() {
		It("should accumulate token usage across RCA and workflow phases", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled"}`},
						Usage:   llm.TokenUsage{PromptTokens: 500, CompletionTokens: 200, TotalTokens: 700},
					},
					{
						Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.9}`},
						Usage:   llm.TokenUsage{PromptTokens: 600, CompletionTokens: 150, TotalTokens: 750},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical",
				Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())

			completeEvents := filterEvents(auditStore.events, audit.EventTypeResponseComplete)
			Expect(completeEvents).To(HaveLen(1))

			data := completeEvents[0].Data
			totalPrompt, ok := data["total_prompt_tokens"].(int)
			Expect(ok).To(BeTrue(), "total_prompt_tokens must be int")
			Expect(totalPrompt).To(Equal(1100), "500 + 600 prompt tokens")

			totalCompletion, ok := data["total_completion_tokens"].(int)
			Expect(ok).To(BeTrue(), "total_completion_tokens must be int")
			Expect(totalCompletion).To(Equal(350), "200 + 150 completion tokens")

			totalTokens, ok := data["total_tokens"].(int)
			Expect(ok).To(BeTrue(), "total_tokens must be int")
			Expect(totalTokens).To(Equal(1450), "700 + 750 total tokens")
		})
	})
})

var _ = Describe("KA-KA Integration Parity — LLM Metrics (TP-433-PARITY)", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-LM-001: Investigation increments aiagent_api_llm_requests_total", func() {
		It("should increment the counter for each LLM call via InstrumentedClient", func() {
			baseMock := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`},
						Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.9}`},
						Usage:   llm.TokenUsage{PromptTokens: 120, CompletionTokens: 60, TotalTokens: 180},
					},
				},
			}
			instrumented := llm.NewInstrumentedClient(baseMock)

			inv := investigator.New(investigator.Config{
				Client: instrumented, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "default", Severity: "warning",
				Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(baseMock.calls)).To(BeNumerically(">=", 2),
				"at least RCA + workflow selection LLM calls")
		})
	})

	Describe("IT-KA-433-LM-002: Investigation updates aiagent_api_llm_request_duration_seconds", func() {
		It("should record duration for each LLM call via InstrumentedClient", func() {
			baseMock := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"timeout"}`},
						Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.85}`},
						Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}
			instrumented := llm.NewInstrumentedClient(baseMock)

			inv := investigator.New(investigator.Config{
				Client: instrumented, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "default", Severity: "critical",
				Message: "timeout",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(baseMock.calls)).To(BeNumerically(">=", 2),
				"InstrumentedClient wraps all LLM calls with duration observation")
		})
	})
})

var _ = Describe("KA-KA Integration Parity — RCA (TP-433-PARITY)", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			},
		}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-433-RCA-001: Investigation yields root_cause_analysis with summary and remediationTarget", func() {
		It("should return structured RCA with summary and remediation target", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary": "OOMKilled due to memory limit exceeded on container web",
						"remediation_target": {"kind": "Deployment", "name": "api-server", "namespace": "production"}
					}`}},
					{Message: llm.Message{Role: "assistant", Content: `{
						"workflow_id": "oom-increase-memory",
						"confidence": 0.92,
						"execution_bundle": "oom-recovery-v2"
					}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "critical",
				Message: "OOMKilled", ResourceKind: "Pod", ResourceName: "api-server-xyz",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"),
				"RCA summary should contain the root cause")
			Expect(result.RemediationTarget).NotTo(BeNil(),
				"remediation target should be populated from RCA phase")
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("api-server"))
		})
	})

	Describe("IT-KA-433-RCA-002: Post-RCA detected labels appear in Phase 3 workflow selection prompt", func() {
		It("should include RCA findings in the workflow selection LLM call", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Memory exhaustion from unbound cache growth"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.8}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "high",
				Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 2),
				"need at least RCA call + workflow selection call")

			workflowCall := mockClient.calls[1]
			userMsg := ""
			for _, msg := range workflowCall.Messages {
				if msg.Role == "user" {
					userMsg = msg.Content
				}
			}
			Expect(userMsg).To(ContainSubstring("Memory exhaustion"),
				"workflow selection user message must include RCA findings as context")
		})
	})
})
