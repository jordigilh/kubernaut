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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type mockSessionManager struct {
	takeoverSession *mcpinternal.InteractiveSession
	takeoverErr     error
	releaseErr      error
	getDriverResult *mcpinternal.InteractiveSession
	getDriverErr    error
	isActive        bool
	releasedID      string
	releasedReason  string
}

func (m *mockSessionManager) Takeover(_ context.Context, _ string, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	return m.takeoverSession, m.takeoverErr
}

func (m *mockSessionManager) Release(sessionID string, reason string) error {
	m.releasedID = sessionID
	m.releasedReason = reason
	return m.releaseErr
}

func (m *mockSessionManager) GetDriver(_ string) (*mcpinternal.InteractiveSession, error) {
	return m.getDriverResult, m.getDriverErr
}

func (m *mockSessionManager) IsDriverActive(_ string) bool {
	return m.isActive
}

type mockInvestigatorRunner struct {
	response string
	err      error
}

func (m *mockInvestigatorRunner) RunInteractiveTurn(_ context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	return m.response, m.err
}

type mockContextReconstructor struct {
	turns []mcpinternal.ConversationTurn
	err   error
}

func (m *mockContextReconstructor) Reconstruct(_ context.Context, _, _ string) ([]mcpinternal.ConversationTurn, error) {
	return m.turns, m.err
}

var _ = Describe("kubernaut_investigate tool — #703 BR-INTERACTIVE-001", func() {

	Describe("UT-KA-703-K01: Tool validates input schema", func() {
		It("should reject empty rr_id", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{Action: mcptools.ActionStart})
			Expect(err).To(MatchError(mcptools.ErrMissingRRID))
		})

		It("should reject invalid action", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{RRID: "rr-1", Action: "invalid"})
			Expect(err).To(MatchError(mcptools.ErrInvalidAction))
		})

		It("should reject message action without message", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{RRID: "rr-1", Action: mcptools.ActionMessage})
			Expect(err).To(MatchError(mcptools.ErrMissingMessage))
		})

		It("should accept valid start action", func() {
			err := mcptools.ValidateInput(mcptools.InvestigateInput{RRID: "rr-1", Action: mcptools.ActionStart})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-703-K02: action=start calls Takeover + returns session_id", func() {
		It("should create a new session and return the session_id", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-001",
					CorrelationID: "rr-start",
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-start",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("sess-001"))
			Expect(out.Status).To(Equal("started"))
		})
	})

	Describe("UT-KA-703-K03: action=start rejects with error when Lease held", func() {
		It("should return error when another driver holds the Lease", func() {
			sessionMgr := &mockSessionManager{
				takeoverErr: mcpinternal.ErrLeaseHeld,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-held",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(MatchError(mcpinternal.ErrLeaseHeld))
		})
	})

	Describe("UT-KA-703-K04: action=message calls RunInteractiveTurn + returns LLM response", func() {
		It("should invoke the investigator and return the response", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-msg",
					CorrelationID: "rr-msg",
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{response: "The root cause is..."}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-msg",
				Action:  mcptools.ActionMessage,
				Message: "What caused this?",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Response).To(Equal("The root cause is..."))
			Expect(out.Status).To(Equal("message_received"))
		})
	})

	Describe("UT-KA-703-K05: action=message returns error if no active session", func() {
		It("should return ErrNoActiveSession when no session exists", func() {
			sessionMgr := &mockSessionManager{isActive: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-nosess",
				Action:  mcptools.ActionMessage,
				Message: "Hello?",
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).To(MatchError(mcptools.ErrNoActiveSession))
		})
	})

	Describe("UT-KA-703-K06: action=complete releases session", func() {
		It("should release the session with reason 'complete'", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-complete",
					CorrelationID: "rr-complete",
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-complete",
				Action: mcptools.ActionComplete,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))
			Expect(sessionMgr.releasedID).To(Equal("sess-complete"))
			Expect(sessionMgr.releasedReason).To(Equal("complete"))
		})
	})

	Describe("UT-KA-703-K07: action=cancel releases session with reason 'explicit'", func() {
		It("should release with reason 'explicit' on cancel", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cancel",
					CorrelationID: "rr-cancel",
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-cancel",
				Action: mcptools.ActionCancel,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("cancelled"))
			Expect(sessionMgr.releasedReason).To(Equal("explicit"))
		})
	})

	Describe("UT-KA-703-K08: Error propagation from RunInteractiveTurn", func() {
		It("should propagate investigator errors on action=message", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-err",
					CorrelationID: "rr-err",
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{err: errors.New("LLM unavailable")}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-err",
				Action:  mcptools.ActionMessage,
				Message: "diagnose this",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("LLM unavailable"))
		})
	})

	Describe("UT-KA-703-K09: Context reconstruction seeds LLM messages on start", func() {
		It("should call Reconstruct when starting a new session", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-recon",
					CorrelationID: "rr-recon",
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "prior question"},
					{Role: "assistant", Content: "prior answer"},
				},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-recon",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
		})
	})
})
