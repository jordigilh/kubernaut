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
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ctxCapturingDiscoveryRunner captures the context passed to RunWorkflowDiscovery
// for verifying session context propagation (Bug A #1384).
type ctxCapturingDiscoveryRunner struct {
	mu                  sync.Mutex
	capturedDiscoveryCtx context.Context
	rcaResult           *katypes.InvestigationResult
	discoveryResult     *katypes.InvestigationResult
}

func (r *ctxCapturingDiscoveryRunner) RunInteractiveTurn(ctx context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	return "interactive response", nil
}

func (r *ctxCapturingDiscoveryRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	if r.rcaResult != nil {
		return r.rcaResult, nil
	}
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (r *ctxCapturingDiscoveryRunner) RunWorkflowDiscovery(ctx context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	r.mu.Lock()
	r.capturedDiscoveryCtx = ctx
	r.mu.Unlock()

	if r.discoveryResult != nil {
		return r.discoveryResult, nil
	}
	return &katypes.InvestigationResult{
		RCASummary: "mock RCA",
		WorkflowID: "mock-workflow-001",
		Confidence: 0.85,
	}, nil
}

func (r *ctxCapturingDiscoveryRunner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock"}, nil
}

func (r *ctxCapturingDiscoveryRunner) getCapturedCtx() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capturedDiscoveryCtx
}

// mockAutoMgrWithHTTPSession extends NopAutonomousManager to simulate an HTTP
// session in user_driving state with a stored RCA result. This mirrors the
// production scenario where launchInvestigation has created a session with
// a LazySink and session_id, and the session has been transitioned to
// user_driving status via takeover.
type mockAutoMgrWithHTTPSession struct {
	mcptools.NopAutonomousManager
	mu             sync.Mutex
	httpSessionID  string
	rcaResult      *katypes.InvestigationResult
	subscribeCh    <-chan session.InvestigationEvent
	subscribeErr   error
	subscribedID   string
}

func (m *mockAutoMgrWithHTTPSession) FindUserDrivingByRemediationID(_ string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.httpSessionID == "" {
		return "", false
	}
	return m.httpSessionID, true
}

func (m *mockAutoMgrWithHTTPSession) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rcaResult == nil {
		return nil, false
	}
	return m.rcaResult, true
}

func (m *mockAutoMgrWithHTTPSession) Subscribe(_ context.Context, id string) (<-chan session.InvestigationEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribedID = id
	return m.subscribeCh, m.subscribeErr
}

func (m *mockAutoMgrWithHTTPSession) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}

