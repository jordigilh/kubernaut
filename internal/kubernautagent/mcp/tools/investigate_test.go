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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type mockSessionManager struct {
	mu              sync.Mutex
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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.takeoverSession, m.takeoverErr
}

func (m *mockSessionManager) Release(sessionID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releasedID = sessionID
	m.releasedReason = reason
	return m.releaseErr
}

func (m *mockSessionManager) GetDriver(_ string) (*mcpinternal.InteractiveSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getDriverResult, m.getDriverErr
}

func (m *mockSessionManager) IsDriverActive(_ string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isActive
}

func (m *mockSessionManager) TouchActivity(_ string) {}

func (m *mockSessionManager) getReleased() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releasedID, m.releasedReason
}

type mockInvestigatorRunner struct {
	response    string
	err         error
	rcaResult   *katypes.InvestigationResult
	capturedCtx context.Context
}

func (m *mockInvestigatorRunner) RunInteractiveTurn(ctx context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	m.capturedCtx = ctx
	return m.response, m.err
}

func (m *mockInvestigatorRunner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	if m.rcaResult != nil {
		return m.rcaResult, m.err
	}
	return &katypes.InvestigationResult{RCASummary: "mock autonomous result", Confidence: 0.8}, m.err
}

func (m *mockInvestigatorRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	if m.rcaResult != nil {
		return m.rcaResult, m.err
	}
	return &katypes.InvestigationResult{RCASummary: "mock RCA", Confidence: 0.9}, nil
}

func (m *mockInvestigatorRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA", WorkflowID: "mock-workflow", Confidence: 0.85}, nil
}

type mockContextReconstructor struct {
	turns            []mcpinternal.ConversationTurn
	err              error
	reconstructCalls atomic.Int32
}

func (m *mockContextReconstructor) Reconstruct(_ context.Context, _, _ string) ([]mcpinternal.ConversationTurn, error) {
	m.reconstructCalls.Add(1)
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

	Describe("UT-KA-COMPLETE-HTTP-001: action=complete delivers proper result to HTTP session", func() {
		It("should build InvestigationResult from RCA and write to HTTP completer", func() {
			completer := &mockHTTPCompleter{foundID: "http-complete-001", found: true}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-complete-http",
					CorrelationID: "rr-complete-http",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult: &katypes.InvestigationResult{
						RCASummary: "Pod OOM due to memory leak",
						Confidence: 0.85,
						Severity:   "critical",
					},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithHTTPCompleter(completer),
			)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-complete-http",
				Action: mcptools.ActionComplete,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))

			Expect(completer.completedResult).NotTo(BeNil(),
				"handleComplete must deliver a non-nil result to the HTTP session so AA can poll it")
			Expect(completer.completedResult.RCASummary).To(Equal("Pod OOM due to memory leak"),
				"RCA from the driver session must propagate to the HTTP session result")
			Expect(completer.completedResult.IsActionable).NotTo(BeNil(),
				"IsActionable must be set for AA routing")
			Expect(*completer.completedResult.IsActionable).To(BeFalse(),
				"completing without workflow selection means not actionable")
			Expect(completer.completedResult.Warnings).To(ContainElement("Alert not actionable"),
				"Warnings must signal AA that no remediation is needed")
		})

		It("should build minimal result when no RCA exists", func() {
			completer := &mockHTTPCompleter{foundID: "http-complete-002", found: true}
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-complete-norca",
					CorrelationID: "rr-complete-norca",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithHTTPCompleter(completer),
			)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-complete-norca",
				Action: mcptools.ActionComplete,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))

			Expect(completer.completedResult).NotTo(BeNil(),
				"even without RCA, handleComplete must provide a non-nil result to avoid AA 409")
			Expect(completer.completedResult.RCASummary).NotTo(BeEmpty(),
				"minimal result must have an RCA summary for AA routing")
			Expect(completer.completedResult.IsActionable).NotTo(BeNil())
			Expect(*completer.completedResult.IsActionable).To(BeFalse())
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

	Describe("UT-KA-1293-007: handleStart rejects when session is reconnected", func() {
		It("should return MCPError session_active with same-user reconnect message", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-reconnect-007",
					CorrelationID: "rr-007",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					Reconnected:   true,
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-007",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"),
				"reconnected session must reject action=start; use action=reconnect instead")
			Expect(mcpErr.Message).To(ContainSubstring("already have an active session"),
				"same-user reconnect should use distinct message from cross-user contention")
			Expect(mcpErr.Details["driver"]).To(Equal("alice"))
			Expect(mcpErr.Details["session_id"]).To(Equal("sess-reconnect-007"),
				"reconnect error should include existing session_id for diagnostics")
		})
	})
})

