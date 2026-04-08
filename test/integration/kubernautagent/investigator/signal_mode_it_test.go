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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("KA-KA Integration Parity — Signal Mode (TP-433-PARITY)", func() {

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

	Describe("IT-KA-433-SM-001: Investigation with signal_mode=proactive produces proactive-style prompt", func() {
		It("should include proactive terminology in the system prompt", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"anticipated failure"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"scale-up","confidence":0.8}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server", Namespace: "production", Severity: "warning",
				Message: "High memory trend", SignalMode: "proactive",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 1))
			systemPrompt := extractSystemPrompt(mockClient.calls[0])
			Expect(systemPrompt).NotTo(BeEmpty())
			Expect(systemPrompt).To(SatisfyAny(
				ContainSubstring("proactive"),
				ContainSubstring("anticipated"),
				ContainSubstring("prevention"),
			), "proactive signal_mode should influence prompt wording")
		})
	})

	Describe("IT-KA-433-SM-002: Investigation with missing signal_mode defaults to reactive", func() {
		It("should not include proactive terminology when signal_mode is empty", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.9}`}},
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

			Expect(len(mockClient.calls)).To(BeNumerically(">=", 1))
			systemPrompt := extractSystemPrompt(mockClient.calls[0])
			Expect(systemPrompt).NotTo(BeEmpty())
			Expect(systemPrompt).NotTo(ContainSubstring("proactive"),
				"missing signal_mode should default to reactive, not proactive")
		})
	})
})

func extractSystemPrompt(req llm.ChatRequest) string {
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			return msg.Content
		}
	}
	return ""
}
