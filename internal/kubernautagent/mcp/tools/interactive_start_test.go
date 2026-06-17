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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// interactiveAutoMgr extends takeoverAutoMgr with pending session support.
type interactiveAutoMgr struct {
	// Existing AutonomousSessionManager fields
	findResult       string
	findOK           bool
	transitionErr    error
	transitionCalled atomic.Int32

	// BR-INTERACTIVE-010: pending session lookup
	pendingResult string
	pendingOK     bool
	launchErr     error
	launchCalled  atomic.Int32
}

func (m *interactiveAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}

func (m *interactiveAutoMgr) CancelInvestigation(_ string) error  { return nil }
func (m *interactiveAutoMgr) SuspendInvestigation(_ string) error { return nil }
func (m *interactiveAutoMgr) UpgradeToInteractive(_ string, _ string, _ []string) error {
	return nil
}

func (m *interactiveAutoMgr) TransitionToUserDriving(_ string, _ string, _ []string) error {
	m.transitionCalled.Add(1)
	return m.transitionErr
}

func (m *interactiveAutoMgr) ForceTransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}

func (m *interactiveAutoMgr) FindPendingByRemediationID(_ string) (string, bool) {
	return m.pendingResult, m.pendingOK
}

func (m *interactiveAutoMgr) LaunchDeferredInvestigation(_ string) error {
	m.launchCalled.Add(1)
	return m.launchErr
}

func (m *interactiveAutoMgr) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	return "", nil
}
func (m *interactiveAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *interactiveAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *interactiveAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (m *interactiveAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}

var _ = Describe("BR-INTERACTIVE-010: handleStart with pending interactive session — #1293", func() {

	Describe("UT-KA-1293-013: action=start detects pending session and launches deferred investigation", func() {
		It("should call LaunchDeferredInvestigation and return started status", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-pending-001",
					CorrelationID: "rr-interactive-013",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "http-sess-pending-001",
				pendingOK:     true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-interactive-013",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("sess-pending-001"))
			Expect(out.Status).To(Equal("started"))

			Expect(autoMgr.launchCalled.Load()).To(Equal(int32(1)),
				"LaunchDeferredInvestigation must be called for pending interactive session")
		})
	})

	Describe("UT-KA-1293-014: action=start skips pending check when no pending session exists", func() {
		It("should follow normal takeover flow when no pending session found", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-normal-014",
					CorrelationID: "rr-normal-014",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingOK: false,
				findOK:    false,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-normal-014",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("sess-normal-014"))
			Expect(out.Status).To(Equal("started"))

			Expect(autoMgr.launchCalled.Load()).To(Equal(int32(0)),
				"LaunchDeferredInvestigation must NOT be called when no pending session")
		})
	})

	Describe("UT-KA-1326-020: action=start populates InvestigationSessionID on successful deferred launch", func() {
		It("should set InvestigationSessionID to the pending session ID", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-020",
					CorrelationID: "rr-interactive-020",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "investigation-sess-020",
				pendingOK:     true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-interactive-020",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.SessionID).To(Equal("lease-sess-020"))
			Expect(out.InvestigationSessionID).To(Equal("investigation-sess-020"),
				"InvestigationSessionID must be populated with the pending session ID after successful launch")
		})
	})

	Describe("UT-KA-1326-021: action=start does not populate InvestigationSessionID when no pending session", func() {
		It("should leave InvestigationSessionID empty", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-021",
					CorrelationID: "rr-no-pending-021",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingOK: false,
				findOK:    false,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-pending-021",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(BeEmpty(),
				"InvestigationSessionID must be empty when no pending session exists")
		})
	})

	Describe("UT-KA-1326-022: action=start does not populate InvestigationSessionID on deferred launch failure", func() {
		It("should leave InvestigationSessionID empty on launch error", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-022",
					CorrelationID: "rr-launch-fail-022",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "investigation-sess-022",
				pendingOK:     true,
				launchErr:     errors.New("deferred launch failed"),
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-launch-fail-022",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(BeEmpty(),
				"InvestigationSessionID must be empty when deferred launch fails")
		})
	})

	Describe("UT-KA-1293-015: action=start handles LaunchDeferredInvestigation failure gracefully", func() {
		It("should still return started (deferred launch is best-effort for MCP response)", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-fail-015",
					CorrelationID: "rr-fail-015",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "http-sess-fail-015",
				pendingOK:     true,
				launchErr:     errors.New("deferred launch failed"),
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-fail-015",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("sess-fail-015"))
			Expect(out.Status).To(Equal("started"))
		})
	})
})

