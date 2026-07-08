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
	"encoding/json"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type spyAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *spyAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *spyAuditStore) Events() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(s.events))
	copy(cp, s.events)
	return cp
}

var _ = Describe("Fix #1390: Jump-In Session Upgrade (BR-INTERACTIVE-004, BR-REL-014)", func() {

	var (
		store   *session.Store
		manager *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
	})

	Describe("UT-KA-1390-001 [SC-24]: UpgradeToInteractive sets atomic flag without cancelling goroutine", func() {
		It("should set upgrade flag while keeping session running and goroutine alive", func() {
			doneCh := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(doneCh)
				return &katypes.InvestigationResult{RCASummary: "cancelled"}, nil
			}, map[string]string{"remediation_id": "rr-upgrade-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			err = manager.UpgradeToInteractive(id, "testuser", []string{"group1"})
			Expect(err).NotTo(HaveOccurred())

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusRunning),
				"status must remain running — upgrade does not change it")

			Consistently(doneCh, 200*time.Millisecond).ShouldNot(BeClosed(),
				"goroutine must NOT be cancelled by upgrade")
		})
	})

	Describe("UT-KA-1390-002 [AU-12]: UpgradeToInteractive writes acting_user and groups to metadata", func() {
		It("should populate acting_user and acting_user_groups in session metadata", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-upgrade-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			err = manager.UpgradeToInteractive(id, "alice", []string{"sre-team", "on-call"})
			Expect(err).NotTo(HaveOccurred())

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Metadata["acting_user"]).To(Equal("alice"))
			Expect(sess.Metadata["acting_user_groups"]).To(ContainSubstring("sre-team"))
			Expect(sess.Metadata["acting_user_groups"]).To(ContainSubstring("on-call"))
		})
	})

	Describe("UT-KA-1390-003 [SI-10]: UpgradeToInteractive on terminal session returns ErrSessionTerminal", func() {
		It("should reject upgrade on a completed session", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, map[string]string{"remediation_id": "rr-upgrade-003"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted))

			err = manager.UpgradeToInteractive(id, "bob", nil)
			Expect(err).To(MatchError(session.ErrSessionTerminal))
		})
	})

	Describe("UT-KA-1390-004 [SI-10]: UpgradeToInteractive on non-existent session returns ErrSessionNotFound", func() {
		It("should return ErrSessionNotFound for unknown ID", func() {
			err := manager.UpgradeToInteractive("nonexistent-session-id", "bob", nil)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1390-007 [SC-24]: Goroutine with upgrade flag stores result via Update(running->user_driving)", func() {
		It("should transition to user_driving with result when upgrade flag is set before InteractiveHold check", func() {
			upgradeDone := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-upgradeDone
				return &katypes.InvestigationResult{
					RCASummary:      "RCA with upgrade",
					Confidence:      0.95,
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-upgrade-007"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.UpgradeToInteractive(id, "testuser", nil)).To(Succeed())
			close(upgradeDone)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusUserDriving))

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.InteractiveHold).To(BeTrue())
			Expect(sess.Result.RCASummary).To(Equal("RCA with upgrade"))
		})
	})

	Describe("UT-KA-1390-008 [SI-4]: Event channel stays open after upgrade + InteractiveHold completion", func() {
		It("should keep event channel open for post-RCA streaming (workflow discovery)", func() {
			ready := make(chan struct{})
			proceed := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(ready)
				<-proceed
				return &katypes.InvestigationResult{
					RCASummary:      "RCA complete",
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-upgrade-008"})
			Expect(err).NotTo(HaveOccurred())

			<-ready
			Expect(manager.UpgradeToInteractive(id, "testuser", nil)).To(Succeed())
			close(proceed)

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusUserDriving))

			ch, err := manager.Subscribe(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil(), "event channel must be open for post-RCA events")
		})
	})

	Describe("UT-KA-1390-009 [SI-4]: LazySink activation post-upgrade delivers events to subscriber", func() {
		It("should deliver events via LazySink after Subscribe on an upgraded session", func() {
			eventSent := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				sink := session.EventSinkFromContext(ctx)
				for sink == nil {
					time.Sleep(10 * time.Millisecond)
					sink = session.EventSinkFromContext(ctx)
				}
				sink <- session.InvestigationEvent{Type: session.EventTypeReasoningDelta, Data: json.RawMessage(`"streaming-after-upgrade"`)}
				close(eventSent)
				return &katypes.InvestigationResult{
					RCASummary:      "streamed RCA",
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-upgrade-009"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(manager.UpgradeToInteractive(id, "testuser", nil)).To(Succeed())

			ch, err := manager.Subscribe(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())

			Eventually(eventSent, 2*time.Second).Should(BeClosed())
			Eventually(ch, 500*time.Millisecond).Should(Receive(HaveField("Data", json.RawMessage(`"streaming-after-upgrade"`))))
		})
	})

	Describe("Store-Level Deterministic Upgrade (eliminates race window)", func() {

		Context("UT-KA-1390-027 [SC-24]: store.Update with StatusCompleted + interactiveUpgrade=true forces StatusUserDriving", func() {
			It("should force user_driving and set InteractiveHold when upgrade flag was set before completion", func() {
				// Issue #1631: the investigation function must not return until
				// UpgradeToInteractive has run, otherwise the goroutine can win the
				// race and call store.Update(StatusCompleted) before the upgrade
				// flag is set — collapsing this into the UT-KA-1390-029 scenario
				// (ErrSessionTerminal) instead of exercising the intended
				// "upgrade before completion" ordering. Same ready/proceed gate
				// used by UT-KA-1390-008 above.
				ready := make(chan struct{})
				proceed := make(chan struct{})
				id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
					close(ready)
					<-proceed
					return &katypes.InvestigationResult{
						RCASummary:      "Autonomous RCA — investigator did not see upgrade flag",
						Confidence:      0.9,
						InteractiveHold: false,
					}, nil
				}, map[string]string{"remediation_id": "rr-upgrade-027"})
				Expect(err).NotTo(HaveOccurred())

				<-ready
				Expect(manager.UpgradeToInteractive(id, "testuser", nil)).To(Succeed())
				close(proceed)

				Eventually(func() session.Status {
					s, _ := manager.GetSession(id)
					if s == nil {
						return ""
					}
					return s.Status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusUserDriving),
					"store.Update must force user_driving when interactiveUpgrade is set")

				sess, err := manager.GetSession(id)
				Expect(err).NotTo(HaveOccurred())
				Expect(sess.Result).NotTo(BeNil())
				Expect(sess.Result.InteractiveHold).To(BeTrue(),
					"store.Update must set InteractiveHold=true on the result")
				Expect(sess.Result.RCASummary).To(Equal("Autonomous RCA — investigator did not see upgrade flag"))
			})
		})

		Context("UT-KA-1390-028 [SC-24]: store.Update with StatusCompleted + interactiveUpgrade=false remains StatusCompleted", func() {
			It("should not falsely promote to user_driving when no upgrade was requested", func() {
				id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
					return &katypes.InvestigationResult{
						RCASummary:      "Normal autonomous completion",
						InteractiveHold: false,
					}, nil
				}, map[string]string{"remediation_id": "rr-upgrade-028"})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() session.Status {
					s, _ := manager.GetSession(id)
					if s == nil {
						return ""
					}
					return s.Status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted))

				sess, err := manager.GetSession(id)
				Expect(err).NotTo(HaveOccurred())
				Expect(sess.Result).NotTo(BeNil())
				Expect(sess.Result.InteractiveHold).To(BeFalse(),
					"no upgrade flag — InteractiveHold must remain false")
			})
		})

		Context("UT-KA-1390-029 [SC-24]: UpgradeToInteractive after store.Update(completed) returns ErrSessionTerminal", func() {
			It("should return ErrSessionTerminal when goroutine won the lock race", func() {
				id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
					return &katypes.InvestigationResult{
						RCASummary:      "Fast autonomous completion",
						InteractiveHold: false,
					}, nil
				}, map[string]string{"remediation_id": "rr-upgrade-029"})
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() session.Status {
					s, _ := manager.GetSession(id)
					if s == nil {
						return ""
					}
					return s.Status
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted))

				err = manager.UpgradeToInteractive(id, "late-user", nil)
				Expect(err).To(MatchError(session.ErrSessionTerminal),
					"upgrade on a completed session must fail so handleStart falls back to ForceTransitionToUserDriving")
			})
		})
	})
})

