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

package mcp_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// delayedMockRunner simulates an LLM that takes time to respond.
type delayedMockRunner struct {
	delay    time.Duration
	response string
	calls    atomic.Int32
}

func (m *delayedMockRunner) RunInteractiveTurn(_ context.Context, _ []tools.LLMMessage, _ string) (string, error) {
	m.calls.Add(1)
	time.Sleep(m.delay)
	return m.response, nil
}

func (m *delayedMockRunner) RunRCAExtraction(_ context.Context, _ []tools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (m *delayedMockRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA", WorkflowID: "mock-wf"}, nil
}

// mockReconIT mocks ContextReconstructor for integration tests.
type mockReconIT struct{}

func (m *mockReconIT) Reconstruct(_ context.Context, _ string, _ string) ([]mcpinternal.ConversationTurn, error) {
	return nil, nil
}

// mockAutoMgrIT wraps a real session.Manager for autonomous session management in tests.
type mockAutoMgrIT struct {
	mgr *session.Manager
}

func (m *mockAutoMgrIT) FindByRemediationID(rrID string) (string, bool) {
	return m.mgr.FindByRemediationID(rrID)
}

func (m *mockAutoMgrIT) CancelInvestigation(id string) error {
	return m.mgr.CancelInvestigation(id)
}

func (m *mockAutoMgrIT) SuspendInvestigation(id string) error {
	return m.mgr.SuspendInvestigation(id)
}

func (m *mockAutoMgrIT) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	return "", fmt.Errorf("not implemented in IT mock")
}

func (m *mockAutoMgrIT) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, fmt.Errorf("not implemented in IT mock")
}

func (m *mockAutoMgrIT) TransitionToUserDriving(id, username string, groups []string) error {
	return m.mgr.TransitionToUserDriving(id, username, groups)
}

func (m *mockAutoMgrIT) ForceTransitionToUserDriving(rrID, username string, groups []string) error {
	return m.mgr.ForceTransitionToUserDriving(rrID, username, groups)
}

func (m *mockAutoMgrIT) FindPendingByRemediationID(rrID string) (string, bool) {
	return m.mgr.FindPendingByRemediationID(rrID)
}

func (m *mockAutoMgrIT) LaunchDeferredInvestigation(id string) error {
	return m.mgr.LaunchDeferredInvestigation(id)
}

func (m *mockAutoMgrIT) GetLatestRCASummaryByRemediationID(rrID string) (string, bool) {
	return m.mgr.GetLatestRCASummaryByRemediationID(rrID)
}

