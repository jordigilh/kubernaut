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

package tools_test

import (
	"context"
	"errors"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// mockHTTPCompleter implements mcptools.HTTPSessionCompleter for unit tests.
type mockHTTPCompleter struct {
	completedID     string
	completedResult *katypes.InvestigationResult
	completeErr     error
	foundID         string
	found           bool
}

func (m *mockHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	m.completedID = id
	m.completedResult = result
	return m.completeErr
}

func (m *mockHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	return m.foundID, m.found
}

func (m *mockHTTPCompleter) ForceCompleteByRemediationID(_ string, result *katypes.InvestigationResult) error {
	m.completedResult = result
	return m.completeErr
}

// discoveryWithWorkflow creates a DiscoveryResult with a recommended workflow.
func discoveryWithWorkflow(wfID string) *mcpinternal.WorkflowDiscoveryResult {
	return &mcpinternal.WorkflowDiscoveryResult{
		Recommended: &mcpinternal.DiscoveredWorkflow{
			WorkflowID: wfID,
			Confidence: 0.9,
			Rationale:  "test recommended",
		},
	}
}

type mockEnrichmentRunner struct {
	result *enrichment.EnrichmentResult
	err    error
}

func (m *mockEnrichmentRunner) Enrich(_ context.Context, _, _, _, _, _, _ string) (*enrichment.EnrichmentResult, error) {
	return m.result, m.err
}

type mockWorkflowCatalog struct {
	workflow *mcptools.CatalogWorkflow
	err      error
}

func (m *mockWorkflowCatalog) GetWorkflowByID(_ context.Context, workflowID string) (*mcptools.CatalogWorkflow, error) {
	return m.workflow, m.err
}

var _ = Describe("kubernaut_select_workflow tool — #703 BR-INTERACTIVE-005", func() {

	Describe("UT-KA-703-TOOL-005: Input validation", func() {
		It("should reject empty rr_id", func() {
			tool := mcptools.NewSelectWorkflowTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})

		It("should reject empty workflow_id", func() {
			tool := mcptools.NewSelectWorkflowTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-001",
				WorkflowID: "",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow_id"))
		})
	})

	Describe("UT-KA-703-TOOL-006: Successful workflow selection", func() {
		It("should look up workflow from catalog and return selection confirmation", func() {
			wfID := uuid.New().String()
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:      wfID,
					WorkflowName:    "increase-memory",
					ActionType:      "scale-vertical",
					Version:         "v1.2.0",
					ExecutionEngine: "argo-workflows",
					ExecutionBundle: "oci://registry/increase-memory:v1.2.0",
				},
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-wf-001",
					CorrelationID:   "rr-wf-001",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-wf-001",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))
			Expect(output.Workflow).NotTo(BeNil())
			Expect(output.Workflow.WorkflowID).To(Equal(wfID))
			Expect(output.Workflow.WorkflowName).To(Equal("increase-memory"))
			Expect(output.Workflow.ActionType).To(Equal("scale-vertical"))
			Expect(output.Workflow.ExecutionEngine).To(Equal("argo-workflows"))
			Expect(output.Confidence).To(Equal(1.0))
			Expect(output.Rationale).To(Equal("User-selected via interactive mode"))
		})
	})

	Describe("UT-KA-703-TOOL-006b: Workflow not found in catalog", func() {
		It("should return error when workflow_id does not exist in catalog", func() {
			wfID := "nonexistent-wf"
			catalog := &mockWorkflowCatalog{err: errors.New("workflow not found")}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-wf-002",
					CorrelationID:   "rr-wf-002",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-wf-002",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow"))
		})
	})

	Describe("UT-KA-703-TOOL-006c: Tool rejects requests when no active session", func() {
		It("should return error when no interactive session is active for rr_id", func() {
			catalog := &mockWorkflowCatalog{}
			sessions := &mockSessionManager{isActive: false}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-no-sess",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session"))
		})
	})

	Describe("UT-KA-703-TOOL-006d: Tool enforces driver identity", func() {
		It("should reject requests from a user who is not the active driver", func() {
			catalog := &mockWorkflowCatalog{workflow: &mcptools.CatalogWorkflow{}}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-wf-003",
					CorrelationID: "rr-authz-wf",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-authz-wf",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("driver"))
		})
	})

	Describe("UT-KA-1012-001: Internalized enrichment (#1012)", func() {
		It("should call enrichment before catalog lookup and include result in output", func() {
			wfID := uuid.New().String()
			enrichResult := &enrichment.EnrichmentResult{
				ResourceKind:      "Deployment",
				ResourceName:      "api-server",
				ResourceNamespace: "production",
			}
			runner := &mockEnrichmentRunner{result: enrichResult}
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:   wfID,
					WorkflowName: "restart-pods",
				},
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-enrich-001",
					CorrelationID:   "rr-enrich-001",
					ActingUser:      mcpinternal.UserInfo{Username: "alice", Groups: []string{"sre"}},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions, mcptools.WithEnrichmentRunner(runner))
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-enrich-001",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "api-server",
				Namespace:  "production",
			}, mcpinternal.UserInfo{Username: "alice", Groups: []string{"sre"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))
			Expect(output.Enrichment).NotTo(BeNil())
			Expect(output.Enrichment.ResourceKind).To(Equal("Deployment"))
			Expect(output.Workflow.WorkflowID).To(Equal(wfID))
		})

		It("should propagate ErrRBACForbidden from enrichment as ErrCodeForbidden", func() {
			wfID := "wf-enrich-rbac"
			runner := &mockEnrichmentRunner{
				err: enrichment.ErrRBACForbidden,
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-enrich-002",
					CorrelationID:   "rr-enrich-002",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(nil, sessions, mcptools.WithEnrichmentRunner(runner))
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-enrich-002",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "api-server",
				Namespace:  "restricted-ns",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forbidden"))
		})

		It("should propagate generic enrichment errors", func() {
			wfID := "wf-enrich-generic"
			runner := &mockEnrichmentRunner{
				err: errors.New("k8s API unreachable"),
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-enrich-003",
					CorrelationID:   "rr-enrich-003",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(nil, sessions, mcptools.WithEnrichmentRunner(runner))
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-enrich-003",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "api-server",
				Namespace:  "production",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enrich"))
		})

		It("should skip enrichment when no runner is configured", func() {
			wfID := uuid.New().String()
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{WorkflowID: wfID, WorkflowName: "restart-pods"},
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-enrich-004",
					CorrelationID:   "rr-enrich-004",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-enrich-004",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))
			Expect(output.Enrichment).To(BeNil())
		})
	})
})