// ========================================
// Fix #1390 Layer 4: Observability
// ========================================

var _ = Describe("Fix #1390 Layer 4: Session Observability", func() {
	var (
		manager  *session.Manager
		auditSpy *spyAuditStore
	)

	BeforeEach(func() {
		store := session.NewStore(5 * time.Minute)
		auditSpy = &spyAuditStore{}
		manager = session.NewManager(store, logr.Discard(), auditSpy, nil)
	})

	Context("UT-KA-1390-025 [SC-8, AU-12]: Autonomous session bgCtx carries session_id", func() {
		It("should propagate session_id into the investigation goroutine context", func() {
			var capturedSessionID string
			ready := make(chan struct{})
			proceed := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				capturedSessionID = session.SessionIDFromContext(ctx)
				close(ready)
				<-proceed
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			<-ready
			close(proceed)

			Expect(capturedSessionID).To(Equal(id),
				"bgCtx must carry session_id via WithSessionID for audit traceability")
		})
	})

	Context("UT-KA-1390-026 [AU-12]: UpgradeToInteractive emits audit event with session_id and acting_user", func() {
		It("should emit session.suspended audit event with correct fields on upgrade", func() {
			ready := make(chan struct{})
			proceed := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				close(ready)
				<-proceed
				return &katypes.InvestigationResult{
					RCASummary:      "RCA with upgrade",
					InteractiveHold: session.InteractiveUpgradeFromContext(ctx),
				}, nil
			}, map[string]string{"remediation_id": "rr-audit-026"})
			Expect(err).NotTo(HaveOccurred())

			<-ready
			err = manager.UpgradeToInteractive(id, "sre-auditor@example.com", []string{"sre-team"})
			Expect(err).NotTo(HaveOccurred())
			close(proceed)

			sess, getErr := manager.GetSession(id)
			Expect(getErr).NotTo(HaveOccurred())
			Expect(sess.Metadata["acting_user"]).To(Equal("sre-auditor@example.com"),
				"acting_user must be set in metadata for audit traceability")

			events := auditSpy.Events()
			var upgradeEvent *audit.AuditEvent
			for _, e := range events {
				if e.EventType == audit.EventTypeSessionSuspended && e.SessionID == id {
					upgradeEvent = e
					break
				}
			}
			Expect(upgradeEvent).NotTo(BeNil(),
				"UpgradeToInteractive must emit EventTypeSessionSuspended audit event")
			Expect(upgradeEvent.CorrelationID).To(Equal("rr-audit-026"),
				"audit event must carry correlation ID from session metadata")
			Expect(upgradeEvent.Data).To(HaveKeyWithValue("acting_user", "sre-auditor@example.com"),
				"audit event must include acting_user in data")
			Expect(upgradeEvent.Data).To(HaveKeyWithValue("upgrade_type", "jump_in"),
				"audit event must include upgrade_type=jump_in in data")
		})
	})
})
