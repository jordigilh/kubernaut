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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// fallbackAutoMgr simulates a session manager where:
// - No pending session exists (FindPendingByRemediationID returns false)
// - FindByRemediationID returns configurable result (for no-session vs terminal-session tests)
// - StartInvestigation is tracked to verify fallback creation
type fallbackAutoMgr struct {
	findResult    string
	findOK        bool
	upgradeErr    error
	upgradeCalled atomic.Int32

	startCalled atomic.Int32
	startResult string
	startErr    error

	rcaSummary string
	rcaFound   bool
	rcaResult  *katypes.InvestigationResult
	rcaOK      bool

	// #1818: captures the metadata and InvestigateFunc passed to the most
	// recent StartInvestigation call, so tests can assert on the session
	// "mode" tag and the seeded RCA content without a real Manager.
	mu           sync.Mutex
	lastMetadata map[string]string
	lastFn       session.InvestigateFunc
}

func (m *fallbackAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}
func (m *fallbackAutoMgr) CancelInvestigation(_ string) error  { return nil }
func (m *fallbackAutoMgr) SuspendInvestigation(_ string) error { return nil }
func (m *fallbackAutoMgr) TransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *fallbackAutoMgr) ForceTransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *fallbackAutoMgr) UpgradeToInteractive(_ string, _ string, _ []string) error {
	m.upgradeCalled.Add(1)
	return m.upgradeErr
}
func (m *fallbackAutoMgr) FindPendingByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *fallbackAutoMgr) LaunchDeferredInvestigation(_ string) error { return nil }
func (m *fallbackAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return m.rcaSummary, m.rcaFound
}
func (m *fallbackAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return m.rcaResult, m.rcaOK
}
func (m *fallbackAutoMgr) StartInvestigation(_ context.Context, fn session.InvestigateFunc, metadata map[string]string) (string, error) {
	m.startCalled.Add(1)
	m.mu.Lock()
	m.lastMetadata = metadata
	m.lastFn = fn
	m.mu.Unlock()
	return m.startResult, m.startErr
}

// capturedMode returns the "mode" metadata tag from the most recent
// StartInvestigation call (#1818: distinguishes interactive_fallback from
// interactive_reattached).
func (m *fallbackAutoMgr) capturedMode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastMetadata == nil {
		return ""
	}
	return m.lastMetadata["mode"]
}

// capturedResult invokes the most recently captured InvestigateFunc and
// returns its result, so tests can assert on the seeded RCA content (#1818).
func (m *fallbackAutoMgr) capturedResult() *katypes.InvestigationResult {
	m.mu.Lock()
	fn := m.lastFn
	m.mu.Unlock()
	if fn == nil {
		return nil
	}
	result, _ := fn(context.Background())
	return result
}
func (m *fallbackAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *fallbackAutoMgr) EmitSessionEndedByRR(_, _ string)                      {}
func (m *fallbackAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) { return nil, false }

// reattachHTTPCompleter is a minimal HTTPSessionCompleter stub for #1818
// tests: only FindUserDrivingByRemediationID is exercised by the reattach
// decision logic; CompleteUserDriving/ForceCompleteByRemediationID are no-ops
// since handleStart's start path never calls them.
type reattachHTTPCompleter struct {
	foundID string
	found   bool
}

func (c *reattachHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	return c.foundID, c.found
}
func (c *reattachHTTPCompleter) CompleteUserDriving(_ string, _ *katypes.InvestigationResult) error {
	return nil
}
func (c *reattachHTTPCompleter) ForceCompleteByRemediationID(_ string, _ *katypes.InvestigationResult) error {
	return nil
}
func (c *reattachHTTPCompleter) PersistPendingDecisionResult(_ string, _ *katypes.InvestigationResult) {}

