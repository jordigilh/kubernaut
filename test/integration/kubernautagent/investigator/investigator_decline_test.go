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

var _ = Describe("Workflow Selection Decline Classification — #760", func() {

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
				{Kind: "Deployment", Name: "api-server", Namespace: "demo-quota"},
			},
		}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	signal := katypes.SignalContext{
		Name:          "api-server-quota-abc",
		Namespace:     "demo-quota",
		Severity:      "medium",
		Message:       "ResourceQuota exhausted",
		ResourceKind:  "Deployment",
		ResourceName:  "api-server",
		RemediationID: "rem-it-760-decline",
	}

	Describe("IT-KA-760-001: Free text decline -> no_matching_workflows", func() {
		It("should classify LLM free text during workflow selection as no_matching_workflows", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"ResourceQuota exhausted — namespace-quota memory limit (512Mi) fully consumed by 2 running pods","confidence":0.95}`}},
					{Message: llm.Message{Role: "assistant", Content: "After reviewing the 21 registered workflows, none of them can adjust namespace ResourceQuota limits. This requires manual intervention by a cluster administrator to increase the quota ceiling."}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"workflow decline must trigger human review")
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"#760: free text decline must be classified as no_matching_workflows, not llm_parsing_error")
			Expect(result.RCASummary).To(ContainSubstring("ResourceQuota"),
				"RCA summary from Phase 1 must be preserved")
		})
	})

	Describe("IT-KA-760-002: Garbage JSON -> generic parse error path", func() {
		It("should NOT classify garbage JSON as no_matching_workflows", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"foo":"bar","baz":42}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"garbage JSON must trigger human review")
			Expect(result.HumanReviewReason).NotTo(Equal("no_matching_workflows"),
				"#760: garbage JSON must NOT be classified as no_matching_workflows")
			Expect(result.Reason).To(ContainSubstring("parse"),
				"reason should indicate a parse failure")
		})
	})

	Describe("IT-KA-760-003: Catalog validation self-correction still works", func() {
		It("should self-correct invalid workflow and return valid result", func() {
			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"unknown-workflow","confidence":0.8}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: validator}},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart"),
				"self-correction must still produce valid workflow")
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"successful self-correction should not require human review")
		})
	})
})
