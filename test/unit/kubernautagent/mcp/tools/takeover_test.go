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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// takeoverAutoMgr mocks the AutonomousSessionManager interface for takeover tests.
type takeoverAutoMgr struct {
	findResult   string
	findOK       bool
	cancelErr    error
	cancelCalled atomic.Int32
	cancelDelay  time.Duration
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
		tool = tools.NewInvestigateTool(sessMgr, runner, recon, autoMgr)
		ctx = context.Background()
	})

	Describe("UT-KA-TAKE-001: Takeover timing race — cancel returns ErrSessionTerminal when investigation already completed", func() {
		It("should return ErrCodeInvestigationCompleted when autonomous session is already terminal", func() {
			autoMgr.cancelErr = session.ErrSessionTerminal

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionTakeover,
			}
			_, err := tool.Handle(ctx, input, testUser)
			Expect(err).To(HaveOccurred())

			var mcpErr *tools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeTrue(), "error should be *MCPError")
			Expect(mcpErr.Code).To(Equal("investigation_completed"))
			Expect(mcpErr.Message).NotTo(BeEmpty())
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

	Describe("UT-KA-SESS-007: Double-complete is idempotent (no error on second release)", func() {
		It("should not error when completing an already-released session", func() {
			callCount := 0
			sessMgr.releaseErr = nil
			originalRelease := sessMgr.Release
			_ = originalRelease

			input := tools.InvestigateInput{
				RRID:   "rr-001",
				Action: tools.ActionComplete,
			}

			// First complete succeeds
			out, err := tool.Handle(ctx, input, testUser)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed"))
			callCount++

			// After release, driver is no longer active
			sessMgr.driverActive = false
			sessMgr.driverSession = nil
			sessMgr.releaseErr = mcpinternal.ErrSessionNotFound

			// Second complete: session already released → idempotent
			_, err = tool.Handle(ctx, input, testUser)
			Expect(err).NotTo(HaveOccurred())
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
})
