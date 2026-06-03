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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("#1351 KA Session Lifecycle — handleCancel HTTP bridge", func() {

	Describe("UT-KA-1351-001: handleCancel calls httpCompleter.CompleteUserDriving", func() {
		It("should transition the HTTP session to completed after MCP cancel", func() {
			completer := &cancelLifecycleHTTPCompleter{
				foundID: "http-sess-001",
				found:   true,
			}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-sess-001",
					CorrelationID: "rr-cancel-001",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewInvestigateTool(
				sessionMgr,
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				mcptools.NopAutonomousManager{},
				mcptools.WithHTTPCompleter(completer),
			)

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-cancel-001",
				Action: mcptools.ActionCancel,
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("cancelled"))

			completedID, completedResult := completer.getCompleted()
			Expect(completedID).To(Equal("http-sess-001"),
				"handleCancel must call CompleteUserDriving on the HTTP session (KA-CRIT-1)")
			_ = completedResult
		})
	})

	Describe("UT-KA-1351-002: handleCancel uses ForceComplete as fallback", func() {
		It("should force-complete the HTTP session when FindUserDriving returns not-found", func() {
			completer := &cancelLifecycleHTTPCompleter{
				found: false, // FindUserDrivingByRemediationID returns not-found
			}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-sess-002",
					CorrelationID: "rr-cancel-002",
					ActingUser:    mcpinternal.UserInfo{Username: "bob"},
				},
			}

			tool := mcptools.NewInvestigateTool(
				sessionMgr,
				&mockInvestigatorRunner{},
				&mockContextReconstructor{},
				mcptools.NopAutonomousManager{},
				mcptools.WithHTTPCompleter(completer),
			)

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-cancel-002",
				Action: mcptools.ActionCancel,
			}, mcpinternal.UserInfo{Username: "bob"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("cancelled"))

			Expect(completer.forceCompleteCalled()).To(BeTrue(),
				"handleCancel must call ForceCompleteByRemediationID as fallback (KA-CRIT-1)")
		})
	})

	Describe("UT-KA-1351-005: complete_no_action calls StopTracking", func() {
		It("should stop the timeout tracker on completion", func() {
			tracker := &mockTimeoutTracker{}
			completer := &cancelLifecycleHTTPCompleter{
				foundID: "http-sess-005",
				found:   true,
			}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-sess-005",
					CorrelationID: "rr-cna-001",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewCompleteNoActionTool(
				sessionMgr,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
				mcptools.WithCompleteNoActionTimeoutTracker(tracker),
			)

			out, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID:   "rr-cna-001",
				Reason: "not actionable",
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed_no_action"))
			Expect(tracker.stoppedSessionID()).To(Equal("mcp-sess-005"),
				"complete_no_action must call StopTracking (KA-MED-4)")
		})
	})
})

var _ = Describe("UT-KA-1351-003: TimeoutManager onExpire resolves HTTP session (KA-CRIT-2)", func() {
	It("should complete the HTTP session when called with a valid rrID", func() {
		completer := &cancelLifecycleHTTPCompleter{
			foundID: "http-sess-timeout",
			found:   true,
		}
		logger := logr.Discard()

		mcptools.CompleteHTTPSession(completer, "rr-timeout-001", nil, logger, "inactivity_timeout")

		completedID, _ := completer.getCompleted()
		Expect(completedID).To(Equal("http-sess-timeout"),
			"CompleteHTTPSession must call CompleteUserDriving for timeout path (KA-CRIT-2)")
	})
})

var _ = Describe("UT-KA-1351-004: SessionClosedHandler disconnect resolves HTTP session (KA-CRIT-2)", func() {
	It("should complete the HTTP session via ForceComplete when no user_driving match", func() {
		completer := &cancelLifecycleHTTPCompleter{
			found: false, // No active user_driving session
		}
		logger := logr.Discard()

		mcptools.CompleteHTTPSession(completer, "rr-disconnect-001", nil, logger, "disconnect")

		Expect(completer.forceCompleteCalled()).To(BeTrue(),
			"CompleteHTTPSession must fall back to ForceComplete on disconnect (KA-CRIT-2)")
	})
})

// cancelLifecycleHTTPCompleter tracks calls for lifecycle test assertions.
type cancelLifecycleHTTPCompleter struct {
	mu                    sync.Mutex
	foundID               string
	found                 bool
	completedID           string
	completedResult       *katypes.InvestigationResult
	forceCompleteWasCalled bool
}

func (c *cancelLifecycleHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.foundID, c.found
}

func (c *cancelLifecycleHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedID = id
	c.completedResult = result
	return nil
}

func (c *cancelLifecycleHTTPCompleter) ForceCompleteByRemediationID(_ string, result *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forceCompleteWasCalled = true
	c.completedResult = result
	return nil
}

func (c *cancelLifecycleHTTPCompleter) getCompleted() (string, *katypes.InvestigationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.completedID, c.completedResult
}

func (c *cancelLifecycleHTTPCompleter) forceCompleteCalled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.forceCompleteWasCalled
}

// mockTimeoutTracker tracks StopTracking calls.
type mockTimeoutTracker struct {
	mu        sync.Mutex
	sessionID string
}

func (t *mockTimeoutTracker) StartTracking(_ string, _ func(string)) {}

func (t *mockTimeoutTracker) StopTracking(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessionID = sessionID
}

func (t *mockTimeoutTracker) ResetInactivity(_ string) {}

func (t *mockTimeoutTracker) stoppedSessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

