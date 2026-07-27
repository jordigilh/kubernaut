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

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Kubernaut Agent Validator Wiring — TP-433-WIR Phase 5", func() {

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

	Describe("IT-KA-433W-015: Investigator with validator rejects unknown workflow → self-corrects", func() {
		It("should run self-correction loop and return corrected result", func() {
			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					wfToolResp(`{"workflow_id":"unknown-workflow","confidence":0.8}`),
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools, Pipeline: investigator.Pipeline{CatalogFetcher: &staticCatalogFetcher{validator: validator}, AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil)}})
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

	Describe("IT-KA-433W-016: Investigator with nil CatalogFetcher forces human review instead of passing through (#1677 hardening)", func() {
		It("should flag human review with catalog_unavailable instead of returning an unvalidated workflow_id", func() {
			// #1677 hardening (DD-WORKFLOW-019): a nil CatalogFetcher used to be
			// a silent "skip validation" dev-mode shortcut -- production always
			// wires a CatalogFetcher (a LazyCatalog-backed workflowCatalogFetcher),
			// so nil here is a wiring gap, not a supported degraded mode.
			// runWorkflowSelection now fails closed: an otherwise-selected
			// workflow forces HumanReviewNeeded=true rather than passing through
			// unvalidated, mirroring selfCorrectWorkflowSelection's own
			// fetchErr-handling contract.
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					wfToolResp(`{"workflow_id":"totally-unknown","confidence":0.9}`),
				},
			}

			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: invLogger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"nil CatalogFetcher is a wiring gap: must fail closed to human review, never pass through unvalidated")
			Expect(result.HumanReviewReason).To(Equal("catalog_unavailable"),
				"nil CatalogFetcher must classify as catalog_unavailable")
		})
	})
})
