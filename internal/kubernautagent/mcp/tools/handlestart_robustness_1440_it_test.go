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

// itFallbackAutoMgr provides a real-like autonomous session manager for
// integration testing. It simulates the production path where:
// - No pending session, no running session, StartInvestigation creates one.
type itFallbackAutoMgr struct {
	findResult    string
	findOK        bool
	upgradeErr    error
	startCalled   atomic.Int32
	startResult   string
	startErr      error
	pendingResult string
	pendingOK     bool
	// forceErr configures ForceTransitionToUserDriving's return value.
	// Defaults to nil; DD-AA-KA-001 Amendment Gap 3's fail-closed scenario
	// (IT-KA-1440-010) sets this to session.ErrSessionNotFound to match the
	// real Manager's behavior when genuinely no session exists for the RR.
	forceErr error
}

func (m *itFallbackAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}
func (m *itFallbackAutoMgr) CancelInvestigation(_ string) error  { return nil }
func (m *itFallbackAutoMgr) SuspendInvestigation(_ string) error { return nil }
func (m *itFallbackAutoMgr) TransitionToUserDriving(_ string, _ string, _ []string) error {
	return nil
}
func (m *itFallbackAutoMgr) ForceTransitionToUserDriving(_ string, _ string, _ []string) error {
	return m.forceErr
}
func (m *itFallbackAutoMgr) UpgradeToInteractive(_ string, _ string, _ []string) error {
	return m.upgradeErr
}
func (m *itFallbackAutoMgr) FindPendingByRemediationID(_ string) (string, bool) {
	return m.pendingResult, m.pendingOK
}
func (m *itFallbackAutoMgr) LaunchDeferredInvestigation(_ string) error { return nil }
func (m *itFallbackAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return "", false
}
func (m *itFallbackAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}
func (m *itFallbackAutoMgr) StartInvestigation(_ context.Context, _ session.InvestigateFunc, _ map[string]string) (string, error) {
	m.startCalled.Add(1)
	return m.startResult, m.startErr
}
func (m *itFallbackAutoMgr) Subscribe(_ context.Context, _ string) (<-chan session.InvestigationEvent, error) {
	return nil, nil
}
func (m *itFallbackAutoMgr) EmitSessionEndedByRR(_, _ string) {}

func (m *itFallbackAutoMgr) WaitForCompletionByRemediationID(_ string) <-chan struct{} {
	return mcptools.ClosedChan()
}

func (m *itFallbackAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}

var _ = Describe("Fix #1440 Integration: KA handleStart fallback session creation", func() {

	// IT-KA-1440-010 (superseded by DD-AA-KA-001 Amendment Gap 3,
	// BR-AA-KA-065.12): SC-24's original placeholder-creation contract is
	// gone. A genuinely nonexistent RR-backed investigation (no running/
	// terminal autonomous session, no completed RCA anywhere) now fails
	// closed through the production dispatch path instead of fabricating a
	// session via StartInvestigation.
	Describe("IT-KA-1440-010: MCP action=start with no prior session fails closed through the production dispatch path (SC-24 / Gap 3)", func() {
		It("should return ErrCodeNoInvestigationAvailable without ever calling StartInvestigation", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-lease-it-010",
					CorrelationID: "rr-it-no-session-010",
				},
			}
			autoMgr := &itFallbackAutoMgr{
				findOK:   false,
				forceErr: session.ErrSessionNotFound,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-it-no-session-010",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})

			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal(mcptools.ErrCodeNoInvestigationAvailable.Code))
			Expect(autoMgr.startCalled.Load()).To(Equal(int32(0)),
				"Gap 3: StartInvestigation must never be called to fabricate an unbacked placeholder session")
		})
	})
})