var _ = Describe("Fix #1384 Bug A — Session context propagation (AU-2/AU-3, BR-INTERACTIVE-010 SC-2)", func() {

	Describe("UT-KA-1384-A01: Session context propagated to workflow_discovery (streaming path)", func() {
		It("should enrich ctx with session_id from the HTTP investigation session", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-a01",
				CorrelationID: "rr-a01",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &ctxCapturingDiscoveryRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					Confidence: 0.9,
				},
			}
			recon := &mockContextReconstructor{}
			resolver := &mockSignalResolver{}

			autoMgr := &mockAutoMgrWithHTTPSession{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					Confidence: 0.9,
				},
			}

			completer := &mockHTTPCompleter{
				foundID: "http-sess-a01",
				found:   true,
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-a01",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			capturedCtx := runner.getCapturedCtx()
			Expect(capturedCtx).NotTo(BeNil(), "RunWorkflowDiscovery should have been called")

			sessionID := session.SessionIDFromContext(capturedCtx)
			Expect(sessionID).NotTo(BeEmpty(),
				"session_id MUST be propagated to workflow_discovery ctx — AU-2/AU-3 requires traceable audit events")
			Expect(sessionID).To(Equal("http-sess-a01"),
				"session_id should match the HTTP investigation session, not the MCP lease ID")
		})
	})

	Describe("UT-KA-1384-A02: Without HTTP session, degrades gracefully (no crash)", func() {
		It("should still succeed with empty session context when no HTTP session exists", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-a02",
				CorrelationID: "rr-a02",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &ctxCapturingDiscoveryRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "pod crash"},
				{Role: "assistant", Content: "investigating"},
			}}
			resolver := &mockSignalResolver{}

			autoMgr := &mockAutoMgrWithHTTPSession{
				httpSessionID: "",
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-a02",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))
		})
	})

	Describe("UT-KA-1384-A03: Investigation events reach sink during workflow_discovery", func() {
		It("should propagate LazySink so events can be streamed to subscriber", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-a03",
				CorrelationID: "rr-a03",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &ctxCapturingDiscoveryRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					Confidence: 0.9,
				},
			}
			recon := &mockContextReconstructor{}
			resolver := &mockSignalResolver{}

			completer := &mockHTTPCompleter{
				foundID: "http-sess-a03",
				found:   true,
			}

			autoMgr := &mockAutoMgrWithHTTPSession{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					Confidence: 0.9,
				},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-a03",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			capturedCtx := runner.getCapturedCtx()
			Expect(capturedCtx).NotTo(BeNil())

			sessionID := session.SessionIDFromContext(capturedCtx)
			Expect(sessionID).To(Equal("http-sess-a03"),
				"session_id must be propagated for event correlation")
		})
	})

	Describe("UT-KA-1384-A04: Missing subscriber does not block or crash", func() {
		It("should not panic when no subscriber exists for the HTTP session", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-a04",
				CorrelationID: "rr-a04",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &ctxCapturingDiscoveryRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "pod crash"},
				{Role: "assistant", Content: "investigating"},
			}}
			resolver := &mockSignalResolver{}

			// No HTTP session → no sink
			autoMgr := &mockAutoMgrWithHTTPSession{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			Expect(func() {
				_, _ = tool.Handle(context.Background(), mcptools.InvestigateInput{
					RRID:   "rr-a04",
					Action: mcptools.ActionDiscoverWorkflows,
				}, mcpinternal.UserInfo{Username: "alice"})
			}).NotTo(Panic(), "discover_workflows must not panic when no subscriber exists")
		})
	})

	Describe("UT-KA-1384-A05: buildFinalResult merges RCA + discovery into single result", func() {
		It("should produce InvestigationResult with both RCASummary and WorkflowID", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "OOMKilled due to memory leak in api-server",
				Confidence: 0.92,
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Pod",
					Name:      "api-server",
					Namespace: "prod",
				},
			}

			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "restart-pod-v2",
					Confidence: 0.88,
					Rationale:  "Pod restart resolves transient OOM",
					Parameters: map[string]interface{}{
						"pod_name":  "api-server",
						"namespace": "prod",
					},
				},
				FullResult: &katypes.InvestigationResult{
					Confidence:           0.88,
					InvestigationOutcome: "actionable",
				},
			}

			workflow := &mcptools.CatalogWorkflow{
				WorkflowID:      "restart-pod-v2",
				ExecutionEngine: "argo",
				ExecutionBundle: "ghcr.io/kubernaut/workflows/restart-pod:v2",
				Version:         "2.0.0",
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)

			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("OOMKilled due to memory leak in api-server"),
				"RCA summary from Phase 2 must be preserved")
			Expect(result.WorkflowID).To(Equal("restart-pod-v2"),
				"WorkflowID from user selection must be set")
			Expect(result.ExecutionEngine).To(Equal("argo"))
			Expect(result.RemediationTarget.Kind).To(Equal("Pod"),
				"RemediationTarget from RCA should be preserved when discovery doesn't override")
			Expect(result.Confidence).To(BeNumerically("==", 0.88),
				"Confidence should use Phase 3 discovery confidence")
		})
	})
})

