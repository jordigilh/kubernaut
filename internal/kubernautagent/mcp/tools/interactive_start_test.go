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

func (m *interactiveAutoMgr) CancelInvestigation(_ string) error { return nil }
func (m *interactiveAutoMgr) SuspendInvestigation(_ string) error { return nil }

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