var _ = Describe("Fix #1440: handleStart robustness — SC-24", func() {

	Describe("UT-KA-1440-010: handleStart creates fresh interactive session when no session exists for RR", func() {
		It("should create a fresh session and return valid session_id when no prior session found", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1440-010",
					CorrelationID: "rr-no-session-010",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:      false, // no running session
				startResult: "fresh-investigation-010",
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-session-010",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).NotTo(BeEmpty())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).NotTo(BeEmpty(),
				"SC-24: InvestigationSessionID must be populated — user must never get a lease without an investigation")

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"SC-24: StartInvestigation must be called to create a fresh session when none exists")
		})
	})

	Describe("UT-KA-1440-011: handleStart creates fresh interactive session when prior session is terminal", func() {
		It("should create a fresh session when UpgradeToInteractive returns ErrSessionTerminal and force-transition also fails", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1440-011",
					CorrelationID: "rr-terminal-011",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findResult:  "old-completed-session-011",
				findOK:      true,
				upgradeErr:  session.ErrSessionTerminal,
				startResult: "fresh-investigation-011",
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-terminal-011",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-bob", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).NotTo(BeEmpty(),
				"SC-24: must create fresh session when prior is terminal — user needs an investigation to drive")

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"SC-24: StartInvestigation must be called as fallback when session is terminal")
		})
	})

	Describe("UT-KA-1440-012: handleStart preserves RCA context from completed autonomous session", func() {
		It("should create fresh session AND populate reconHistory with prior RCA when terminal session has RCA", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1440-012",
					CorrelationID: "rr-context-012",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findResult:  "old-completed-session-012",
				findOK:      true,
				upgradeErr:  session.ErrSessionTerminal,
				startResult: "fresh-investigation-012",
				rcaSummary:  "Pod OOMKilled due to memory leak in /api/v2/reports endpoint",
				rcaFound:    true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-context-012",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"SC-24: StartInvestigation must be called for terminal session fallback")
			Expect(out.InvestigationSessionID).To(Equal("fresh-investigation-012"),
				"SC-24: InvestigationSessionID must be the fresh session, not the terminal one")

			reconHistory := tool.GetReconstructedHistory("rr-context-012")
			Expect(reconHistory).NotTo(BeNil(),
				"SC-24: reconstructed history must be populated with prior RCA context")
			Expect(reconHistory).To(HaveLen(1))
			Expect(reconHistory[0].Content).To(ContainSubstring("OOMKilled"),
				"SC-24: RCA summary from terminal session must be available to the fresh investigation")
		})
	})

	Describe("UT-KA-1818-001: handleStart reattaches to an already-user_driving session instead of creating a duplicate", func() {
		It("should reuse the existing user_driving session ID and skip StartInvestigation entirely", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1818-001",
					CorrelationID: "rr-already-driving-001",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK: false, // no Running session — reattach path is entered
			}
			completer := &reattachHTTPCompleter{
				foundID: "existing-user-driving-001",
				found:   true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-already-driving-001",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).To(Equal("existing-user-driving-001"),
				"#1818: must reattach to the already-user_driving session rather than creating a duplicate placeholder")

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"#1818: StartInvestigation must NOT be called when an existing user_driving session already covers this rr_id")
		})
	})

	Describe("UT-KA-1818-002: handleStart seeds the fresh session with the real RCA from a completed autonomous investigation (no-session branch)", func() {
		It("should tag the fresh session interactive_reattached and seed it with the real RCASummary, not a placeholder", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1818-002",
					CorrelationID: "rr-race-completed-002",
				},
			}
			realRCA := &katypes.InvestigationResult{
				RCASummary: "Pod OOMKilled due to memory leak in /api/v2/reports endpoint",
				Confidence: 0.87,
			}
			autoMgr := &fallbackAutoMgr{
				findOK:      false, // no Running session — the autonomous investigation already completed
				startResult: "fresh-investigation-1818-002",
				rcaResult:   realRCA,
				rcaOK:       true,
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-race-completed-002",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-bob", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).To(Equal("fresh-investigation-1818-002"))

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)))
			Expect(autoMgr.capturedMode()).To(Equal("interactive_reattached"),
				"#1818: session created from a real completed RCA must be tagged interactive_reattached, not interactive_fallback")

			seeded := autoMgr.capturedResult()
			Expect(seeded).NotTo(BeNil())
			Expect(seeded.RCASummary).To(Equal(realRCA.RCASummary),
				"#1818: the fresh session's immediate result must carry the real RCA, not the canned placeholder — this is what AF's bridge streams back to the user as the early RCA artifact")
			Expect(seeded.Confidence).To(Equal(realRCA.Confidence))
			Expect(seeded.InteractiveHold).To(BeTrue(),
				"#1818: reattached session must stay in InteractiveHold so it behaves like a fresh interactive session, not an immediately-completed one")
		})
	})

	Describe("UT-KA-1818-003: handleStart seeds the fresh session with the real RCA when the prior session is terminal (upgrade-failure branch)", func() {
		It("should tag the fresh session interactive_reattached when UpgradeToInteractive fails with ErrSessionTerminal and a real RCA exists", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1818-003",
					CorrelationID: "rr-terminal-with-rca-003",
				},
			}
			realRCA := &katypes.InvestigationResult{
				RCASummary: "CrashLoopBackOff caused by missing ConfigMap key",
			}
			autoMgr := &fallbackAutoMgr{
				findResult:  "old-completed-session-003",
				findOK:      true,
				upgradeErr:  session.ErrSessionTerminal,
				startResult: "fresh-investigation-1818-003",
				rcaResult:   realRCA,
				rcaOK:       true,
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-terminal-with-rca-003",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-carol", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).To(Equal("fresh-investigation-1818-003"))

			Expect(autoMgr.capturedMode()).To(Equal("interactive_reattached"),
				"#1818: the terminal-session fallback path must also reattach real RCA content when available")
			seeded := autoMgr.capturedResult()
			Expect(seeded).NotTo(BeNil())
			Expect(seeded.RCASummary).To(Equal(realRCA.RCASummary))
		})
	})

	Describe("UT-KA-1818-004: handleStart falls back to a genuine placeholder when neither a user_driving session nor a real RCA exists", func() {
		It("should tag the fresh session interactive_fallback (regression guard for the pre-#1818 behavior)", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1818-004",
					CorrelationID: "rr-no-rca-004",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:      false,
				startResult: "fresh-investigation-1818-004",
				rcaOK:       false,
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-rca-004",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-dave", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).To(Equal("fresh-investigation-1818-004"))

			Expect(autoMgr.capturedMode()).To(Equal("interactive_fallback"),
				"#1818: with no user_driving session and no real RCA, must still create the genuine placeholder session")
			seeded := autoMgr.capturedResult()
			Expect(seeded).NotTo(BeNil())
			Expect(seeded.RCASummary).To(Equal("Interactive session — awaiting user direction"))
			Expect(seeded.InteractiveHold).To(BeTrue())
		})
	})
})
