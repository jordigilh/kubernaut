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

var _ = Describe("Kubernaut Agent Validator Wiring — TP-433-WIR Phase 5", func() {

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

	Describe("IT-KA-433W-015: Investigator with validator rejects unknown workflow → self-corrects", func() {
		It("should run self-correction loop and return corrected result", func() {
			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"unknown-workflow","confidence":0.8}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"restart","confidence":0.7}`}},
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools, Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: validator}}})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart"),
				"validator should trigger correction to a valid workflow_id")
			Expect(result.HumanReviewNeeded).To(BeFalse())
		})
	})

	Describe("IT-KA-433W-016: Investigator with nil validator passes through any workflow", func() {
		It("should return any workflow_id without validation when validator is nil", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"totally-unknown","confidence":0.9}`}},
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("totally-unknown"),
				"unknown workflow should pass through when validator is nil")
			Expect(result.HumanReviewNeeded).To(BeFalse())
		})
	})
})
