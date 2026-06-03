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
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// mockHTTPCompleter implements mcptools.HTTPSessionCompleter for unit tests.
// Fields written by goroutines are guarded by mu.
type mockHTTPCompleter struct {
	mu              sync.Mutex
	completedID     string
	completedResult *katypes.InvestigationResult
	completeErr     error
	foundID         string
	found           bool
}

func (m *mockHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedID = id
	m.completedResult = result
	return m.completeErr
}

func (m *mockHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.foundID, m.found
}

func (m *mockHTTPCompleter) ForceCompleteByRemediationID(_ string, result *katypes.InvestigationResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedResult = result
	return m.completeErr
}

func (m *mockHTTPCompleter) getCompleted() (string, *katypes.InvestigationResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completedID, m.completedResult
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

			// Session completion and lease release are deferred to a goroutine
			// to avoid closing the transport before the MCP response is sent.
			// All assertions use mutex-safe getters to avoid data races.
			Eventually(func(g Gomega) {
				id, result := completer.getCompleted()
				g.Expect(id).To(Equal("http-sess-001"))
				g.Expect(result).NotTo(BeNil())
				g.Expect(result.WorkflowID).To(Equal(wfID))
				g.Expect(result.RCASummary).To(Equal("OOM crash"))
				g.Expect(result.WorkflowRationale).To(Equal("User-selected via interactive mode"))
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				id, reason := sessions.getReleased()
				g.Expect(id).To(Equal("sess-ac-001"))
				g.Expect(reason).To(Equal("workflow_selected"))
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())
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

			result := mcptools.BuildFinalResult(rca, workflow, nil)
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

			result := mcptools.BuildFinalResult(rca, nil, nil)
			Expect(result.RCASummary).To(Equal("test rca"))
			Expect(result.WorkflowID).To(BeEmpty())
		})

		It("should not mutate the original RCA", func() {
			rca := &katypes.InvestigationResult{RCASummary: "original"}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-001"}

			result := mcptools.BuildFinalResult(rca, workflow, nil)
			Expect(result.WorkflowID).To(Equal("wf-001"))
			Expect(rca.WorkflowID).To(BeEmpty(), "original RCA must not be mutated")
		})
	})

	Describe("UT-KA-SW-BUILDFINAL-002: buildFinalResult merges per-workflow parameters (BR-INTERACTIVE, #1169)", func() {
		It("should use discovery recommended parameters instead of stale RCA params", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "OOM",
				Parameters: map[string]interface{}{
					"STALE_KEY": "stale_val",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-rec"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-rec",
					Parameters: map[string]interface{}{"MEMORY_LIMIT_NEW": "512Mi"},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).To(HaveKey("MEMORY_LIMIT_NEW"),
				"interactive workflow selection must expose parameters from the discovery recommendation")
			Expect(result.Parameters["MEMORY_LIMIT_NEW"]).To(Equal("512Mi"),
				"downstream automation needs the discovery memory limit, not an arbitrary placeholder")
			Expect(result.Parameters).NotTo(HaveKey("STALE_KEY"),
				"stale RCA parameters must not leak after a fresh discovery recommendation")
		})

		It("should replace parameters when selecting an alternative workflow", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "rollout stuck",
				Parameters: map[string]interface{}{
					"RECOMMENDED_ONLY": "should-not-appear",
				},
			}
			altID := "wf-alt-params"
			workflow := &mcptools.CatalogWorkflow{WorkflowID: altID}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-recommended",
					Parameters: map[string]interface{}{"RECOMMENDED_ONLY": "rec-val"},
				},
				Alternatives: []mcpinternal.DiscoveredWorkflow{
					{
						WorkflowID: altID,
						Parameters: map[string]interface{}{
							"REPLICA_COUNT": "5",
							"MAX_SURGE":     "2",
						},
					},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).To(HaveKey("REPLICA_COUNT"),
				"the chosen alternative workflow must drive rollout parameters for AA")
			Expect(result.Parameters).To(HaveKey("MAX_SURGE"),
				"surge settings must come from the alternative workflow the user picked")
			Expect(result.Parameters).NotTo(HaveKey("RECOMMENDED_ONLY"),
				"recommended-workflow parameters must not remain when an alternative is selected")
		})

		It("should pass through RCA params when discovery is nil", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "no discovery snapshot",
				Parameters: map[string]interface{}{
					"PRESERVE_ME": "yes",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-001"}

			result := mcptools.BuildFinalResult(rca, workflow, nil)
			Expect(result.Parameters).To(HaveKey("PRESERVE_ME"),
				"without discovery context, hand-off must keep the RCA-supplied parameters")
			Expect(result.Parameters["PRESERVE_ME"]).To(Equal("yes"),
				"legacy completion paths must not drop parameters when discovery is unavailable")
		})

		It("should clear params when selected alternative has nil parameters", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "alt without params",
				Parameters: map[string]interface{}{
					"OLD": "old",
				},
			}
			altID := "wf-alt-empty-params"
			workflow := &mcptools.CatalogWorkflow{WorkflowID: altID}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-rec",
					Parameters: map[string]interface{}{"OLD": "rec"},
				},
				Alternatives: []mcpinternal.DiscoveredWorkflow{
					{WorkflowID: altID, Parameters: nil},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).To(BeEmpty(),
				"an alternative that defines no parameters must yield an empty parameter map for execution")
		})

		It("should keep RCA params when workflow not found in discovery", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "fallback path",
				Parameters: map[string]interface{}{
					"FROM_RCA": "keep",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-not-in-discovery"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-rec",
					Parameters: map[string]interface{}{"DISC_ONLY": "x"},
				},
				Alternatives: []mcpinternal.DiscoveredWorkflow{
					{WorkflowID: "wf-alt", Parameters: map[string]interface{}{"ALT_ONLY": "y"}},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).To(HaveKey("FROM_RCA"),
				"graceful fallback must retain RCA parameters when discovery cannot map the catalog workflow")
			Expect(result.Parameters["FROM_RCA"]).To(Equal("keep"),
				"operators rely on RCA parameters when discovery IDs and catalog IDs diverge")
		})

		It("should clear recommended params when recommended has nil parameters", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "rec nil params",
				Parameters: map[string]interface{}{
					"RCA_PARAM": "v",
				},
			}
			recID := "wf-rec-nil-params"
			workflow := &mcptools.CatalogWorkflow{WorkflowID: recID}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: recID,
					Parameters: nil,
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).To(BeEmpty(),
				"explicit nil parameters on the recommended workflow must override stale RCA parameter maps")
		})

		It("should not mutate original RCA or discovery parameter maps", func() {
			rcaParams := map[string]interface{}{"RCA_K": "rca_v"}
			recParams := map[string]interface{}{"DISC_K": "disc_v"}
			rca := &katypes.InvestigationResult{
				RCASummary: "immutability",
				Parameters: rcaParams,
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-immut"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-immut",
					Parameters: recParams,
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Parameters).NotTo(BeNil(),
				"buildFinalResult should produce a distinct parameters map for safe mutation downstream")
			result.Parameters["MUTATED"] = "after"
			Expect(rcaParams).NotTo(HaveKey("MUTATED"),
				"session RCA storage must remain untouched when the completer mutates the merged result")
			Expect(recParams).NotTo(HaveKey("MUTATED"),
				"stored discovery recommendations must stay immutable for audit and replay")
			Expect(rcaParams).To(HaveKey("RCA_K"),
				"original RCA parameter keys must be preserved in the source map")
			Expect(recParams).To(HaveKey("DISC_K"),
				"original discovery parameter keys must be preserved in the source map")
		})
	})

	Describe("UT-KA-SW-BUILDFINAL-PARAMS-001: buildFinalResult injects TARGET_RESOURCE_* from RemediationTarget", func() {
		It("should include TARGET_RESOURCE_* alongside discovery params when RemediationTarget is set", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "OOM on api-server",
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "production",
				},
				Parameters: map[string]interface{}{
					"STALE_KEY": "stale_val",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-increase-mem"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-increase-mem",
					Parameters: map[string]interface{}{"MEMORY_LIMIT_NEW": "512Mi"},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
				"discovery params must be preserved")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "api-server"),
				"KA-managed TARGET_RESOURCE_NAME must be injected from RemediationTarget")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"KA-managed TARGET_RESOURCE_KIND must be injected from RemediationTarget")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "production"),
				"KA-managed TARGET_RESOURCE_NAMESPACE must be injected from RemediationTarget")
			Expect(result.Parameters).NotTo(HaveKey("STALE_KEY"),
				"stale RCA params must not leak when discovery params are found")
		})

		It("should inject TARGET_RESOURCE_* even when discovery has nil params for the selected workflow", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "crash loop",
				RemediationTarget: katypes.RemediationTarget{
					Kind:       "Deployment",
					Name:       "worker",
					Namespace:  "demo-crashloop",
					APIVersion: "apps/v1",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-rollback"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-rollback",
					Parameters: nil,
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "worker"),
				"TARGET_RESOURCE_NAME must be injected even when discovery params are nil")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"TARGET_RESOURCE_KIND must be injected even when discovery params are nil")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "demo-crashloop"),
				"TARGET_RESOURCE_NAMESPACE must be injected even when discovery params are nil")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"),
				"TARGET_RESOURCE_API_VERSION must be injected when RemediationTarget has apiVersion")
		})

		It("should prefer Phase 3's K8s-verified RemediationTarget over Phase 2's LLM-parsed values", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "OOM analysis",
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-increase-mem"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-increase-mem",
					Parameters: map[string]interface{}{"MEMORY_LIMIT_NEW": "512Mi"},
				},
				FullResult: &katypes.InvestigationResult{
					RemediationTarget: katypes.RemediationTarget{
						Kind:      "Deployment",
						Name:      "api-server",
						Namespace: "production",
					},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "api-server"),
				"must use Phase 3's K8s-verified name, not Phase 2's LLM-parsed 'test-pod'")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"must use Phase 3's K8s-verified kind, not Phase 2's LLM-parsed 'Pod'")
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "production"),
				"must use Phase 3's K8s-verified namespace, not Phase 2's LLM-parsed 'default'")
			Expect(result.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
				"discovery params must still be present alongside corrected TARGET_RESOURCE_*")
		})

		It("should not inject TARGET_RESOURCE_* when RemediationTarget.Kind is empty", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "no target",
				Parameters: map[string]interface{}{"EXISTING": "val"},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-generic"}

			result := mcptools.BuildFinalResult(rca, workflow, nil)

			Expect(result.Parameters).To(HaveKeyWithValue("EXISTING", "val"),
				"RCA params should pass through when no discovery and no RemediationTarget")
			Expect(result.Parameters).NotTo(HaveKey("TARGET_RESOURCE_NAME"),
				"should not inject TARGET_RESOURCE_* when RemediationTarget.Kind is empty")
		})
	})

	Describe("UT-KA-1351-010: buildFinalResult propagates Phase 3 confidence (KA-HIGH-1)", func() {
		It("should use discovery confidence when higher than Phase 2 RCA", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "Pod OOM",
				Confidence: 0.65,
			}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-scale",
					Confidence: 0.92,
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-scale"}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result.Confidence).To(Equal(0.92),
				"buildFinalResult must use Phase 3 discovery confidence, not stale Phase 2 RCA confidence (KA-HIGH-1)")
		})

		It("should keep RCA confidence when discovery is nil", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "Pod OOM",
				Confidence: 0.88,
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-restart"}

			result := mcptools.BuildFinalResult(rca, workflow, nil)

			Expect(result.Confidence).To(Equal(0.88),
				"without discovery context, confidence must remain from Phase 2")
		})
	})

	Describe("UT-KA-1351-011: buildFinalResult propagates AlternativeWorkflows (KA-MED-2)", func() {
		It("should populate AlternativeWorkflows from discovery alternatives", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "CPU throttled",
				Confidence: 0.85,
			}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "wf-scale",
					Confidence: 0.92,
					Rationale:  "high correlation",
				},
				Alternatives: []mcpinternal.DiscoveredWorkflow{
					{WorkflowID: "wf-restart", Confidence: 0.75, Rationale: "simpler fix"},
					{WorkflowID: "wf-rollback", Confidence: 0.60, Rationale: "last resort"},
				},
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-scale"}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result.AlternativeWorkflows).To(HaveLen(2),
				"buildFinalResult must propagate discovery alternatives to the final result (KA-MED-2)")
			Expect(result.AlternativeWorkflows[0].WorkflowID).To(Equal("wf-restart"))
			Expect(result.AlternativeWorkflows[1].WorkflowID).To(Equal("wf-rollback"))
		})
	})

	Describe("UT-KA-1351-011-digest: buildFinalResult propagates ExecutionBundleDigest (KA-MED-1)", func() {
		It("should copy ExecutionBundleDigest from workflow to result", func() {
			rca := &katypes.InvestigationResult{RCASummary: "OOM", Confidence: 0.8}
			workflow := &mcptools.CatalogWorkflow{
				WorkflowID:            "wf-restart",
				ExecutionBundle:       "bundle.tar.gz",
				ExecutionBundleDigest: "sha256:abc123",
			}
			result := mcptools.BuildFinalResult(rca, workflow, nil)
			Expect(result.ExecutionBundleDigest).To(Equal("sha256:abc123"),
				"buildFinalResult must propagate ExecutionBundleDigest from catalog (KA-MED-1)")
		})
	})

	Describe("UT-KA-1351-014: buildFinalResult with nil discovery gracefully degrades (KA-MED-8)", func() {
		It("should not panic and fall back to RCA-only fields", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "Disk full",
				Confidence: 0.75,
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-cleanup"}

			// Should not panic with nil discovery
			result := mcptools.BuildFinalResult(rca, workflow, nil)
			Expect(result.Confidence).To(Equal(0.75))
			Expect(result.RCASummary).To(Equal("Disk full"))
			Expect(result.WorkflowID).To(Equal("wf-cleanup"))
		})
	})

	Describe("UT-KA-SW-BUILDFINAL-003: buildFinalResult propagates Phase 3 fields from FullResult", func() {
		It("should propagate DetectedLabels from FullResult", func() {
			rca := &katypes.InvestigationResult{RCASummary: "OOM", Confidence: 0.7}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-labels"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-labels", Confidence: 0.9},
				FullResult: &katypes.InvestigationResult{
					DetectedLabels: map[string]interface{}{"app": "nginx", "team": "platform"},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.DetectedLabels).To(HaveKeyWithValue("app", "nginx"),
				"Phase 3 DetectedLabels must propagate to final result for GitOps-aware scoring")
			Expect(result.DetectedLabels).To(HaveKeyWithValue("team", "platform"))
		})

		It("should propagate Warnings from FullResult", func() {
			rca := &katypes.InvestigationResult{RCASummary: "OOM", Confidence: 0.7}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-warn"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-warn", Confidence: 0.9},
				FullResult: &katypes.InvestigationResult{
					Warnings: []string{"high blast radius", "production namespace"},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Warnings).To(ContainElement("high blast radius"),
				"Phase 3 Warnings must propagate to final result for operator visibility")
			Expect(result.Warnings).To(ContainElement("production namespace"))
		})

		It("should propagate IsActionable from FullResult", func() {
			rca := &katypes.InvestigationResult{RCASummary: "OOM", Confidence: 0.7}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-actionable"}
			actionable := true
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-actionable", Confidence: 0.9},
				FullResult: &katypes.InvestigationResult{
					IsActionable: &actionable,
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.IsActionable).NotTo(BeNil(),
				"Phase 3 IsActionable must propagate to final result for AA routing")
			Expect(*result.IsActionable).To(BeTrue())
		})

		It("should propagate InvestigationOutcome from FullResult", func() {
			rca := &katypes.InvestigationResult{RCASummary: "OOM", Confidence: 0.7}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-outcome"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-outcome", Confidence: 0.9},
				FullResult: &katypes.InvestigationResult{
					InvestigationOutcome: "actionable",
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.InvestigationOutcome).To(Equal("actionable"),
				"Phase 3 InvestigationOutcome must propagate for AA outcome routing")
		})

		It("should not clobber Phase 2 values when FullResult fields are empty/nil", func() {
			actionable := true
			rca := &katypes.InvestigationResult{
				RCASummary:           "OOM",
				Confidence:           0.7,
				DetectedLabels:       map[string]interface{}{"phase2": "label"},
				Warnings:             []string{"phase2 warning"},
				IsActionable:         &actionable,
				InvestigationOutcome: "inconclusive",
			}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-preserve"}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-preserve", Confidence: 0.9},
				FullResult:  &katypes.InvestigationResult{},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.DetectedLabels).To(HaveKeyWithValue("phase2", "label"),
				"Phase 2 DetectedLabels must be preserved when Phase 3 has none")
			Expect(result.Warnings).To(ContainElement("phase2 warning"),
				"Phase 2 Warnings must be preserved when Phase 3 has none")
			Expect(result.IsActionable).NotTo(BeNil(),
				"Phase 2 IsActionable must be preserved when Phase 3 has nil")
			Expect(*result.IsActionable).To(BeTrue())
			Expect(result.InvestigationOutcome).To(Equal("inconclusive"),
				"Phase 2 InvestigationOutcome must be preserved when Phase 3 is empty")
		})

		It("should propagate all Phase 3 fields together", func() {
			rca := &katypes.InvestigationResult{RCASummary: "combined test", Confidence: 0.5}
			workflow := &mcptools.CatalogWorkflow{WorkflowID: "wf-all"}
			actionable := true
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-all", Confidence: 0.95},
				FullResult: &katypes.InvestigationResult{
					Confidence:           0.95,
					DetectedLabels:       map[string]interface{}{"env": "prod"},
					Warnings:             []string{"critical path"},
					IsActionable:         &actionable,
					InvestigationOutcome: "actionable",
					RemediationTarget:    katypes.RemediationTarget{Kind: "Deployment", Name: "web", Namespace: "prod"},
				},
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.Confidence).To(Equal(0.95))
			Expect(result.DetectedLabels).To(HaveKeyWithValue("env", "prod"))
			Expect(result.Warnings).To(ContainElement("critical path"))
			Expect(*result.IsActionable).To(BeTrue())
			Expect(result.InvestigationOutcome).To(Equal("actionable"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
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

var _ = Describe("extractDiscoveryResult — parameter propagation (BR-LLM-026, #1169)", func() {

	Describe("UT-KA-EDR-001: recommended parameters (BR-LLM-026, #1169)", func() {
		It("should copy recommended workflow parameters from InvestigationResult", func() {
			inv := &katypes.InvestigationResult{
				WorkflowID: "wf-rec",
				Parameters: map[string]interface{}{"KEY": "val"},
			}

			dr := mcptools.ExtractDiscoveryResult(inv)
			Expect(dr).NotTo(BeNil(), "operators need a discovery envelope even when only parameters change")
			Expect(dr.Recommended).NotTo(BeNil(), "a primary workflow must be advertised when WorkflowID is present")
			Expect(dr.Recommended.Parameters).To(HaveKeyWithValue("KEY", "val"),
				"discovered recommended workflow must surface LLM parameter bindings for execution")
		})
	})

	Describe("UT-KA-EDR-002: alternative parameters (BR-LLM-026, #1169)", func() {
		It("should copy alternative workflow parameters", func() {
			inv := &katypes.InvestigationResult{
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{
						WorkflowID: "wf-alt",
						Parameters: map[string]interface{}{"ALT_KEY": "alt_val"},
					},
				},
			}

			dr := mcptools.ExtractDiscoveryResult(inv)
			Expect(dr).NotTo(BeNil(), "discovery extraction must return a non-nil result for downstream MCP serialization")
			Expect(dr.Alternatives).NotTo(BeEmpty(), "alternatives from LLM output must appear in discovery")
			Expect(dr.Alternatives[0].Parameters).To(HaveKeyWithValue("ALT_KEY", "alt_val"),
				"each alternative must retain its own parameter map for fair operator comparison")
		})
	})

	Describe("UT-KA-EDR-003: nil alternative parameters (BR-LLM-026, #1169)", func() {
		It("should propagate nil parameters on alternatives without parameters", func() {
			inv := &katypes.InvestigationResult{
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{WorkflowID: "wf-alt-no-params"},
				},
			}

			dr := mcptools.ExtractDiscoveryResult(inv)
			Expect(dr.Alternatives).To(HaveLen(1), "single alternative must be preserved without inventing fields")
			Expect(dr.Alternatives[0].Parameters).To(BeNil(),
				"absent alternative parameters must stay nil so callers can distinguish missing from empty maps")
		})
	})

	Describe("UT-KA-EDR-004: nil investigation input (BR-LLM-026, #1169)", func() {
		It("should return empty result for nil input", func() {
			dr := mcptools.ExtractDiscoveryResult(nil)
			Expect(dr).NotTo(BeNil(), "nil-safe extraction avoids panics in interactive session handlers")
			Expect(dr.Recommended).To(BeNil(), "without an investigation there must be no recommended workflow")
			Expect(dr.Alternatives).To(BeEmpty(), "without an investigation there must be no alternative list")
		})
	})

	Describe("UT-KA-EDR-005: alternatives only (BR-LLM-026, #1169)", func() {
		It("should populate alternatives without recommended when WorkflowID is empty", func() {
			inv := &katypes.InvestigationResult{
				WorkflowID: "",
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{
						WorkflowID: "wf-alt-only",
						Parameters: map[string]interface{}{"ONLY": "one"},
					},
				},
			}

			dr := mcptools.ExtractDiscoveryResult(inv)
			Expect(dr.Recommended).To(BeNil(), "no primary workflow must be fabricated when WorkflowID is absent")
			Expect(dr.Alternatives).To(HaveLen(1), "alternatives-only Phase 3 output must still populate discovery")
			Expect(dr.Alternatives[0].Parameters).To(HaveKeyWithValue("ONLY", "one"),
				"parameters on alternatives must propagate even when no recommended row exists")
		})
	})

	Describe("UT-KA-EDR-006: empty recommended parameter map (BR-LLM-026, #1169)", func() {
		It("should propagate empty parameter map on recommended", func() {
			inv := &katypes.InvestigationResult{
				WorkflowID: "wf-rec",
				Parameters: map[string]interface{}{},
			}

			dr := mcptools.ExtractDiscoveryResult(inv)
			Expect(dr.Recommended).NotTo(BeNil(), "recommended workflow must exist when WorkflowID is set")
			Expect(dr.Recommended.Parameters).NotTo(BeNil(),
				"explicit empty maps from the LLM must not collapse to nil and hide the parameter layer")
			Expect(dr.Recommended.Parameters).To(BeEmpty(),
				"non-nil parameter map with zero entries must round-trip for schema-stable clients")
		})
	})
})

