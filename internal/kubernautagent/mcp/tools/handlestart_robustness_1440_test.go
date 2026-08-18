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

	// forceErr configures ForceTransitionToUserDriving's return value.
	// Defaults to nil (existing tests' assumed always-succeeds behavior);
	// #2100's fallback-exhausted test sets this to force handleStart's new
	// fail-closed branch.
	forceErr error

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
	return m.forceErr
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
func (m *fallbackAutoMgr) WaitForCompletionByRemediationID(_ string) <-chan struct{} {
	return mcptools.ClosedChan()
}

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

	// UT-KA-1440-010 (superseded by DD-AA-KA-001 Amendment Gap 3, BR-AA-KA-065.12):
	// SC-24's original fix created a hardcoded placeholder session so a Lease
	// was never left with nothing to drive. Gap 3 replaced that placeholder
	// with a fail-closed contract instead: a genuinely nonexistent RR-backed
	// investigation is a dead end for both of AF's supported flows
	// (interactive remediation, autonomous fix), so the request must now fail
	// closed rather than fabricate a chat session with no execution path.
	// See UT-KA-1818-004 for the current contract this scenario maps to.
	Describe("UT-KA-1440-010: handleStart fails closed (not a placeholder) when genuinely no session exists for RR", func() {
		It("should release the lease and return ErrCodeNoInvestigationAvailable", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1440-010",
					CorrelationID: "rr-no-session-010",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK:   false, // no running session
				forceErr: session.ErrSessionNotFound,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-session-010",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})

			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal(mcptools.ErrCodeNoInvestigationAvailable.Code))
			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"Gap 3: StartInvestigation must never be called to fabricate an unbacked placeholder session")
		})
	})

	// UT-KA-1440-011 (updated for Gap 3): a terminal session still has a
	// real, if stale, session to fall back to -- reusing autoSessionID
	// directly (this path never reported exhausted=true, see the doc
	// comment on upgradeOrCreateInteractiveSession). Without a real RCA to
	// seed a fresh session with, no fresh session is created.
	Describe("UT-KA-1440-011: handleStart reuses the terminal session's own ID when it has no real RCA to reattach", func() {
		It("should fall back to autoSessionID directly, without calling StartInvestigation, when UpgradeToInteractive reports terminal and no RCA exists", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1440-011",
					CorrelationID: "rr-terminal-011",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findResult: "old-completed-session-011",
				findOK:     true,
				upgradeErr: session.ErrSessionTerminal,
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
			Expect(out.InvestigationSessionID).To(Equal("old-completed-session-011"),
				"Gap 3: with no real RCA to reattach, fall back to the terminal session's own (real, if stale) ID rather than fabricating a placeholder")

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"Gap 3: StartInvestigation must not be called when there is nothing real to seed a fresh session with")
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
			realRCA := &katypes.InvestigationResult{
				RCASummary: "Pod OOMKilled due to memory leak in /api/v2/reports endpoint",
			}
			autoMgr := &fallbackAutoMgr{
				findResult:  "old-completed-session-012",
				findOK:      true,
				upgradeErr:  session.ErrSessionTerminal,
				startResult: "fresh-investigation-012",
				// #1818 / Gap 3: GetLatestRCASummaryByRemediationID and
				// GetLatestRCAResultByRemediationID both read off the same
				// underlying sess.Result in the real Manager, so a completed
				// session with an RCA summary always has a matching seed
				// result too -- both must be configured consistently here.
				rcaSummary: realRCA.RCASummary,
				rcaFound:   true,
				rcaResult:  realRCA,
				rcaOK:      true,
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
			Expect(autoMgr.capturedMode()).To(Equal("interactive_reattached"))

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

	Describe("UT-KA-1818-004 / DD-AA-KA-001 Amendment Gap 3, BR-AA-KA-065.12: reattachOrCreateFallback returns exhausted (not a placeholder) when neither a user_driving session nor a real RCA exists", func() {
		It("should skip StartInvestigation entirely and let handleStart's existing #2100 fail-closed path release the lease and return ErrCodeNoInvestigationAvailable", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-1818-004",
					CorrelationID: "rr-no-rca-004",
				},
			}
			autoMgr := &fallbackAutoMgr{
				findOK: false, // no autonomous session at all for this RR
				rcaOK:  false, // ...and no completed RCA anywhere either
				// #2100-shaped fail-closed contract: ForceTransitionToUserDriving
				// on a genuinely nonexistent session returns ErrSessionNotFound in
				// the real Manager (manager_interactive.go) -- there is nothing,
				// anywhere, for this rr_id to attach to.
				forceErr: session.ErrSessionNotFound,
			}
			completer := &reattachHTTPCompleter{found: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr, mcptools.WithHTTPCompleter(completer))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-rca-004",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-dave", Groups: []string{"sre-team"}})

			Expect(err).To(HaveOccurred(),
				"Gap 3: without an RR-backed investigation to attach to, AF's two supported flows (interactive remediation, autonomous fix) both dead-end -- fail closed rather than hand back a chat session with no execution path")
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal(mcptools.ErrCodeNoInvestigationAvailable.Code))
			Expect(out).To(Equal(mcptools.InvestigateOutput{}))

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"Gap 3: StartInvestigation must never be called to fabricate an unbacked placeholder session")

			releasedID, releasedReason := sessionMgr.getReleased()
			Expect(releasedID).To(Equal("lease-sess-1818-004"))
			Expect(releasedReason).To(Equal("no_investigation_available"))
		})
	})
})