var _ = Describe("Fix #1384 — Wiring Chain IT (discover -> select -> HTTP complete)", func() {

	Describe("IT-KA-1384-004: End-to-end discover->select->HTTP complete (CP-10, BR-INTERACTIVE-010)", func() {
		It("should propagate session context through discovery and deliver result to AA via HTTP complete", func() {
			rrID := "rr-it-004"
			wfID := "restart-pod-v2"

			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-it004",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}

			discoveryRunner := &ctxCapturingDiscoveryRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on api-server pod",
					Confidence: 0.9,
					RemediationTarget: katypes.RemediationTarget{
						Kind: "Pod", Name: "api-server", Namespace: "prod",
					},
				},
				discoveryResult: &katypes.InvestigationResult{
					RCASummary: "OOM on api-server pod",
					WorkflowID: wfID,
					Confidence: 0.85,
				},
			}

			completer := &mockHTTPCompleter{
				foundID: "http-sess-it004",
				found:   true,
			}

			autoMgr := &mockAutoMgrWithHTTPSession{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on api-server pod",
					Confidence: 0.9,
					RemediationTarget: katypes.RemediationTarget{
						Kind: "Pod", Name: "api-server", Namespace: "prod",
					},
				},
			}

			resolver := &mockSignalResolver{
				signal: &katypes.SignalContext{IncidentID: "inc-it004", Severity: "critical"},
			}

			investTool := mcptools.NewInvestigateTool(sessionMgr, discoveryRunner, &mockContextReconstructor{}, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			By("Step 1: discover_workflows with session context enrichment")
			discoverOut, err := investTool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(discoverOut.Status).To(Equal("workflows_discovered"))

			capturedCtx := discoveryRunner.getCapturedCtx()
			Expect(session.SessionIDFromContext(capturedCtx)).To(Equal("http-sess-it004"),
				"session_id must be propagated to workflow_discovery")

			By("Step 2: Verify session has discovery results stored")
			Expect(sess.DiscoveryResult).NotTo(BeNil())
			Expect(sess.RCAResult).NotTo(BeNil())

			By("Step 3: select_workflow triggers CompleteHTTPSession with workflow result")
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{
					WorkflowID:      wfID,
					ExecutionEngine: "argo",
					ExecutionBundle: "ghcr.io/kubernaut/workflows/restart-pod:v2",
					Version:         "2.0.0",
				},
			}

			selectTool := mcptools.NewSelectWorkflowTool(catalog, sessionMgr,
				mcptools.WithHTTPSessionCompleter(completer),
				mcptools.WithMutexProvider(investTool))

			selectOut, err := selectTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       rrID,
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(selectOut.Status).To(Equal("workflow_selected"))
			Expect(selectOut.Workflow).NotTo(BeNil())
			Expect(selectOut.Workflow.WorkflowID).To(Equal(wfID))

			By("Step 4: Verify HTTP completer received the final result (async goroutine)")
			Eventually(func() string {
				id, _ := completer.getCompleted()
				return id
			}).WithTimeout(time.Second).WithPolling(10 * time.Millisecond).Should(Equal("http-sess-it004"),
				"IT-KA-1384-004: HTTP session must be completed via CompleteUserDriving")

			_, completedResult := completer.getCompleted()
			Expect(completedResult).NotTo(BeNil())
			Expect(completedResult.WorkflowID).To(Equal(wfID),
				"IT-KA-1384-004: result must contain the selected WorkflowID")
			Expect(completedResult.RCASummary).NotTo(BeEmpty(),
				"IT-KA-1384-004: result must preserve RCA summary from Phase 2")
		})
	})
})

var _ = Describe("Fix #1384 Bug A — Integration Tests (session wiring)", func() {

	Describe("IT-KA-1384-001: Workflow discovery audit events traceable to session (AU-2/AU-3)", func() {
		It("should propagate HTTP session_id to RunWorkflowDiscovery context", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-it001",
				CorrelationID: "rr-it-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &ctxCapturingDiscoveryRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM",
					Confidence: 0.9,
				},
			}
			recon := &mockContextReconstructor{}
			resolver := &mockSignalResolver{
				signal: &katypes.SignalContext{IncidentID: "inc-it001", Severity: "critical"},
			}

			completer := &mockHTTPCompleter{
				foundID: "http-sess-it001",
				found:   true,
			}

			autoMgr := &mockAutoMgrWithHTTPSession{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM",
					Confidence: 0.9,
				},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-it-001",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			capturedCtx := runner.getCapturedCtx()
			sessionID := session.SessionIDFromContext(capturedCtx)
			Expect(sessionID).To(Equal("http-sess-it001"),
				"IT-KA-1384-001: session_id MUST be non-empty and match HTTP session — AU-2/AU-3 audit correlation")
		})
	})

	Describe("IT-KA-1384-002: Workflow result flows to HTTP session for AA polling (BR-INTERACTIVE-010)", func() {
		It("should allow CompleteUserDriving to receive result with WorkflowID after discover+select", func() {
			completer := &mockHTTPCompleter{
				foundID: "http-sess-it002",
				found:   true,
			}

			rca := &katypes.InvestigationResult{
				RCASummary: "OOM crash",
				Confidence: 0.9,
			}
			discovery := &mcpinternal.WorkflowDiscoveryResult{
				Recommended: &mcpinternal.DiscoveredWorkflow{
					WorkflowID: "restart-pod-v2",
					Confidence: 0.85,
					Parameters: map[string]interface{}{"pod_name": "api-server"},
				},
			}
			workflow := &mcptools.CatalogWorkflow{
				WorkflowID:      "restart-pod-v2",
				ExecutionEngine: "argo",
				ExecutionBundle: "ghcr.io/kubernaut/workflows/restart-pod:v2",
			}

			result := mcptools.BuildFinalResult(rca, workflow, discovery)
			Expect(result.WorkflowID).To(Equal("restart-pod-v2"))

			mcptools.CompleteHTTPSession(completer, "rr-it-002", result, logr.Discard(), "select_workflow")

			completedID, completedResult := completer.getCompleted()
			Expect(completedID).To(Equal("http-sess-it002"),
				"CompleteUserDriving should be called with the HTTP session ID")
			Expect(completedResult).NotTo(BeNil())
			Expect(completedResult.WorkflowID).To(Equal("restart-pod-v2"),
				"IT-KA-1384-002: AA must receive result with WorkflowID from interactive discovery")
		})
	})
})
