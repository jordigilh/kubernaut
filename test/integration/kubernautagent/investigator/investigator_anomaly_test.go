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
)

func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

var _ = Describe("Kubernaut Agent Anomaly Detector Wiring — TP-433-WIR Phase 4", func() {

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

	Describe("IT-KA-433W-012: executeTool rejects 6th call to same tool (per-tool limit)", func() {
		It("should return error JSON on 6th call when MaxToolCallsPerTool=5", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{"status":"ok"}`})

			detector := investigator.NewAnomalyDetector(
				investigator.AnomalyConfig{MaxToolCallsPerTool: 5, MaxTotalToolCalls: 100, MaxRepeatedFailures: 10},
				nil,
			)

			toolCalls := make([]llm.ToolCall, 6)
			for i := range toolCalls {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i+1), Name: "kubectl_describe", Arguments: `{}`}
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "checking"},
						ToolCalls: toolCalls,
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"done"}`}},
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg, Pipeline: investigator.Pipeline{AnomalyDetector: detector}})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(3))
			secondCall := mockClient.calls[1]
			var foundRejection bool
			for _, msg := range secondCall.Messages {
				if msg.Role == "tool" && containsSubstring(msg.Content, "per-tool call limit exceeded") {
					foundRejection = true
					break
				}
			}
			Expect(foundRejection).To(BeTrue(),
				"6th tool call should be rejected with per-tool limit error")
		})
	})

	Describe("IT-KA-433W-013: executeTool rejects 3rd repeated identical failure", func() {
		It("should return error JSON on 3rd failure with MaxRepeatedFailures=3", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", err: fmt.Errorf("connection refused")})

			detector := investigator.NewAnomalyDetector(
				investigator.AnomalyConfig{MaxToolCallsPerTool: 100, MaxTotalToolCalls: 100, MaxRepeatedFailures: 3},
				nil,
			)

			toolCalls := make([]llm.ToolCall, 3)
			for i := range toolCalls {
				toolCalls[i] = llm.ToolCall{ID: fmt.Sprintf("tc_%d", i+1), Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`}
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{
						Message:   llm.Message{Role: "assistant", Content: "trying"},
						ToolCalls: toolCalls,
					},
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"failed"}`}},
					wfToolResp(`{"workflow_id":"restart","confidence":0.5}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg, Pipeline: investigator.Pipeline{AnomalyDetector: detector}})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockClient.calls).To(HaveLen(3))
			secondCall := mockClient.calls[1]
			var foundRejection bool
			for _, msg := range secondCall.Messages {
				if msg.Role == "tool" && containsSubstring(msg.Content, "repeated identical failure") {
					foundRejection = true
					break
				}
			}
			Expect(foundRejection).To(BeTrue(),
				"3rd identical failure should trigger repeated-failure anomaly")
		})
	})

	Describe("IT-KA-433W-014: Investigation with >30 tool calls returns HumanReviewNeeded", func() {
		It("should return HumanReviewNeeded when total tool calls exceed MaxTotalToolCalls", func() {
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_describe", result: `{"status":"ok"}`})

			detector := investigator.NewAnomalyDetector(
				investigator.AnomalyConfig{MaxToolCallsPerTool: 100, MaxTotalToolCalls: 5, MaxRepeatedFailures: 100},
				nil,
			)

			mockClient := &mockLLMClient{}
			for i := 0; i < 10; i++ {
				mockClient.responses = append(mockClient.responses, llm.ChatResponse{
					Message:   llm.Message{Role: "assistant", Content: "checking more"},
					ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("tc_%d", i+1), Name: "kubectl_describe", Arguments: `{}`}},
				})
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Registry: reg, Pipeline: investigator.Pipeline{AnomalyDetector: detector}})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"investigation should require human review when total tool calls exceeded")
		})
	})
})
