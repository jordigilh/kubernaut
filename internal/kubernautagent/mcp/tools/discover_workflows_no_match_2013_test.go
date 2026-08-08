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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// orderRecorder captures the relative ordering of the async auto-close steps
// so tests can assert the #1438 ordering invariant: session_ended must be
// emitted BEFORE the HTTP session is completed, so EventLogBridge can forward
// it to AF before the channel closes.
type orderRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *orderRecorder) record(step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

func (r *orderRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.steps))
	copy(out, r.steps)
	return out
}

type emitCall struct {
	RRID, Reason string
}

// mockAutoMgrDW2013 implements AutonomousSessionManager for #2013 tests. It
// embeds NopAutonomousManager for the methods this scenario doesn't exercise,
// and overrides GetLatestRCAResultByRemediationID (to serve a stored RCA
// without touching the audit-reconstruction fallback) and EmitSessionEndedByRR
// (to record calls and their ordering).
type mockAutoMgrDW2013 struct {
	mcptools.NopAutonomousManager
	mu        sync.Mutex
	rcaResult *katypes.InvestigationResult
	emitCalls []emitCall
	order     *orderRecorder
}

func (m *mockAutoMgrDW2013) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rcaResult != nil {
		return m.rcaResult, true
	}
	return nil, false
}

func (m *mockAutoMgrDW2013) EmitSessionEndedByRR(rrID, reason string) {
	m.mu.Lock()
	m.emitCalls = append(m.emitCalls, emitCall{RRID: rrID, Reason: reason})
	m.mu.Unlock()
	if m.order != nil {
		m.order.record("emit_session_ended")
	}
}

func (m *mockAutoMgrDW2013) getEmitCalls() []emitCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]emitCall, len(m.emitCalls))
	copy(out, m.emitCalls)
	return out
}

// orderedHTTPCompleter is a minimal HTTPSessionCompleter for #2013 ordering
// assertions — distinct from mockHTTPCompleter (select_workflow_test.go)
// because this scenario specifically needs to record the completion step
// into the shared orderRecorder, not just capture the final result value.
type orderedHTTPCompleter struct {
	mu          sync.Mutex
	completedID string
	completed   *katypes.InvestigationResult
	found       bool
	foundID     string
	order       *orderRecorder
}

func (c *orderedHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.foundID, c.found
}

func (c *orderedHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	c.mu.Lock()
	c.completedID = id
	c.completed = result
	c.mu.Unlock()
	if c.order != nil {
		c.order.record("complete_user_driving")
	}
	return nil
}

func (c *orderedHTTPCompleter) ForceCompleteByRemediationID(_ string, _ *katypes.InvestigationResult) error {
	return nil
}

func (c *orderedHTTPCompleter) getCompleted() (string, *katypes.InvestigationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.completedID, c.completed
}

var _ = Describe("#2013: discover_workflows auto-closes session on no_matching_workflows (main port of #2003)", func() {

	Describe("UT-KA-2013-001: no_matching_workflows emits session_ended before completing (order per #1438)", func() {
		It("should call EmitSessionEndedByRR before CompleteUserDriving and release the MCP lease", func() {
			order := &orderRecorder{}
			completer := &orderedHTTPCompleter{foundID: "http-sess-2013-001", found: true, order: order}
			autoMgr := &mockAutoMgrDW2013{
				rcaResult: &katypes.InvestigationResult{RCASummary: "OOM crash", Confidence: 0.4},
				order:     order,
			}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:  "sess-2013-001",
					ActingUser: mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{
				workflowDiscoveryResult: &katypes.InvestigationResult{
					RCASummary:        "OOM crash",
					HumanReviewNeeded: true,
					HumanReviewReason: "no_matching_workflows",
				},
			}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{}),
			)

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2013-001",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"),
				"the discovery response body is unchanged; auto-close is a side effect")

			// Session completion and lease release are deferred to a goroutine so
			// the MCP response reaches the caller before transport teardown.
			Eventually(func(g Gomega) {
				calls := autoMgr.getEmitCalls()
				g.Expect(calls).To(HaveLen(1))
				g.Expect(calls[0].RRID).To(Equal("rr-2013-001"))
				g.Expect(calls[0].Reason).To(Equal("no_matching_workflows"))
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				id, result := completer.getCompleted()
				g.Expect(id).To(Equal("http-sess-2013-001"))
				g.Expect(result).NotTo(BeNil())
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				id, reason := sessionMgr.getReleased()
				g.Expect(id).To(Equal("sess-2013-001"))
				g.Expect(reason).To(Equal("no_matching_workflows"))
			}).WithTimeout(2 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())

			Expect(order.snapshot()).To(Equal([]string{"emit_session_ended", "complete_user_driving"}),
				"#1438: session_ended must be emitted BEFORE the HTTP session completes, "+
					"so EventLogBridge can forward it to AF before the channel closes (otherwise "+
					"AF's WatchTerminalEvents, which has no timer-based safety net, never updates "+
					"the console phase off 'Investigating')")
		})
	})

	Describe("UT-KA-2013-002: legitimate discovery results do not auto-close (regression guard)", func() {
		It("should not emit session_ended or release the session when a workflow was recommended", func() {
			autoMgr := &mockAutoMgrDW2013{
				rcaResult: &katypes.InvestigationResult{RCASummary: "OOM crash"},
			}
			completer := &orderedHTTPCompleter{foundID: "http-sess-2013-002", found: true}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:  "sess-2013-002",
					ActingUser: mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{
				workflowDiscoveryResult: &katypes.InvestigationResult{
					RCASummary: "OOM crash",
					WorkflowID: "wf-recommended-2013",
					Confidence: 0.9,
				},
			}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "wf-recommended-2013", WorkflowName: "Mock Workflow"},
				}),
			)

			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2013-002",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			Consistently(func(g Gomega) {
				g.Expect(autoMgr.getEmitCalls()).To(BeEmpty(),
					"a legitimate recommendation must not trigger the no_matching_workflows auto-close")
				id, _ := sessionMgr.getReleased()
				g.Expect(id).To(BeEmpty(), "session must remain active for the user to select a workflow")
			}).WithTimeout(300 * time.Millisecond).WithPolling(50 * time.Millisecond).Should(Succeed())
		})
	})
})
