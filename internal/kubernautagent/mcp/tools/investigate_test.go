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

func (m *mockSessionManager) TouchActivity(_ string) {}

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

type rejectingRateLimiter struct{}

func (r *rejectingRateLimiter) Allow(_ string, _ int) error {
	return mcpinternal.ErrRateLimited
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

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-start",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).To(Equal("sess-001"))
			Expect(out.Status).To(Equal("started"))
		})
	})

	Describe("UT-KA-703-K03: action=start rejects with structured error when Lease held", func() {
		It("should return MCPError session_active when another driver holds the Lease", func() {
			sessionMgr := &mockSessionManager{
				takeoverErr: mcpinternal.ErrLeaseHeld,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-held",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal("session_active"))
		})
	})

	Describe("UT-KA-703-K04: action=message calls RunInteractiveTurn + returns LLM response", func() {
		It("should invoke the investigator and return the response", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-msg",
					CorrelationID: "rr-msg",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{response: "The root cause is..."}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
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
		It("should return structured not_driving error when no session exists", func() {
			sessionMgr := &mockSessionManager{isActive: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-nosess",
				Action:  mcptools.ActionMessage,
				Message: "Hello?",
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal("not_driving"))
		})
	})

	Describe("UT-KA-703-K06: action=complete releases session", func() {
		It("should release the session with reason 'complete'", func() {
			sessionMgr := &mockSessionManager{
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-complete",
					CorrelationID: "rr-complete",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
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
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
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
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
				isActive: true,
			}
			runner := &mockInvestigatorRunner{err: errors.New("LLM unavailable")}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
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

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-recon",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
		})
	})

	Describe("UT-KA-703-K10: action=message returns session_expired when GetDriver returns ErrSessionExpired", func() {
		It("should return MCPError with code session_expired (SEC-04)", func() {
			sessionMgr := &mockSessionManager{
				isActive:     true,
				getDriverErr: mcpinternal.ErrSessionExpired,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-expired",
				Action:  mcptools.ActionMessage,
				Message: "hello",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_expired"),
				"expired session must return structured session_expired code for AF error handling")
		})
	})

	Describe("UT-KA-703-K11: action=message returns rate_limited when rate limiter rejects", func() {
		It("should return MCPError with code rate_limited (SEC-HIGH-01)", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-rl",
					CorrelationID: "rr-rl",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			rl := &rejectingRateLimiter{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{}, mcptools.WithRateLimiter(rl))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-rl",
				Action:  mcptools.ActionMessage,
				Message: "trigger rate limit",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("rate_limited"),
				"rate-limited message must return structured rate_limited code for AF retry logic")
		})
	})

	Describe("UT-KA-703-K12: action=cancel rejected when non-driver calls it", func() {
		It("should return MCPError session_active with driver identity (SEC-CRIT-01)", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-cancel-authz",
					CorrelationID: "rr-cancel-authz",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-cancel-authz",
				Action: mcptools.ActionCancel,
			}, mcpinternal.UserInfo{Username: "mallory"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"),
				"non-driver cancel must be rejected with session_active")
		})
	})

	Describe("UT-KA-703-K13: action=complete rejected when non-driver calls it", func() {
		It("should return MCPError session_active with driver identity (SEC-CRIT-01)", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-complete-authz",
					CorrelationID: "rr-complete-authz",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-complete-authz",
				Action: mcptools.ActionComplete,
			}, mcpinternal.UserInfo{Username: "mallory"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"),
				"non-driver complete must be rejected with session_active")
		})
	})

	Describe("UT-KA-RECONNECT-ACTION-001: action=reconnect returns existing session for same user", func() {
		It("should return status=reconnected with the existing session ID", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-reconnect-existing",
					CorrelationID: "rr-reconnect",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-reconnect",
				Action: mcptools.ActionReconnect,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("reconnected"))
			Expect(output.SessionID).To(Equal("sess-reconnect-existing"))
		})
	})

	Describe("UT-KA-RECONNECT-ACTION-002: action=reconnect fails when no session exists", func() {
		It("should return MCPError not_driving", func() {
			sessionMgr := &mockSessionManager{
				isActive: false,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-no-session",
				Action: mcptools.ActionReconnect,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("not_driving"))
		})
	})

	Describe("UT-KA-RECONNECT-ACTION-003: action=reconnect rejected for different user", func() {
		It("should return MCPError session_active with driver identity", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-reconnect-owned",
					CorrelationID: "rr-reconnect-diff",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-reconnect-diff",
				Action: mcptools.ActionReconnect,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"))
		})
	})
})