var _ = Describe("Fix #1390: handleStart upgrade wiring — BR-INTERACTIVE-004", func() {

	Describe("UT-KA-1390-012 [SC-24]: handleStart calls UpgradeToInteractive (not TransitionToUserDriving) for running sessions", func() {
		It("should call UpgradeToInteractive instead of TransitionToUserDriving for running auto sessions", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-012",
					CorrelationID: "rr-upgrade-012",
				},
			}
			autoMgr := &upgradeTrackingAutoMgr{
				findResult: "http-auto-sess-012",
				findOK:     true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-upgrade-012",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice", Groups: []string{"sre"}})
			Expect(err).NotTo(HaveOccurred())

			Expect(autoMgr.upgradeCalled.Load()).To(Equal(int32(1)),
				"UpgradeToInteractive must be called once for running session")
			Expect(autoMgr.transitionCalled.Load()).To(Equal(int32(0)),
				"TransitionToUserDriving must NOT be called — replaced by UpgradeToInteractive")
		})
	})

	Describe("UT-KA-1390-013 [SI-4]: handleStart sets InvestigationSessionID for running sessions (enables EventLogBridge)", func() {
		It("should populate InvestigationSessionID with the auto session ID", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-sess-013",
					CorrelationID: "rr-upgrade-013",
				},
			}
			autoMgr := &upgradeTrackingAutoMgr{
				findResult: "http-auto-sess-013",
				findOK:     true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-upgrade-013",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())

			Expect(out.InvestigationSessionID).To(Equal("http-auto-sess-013"),
				"InvestigationSessionID must be set to enable EventLogBridge for upgraded sessions")
		})
	})
})

var _ = Describe("Fix #1452: handleStart prefers AF-provided session ID — BR-INTERACTIVE-010", func() {

	Describe("UT-KA-1452-001 [AC-4]: AF-provided SessionID used for direct pending session lookup", func() {
		It("should use input.SessionID directly instead of RRID scan", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-1452-001",
					CorrelationID: "rr-1452-001",
				},
			}
			autoMgr := &sessionIDTrackingAutoMgr{
				launchOK: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:      "rr-1452-001",
				Action:    mcptools.ActionStart,
				SessionID: "af-provided-sess-001",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(Equal("af-provided-sess-001"),
				"AC-4: AF-provided session ID must be used for direct lookup, not RRID scan")
			Expect(autoMgr.launchedID).To(Equal("af-provided-sess-001"),
				"LaunchDeferredInvestigation must receive the AF-provided session ID")
			Expect(autoMgr.findPendingCalled.Load()).To(Equal(int32(0)),
				"FindPendingByRemediationID must NOT be called when AF provides a session ID")
		})
	})

	Describe("UT-KA-1452-002 [AC-4]: empty SessionID falls back to RRID scan (existing behavior)", func() {
		It("should call FindPendingByRemediationID when SessionID is empty", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-1452-002",
					CorrelationID: "rr-1452-002",
				},
			}
			autoMgr := &sessionIDTrackingAutoMgr{
				pendingResult: "rrid-scanned-sess-002",
				pendingOK:     true,
				launchOK:      true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-1452-002",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(Equal("rrid-scanned-sess-002"),
				"AC-4: RRID scan must be used as fallback when no AF session ID provided")
			Expect(autoMgr.findPendingCalled.Load()).To(Equal(int32(1)),
				"FindPendingByRemediationID must be called when SessionID is empty")
		})
	})

	Describe("UT-KA-1452-003 [AC-4]: AF-provided SessionID with launch failure falls through gracefully", func() {
		It("should handle launch failure for AF-provided session ID and continue to upgrade path", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "lease-1452-003",
					CorrelationID: "rr-1452-003",
				},
			}
			autoMgr := &sessionIDTrackingAutoMgr{
				launchOK:   false,
				launchErr:  errors.New("session not found"),
				findResult: "auto-running-sess-003",
				findOK:     true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:      "rr-1452-003",
				Action:    mcptools.ActionStart,
				SessionID: "af-stale-sess-003",
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(Equal("auto-running-sess-003"),
				"AC-4: when AF-provided session launch fails, must fall through to UpgradeToInteractive path")
		})
	})
})