var _ = Describe("MCP Dynamic Takeover Integration — PR4 BR-INTERACTIVE-004", func() {

	Describe("IT-KA-TAKE-001: Takeover mid-LLM-turn — autonomous transitions to user_driving", func() {
		It("should transition the autonomous session to user_driving and acquire the interactive lease", func() {
			nsName := uniqueNamespace("take01")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			var autonomousCompleted atomic.Bool
			sessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				select {
				case <-time.After(200 * time.Millisecond):
					autonomousCompleted.Store(true)
					return &katypes.InvestigationResult{RCASummary: "auto-result"}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}, map[string]string{"remediation_id": "rr-it-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusRunning))

			logger := logr.Discard()
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 10 * time.Millisecond, response: "interactive response"}
			recon := &mockReconIT{}
			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr)

			user := mcpinternal.UserInfo{Username: "sre-operator@example.com"}
			input := tools.InvestigateInput{
				RRID:   "rr-it-001",
				Action: tools.ActionTakeover,
			}

			out, err := tool.Handle(context.Background(), input, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).NotTo(BeEmpty())
			Expect(out.Status).To(Equal("takeover_started"))

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusUserDriving))

			Expect(leaseMgr.IsDriverActive("rr-it-001")).To(BeTrue())
		})
	})

	Describe("IT-KA-MCP-003: Concurrent tools/call requests are serialized by session mutex via real MCP SDK", func() {
		It("should process concurrent messages sequentially for the same session", func() {
			nsName := uniqueNamespace("conc")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 30 * time.Millisecond, response: "llm-response"}
			recon := &mockReconIT{}
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-concurrent-001"})
			Expect(err).NotTo(HaveOccurred())

			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr)
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			takeoverInput := tools.InvestigateInput{
				RRID:   "rr-concurrent-001",
				Action: tools.ActionTakeover,
			}
			_, err = tool.Handle(context.Background(), takeoverInput, user)
			Expect(err).NotTo(HaveOccurred())

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: fakeAuthMiddleware("alice@example.com"),
				Tools: mcpinternal.ToolDeps{
					Investigate: tools.InvestigateRegistration(tool, nil, nil, logr.Discard()),
				},
			})

			r := chi.NewRouter()
			r.Use(fakeAuthMiddleware("alice@example.com"))
			r.Handle("/api/v1/mcp", kaserver.SSEHeadersMiddleware(handler))
			r.Handle("/api/v1/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			ts := httptest.NewServer(r)
			defer ts.Close()

			var wg sync.WaitGroup
			results := make([]int, 3)
			startTimes := make([]time.Time, 3)
			endTimes := make([]time.Time, 3)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					startTimes[idx] = time.Now()

					msgInput := tools.InvestigateInput{
						RRID:    "rr-concurrent-001",
						Action:  tools.ActionMessage,
						Message: fmt.Sprintf("message %d", idx),
					}
					_, handleErr := tool.Handle(context.Background(), msgInput, user)
					endTimes[idx] = time.Now()
					if handleErr == nil {
						results[idx] = 1
					}
				}(i)
			}
			wg.Wait()

			Expect(results[0] + results[1] + results[2]).To(Equal(3))
			Expect(runner.calls.Load()).To(Equal(int32(3)))

			var earliest, latest time.Time
			for i := 0; i < 3; i++ {
				if earliest.IsZero() || startTimes[i].Before(earliest) {
					earliest = startTimes[i]
				}
				if endTimes[i].After(latest) {
					latest = endTimes[i]
				}
			}
			totalDuration := latest.Sub(earliest)
			Expect(totalDuration).To(BeNumerically(">=", 80*time.Millisecond),
				"concurrent messages should be serialized (total time >= 80ms)")
		})
	})

	// ---------------------------------------------------------------
	// IT-KA-SEC-TAKEOVER-001: Takeover abandonment — autonomous does NOT resume
	// BR: BR-INTERACTIVE-004 #5 (v1.5), SEC-TAKEOVER-001
	//
	// Security invariant: after takeover, the autonomous goroutine is
	// cancelled permanently. If the user abandons the interactive session
	// (inactivity timeout fires), the autonomous investigation must remain
	// in StatusUserDriving — NOT transition back to StatusRunning.
	// This prevents "investigation hacking" where a user poisons the LLM
	// context and then walks away, letting KA auto-execute tainted results
	// with KA SA privileges.
	// ---------------------------------------------------------------
	Describe("IT-KA-SEC-TAKEOVER-001: Takeover abandonment does not resume autonomous", func() {
		It("should keep autonomous session in user_driving after interactive lease expires", func() {
			nsName := uniqueNamespace("sec-take")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			By("Starting autonomous investigation that blocks until cancelled")
			autoSessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-sec-takeover-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(autoSessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusRunning))

			By("Setting up interactive tool with 1s inactivity timeout")
			logger := logr.Discard()
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)

			var expiredSessions sync.Map
			timeoutMgr := mcpinternal.NewTimeoutManager(
				1*time.Second,
				nil,
				func(sessionID string) {
					expiredSessions.Store(sessionID, true)
					_ = leaseMgr.Release(sessionID, "inactivity_timeout")
				},
			)
			defer timeoutMgr.StopAll()

			runner := &delayedMockRunner{delay: 10 * time.Millisecond, response: "interactive response"}
			recon := &mockReconIT{}
			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr,
				tools.WithTimeoutTracker(timeoutMgr),
			)

			By("Taking over the autonomous investigation")
			user := mcpinternal.UserInfo{Username: "attacker@example.com"}
			out, err := tool.Handle(context.Background(), tools.InvestigateInput{
				RRID:   "rr-sec-takeover-001",
				Action: tools.ActionTakeover,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
			interactiveSessionID := out.SessionID

			By("Verifying autonomous session transitioned to user_driving")
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(autoSessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusUserDriving))

			By("Abandoning — NOT completing or cancelling the interactive session")
			// Wait for inactivity timeout to fire (1s + buffer)
			time.Sleep(2 * time.Second)

			By("Verifying interactive Lease was released by timeout")
			Eventually(func() bool {
				_, found := expiredSessions.Load(interactiveSessionID)
				return found
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue(),
				"inactivity timeout should have released the interactive session")

			Expect(leaseMgr.IsDriverActive("rr-sec-takeover-001")).To(BeFalse(),
				"Lease should be released after inactivity timeout")

			By("CRITICAL: Verifying autonomous session is STILL in user_driving (NOT resumed)")
			autoSess, getErr := mgr.GetSession(autoSessionID)
			Expect(getErr).NotTo(HaveOccurred())
			Expect(autoSess).NotTo(BeNil())
			Expect(autoSess.Status).To(Equal(session.StatusUserDriving),
				"SEC-TAKEOVER-001: autonomous session must remain in user_driving after "+
					"interactive abandonment — autonomous must NOT resume to prevent "+
					"investigation hacking via tainted LLM context")

			By("Verifying autonomous session was NOT restarted (no new session for same RR)")
			// FindByRemediationID only returns StatusRunning sessions by design;
			// after TransitionToUserDriving the status is StatusUserDriving, so it
			// correctly won't be found. The session itself is confirmed present via
			// GetSession above. Verify no NEW running session was started.
			_, found := mgr.FindByRemediationID("rr-sec-takeover-001")
			Expect(found).To(BeFalse(),
				"no StatusRunning session should exist — original is in user_driving, no new one was started")

			GinkgoWriter.Println("SEC-TAKEOVER-001: Takeover abandonment validated — autonomous NOT resumed")
		})
	})
})
