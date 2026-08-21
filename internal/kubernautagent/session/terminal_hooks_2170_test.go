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

package session_test

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ============================================================================
// BR-AA-KA-065.11/.5: session.Manager TerminalHook/InteractiveUpgradeHook —
// DD-AA-KA-001 Amendment Gap 2 (no silent drop on out-of-band terminal
// completion) and Gap 1 (KA is the sole writer of AgentSession.Status.Interactive).
//
// These hooks are the single mechanism by which session.Manager notifies a
// CRD-status writer (agentsession.Dispatcher) that a session's outcome has
// been definitively committed. Exactly one call site ever fires a given
// hook for a given session -- whichever one actually wins the underlying
// store.Update/CompleteUserDriving/direct-mutation race.
// ============================================================================

// terminalHookRecorder is a thread-safe spy for TerminalHook invocations.
type terminalHookRecorder struct {
	mu    sync.Mutex
	calls []session.TerminalSnapshot
}

func (r *terminalHookRecorder) record(snap session.TerminalSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, snap)
}

func (r *terminalHookRecorder) Calls() []session.TerminalSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]session.TerminalSnapshot, len(r.calls))
	copy(cp, r.calls)
	return cp
}

// interactiveUpgradeHookRecorder is a thread-safe spy for
// InteractiveUpgradeHook invocations.
type interactiveUpgradeHookRecorder struct {
	mu    sync.Mutex
	calls []interactiveUpgradeCall
}

type interactiveUpgradeCall struct {
	SessionID     string
	RemediationID string
	Username      string
	Groups        []string
}

func (r *interactiveUpgradeHookRecorder) record(sessionID, remediationID, username string, groups []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, interactiveUpgradeCall{sessionID, remediationID, username, groups})
}

func (r *interactiveUpgradeHookRecorder) Calls() []interactiveUpgradeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]interactiveUpgradeCall, len(r.calls))
	copy(cp, r.calls)
	return cp
}

var _ = Describe("BR-AA-KA-065.11: session.Manager.TerminalHook — no silent drop on terminal completion", func() {
	var (
		store   *session.Store
		manager *session.Manager
		hooks   *terminalHookRecorder
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
		hooks = &terminalHookRecorder{}
		manager.SetTerminalHook(hooks.record)
	})

	Describe("UT-KA-2170-001: normal autonomous completion fires TerminalHook exactly once with StatusCompleted", func() {
		It("should invoke the hook with the completed session's result", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "leak found", Confidence: 0.9}, nil
			}, map[string]string{"remediation_id": "rr-2170-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))

			calls := hooks.Calls()
			Expect(calls[0].SessionID).To(Equal(id))
			Expect(calls[0].RemediationID).To(Equal("rr-2170-001"))
			Expect(calls[0].Status).To(Equal(session.StatusCompleted))
			Expect(calls[0].Result).NotTo(BeNil())
			Expect(calls[0].Result.RCASummary).To(Equal("leak found"))
			Expect(calls[0].Err).ToNot(HaveOccurred())
		})
	})

	Describe("UT-KA-2170-002: failed investigation fires TerminalHook exactly once with StatusFailed", func() {
		It("should invoke the hook with the underlying error", func() {
			boom := context.DeadlineExceeded
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return nil, boom
			}, map[string]string{"remediation_id": "rr-2170-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))

			calls := hooks.Calls()
			Expect(calls[0].SessionID).To(Equal(id))
			Expect(calls[0].Status).To(Equal(session.StatusFailed))
			Expect(calls[0].Err).To(MatchError(boom))
		})
	})

	Describe("UT-KA-2170-003: autonomous InteractiveHold fires TerminalHook with StatusUserDriving, no acting user", func() {
		It("should invoke the hook with StatusUserDriving and an empty acting-user identity", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "RCA complete", InteractiveHold: true}, nil
			}, map[string]string{"remediation_id": "rr-2170-003"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))

			calls := hooks.Calls()
			Expect(calls[0].SessionID).To(Equal(id))
			Expect(calls[0].Status).To(Equal(session.StatusUserDriving))
			Expect(calls[0].Result.InteractiveHold).To(BeTrue())
		})
	})

	Describe("UT-KA-2170-004: CompleteUserDriving fires TerminalHook exactly once with StatusCompleted", func() {
		It("should invoke the hook when an MCP tool completes a user-driving session", func() {
			ready := make(chan struct{})
			proceed := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(ready)
				<-proceed
				return &katypes.InvestigationResult{RCASummary: "autonomous RCA", InteractiveHold: false}, nil
			}, map[string]string{"remediation_id": "rr-2170-004"})
			Expect(err).NotTo(HaveOccurred())

			<-ready
			Expect(manager.UpgradeToInteractive(id, "alice", nil)).To(Succeed())
			close(proceed)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusUserDriving))

			// The InteractiveHold-via-upgrade transition fires TerminalHook once
			// already (StatusUserDriving) -- CompleteUserDriving fires it a
			// second time with the final StatusCompleted outcome.
			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))

			finalResult := &katypes.InvestigationResult{RCASummary: "autonomous RCA", WorkflowID: "restart-pod-v1"}
			Expect(manager.CompleteUserDriving(id, finalResult)).To(Succeed())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(2))

			calls := hooks.Calls()
			last := calls[len(calls)-1]
			Expect(last.SessionID).To(Equal(id))
			Expect(last.RemediationID).To(Equal("rr-2170-004"))
			Expect(last.Status).To(Equal(session.StatusCompleted))
			Expect(last.Result.WorkflowID).To(Equal("restart-pod-v1"))
		})
	})

	Describe("UT-KA-2170-005 [BR-AA-KA-065.11]: ForceCompleteByRemediationID racing a still-running goroutine fires TerminalHook exactly once with the out-of-band outcome, never the stale goroutine return", func() {
		It("should not let the cancelled goroutine's own return value overwrite the force-completed outcome", func() {
			gate := make(chan struct{})
			goroutineReturned := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done() // blocks until ForceCompleteByRemediationID cancels this goroutine
				close(goroutineReturned)
				return &katypes.InvestigationResult{RCASummary: "stale — must never be the hook's final value"}, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-005"})
			Expect(err).NotTo(HaveOccurred())
			close(gate)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			winningResult := &katypes.InvestigationResult{RCASummary: "operator closed with no action"}
			Expect(manager.ForceCompleteByRemediationID("rr-2170-005", winningResult)).To(Succeed())

			Eventually(goroutineReturned, 2*time.Second).Should(BeClosed(),
				"the cancelled goroutine must still run to completion and attempt its own (losing) write")

			// Give the goroutine's own rejected write a chance to race in, if
			// it were (incorrectly) going to fire the hook a second time.
			time.Sleep(50 * time.Millisecond)

			calls := hooks.Calls()
			Expect(calls).To(HaveLen(1),
				"BR-AA-KA-065.11: exactly one call site may commit the terminal transition; "+
					"the goroutine's own rejected store.Update must never re-fire the hook")
			Expect(calls[0].SessionID).To(Equal(id))
			Expect(calls[0].Status).To(Equal(session.StatusCompleted))
			Expect(calls[0].Result.RCASummary).To(Equal("operator closed with no action"),
				"the out-of-band ForceComplete outcome must win, never the stale goroutine return")
		})
	})

	Describe("UT-KA-2170-006: nil-safe when no hook is registered", func() {
		It("should not panic when TerminalHook is never set", func() {
			bareManager := session.NewManager(session.NewStore(5*time.Minute), logr.Discard(), nil, nil)
			Expect(func() {
				id, err := bareManager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
					return &katypes.InvestigationResult{RCASummary: "ok"}, nil
				}, map[string]string{"remediation_id": "rr-2170-006"})
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() session.Status {
					s, _ := bareManager.GetSession(id)
					if s == nil {
						return ""
					}
					return s.Status
				}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusCompleted))
			}).NotTo(Panic())
		})
	})
})

