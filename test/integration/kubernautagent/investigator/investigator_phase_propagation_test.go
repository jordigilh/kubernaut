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

var _ = Describe("Phase 1-to-Phase 3 Context Propagation — #715", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		mockClient *mockLLMClient
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		mockClient = &mockLLMClient{}
		builder, _ = prompt.NewBuilder(prompt.WithStructuredOutput(true))
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
			},
		}
		dsClient := &fakeDataStorageClient{
			history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "oom-increase-memory", Outcome: "success"},
				},
			},
		}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-715-001: Phase 3 system prompt contains Phase 1 structured data", func() {
		It("should inject Phase 1 severity and contributing factors into workflow selection prompt", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"root_cause_analysis": {
						"summary": "OOMKilled due to memory limit exceeded on api-server container",
						"severity": "high",
						"contributing_factors": ["memory leak in api-server container", "no HPA configured"],
						"remediation_target": {"kind": "Deployment", "name": "api-server", "namespace": "production"}
					},
					"confidence": 0.85
				}`}},
				wfToolResp(`{
					"selected_workflow": {"workflow_id": "oom-increase-memory", "confidence": 0.9},
					"confidence": 0.9
				}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			By("checking Phase 3 system prompt contains Phase 1 data")
			wdSystemPrompt := mockClient.calls[1].Messages[0].Content
			Expect(wdSystemPrompt).To(ContainSubstring("Phase 1 Assessment"),
				"IT-KA-715-001: Phase 3 prompt must contain Phase 1 Assessment section")
			Expect(wdSystemPrompt).To(ContainSubstring("high"),
				"IT-KA-715-001: Phase 3 prompt must contain Phase 1 severity")
			Expect(wdSystemPrompt).To(ContainSubstring("memory leak"),
				"IT-KA-715-001: Phase 3 prompt must contain Phase 1 contributing factors")
		})
	})

	Describe("IT-KA-715-002: Phase 1 inconclusive fallback merge", func() {
		It("should propagate Phase 1 investigation_outcome=inconclusive when Phase 3 returns no outcome", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Inconclusive — resource has been remediated 3 times without improvement",
					"investigation_outcome": "inconclusive",
					"confidence": 0.4
				}`}},
				wfToolResp(`{
					"selected_workflow": {"workflow_id": "generic-restart", "confidence": 0.6},
					"confidence": 0.6
				}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "warning", Message: "High memory usage",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("verifying Phase 1 inconclusive fallback applied")
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"IT-KA-715-002: Phase 1 inconclusive must propagate as HumanReviewNeeded when Phase 3 has no outcome")
		})
	})

	Describe("IT-KA-715-003: Phase 3 explicit outcome takes precedence", func() {
		It("should use Phase 3 investigation_outcome=actionable over Phase 1 inconclusive", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{
					"rca_summary": "Inconclusive initial assessment",
					"investigation_outcome": "inconclusive",
					"confidence": 0.4
				}`}},
				wfToolResp(`{
					"selected_workflow": {"workflow_id": "oom-increase-memory", "confidence": 0.95},
					"investigation_outcome": "actionable",
					"actionable": true,
					"confidence": 0.95
				}`),
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("verifying Phase 3 explicit outcome wins")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"IT-KA-715-003: Phase 3 actionable must override Phase 1 inconclusive")
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"),
				"IT-KA-715-003: workflow should be selected despite Phase 1 inconclusive")
			isActionable := result.IsActionable
			Expect(isActionable).NotTo(BeNil())
			Expect(*isActionable).To(BeTrue(),
				"IT-KA-715-003: Phase 3 actionable=true must take precedence")
		})
	})
})