var _ = Describe("kubernaut_select_workflow — per-workflow parameter hand-off (BR-INTERACTIVE, #1169)", func() {
	Describe("UT-KA-SW-AC-002: select_workflow propagates per-workflow parameters to completer (BR-INTERACTIVE, #1169)", func() {
		It("should deliver alternative workflow parameters to the interactive session completer", func() {
			altID := "wf-alt-params"
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:      altID,
					WorkflowName:    "alt-with-params",
					ExecutionEngine: "argo",
					ExecutionBundle: "oci://alt:v1",
					Version:         "v1.0",
				},
			}
			completer := &mockHTTPCompleter{foundID: "http-ac-002", found: true}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-ac-002",
					CorrelationID: "rr-ac-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "needs rollout tuning"},
					DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
						Recommended: &mcpinternal.DiscoveredWorkflow{
							WorkflowID: "wf-recommended",
							Parameters: map[string]interface{}{"REC_KEY": "rec_val"},
						},
						Alternatives: []mcpinternal.DiscoveredWorkflow{
							{
								WorkflowID: altID,
								Parameters: map[string]interface{}{"ALT_KEY": "alt_val"},
							},
						},
					},
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions,
				mcptools.WithHTTPSessionCompleter(completer),
			)
			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-ac-002",
				WorkflowID: altID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred(),
				"select_workflow must succeed when the user picks a discovery-listed alternative")

			// Session completion is deferred to a goroutine.
			Eventually(func(g Gomega) {
				_, result := completer.getCompleted()
				g.Expect(result).NotTo(BeNil(),
					"the HTTP completer must receive the merged investigation result for session completion")
				g.Expect(result.Parameters).To(HaveKey("ALT_KEY"),
					"the completer must forward parameters for the workflow the user actually selected")
				g.Expect(result.Parameters["ALT_KEY"]).To(Equal("alt_val"),
					"downstream automation must see concrete values from the alternative workflow")
				g.Expect(result.Parameters).NotTo(HaveKey("REC_KEY"),
					"recommended-workflow parameters must not leak after an explicit alternative choice")
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())
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
