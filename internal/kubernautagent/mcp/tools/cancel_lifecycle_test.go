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
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// jsonLogCapture captures funcr.NewJSON log records for substring assertions
// on error/warning output without depending on exact structured formatting.
type jsonLogCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *jsonLogCapture) capture(obj string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, obj)
}

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

			completedID := completer.getCompleted()
			Expect(completedID).To(Equal("http-sess-001"),
				"handleCancel must call CompleteUserDriving on the HTTP session (KA-CRIT-1)")
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

		completedID := completer.getCompleted()
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

// #1654: E2E-FP-1456-001 RCA found that a duplicate session sharing the same
// remediation_id (MCP action=start fallback session alongside AA's own
// autonomous investigation session) is left non-terminal when
// CompleteHTTPSession only calls CompleteUserDriving for the one match found
// by FindUserDrivingByRemediationID. AA may be polling the OTHER
// (non-user_driving) sibling session, which never gets force-completed
// because the previous either/or branching short-circuited on the first
// match. CompleteHTTPSession must always attempt BOTH completion paths so
// every sibling session sharing the remediation_id is resolved.
var _ = Describe("#1654: CompleteHTTPSession completes ALL sibling sessions, not just the first match", func() {
	It("should call ForceCompleteByRemediationID even when a user_driving session was found and completed", func() {
		completer := &cancelLifecycleHTTPCompleter{
			foundID: "http-sess-dual-001",
			found:   true,
		}
		logger := logr.Discard()

		mcptools.CompleteHTTPSession(completer, "rr-dual-001", nil, logger, "select_workflow")

		completedID := completer.getCompleted()
		Expect(completedID).To(Equal("http-sess-dual-001"),
			"CompleteHTTPSession must still complete the user_driving session it found")
		Expect(completer.forceCompleteCalled()).To(BeTrue(),
			"CompleteHTTPSession must ALSO call ForceCompleteByRemediationID to resolve any sibling "+
				"session sharing the same remediation_id that AA might be polling instead (#1654)")
	})

	It("should not report a critical failure when the user_driving path succeeds and force-complete finds no sibling (ErrSessionNotFound)", func() {
		completer := &cancelLifecycleHTTPCompleter{
			foundID:            "http-sess-dual-002",
			found:              true,
			forceCompleteError: session.ErrSessionNotFound,
		}
		capture := &jsonLogCapture{}
		logger := funcr.NewJSON(capture.capture, funcr.Options{})

		mcptools.CompleteHTTPSession(completer, "rr-dual-002", nil, logger, "select_workflow")

		completedID := completer.getCompleted()
		Expect(completedID).To(Equal("http-sess-dual-002"))
		for _, line := range capture.lines {
			Expect(line).NotTo(ContainSubstring("CRITICAL"),
				"a successful user_driving completion must not be reported as a critical failure "+
					"just because there was no sibling session left to force-complete (#1654)")
		}
	})
})

// cancelLifecycleHTTPCompleter tracks calls for lifecycle test assertions.
// The *katypes.InvestigationResult passed to CompleteUserDriving /
// ForceCompleteByRemediationID is accepted (required by the HTTPCompleter
// interface) but intentionally not retained: no assertion in this file
// inspects it, unlike the sibling mockHTTPCompleter in select_workflow_test.go
// whose completedResult field backs real assertions there.
type cancelLifecycleHTTPCompleter struct {
	mu                     sync.Mutex
	foundID                string
	found                  bool
	completedID            string
	forceCompleteWasCalled bool
	forceCompleteError     error
}

func (c *cancelLifecycleHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.foundID, c.found
}

func (c *cancelLifecycleHTTPCompleter) CompleteUserDriving(id string, _ *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedID = id
	return nil
}

func (c *cancelLifecycleHTTPCompleter) ForceCompleteByRemediationID(_ string, _ *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forceCompleteWasCalled = true
	if c.forceCompleteError != nil {
		return c.forceCompleteError
	}
	return nil
}

func (c *cancelLifecycleHTTPCompleter) getCompleted() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.completedID
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