// mockSignalResolver implements mcptools.SignalContextResolver for UTs.
type mockSignalResolver struct {
	signal    *katypes.SignalContext
	signalErr error
}

func (m *mockSignalResolver) ResolveSignalContext(_ context.Context, _ string) (*katypes.SignalContext, error) {
	if m.signal != nil {
		return m.signal, m.signalErr
	}
	return &katypes.SignalContext{Severity: "critical"}, m.signalErr
}

var _ = Describe("kubernaut_investigate — discover_workflows action", func() {

	Describe("UT-KA-DW-001: discover_workflows stores RCA + DiscoveryResult", func() {
		It("should return recommendations and store both results on session", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-dw-001",
				CorrelationID: "rr-dw-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "my pod is crashing"},
				{Role: "assistant", Content: "I see OOM errors"},
			}}
			resolver := &mockSignalResolver{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-001",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			Expect(sess.RCAResult).NotTo(BeNil(), "RCA result should be stored on session")
			Expect(sess.DiscoveryResult).NotTo(BeNil(), "Discovery result should be stored on session")
		})
	})

	Describe("UT-KA-DW-002: discover_workflows rejects non-driver", func() {
		It("should reject caller who is not the active driver", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-dw-002",
					CorrelationID: "rr-dw-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-002",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-DW-003: discover_workflows rejects when no session", func() {
		It("should reject when no active session exists", func() {
			sessionMgr := &mockSessionManager{isActive: false}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-003",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-1374-PF01-001: discover_workflows delegates enrichment to investigator", func() {
		It("should pass nil enrichData since enrichment is handled by investigator pipeline", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-dw-011",
				CorrelationID: "rr-dw-011",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on api-server pod",
					Confidence: 0.92,
					RemediationTarget: katypes.RemediationTarget{
						Kind:      "Pod",
						Name:      "api-server",
						Namespace: "prod",
					},
				},
			}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "my pod is crashing"},
				{Role: "assistant", Content: "I see OOM errors"},
			}}
			resolver := &mockSignalResolver{
				signal: &katypes.SignalContext{IncidentID: "inc-011", Severity: "critical"},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-011",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))
		})
	})
})

var _ = Describe("kubernaut_investigate — discover_workflows additional scenarios", func() {

	Describe("UT-KA-DW-004: discover_workflows called twice overwrites previous results", func() {
		It("should succeed and overwrite RCA + DiscoveryResult on re-discovery", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-dw-004",
				CorrelationID: "rr-dw-004",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				RCAResult:     &katypes.InvestigationResult{RCASummary: "stale RCA"},
				DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
					Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "stale-wf"},
				},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "my pod is crashing"},
			}}
			resolver := &mockSignalResolver{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-004",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			Expect(sess.RCAResult).NotTo(BeNil())
			Expect(sess.RCAResult.RCASummary).To(Equal("mock RCA"),
				"RCA should be overwritten with fresh extraction result")
			Expect(sess.DiscoveryResult).NotTo(BeNil())
			Expect(sess.DiscoveryResult.Recommended).NotTo(BeNil())
			Expect(sess.DiscoveryResult.Recommended.WorkflowID).To(Equal("mock-workflow"),
				"DiscoveryResult should be overwritten with fresh discovery result")
		})
	})

	Describe("UT-KA-RECONNECT-DW: reconnect after discover_workflows preserves session state", func() {
		It("should allow select_workflow after reconnect without re-discovery", func() {
			wfID := "wf-reconnect-test"
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-reconnect-dw",
				CorrelationID: "rr-reconnect-dw",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				RCAResult:     &katypes.InvestigationResult{RCASummary: "OOM crash", Confidence: 0.9},
				DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
					Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: wfID, Confidence: 0.85},
				},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}

			investigateTool := mcptools.NewInvestigateTool(sessionMgr, &mockInvestigatorRunner{}, &mockContextReconstructor{}, mcptools.NopAutonomousManager{})
			reconnectOut, err := investigateTool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-reconnect-dw",
				Action: mcptools.ActionReconnect,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(reconnectOut.Status).To(Equal("reconnected"))

			Expect(sess.DiscoveryResult).NotTo(BeNil(),
				"DiscoveryResult must survive reconnect")
			Expect(sess.RCAResult).NotTo(BeNil(),
				"RCAResult must survive reconnect")

			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{WorkflowID: wfID, WorkflowName: "restart-pod"},
			}
			selectTool := mcptools.NewSelectWorkflowTool(catalog, sessionMgr)
			selectOut, err := selectTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-reconnect-dw",
				WorkflowID: wfID,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(selectOut.Status).To(Equal("workflow_selected"))
		})
	})
})

