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
	"fmt"
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

type alwaysFailLLM struct{}

func (f *alwaysFailLLM) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, errors.New("simulated context window overflow")
}

var _ = Describe("Kubernaut Agent Tool Output Truncation Pipeline — #752", func() {

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

	Describe("IT-KA-752-001: Pipeline truncates oversized tool output before LLM receives it", func() {
		It("should truncate tool output exceeding MaxToolOutputSize", func() {
			oversizedOutput := strings.Repeat("x", 200000)
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_get_by_kind_in_cluster", result: oversizedOutput})

			investigationLLM := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "investigating"},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_get_by_kind_in_cluster", Arguments: `{"kind":"Secret"}`}},
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
				},
			}

			maxOutput := 1000
			inv := investigator.New(investigator.Config{
				Client: investigationLLM, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{MaxToolOutputSize: maxOutput},
			})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(investigationLLM.calls).To(HaveLen(3))
			secondCall := investigationLLM.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Role).To(Equal("tool"))
			Expect(len(toolMsg.Content)).To(BeNumerically("<=", maxOutput+300),
				fmt.Sprintf("tool output should be truncated to ~%d chars, got %d", maxOutput, len(toolMsg.Content)))
			Expect(toolMsg.Content).To(ContainSubstring("[TRUNCATED]"),
				"truncated output should contain truncation marker")
		})
	})

	Describe("IT-KA-752-002: Pipeline passes normal-size tool output unchanged", func() {
		It("should not truncate output below MaxToolOutputSize", func() {
			normalOutput := strings.Repeat("y", 500)
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: normalOutput})

			investigationLLM := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking"},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`}},
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: investigationLLM, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{MaxToolOutputSize: 100000},
			})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(investigationLLM.calls).To(HaveLen(3))
			secondCall := investigationLLM.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(toolMsg.Content).To(Equal(normalOutput),
				"normal-size output should pass through unchanged")
		})
	})

	Describe("IT-KA-752-003: Summarizer failure + hard truncation fallback", func() {
		It("should truncate when summarizer fails and output exceeds MaxToolOutputSize", func() {
			oversizedOutput := strings.Repeat("z", 50000)
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_get_by_kind_in_cluster", result: oversizedOutput})

			sum := summarizer.New(&alwaysFailLLM{}, 100)

			investigationLLM := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "investigating"},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "kubectl_get_by_kind_in_cluster", Arguments: `{"kind":"Pod"}`}},
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
				},
			}

			maxOutput := 2000
			inv := investigator.New(investigator.Config{
				Client: investigationLLM, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
				Pipeline: investigator.Pipeline{
					Summarizer:        sum,
					MaxToolOutputSize: maxOutput,
				},
			})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(investigationLLM.calls).To(HaveLen(3))
			secondCall := investigationLLM.calls[1]
			toolMsg := secondCall.Messages[len(secondCall.Messages)-1]
			Expect(len(toolMsg.Content)).To(BeNumerically("<=", maxOutput+300),
				"tool output should be truncated by safety net after summarizer failure")
			Expect(toolMsg.Content).To(ContainSubstring("[TRUNCATED]"),
				"should contain truncation marker")
		})
	})
})
