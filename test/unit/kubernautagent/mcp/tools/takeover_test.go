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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// recordingAuditStore captures audit events for assertion in regression tests.
type recordingAuditStore struct {
	events []*audit.AuditEvent
	mu     sync.Mutex
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

// takeoverAutoMgr mocks the AutonomousSessionManager interface for takeover tests.
type takeoverAutoMgr struct {
	findResult    string
	findOK        bool
	cancelErr     error
	suspendErr    error
	cancelCalled  atomic.Int32
	suspendCalled atomic.Int32
	cancelDelay   time.Duration
}

func (m *takeoverAutoMgr) FindByRemediationID(_ string) (string, bool) {
	return m.findResult, m.findOK
}

func (m *takeoverAutoMgr) CancelInvestigation(_ string) error {
	m.cancelCalled.Add(1)
	if m.cancelDelay > 0 {
		time.Sleep(m.cancelDelay)
	}
	return m.cancelErr
}

func (m *takeoverAutoMgr) SuspendInvestigation(_ string) error {
	m.suspendCalled.Add(1)
	if m.cancelDelay > 0 {
		time.Sleep(m.cancelDelay)
	}
	if m.suspendErr != nil {
		return m.suspendErr
	}
	return m.cancelErr
}

// takeoverSessMgr mocks mcpinternal.SessionManager for takeover tests.
type takeoverSessMgr struct {
	takeoverSession *mcpinternal.InteractiveSession
	takeoverErr     error
	releaseErr      error
	driverSession   *mcpinternal.InteractiveSession
	driverActive    bool
	releaseCalled   atomic.Int32
}

func (m *takeoverSessMgr) Takeover(_ context.Context, _ string, _ mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	return m.takeoverSession, m.takeoverErr
}

func (m *takeoverSessMgr) Release(_ string, _ string) error {
	m.releaseCalled.Add(1)
	return m.releaseErr
}

func (m *takeoverSessMgr) GetDriver(_ string) (*mcpinternal.InteractiveSession, error) {
	return m.driverSession, nil
}

func (m *takeoverSessMgr) IsDriverActive(_ string) bool {
	return m.driverActive
}

func (m *takeoverSessMgr) TouchActivity(_ string) {}

// takeoverRunner mocks tools.InvestigatorRunner for takeover tests.
type takeoverRunner struct {
	response string
	err      error
	delay    time.Duration
	called   atomic.Int32
}

func (m *takeoverRunner) RunInteractiveTurn(_ context.Context, _ []tools.LLMMessage, _ string) (string, error) {
	m.called.Add(1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.response, m.err
}

// takeoverRecon mocks mcpinternal.ContextReconstructor for takeover tests.
type takeoverRecon struct {
	turns []mcpinternal.ConversationTurn
	err   error
}

func (m *takeoverRecon) Reconstruct(_ context.Context, _ string, _ string) ([]mcpinternal.ConversationTurn, error) {
	return m.turns, m.err
}

var _ = Describe("kubernaut_investigate — Dynamic Takeover (PR4, BR-INTERACTIVE-004)", func() {

	var (
		autoMgr   *takeoverAutoMgr
		sessMgr   *takeoverSessMgr
		runner    *takeoverRunner
		recon     *takeoverRecon
		tool      *tools.InvestigateTool
		ctx       context.Context
		testUser  mcpinternal.UserInfo
		otherUser mcpinternal.UserInfo
	)

	BeforeEach(func() {
		testUser = mcpinternal.UserInfo{Username: "alice@example.com", Groups: []string{"sre"}}
		otherUser = mcpinternal.UserInfo{Username: "bob@example.com", Groups: []string{"dev"}}
		autoMgr = &takeoverAutoMgr{
			findResult: "auto-session-123",
			findOK:     true,
		}
		sessMgr = &takeoverSessMgr{
			takeoverSession: &mcpinternal.InteractiveSession{
				SessionID:     "interactive-session-456",
				CorrelationID: "rr-001",
				ActingUser:    testUser,
				StartedAt:     time.Now(),
			},
			driverActive: true,
			driverSession: &mcpinternal.InteractiveSession{
				SessionID:     "interactive-session-456",
				CorrelationID: "rr-001",
				ActingUser:    testUser,
			},
		}
		runner = &takeoverRunner{response: "LLM response here"}
		recon = &takeoverRecon{}
		tool = tools.NewInvestigateTool(sessMgr, runner, recon, tools.WithAutonomousManager(autoMgr))
		ctx = context.Background()
	})

	Describe("UT-KA-TAKE-001: Takeover timing race — suspend returns ErrSessionTerminal after lease acquired", func() {
		It("should succeed with takeover_started when autonomous session is already terminal (H4: lease-first)", func() {
			autoMgr.cancelErr = session.ErrSessionTerminal

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionTakeover,
			}
			out, err := tool.Handle(ctx, input, testUser)
			Expect(err).NotTo(HaveOccurred(), "H4: ErrSessionTerminal after lease acquired is not an error")
			Expect(out.Status).To(Equal("takeover_started"))
			Expect(out.SessionID).NotTo(BeEmpty())
			Expect(autoMgr.suspendCalled.Load()).To(Equal(int32(1)))
		})
	})

	Describe("UT-KA-TAKE-004: Concurrent takeover rejection — second takeover gets ErrLeaseHeld", func() {
		It("should return ErrCodeSessionActive when Lease is held by another driver", func() {
			sessMgr.takeoverErr = mcpinternal.ErrLeaseHeld

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionTakeover,
			}
			_, err := tool.Handle(ctx, input, otherUser)
			Expect(err).To(HaveOccurred())

			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"))
			Expect(mcpErr.Details).To(HaveKey("driver"))
		})
	})

	Describe("UT-KA-TAKE-005: Explicit takeover required — action=message without prior takeover returns structured error", func() {
		It("should return ErrCodeNotDriving when no takeover has been performed", func() {
			sessMgr.driverActive = false
			sessMgr.driverSession = nil

			input := tools.InvestigateInput{
				RRID:    "rr-002",
				Action:  tools.ActionMessage,
				Message: "hello",
			}
			_, err := tool.Handle(ctx, input, testUser)
			Expect(err).To(HaveOccurred())

			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("not_driving"))
		})
	})

	Describe("UT-KA-SESS-001: Session hijacking — different user cannot send message on another's session", func() {
		It("should reject message from non-driver user", func() {
			sessMgr.driverSession = &mcpinternal.InteractiveSession{
				SessionID:     "interactive-session-456",
				CorrelationID: "rr-001",
				ActingUser:    testUser,
			}

			input := tools.InvestigateInput{
				RRID:    "rr-001",
				Action:  tools.ActionMessage,
				Message: "I'm not the driver",
			}
			_, err := tool.Handle(ctx, input, otherUser)
			Expect(err).To(HaveOccurred())

			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("session_active"))
		})
	})

	Describe("UT-KA-SESS-003: Concurrent lock contention — two messages serialize (second waits)", func() {
		It("should serialize concurrent messages on the same session via mutex", func() {
			runner.delay = 50 * time.Millisecond

			input := tools.InvestigateInput{
				RRID:    "rr-001",
				Action:  tools.ActionMessage,
				Message: "msg",
			}

			var wg sync.WaitGroup
			var order []int
			var orderMu sync.Mutex

			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = tool.Handle(ctx, input, testUser)
				orderMu.Lock()
				order = append(order, 1)
				orderMu.Unlock()
			}()

			time.Sleep(5 * time.Millisecond) // Ensure first call enters mutex first
			go func() {
				defer wg.Done()
				_, _ = tool.Handle(ctx, input, testUser)
				orderMu.Lock()
				order = append(order, 2)
				orderMu.Unlock()
			}()

			wg.Wait()
			Expect(runner.called.Load()).To(Equal(int32(2)), "both calls should have been processed")
			Expect(order).To(Equal([]int{1, 2}), "second call should wait for first to complete")
		})
	})

	Describe("UT-KA-SESS-007: Double-complete returns not_found on second call", func() {
		It("should return structured not_found error when session already released", func() {
			sessMgr.releaseErr = nil

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionComplete,
			}

			// First complete succeeds
			out, err := tool.Handle(ctx, input, testUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))

			// After release, driver is no longer active
			sessMgr.driverActive = false
			sessMgr.driverSession = nil
			sessMgr.releaseErr = mcpinternal.ErrSessionNotFound

			// Second complete: session already released → structured error
			_, err = tool.Handle(ctx, input, testUser)
			Expect(err).To(HaveOccurred())
			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal("not_found"))
		})
	})

	Describe("UT-KA-SESS-008: Message on completed session returns ErrNoActiveSession", func() {
		It("should return structured not_driving error for message after session completes", func() {
			sessMgr.driverActive = false
			sessMgr.driverSession = nil

			input := tools.InvestigateInput{
				RRID:    "rr-001",
				Action:  tools.ActionMessage,
				Message: "hello",
			}
			_, err := tool.Handle(ctx, input, testUser)
			Expect(err).To(HaveOccurred())

			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue())
			Expect(mcpErr.Code).To(Equal("not_driving"))
		})
	})

	Describe("UT-KA-ERR-001: Structured errors contain code + human message + details", func() {
		It("should produce errors satisfying the error interface with structured fields", func() {
			mcpErr := &tools.MCPError{
				Code:    "session_active",
				Message: "Investigation is being driven by another user",
				Details: map[string]string{"driver": "alice@example.com"},
			}

			Expect(mcpErr.Error()).To(ContainSubstring("session_active"))
			Expect(mcpErr.Error()).To(ContainSubstring("Investigation is being driven by another user"))
			Expect(mcpErr.Code).To(Equal("session_active"))
			Expect(mcpErr.Message).NotTo(BeEmpty())
			Expect(mcpErr.Details["driver"]).To(Equal("alice@example.com"))
		})
	})

	// Regression test: H3 — handleComplete must emit interactive.completed audit
	// even when Release returns ErrSessionNotFound (race with timeout/disconnect).
	// Bug: early-return skipped emitInteractiveCompleted, leaving no audit trail
	// for sessions that were auto-released between driver validation and complete.
	Describe("UT-KA-TAKE-H3: Complete emits audit when Release returns ErrSessionNotFound (H3 regression)", func() {
		It("should emit interactive.completed audit even when session was already released", func() {
			auditRecorder := &recordingAuditStore{}
			toolWithAudit := tools.NewInvestigateTool(sessMgr, runner, recon,
				tools.WithAutonomousManager(autoMgr),
				tools.WithAuditStore(auditRecorder, logr.Discard()),
			)

			// Session is active and driver is correct user — but Release will fail.
			sessMgr.releaseErr = mcpinternal.ErrSessionNotFound

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionComplete,
			}
			out, err := toolWithAudit.Handle(ctx, input, testUser)
			Expect(err).NotTo(HaveOccurred(), "H3: ErrSessionNotFound on Release should not propagate")
			Expect(out.Status).To(Equal("completed"))

			// H3 FIX: audit must be emitted with reason "complete_already_released"
			Expect(auditRecorder.events).NotTo(BeEmpty(),
				"H3: interactive.completed audit must be emitted even on ErrSessionNotFound")
			found := false
			for _, e := range auditRecorder.events {
				if e.EventType == "aiagent.interactive.completed" {
					Expect(e.Data["reason"]).To(Equal("complete_already_released"))
					found = true
				}
			}
			Expect(found).To(BeTrue(), "should find aiagent.interactive.completed event with reason=complete_already_released")
		})
	})
})
