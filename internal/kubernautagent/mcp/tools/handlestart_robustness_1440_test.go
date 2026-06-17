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
func (m *fallbackAutoMgr) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	m.startCalled.Add(1)
	return m.startResult, m.startErr
}
func (m *fallbackAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *fallbackAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) { return nil, false }

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
})