// sessionIDTrackingAutoMgr tracks the session ID passed to LaunchDeferredInvestigation
// and whether FindPendingByRemediationID was called.
type sessionIDTrackingAutoMgr struct {
	findResult         string
	findOK             bool
	pendingResult      string
	pendingOK          bool
	launchOK           bool
	launchErr          error
	launchedID         string
	findPendingCalled  atomic.Int32
}

func (m *sessionIDTrackingAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}
func (m *sessionIDTrackingAutoMgr) CancelInvestigation(_ string) error  { return nil }
func (m *sessionIDTrackingAutoMgr) SuspendInvestigation(_ string) error { return nil }
func (m *sessionIDTrackingAutoMgr) TransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *sessionIDTrackingAutoMgr) ForceTransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *sessionIDTrackingAutoMgr) UpgradeToInteractive(_ string, _ string, _ []string) error {
	return nil
}
func (m *sessionIDTrackingAutoMgr) FindPendingByRemediationID(_ string) (string, bool) {
	m.findPendingCalled.Add(1)
	return m.pendingResult, m.pendingOK
}
func (m *sessionIDTrackingAutoMgr) LaunchDeferredInvestigation(id string) error {
	m.launchedID = id
	if !m.launchOK {
		return m.launchErr
	}
	return nil
}
func (m *sessionIDTrackingAutoMgr) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	return "", nil
}
func (m *sessionIDTrackingAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *sessionIDTrackingAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *sessionIDTrackingAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (m *sessionIDTrackingAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}

// upgradeTrackingAutoMgr tracks calls to UpgradeToInteractive vs TransitionToUserDriving.
type upgradeTrackingAutoMgr struct {
	findResult       string
	findOK           bool
	upgradeCalled    atomic.Int32
	upgradeErr       error
	transitionCalled atomic.Int32
	transitionErr    error
}

func (m *upgradeTrackingAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}
func (m *upgradeTrackingAutoMgr) CancelInvestigation(_ string) error  { return nil }
func (m *upgradeTrackingAutoMgr) SuspendInvestigation(_ string) error { return nil }
func (m *upgradeTrackingAutoMgr) TransitionToUserDriving(_ string, _ string, _ []string) error {
	m.transitionCalled.Add(1)
	return m.transitionErr
}
func (m *upgradeTrackingAutoMgr) ForceTransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *upgradeTrackingAutoMgr) UpgradeToInteractive(_ string, _ string, _ []string) error {
	m.upgradeCalled.Add(1)
	return m.upgradeErr
}
func (m *upgradeTrackingAutoMgr) FindPendingByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *upgradeTrackingAutoMgr) LaunchDeferredInvestigation(_ string) error { return nil }
func (m *upgradeTrackingAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *upgradeTrackingAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (m *upgradeTrackingAutoMgr) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	return "", nil
}
func (m *upgradeTrackingAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *upgradeTrackingAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}
