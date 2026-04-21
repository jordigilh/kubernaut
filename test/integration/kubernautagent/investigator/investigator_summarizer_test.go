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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

var _ = Describe("Kubernaut Agent Summarizer Wiring — TP-433-WIR Phase 6", func() {

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

	Describe("IT-KA-433W-017: executeTool with summarizer shortens long output", func() {
		It("should summarize tool output exceeding threshold", func() {
			longOutput := strings.Repeat("x", 200)
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: longOutput})

			summaryLLM := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: "summarized: pod is running"}},
				},
			}
			sum := summarizer.New(summaryLLM, 100)

			investigationLLM := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking"},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`}},
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: investigationLLM, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg, Pipeline: investigator.Pipeline{Summarizer: sum, AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil)}})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(investigationLLM.calls).To(HaveLen(3))
			secondCall := investigationLLM.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Role).To(Equal("tool"))
			Expect(toolMsg.Content).To(Equal("summarized: pod is running"),
				"long tool output should be summarized")
			Expect(toolMsg.Content).NotTo(ContainSubstring(strings.Repeat("x", 200)),
				"raw long output should not reach LLM")
		})
	})

	Describe("IT-KA-433W-018: executeTool with nil summarizer returns full output", func() {
		It("should return full tool output when summarizer is nil", func() {
			longOutput := strings.Repeat("y", 200)
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: longOutput})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking"},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`}},
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(3))
			secondCall := mockClient.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Role).To(Equal("tool"))
			Expect(toolMsg.Content).To(Equal(longOutput),
				"full output should pass through when summarizer is nil")
		})
	})
})
