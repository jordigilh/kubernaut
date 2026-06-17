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
	return nil
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
func (m *itFallbackAutoMgr) GetSessionLazySink(_ string) (*session.LazySink, bool) {
	return nil, false
}

var _ = Describe("Fix #1440 Integration: KA handleStart fallback session creation", func() {

	Describe("IT-KA-1440-010: MCP action=start with no prior session creates interactive session and returns valid session_id (SC-24)", func() {
		It("should create a session via StartInvestigation and return it in InvestigationSessionID", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-lease-it-010",
					CorrelationID: "rr-it-no-session-010",
				},
			}
			autoMgr := &itFallbackAutoMgr{
				findOK:      false,
				startResult: "ka-session-it-010",
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-it-no-session-010",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice", Groups: []string{"sre-team"}})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("mcp-lease-it-010"),
				"MCP lease session must still be acquired")
			Expect(out.Status).To(Equal("started"))
			Expect(out.InvestigationSessionID).To(Equal("ka-session-it-010"),
				"SC-24: InvestigationSessionID must be the freshly created session")

			Expect(autoMgr.startCalled.Load()).To(Equal(int32(1)),
				"SC-24: StartInvestigation must be called through production dispatch path")
		})
	})
})