var _ = Describe("kubernaut_select_workflow — discovery gating & auto-complete", func() {

	Describe("UT-KA-SW-GATE-001: select_workflow rejects when no discovery result", func() {
		It("should return error when DiscoveryResult is nil", func() {
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-gate-001",
					CorrelationID: "rr-gate-001",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "test rca"},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(nil, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-gate-001",
				WorkflowID: "wf-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("discover_workflows"))
		})
	})

	Describe("UT-KA-SW-GATE-002: select_workflow rejects workflow_id not in discovery", func() {
		It("should return error when workflow_id is not from discovery results", func() {
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-gate-002",
					CorrelationID: "rr-gate-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
						Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-recommended"},
						Alternatives: []mcpinternal.DiscoveredWorkflow{
							{WorkflowID: "wf-alt-1"},
						},
					},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(nil, sessions)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-gate-002",
				WorkflowID: "wf-not-in-discovery",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in discovery"))
		})
	})

	Describe("UT-KA-SW-GATE-003: select_workflow accepts alternative workflow_id", func() {
		It("should accept workflow_id from alternatives list", func() {
			altWfID := "wf-alt-good"
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:   altWfID,
					WorkflowName: "rollback-deploy",
				},
			}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-gate-003",
					CorrelationID: "rr-gate-003",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
						Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-recommended"},
						Alternatives: []mcpinternal.DiscoveredWorkflow{
							{WorkflowID: altWfID},
						},
					},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-gate-003",
				WorkflowID: altWfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))
		})
	})

	Describe("UT-KA-SW-AC-001: select_workflow auto-completes HTTP session", func() {
		It("should call CompleteUserDriving and release MCP lease", func() {
			wfID := "wf-autoclose"
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:      wfID,
					WorkflowName:    "restart-pod",
					ExecutionEngine: "argo",
					ExecutionBundle: "oci://restart:v1",
					Version:         "v1.0",
				},
			}
			completer := &mockHTTPCompleter{foundID: "http-sess-001", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-ac-001",
					CorrelationID:   "rr-ac-001",
					ActingUser:      mcpinternal.UserInfo{Username: "alice"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "OOM crash", Confidence: 0.9},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions,
				mcptools.WithHTTPSessionCompleter(completer),
			)
			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-ac-001",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflow_selected"))

			Expect(completer.completedID).To(Equal("http-sess-001"))
			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.WorkflowID).To(Equal(wfID))
			Expect(completer.completedResult.RCASummary).To(Equal("OOM crash"))
			Expect(completer.completedResult.WorkflowRationale).To(Equal("User-selected via interactive mode"))

			Expect(sessions.releasedID).To(Equal("sess-ac-001"))
			Expect(sessions.releasedReason).To(Equal("workflow_selected"))
		})
	})
})

