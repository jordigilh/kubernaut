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

var _ = Describe("Phase Separation: Investigator — #700", func() {

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

	Describe("UT-KA-700-007: RCA phase submit_result uses RCAResultSchema", func() {
		It("should send submit_result tool definition with RCA-only schema to LLM during RCA phase", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit exceeded","confidence":0.9}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			By("checking RCA phase submit_result schema excludes workflow fields")
			rcaToolDefs := mockClient.calls[0].Tools
			var submitResultDef *llm.ToolDefinition
			for i := range rcaToolDefs {
				if rcaToolDefs[i].Name == "submit_result" {
					submitResultDef = &rcaToolDefs[i]
					break
				}
			}
			Expect(submitResultDef).NotTo(BeNil(), "RCA phase must include submit_result tool")

			var schema map[string]interface{}
			err = json.Unmarshal(submitResultDef.Parameters, &schema)
			Expect(err).NotTo(HaveOccurred())
			props, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			Expect(props).NotTo(HaveKey("selected_workflow"),
				"RCA submit_result schema must NOT include selected_workflow")
			Expect(props).NotTo(HaveKey("alternative_workflows"),
				"RCA submit_result schema must NOT include alternative_workflows")
			Expect(props).NotTo(HaveKey("needs_human_review"),
				"RCA submit_result schema must NOT include needs_human_review")
			Expect(props).To(HaveKey("root_cause_analysis"),
				"RCA submit_result schema must include root_cause_analysis")
			Expect(props).To(HaveKey("confidence"),
				"RCA submit_result schema must include confidence")
		})
	})

	Describe("UT-KA-700-008: Workflow phase submit_result_with_workflow uses InvestigationResultSchema", func() {
		It("should send submit_result_with_workflow tool definition with full schema to LLM during workflow phase", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit exceeded","confidence":0.9}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			By("checking workflow phase submit_result_with_workflow schema includes workflow fields")
			wdToolDefs := mockClient.calls[1].Tools
			var submitResultDef *llm.ToolDefinition
			for i := range wdToolDefs {
				if wdToolDefs[i].Name == "submit_result_with_workflow" {
					submitResultDef = &wdToolDefs[i]
					break
				}
			}
			Expect(submitResultDef).NotTo(BeNil(), "workflow phase must include submit_result_with_workflow tool")

			var schema map[string]interface{}
			err = json.Unmarshal(submitResultDef.Parameters, &schema)
			Expect(err).NotTo(HaveOccurred())
			props, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			Expect(props).To(HaveKey("selected_workflow"),
				"workflow submit_result_with_workflow schema must include selected_workflow")
			Expect(props).To(HaveKey("alternative_workflows"),
				"workflow submit_result_with_workflow schema must include alternative_workflows")
			Expect(props).To(HaveKey("root_cause_analysis"),
				"workflow submit_result_with_workflow schema must include root_cause_analysis")
		})
	})

	Describe("UT-KA-700-009: runRCA clears HumanReviewNeeded after parsing", func() {
		It("should NOT honor needs_human_review from LLM during RCA — proceeds to workflow selection", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","needs_human_review":true,"human_review_reason":"complexity","confidence":0.8}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.85}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("verifying pipeline proceeded to workflow selection despite RCA setting needs_human_review")
			Expect(mockClient.calls).To(HaveLen(2),
				"investigation should make 2 LLM calls — RCA must NOT abort the pipeline via needs_human_review")
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"),
				"final result should have workflow from Phase 3")
		})
	})

	Describe("UT-KA-700-010: Max-turns exhaustion preserves HumanReviewNeeded", func() {
		It("should return HumanReviewNeeded when RCA max turns exhausted", func() {
			mockClient.responses = []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "I need more info"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api","namespace":"default"}`},
					},
				},
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 1, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"max-turns exhaustion is the only valid RCA abort — must preserve HumanReviewNeeded")
			Expect(result.Reason).To(ContainSubstring("max turns"),
				"reason should indicate max turns exhaustion")
		})
	})

	Describe("IT-KA-700-001: Two-session flow: RCA feeds workflow selection", func() {
		It("should produce result with both RCASummary and WorkflowID", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"root_cause_analysis":{"summary":"OOMKilled due to memory limit exceeded on api-server"},"confidence":0.92}`}},
				wfToolResp(`{"selected_workflow":{"workflow_id":"oom-increase-memory","confidence":0.95},"confidence":0.95}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"),
				"RCA summary should be populated from Phase 1")
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"),
				"workflow ID should be populated from Phase 3")
			Expect(result.Confidence).To(BeNumerically(">=", 0.9))
		})
	})

	Describe("IT-KA-700-002: RCA HR fields stripped from pipeline (BR-HAPI-200)", func() {
		It("should proceed to workflow selection — RCA submit_result HR fields are ignored by parser", func() {
			rcaSubmitArgs := `{"root_cause_analysis":{"summary":"Memory leak in api-server"},"confidence":0.7}`
			mockClient.responses = []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: "Found the issue"},
					ToolCalls: []llm.ToolCall{{ID: "tc_submit", Name: "submit_result", Arguments: rcaSubmitArgs}},
				},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.85}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("verifying pipeline did NOT abort — proceeded to workflow selection")
			Expect(mockClient.calls).To(HaveLen(2),
				"investigation should make 2 LLM calls — RCA needs_human_review must be cleared")

			By("verifying final result determined by workflow phase, not RCA")
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
		})
	})

	Describe("IT-KA-700-003: RCA investigation_outcome does not skip workflow selection", func() {
		It("should proceed to workflow selection even when RCA returns investigation_outcome:inconclusive", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Inconclusive — multiple potential causes","investigation_outcome":"inconclusive","confidence":0.4}`}},
				wfToolResp(`{"workflow_id":"generic-restart","confidence":0.6}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "warning", Message: "High memory usage",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("verifying pipeline reached workflow selection")
			Expect(mockClient.calls).To(HaveLen(2),
				"investigation should proceed to workflow selection despite inconclusive RCA")
			Expect(result.WorkflowID).To(Equal("generic-restart"),
				"workflow selection should have produced a result")
		})
	})

	Describe("UT-KA-700-CHK: RCA phase ChatOptions carry OutputSchema", func() {
		It("should propagate OutputSchema in ChatRequest.Options for RCA phase", func() {
			mockClient.responses = []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
				wfToolResp(`{"workflow_id":"oom-increase-memory","confidence":0.9}`),
			}
			inv := investigator.New(investigator.Config{Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher, AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools})
			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", Severity: "critical", Message: "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			By("RCA call should carry OutputSchema matching RCAResultSchema")
			rcaCall := mockClient.calls[0]
			Expect(rcaCall.Options.OutputSchema).NotTo(BeEmpty(),
				"RCA ChatRequest.Options.OutputSchema must be set")

			var rcaSchema map[string]interface{}
			err = json.Unmarshal(rcaCall.Options.OutputSchema, &rcaSchema)
			Expect(err).NotTo(HaveOccurred())
			props := rcaSchema["properties"].(map[string]interface{})
			Expect(props).NotTo(HaveKey("selected_workflow"),
				"RCA OutputSchema must NOT include workflow fields")

			By("Workflow call should carry OutputSchema matching InvestigationResultSchema")
			wdCall := mockClient.calls[1]
			Expect(wdCall.Options.OutputSchema).NotTo(BeEmpty(),
				"workflow ChatRequest.Options.OutputSchema must be set")

			var wdSchema map[string]interface{}
			err = json.Unmarshal(wdCall.Options.OutputSchema, &wdSchema)
			Expect(err).NotTo(HaveOccurred())
			wdProps := wdSchema["properties"].(map[string]interface{})
			Expect(wdProps).To(HaveKey("selected_workflow"),
				"workflow OutputSchema must include selected_workflow")
		})
	})
})
