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

var _ = Describe("Workflow Selection Split Submit Tools — #760 v2", func() {

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
		RemediationID: "rem-it-760v2",
	}

	Describe("UT-KA-760-012: WorkflowDiscovery phase includes split submit tools", func() {
		It("should include submit_result_with_workflow and submit_result_no_workflow in workflow selection tools", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"quota exhausted","confidence":0.9}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"No workflow"},"reasoning":"none available"}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			wdToolNames := toolNamesFromCall(mockClient.calls[1])
			Expect(wdToolNames).To(ContainElement("submit_result_with_workflow"),
				"workflow discovery must include submit_result_with_workflow")
			Expect(wdToolNames).To(ContainElement("submit_result_no_workflow"),
				"workflow discovery must include submit_result_no_workflow")
		})
	})

	Describe("UT-KA-760-013: WorkflowDiscovery phase does NOT include bare submit_result", func() {
		It("should not include the generic submit_result in workflow selection tools", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"quota exhausted","confidence":0.9}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"No workflow"},"reasoning":"none"}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			wdToolNames := toolNamesFromCall(mockClient.calls[1])
			Expect(wdToolNames).NotTo(ContainElement("submit_result"),
				"workflow discovery must NOT include the bare submit_result tool")
		})
	})

	Describe("UT-KA-760-014: RCA phase still includes submit_result (unchanged)", func() {
		It("should include submit_result in RCA phase tools", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"quota exhausted","confidence":0.9}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_with_workflow", Arguments: `{"root_cause_analysis":{"summary":"quota"},"selected_workflow":{"workflow_id":"scale-up","confidence":0.9},"confidence":0.9}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			_, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).To(HaveLen(2))

			rcaToolNames := toolNamesFromCall(mockClient.calls[0])
			Expect(rcaToolNames).To(ContainElement("submit_result"),
				"RCA phase must still include the single submit_result tool")
		})
	})

	Describe("IT-KA-760-004: submit_result_no_workflow → no_matching_workflows", func() {
		It("should classify as no_matching_workflows when LLM calls submit_result_no_workflow", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"ResourceQuota exhausted","confidence":0.95}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"ResourceQuota exhausted"},"reasoning":"No workflow handles namespace quota adjustments"}`},
						},
					},
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
				"submit_result_no_workflow must trigger human review")
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"submit_result_no_workflow must classify as no_matching_workflows")
			Expect(result.RCASummary).To(ContainSubstring("ResourceQuota"),
				"RCA summary from Phase 1 must be preserved")
		})
	})

	Describe("IT-KA-760-005: submit_result_with_workflow → workflow parsed", func() {
		It("should parse workflow from submit_result_with_workflow tool call", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_with_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"selected_workflow":{"workflow_id":"oom-increase-memory","confidence":0.95},"confidence":0.95}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.WorkflowID).To(Equal("oom-increase-memory"),
				"submit_result_with_workflow must parse workflow_id")
			Expect(result.Confidence).To(BeNumerically("~", 0.95, 0.01))
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"successful workflow selection should not require human review")
		})
	})

	Describe("IT-KA-760-006: Text → retry → submit_result_no_workflow", func() {
		It("should retry on text and succeed when LLM uses submit_result_no_workflow", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"ResourceQuota exhausted","confidence":0.95}`}},
					{Message: llm.Message{Role: "assistant", Content: "No workflow handles this scenario."}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"Quota exhausted"},"reasoning":"No matching workflow"}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"retry should produce no_matching_workflows via submit_result_no_workflow")
			Expect(mockClient.calls).To(HaveLen(3),
				"should make 3 LLM calls: RCA + initial workflow + retry")
		})
	})

	Describe("IT-KA-760-007: Unrecognized JSON → retry → submit_result_no_workflow", func() {
		It("should retry on unrecognized JSON and succeed via submit tool", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"foo":"bar","baz":42}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"reasoning":"No suitable workflow"}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"))
		})
	})

	Describe("IT-KA-760-008: Text 1st → submit_result_with_workflow 2nd → workflow parsed", func() {
		It("should retry on text and succeed when LLM uses submit_result_with_workflow", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{Message: llm.Message{Role: "assistant", Content: "I'll select a workflow for this..."}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_with_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"selected_workflow":{"workflow_id":"oom-increase-memory","confidence":0.9},"confidence":0.9}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.WorkflowID).To(Equal("oom-increase-memory"),
				"retry should produce valid workflow via submit_result_with_workflow")
			Expect(result.HumanReviewNeeded).To(BeFalse())
		})
	})

	Describe("IT-KA-760-009: All retries exhausted → no_matching_workflows", func() {
		It("should classify as no_matching_workflows after retry exhaustion", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"ResourceQuota exhausted","confidence":0.95}`}},
					{Message: llm.Message{Role: "assistant", Content: "No workflow handles this."}},
					{Message: llm.Message{Role: "assistant", Content: "I still can't find a suitable workflow."}},
					{Message: llm.Message{Role: "assistant", Content: "There really isn't a workflow for this."}},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"exhausted retries must classify as no_matching_workflows, not llm_parsing_error")
			Expect(result.RCASummary).To(ContainSubstring("ResourceQuota"),
				"RCA must be preserved through retry exhaustion")
		})
	})

	Describe("IT-KA-760-010: RCA phase text → parsed as summary (unaffected by split)", func() {
		It("should parse RCA text as summary without triggering split tool logic", func() {
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: "The pod is OOMKilled due to memory limits being too low."}},
					{Message: llm.Message{Role: "assistant", Content: "still text, retry also fails"}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"reasoning":"No workflow"}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: logger, MaxTurns: 15, PhaseTools: phaseTools,
			})

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"),
				"RCA text should be treated as summary")
		})
	})

	Describe("IT-KA-760-011: Catalog self-correction with submit_result_with_workflow", func() {
		It("should self-correct invalid workflow via split submit tool", func() {
			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit", Name: "submit_result_with_workflow", Arguments: `{"root_cause_analysis":{"summary":"crashed"},"selected_workflow":{"workflow_id":"unknown-workflow","confidence":0.8},"confidence":0.8}`},
						},
					},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_submit2", Name: "submit_result_with_workflow", Arguments: `{"root_cause_analysis":{"summary":"crashed"},"selected_workflow":{"workflow_id":"restart","confidence":0.7},"confidence":0.7}`},
						},
					},
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api", Namespace: "default", Severity: "warning", Message: "CrashLoop",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("restart"),
				"self-correction must produce valid workflow via split submit tool")
			Expect(result.HumanReviewNeeded).To(BeFalse())
		})
	})
})

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

	Describe("IT-KA-760-002: Garbage JSON -> retry exhaustion -> no_matching_workflows", func() {
		It("should classify garbage JSON after retry exhaustion as no_matching_workflows", func() {
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
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"#760 v2: garbage JSON triggers parse retry → exhaustion → no_matching_workflows")
			Expect(result.Reason).To(ContainSubstring("submit tool"),
				"garbage JSON is treated as non-tool workflow output; retries exhaust with this reason")
		})
	})

	Describe("IT-KA-760-003: Catalog validation self-correction still works", func() {
		It("should self-correct invalid workflow and return valid result", func() {
			validator := parser.NewValidator([]string{"restart", "scale-up"})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crashed"}`}},
					wfToolResp(`{"workflow_id":"unknown-workflow","confidence":0.8}`),
					wfToolResp(`{"workflow_id":"restart","confidence":0.7}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
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