var _ = Describe("kubernaut_investigate — DiscoveryResult invalidation on message", func() {

	Describe("UT-KA-INVAL-001: message clears DiscoveryResult", func() {
		It("should clear DiscoveryResult after a message is sent", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-inval-001",
				CorrelationID: "rr-inval-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				DiscoveryResult: &mcpinternal.WorkflowDiscoveryResult{
					Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-stale"},
				},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{response: "Updated analysis shows..."}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{})
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-inval-001",
				Action:  mcptools.ActionMessage,
				Message: "actually it might be a network issue",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("message_received"))

			Expect(sess.DiscoveryResult).To(BeNil(),
				"DiscoveryResult must be cleared after a message to prevent stale recommendations")
		})
	})

	Describe("UT-KA-1374-F9-001: action=message attaches SignalContext to context [BR-INTERACTIVE-010]", func() {
		It("should propagate SignalContext via WithSignalContext on message turns", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-f9-001",
				CorrelationID: "rr-f9-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{response: "Analyzing crash dumps..."}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}

			expectedSignal := &katypes.SignalContext{
				IncidentID:   "inc-f9-001",
				Severity:     "critical",
				ResourceKind: "Deployment",
				ResourceName: "api-server",
				Namespace:    "production",
			}
			resolver := &mockSignalResolver{signal: expectedSignal}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-f9-001",
				Action:  mcptools.ActionMessage,
				Message: "Pod keeps crashing with OOMKilled",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			Expect(runner.capturedCtx).NotTo(BeNil(), "RunInteractiveTurn must receive a non-nil context")
			signal, ok := katypes.SignalContextFromContext(runner.capturedCtx)
			Expect(ok).To(BeTrue(), "context must carry SignalContext after F9 fix")
			Expect(signal.ResourceKind).To(Equal("Deployment"))
			Expect(signal.IncidentID).To(Equal("inc-f9-001"))
			Expect(signal.Namespace).To(Equal("production"))
		})
	})
})

var _ = Describe("kubernaut_investigate — catalog name enrichment", func() {
	Describe("UT-KA-DW-010: discover_workflows enriches Name from WorkflowCatalog", func() {
		It("should populate Name on DiscoveredWorkflow from catalog lookup", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-dw-010",
				CorrelationID: "rr-dw-010",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{
				{Role: "user", Content: "my pod is crashing"},
				{Role: "assistant", Content: "I see OOM errors"},
			}}
			resolver := &mockSignalResolver{}
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Increase Memory Limit"},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithWorkflowCatalog(catalog))
			output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-010",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("workflows_discovered"))

			Expect(sess.DiscoveryResult).NotTo(BeNil())
			Expect(sess.DiscoveryResult.Recommended).NotTo(BeNil())
			Expect(sess.DiscoveryResult.Recommended.Name).To(Equal("Increase Memory Limit"))
		})
	})

	Describe("UT-KA-DW-011: discover_workflows fails closed when catalog is nil", func() {
		It("should return an error if WorkflowCatalog is not wired", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "sess-dw-011",
				CorrelationID: "rr-dw-011",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}
			resolver := &mockSignalResolver{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{},
				mcptools.WithSignalContextResolver(resolver))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-dw-011",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workflow catalog"))
		})
	})
})
