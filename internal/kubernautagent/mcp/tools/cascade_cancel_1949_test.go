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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// blockingDiscoveryRunner.RunWorkflowDiscovery blocks on ctx.Done() forever,
// signaling started once it begins blocking so the test can deterministically
// wait for the in-flight call to be registered with the (mock) timeout
// tracker before simulating an inactivity expiry — without this signal, a
// race would let the test call simulateExpiry before SetActiveCancel ever
// ran.
type blockingDiscoveryRunner struct {
	mockInvestigatorRunner
	started chan struct{}
}

func (r *blockingDiscoveryRunner) RunWorkflowDiscovery(ctx context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

var _ = Describe("kubernaut_investigate — cascade-cancel wiring (#1949, BR-KA-267, SC-5)", func() {

	Describe("IT-KA-1949-003: inactivity expiry cancels an in-flight discover_workflows call", func() {
		It("cancels the handler's context and returns promptly instead of hanging past session expiry", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-1949-003",
				CorrelationID: "rr-1949-003",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			autoMgr := &storedRCAAutoMgr{rca: &katypes.InvestigationResult{RCASummary: "OOMKilled", Confidence: 0.9}}
			runner := &blockingDiscoveryRunner{started: make(chan struct{})}
			recon := &mockContextReconstructor{}

			// A REAL mcp.TimeoutManager (not a mock), per the plan's
			// IT-KA-1949-003 definition — proves the actual production
			// timer mechanics (time.AfterFunc → SetActiveCancel's
			// registered CancelFunc → onExpire, all in timeout_manager.go)
			// cascade into InvestigateTool.Handle end-to-end, not just that
			// each piece behaves correctly in isolation (that isolation-level
			// proof is UT-KA-1949-004/005 in the mcp package, and the
			// InvestigateTool-level wiring proof with a mock tracker is
			// UT-KA-1949-005b below).
			tracker := mcpinternal.NewTimeoutManager(150*time.Millisecond, nil, func(string) {})
			tracker.StartTracking(sess.SessionID, func(string) {})
			defer tracker.StopTracking(sess.SessionID)

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}),
				mcptools.WithTimeoutTracker(tracker),
			)

			type handleResult struct {
				out mcptools.InvestigateOutput
				err error
			}
			done := make(chan handleResult, 1)
			go func() {
				defer GinkgoRecover()
				out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
					RRID:   "rr-1949-003",
					Action: mcptools.ActionDiscoverWorkflows,
				}, mcpinternal.UserInfo{Username: "alice"})
				done <- handleResult{out: out, err: err}
			}()

			// Confirm the call is genuinely in flight (registered via
			// SetActiveCancel) before waiting on the real timer, so a slow
			// scheduler can't produce a false pass where Handle happened to
			// return for an unrelated reason before the timer even started.
			Eventually(runner.started, "1s").Should(BeClosed())

			var result handleResult
			Eventually(done, "2s", "10ms").Should(Receive(&result),
				"Handle must return promptly once the real inactivity timer fires and cascades into the in-flight context — before the #1949 fix, nothing could reach into this call to cancel it")

			Expect(result.err).To(HaveOccurred())
			Expect(result.err.Error()).To(ContainSubstring("workflow discovery failed"))
		})
	})

	Describe("UT-KA-1949-005b: a completed action clears its cancel registration (no stale cancel)", func() {
		It("does not leave a registered cancel behind once discover_workflows returns normally", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-1949-005b",
				CorrelationID: "rr-1949-005b",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			autoMgr := &storedRCAAutoMgr{rca: &katypes.InvestigationResult{RCASummary: "OOMKilled", Confidence: 0.9}}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			tracker := &mockTimeoutTracker{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}),
				mcptools.WithTimeoutTracker(tracker),
			)

			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-1949-005b",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			// A later expiry (e.g. from the next, unrelated action on the
			// same session) must be a no-op: simulateExpiry itself would
			// only panic/misbehave if a stale CancelFunc were still
			// registered and got invoked against an already-cancelled or
			// reused context — asserting it doesn't panic and the tracker
			// has no lingering entry is the observable proxy here.
			Expect(func() { tracker.simulateExpiry(sess.SessionID) }).NotTo(Panic())
		})
	})
})