var _ = Describe("kubernaut_select_workflow — helper functions", func() {

	Describe("UT-KA-SW-BUILDFINAL-001: buildFinalResult merges RCA + workflow fields", func() {
		It("should copy all RCA fields and overlay workflow fields", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "Pod OOM due to memory leak",
				Confidence: 0.92,
				Severity:   "critical",
				Reason:     "original reason",
			}
			workflow := &mcptools.CatalogWorkflow{
				WorkflowID:         "wf-increase-mem",
				WorkflowName:       "increase-memory",
				ExecutionEngine:    "argo-workflows",
				ExecutionBundle:    "oci://registry/increase-memory:v1.2.0",
				ServiceAccountName: "remediation-sa",
				Version:            "v1.2.0",
			}

			result := mcptools.BuildFinalResult(rca, workflow)
			Expect(result.RCASummary).To(Equal("Pod OOM due to memory leak"))
			Expect(result.Confidence).To(Equal(0.92))
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Reason).To(Equal("original reason"))

			Expect(result.WorkflowID).To(Equal("wf-increase-mem"))
			Expect(result.ExecutionEngine).To(Equal("argo-workflows"))
			Expect(result.ExecutionBundle).To(Equal("oci://registry/increase-memory:v1.2.0"))
			Expect(result.ServiceAccountName).To(Equal("remediation-sa"))
			Expect(result.WorkflowVersion).To(Equal("v1.2.0"))
			Expect(result.WorkflowRationale).To(Equal("User-selected via interactive mode"))
		})

		It("should handle nil workflow gracefully", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "test rca",
				Confidence: 0.5,
			}

			result := mcptools.BuildFinalResult(rca, nil)
			Expect(result.RCASummary).To(Equal("test rca"))
			Expect(result.WorkflowID).To(BeEmpty())
		})

		It("should not mutate the original RCA", func() {
			rca := &katypes.InvestigationResult{RCASummary: "original"}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-001"}

			result := mcptools.BuildFinalResult(rca, workflow)
			Expect(result.WorkflowID).To(Equal("wf-001"))
			Expect(rca.WorkflowID).To(BeEmpty(), "original RCA must not be mutated")
		})
	})

	Describe("UT-KA-SW-ISWF-001: isWorkflowInDiscoveryResult edge cases", func() {
		It("should match recommended workflow", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-rec"},
			}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-rec", dr)).To(BeTrue())
		})

		It("should match alternative workflow", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{
				Recommended:  &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-rec"},
				Alternatives: []mcpinternal.DiscoveredWorkflow{{WorkflowID: "wf-alt-1"}, {WorkflowID: "wf-alt-2"}},
			}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-alt-2", dr)).To(BeTrue())
		})

		It("should reject workflow not in results", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{
				Recommended:  &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-rec"},
				Alternatives: []mcpinternal.DiscoveredWorkflow{{WorkflowID: "wf-alt-1"}},
			}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-unknown", dr)).To(BeFalse())
		})

		It("should handle nil recommended with alternatives only", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{
				Alternatives: []mcpinternal.DiscoveredWorkflow{{WorkflowID: "wf-alt-only"}},
			}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-alt-only", dr)).To(BeTrue())
		})

		It("should handle nil alternatives with recommended only", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-rec-only"},
			}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-rec-only", dr)).To(BeTrue())
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-other", dr)).To(BeFalse())
		})

		It("should handle empty discovery result", func() {
			dr := &mcpinternal.WorkflowDiscoveryResult{}
			Expect(mcptools.IsWorkflowInDiscoveryResult("wf-any", dr)).To(BeFalse())
		})
	})
})