var _ = Describe("BR-AA-KA-065.5: session.Manager.InteractiveUpgradeHook — dispatch-agnostic interactive-takeover notification", func() {
	var (
		store   *session.Store
		manager *session.Manager
		hooks   *interactiveUpgradeHookRecorder
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
		hooks = &interactiveUpgradeHookRecorder{}
		manager.SetInteractiveUpgradeHook(hooks.record)
	})

	Describe("UT-KA-2170-010: UpgradeToInteractive (jump-in) fires InteractiveUpgradeHook with the acting user and groups", func() {
		It("should invoke the hook exactly once with sessionID/remediationID/username/groups", func() {
			gate := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-gate
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, map[string]string{"remediation_id": "rr-2170-010"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.UpgradeToInteractive(id, "alice", []string{"sre-team"})).To(Succeed())
			close(gate)

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))
			call := hooks.Calls()[0]
			Expect(call.SessionID).To(Equal(id))
			Expect(call.RemediationID).To(Equal("rr-2170-010"))
			Expect(call.Username).To(Equal("alice"))
			Expect(call.Groups).To(ConsistOf("sre-team"))
		})
	})

	Describe("UT-KA-2170-011: TransitionToUserDriving (dynamic takeover) fires InteractiveUpgradeHook", func() {
		It("should invoke the hook when a running session transitions to user-driving", func() {
			gate := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-011"})
			Expect(err).NotTo(HaveOccurred())
			close(gate)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.TransitionToUserDriving(id, "bob", []string{"on-call"})).To(Succeed())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))
			call := hooks.Calls()[0]
			Expect(call.SessionID).To(Equal(id))
			Expect(call.RemediationID).To(Equal("rr-2170-011"))
			Expect(call.Username).To(Equal("bob"))
		})
	})

	Describe("UT-KA-2170-012: ForceTransitionToUserDriving fires InteractiveUpgradeHook", func() {
		It("should invoke the hook when no running session is found by ID but one matches the remediation ID", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "already done"}, nil
			}, map[string]string{"remediation_id": "rr-2170-012"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusCompleted))

			Expect(manager.ForceTransitionToUserDriving("rr-2170-012", "carol", []string{"escalation"})).To(Succeed())

			Eventually(func() int { return len(hooks.Calls()) }, 2*time.Second, 20*time.Millisecond).Should(Equal(1))
			call := hooks.Calls()[0]
			Expect(call.SessionID).To(Equal(id))
			Expect(call.RemediationID).To(Equal("rr-2170-012"))
			Expect(call.Username).To(Equal("carol"))
		})
	})

	Describe("UT-KA-2170-013: nil-safe when no hook is registered", func() {
		It("should not panic when InteractiveUpgradeHook is never set", func() {
			bareManager := session.NewManager(session.NewStore(5*time.Minute), logr.Discard(), nil, nil)
			gate := make(chan struct{})
			id, err := bareManager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-gate
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, map[string]string{"remediation_id": "rr-2170-013"})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := bareManager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(func() {
				Expect(bareManager.UpgradeToInteractive(id, "dave", nil)).To(Succeed())
				close(gate)
			}).NotTo(Panic())
		})
	})
})