var _ = Describe("kubernaut_complete_no_action — interactive pipeline completion", func() {

	Describe("UT-KA-CNA-001: Complete with RCA and reason", func() {
		It("should complete with RCA result, AA routing signals, and optional reason", func() {
			completer := &mockHTTPCompleter{foundID: "http-cna-001", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cna-001",
					CorrelationID: "rr-cna-001",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "false alarm - metric spike", Confidence: 0.8},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(sessions,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
			)
			output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID:   "rr-cna-001",
				Reason: "false alarm",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("completed_no_action"))
			Expect(output.Reason).To(Equal("false alarm"))

			Expect(completer.completedID).To(Equal("http-cna-001"))
			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.RCASummary).To(Equal("false alarm - metric spike"))
			Expect(completer.completedResult.Reason).To(Equal("false alarm"))
			Expect(completer.completedResult.WorkflowID).To(BeEmpty())

			Expect(completer.completedResult.IsActionable).NotTo(BeNil())
			Expect(*completer.completedResult.IsActionable).To(BeFalse(),
				"IsActionable must be false so AA routes to Completed/WorkflowNotNeeded")
			Expect(completer.completedResult.Warnings).To(ContainElement("Alert not actionable"),
				"Warnings must include 'Alert not actionable' for AA routing")

			Expect(sessions.releasedID).To(Equal("sess-cna-001"))
			Expect(sessions.releasedReason).To(Equal("complete_no_action"))
		})
	})

	Describe("UT-KA-CNA-002: Complete without RCA", func() {
		It("should build minimal result when no RCA exists", func() {
			completer := &mockHTTPCompleter{foundID: "http-cna-002", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cna-002",
					CorrelationID: "rr-cna-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(sessions,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
			)
			output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID:   "rr-cna-002",
				Reason: "will handle manually",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("completed_no_action"))

			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.Reason).To(Equal("will handle manually"))
		})
	})

	Describe("UT-KA-CNA-003: Complete rejects non-driver", func() {
		It("should reject caller who is not the active driver", func() {
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cna-003",
					CorrelationID: "rr-cna-003",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(sessions)
			_, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID: "rr-cna-003",
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("driver"))
		})
	})

	Describe("UT-KA-CNA-004: Complete rejects empty rr_id", func() {
		It("should reject empty rr_id", func() {
			tool := mcptools.NewCompleteNoActionTool(nil)
			_, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID: "",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})
	})

	Describe("UT-KA-CNA-005: Complete with no active session", func() {
		It("should return error when no session exists", func() {
			sessions := &mockSessionManager{isActive: false}
			tool := mcptools.NewCompleteNoActionTool(sessions)
			_, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID: "rr-cna-005",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session"))
		})
	})

	Describe("UT-KA-CNA-006: Default reason when omitted", func() {
		It("should use default reason when none provided", func() {
			completer := &mockHTTPCompleter{foundID: "http-cna-006", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cna-006",
					CorrelationID: "rr-cna-006",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "test rca"},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(sessions,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
			)
			output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID: "rr-cna-006",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("completed_no_action"))

			Expect(completer.completedResult.Reason).To(Equal("no action needed"))
		})
	})

	Describe("UT-KA-CNA-007: complete_no_action after discover_workflows uses stored RCA", func() {
		It("should use RCA from discovery when completing with no action", func() {
			completer := &mockHTTPCompleter{foundID: "http-cna-007", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cna-007",
					CorrelationID: "rr-cna-007",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult: &katypes.InvestigationResult{
						RCASummary: "Pod OOM due to memory leak",
						Confidence: 0.92,
						Severity:   "critical",
					},
					DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
						Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-discovered"},
					},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(sessions,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
			)
			output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID:   "rr-cna-007",
				Reason: "user prefers manual fix",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("completed_no_action"))

			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.RCASummary).To(Equal("Pod OOM due to memory leak"),
				"should use the stored RCA from discover_workflows")
			Expect(completer.completedResult.Confidence).To(Equal(0.92))
			Expect(completer.completedResult.WorkflowID).To(BeEmpty(),
				"no workflow should be selected")
			Expect(completer.completedResult.Reason).To(Equal("user prefers manual fix"))
			Expect(*completer.completedResult.IsActionable).To(BeFalse())
		})
	})
})
