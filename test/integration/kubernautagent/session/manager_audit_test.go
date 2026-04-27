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
	"errors"
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
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

func (s *spyAuditStore) getEvents() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(s.events))
	copy(cp, s.events)
	return cp
}

func (s *spyAuditStore) eventsOfType(eventType string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

type failingAuditStore struct {
	mu     sync.Mutex
	calls  int
}

func (f *failingAuditStore) StoreAudit(_ context.Context, _ *audit.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return errors.New("ds unavailable")
}

func (f *failingAuditStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var _ = Describe("Kubernaut Agent Session Audit Trail — #823 PR 1.5", func() {

	var (
		store *session.Store
		spy   *spyAuditStore
		mgr   *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		spy = &spyAuditStore{}
		mgr = session.NewManager(store, slog.Default(), spy, nil)
	})

	Describe("IT-KA-823-A01: StartInvestigation emits session.started audit event", func() {
		It("should emit a session.started event with session_id, remediation_id, and incident metadata (GAP-T8)", func() {
			userCtx := context.WithValue(context.Background(), auth.UserContextKey, "operator-alice")
			metadata := map[string]string{
				"remediation_id": "rr-123",
				"incident_id":    "inc-456",
				"signal_name":    "OOMKilled",
				"severity":       "critical",
			}
			id, err := mgr.StartInvestigation(userCtx, func(ctx context.Context) (interface{}, error) {
				return "result", nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			Eventually(func() []*audit.AuditEvent {
				return spy.eventsOfType(audit.EventTypeSessionStarted)
			}, 2*time.Second, 10*time.Millisecond).Should(HaveLen(1))

			started := spy.eventsOfType(audit.EventTypeSessionStarted)[0]
			Expect(started.CorrelationID).To(Equal("rr-123"))
			Expect(started.Data).To(HaveKeyWithValue("session_id", id))
			Expect(started.EventAction).To(Equal(audit.ActionSessionStarted))
			Expect(started.EventCategory).To(Equal(audit.EventCategory))

			By("verifying incident metadata fields (AUD-2 / GAP-T8)")
			Expect(started.Data).To(HaveKeyWithValue("incident_id", "inc-456"))
			Expect(started.Data).To(HaveKeyWithValue("signal_name", "OOMKilled"))
			Expect(started.Data).To(HaveKeyWithValue("severity", "critical"))
			Expect(started.Data).To(HaveKeyWithValue("created_by", "operator-alice"))
		})
	})

	Describe("IT-KA-823-A02: Successful investigation emits session.completed", func() {
		It("should emit a session.completed event with outcome=success", func() {
			metadata := map[string]string{"remediation_id": "rr-complete"}
			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "done", nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() []*audit.AuditEvent {
				return spy.eventsOfType(audit.EventTypeSessionCompleted)
			}, 2*time.Second, 10*time.Millisecond).Should(HaveLen(1))

			completed := spy.eventsOfType(audit.EventTypeSessionCompleted)[0]
			Expect(completed.CorrelationID).To(Equal("rr-complete"))
			Expect(completed.EventOutcome).To(Equal(audit.OutcomeSuccess))
			Expect(completed.EventAction).To(Equal(audit.ActionSessionCompleted))
		})
	})

	Describe("IT-KA-823-A03: Failed investigation emits session.failed", func() {
		It("should emit a session.failed event with outcome=failure and error detail", func() {
			metadata := map[string]string{"remediation_id": "rr-fail"}
			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return nil, errors.New("llm timeout")
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() []*audit.AuditEvent {
				return spy.eventsOfType(audit.EventTypeSessionFailed)
			}, 2*time.Second, 10*time.Millisecond).Should(HaveLen(1))

			failed := spy.eventsOfType(audit.EventTypeSessionFailed)[0]
			Expect(failed.CorrelationID).To(Equal("rr-fail"))
			Expect(failed.EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(failed.EventAction).To(Equal(audit.ActionSessionFailed))
			Expect(failed.Data).To(HaveKeyWithValue("error", "llm timeout"))
		})
	})

	Describe("IT-KA-823-A04: CancelInvestigation emits session.cancelled", func() {
		It("should emit a session.cancelled event when a running investigation is cancelled", func() {
			metadata := map[string]string{"remediation_id": "rr-cancel"}
			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return nil, ctx.Err()
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(mgr.CancelInvestigation(id)).To(Succeed())
			close(proceed)

			Eventually(func() []*audit.AuditEvent {
				return spy.eventsOfType(audit.EventTypeSessionCancelled)
			}, 2*time.Second, 10*time.Millisecond).Should(HaveLen(1))

			cancelled := spy.eventsOfType(audit.EventTypeSessionCancelled)[0]
			Expect(cancelled.CorrelationID).To(Equal("rr-cancel"))
			Expect(cancelled.EventOutcome).To(Equal(audit.OutcomeSuccess))
			Expect(cancelled.EventAction).To(Equal(audit.ActionSessionCancelled))
			Expect(cancelled.Data).To(HaveKeyWithValue("session_id", id))
		})
	})

	Describe("IT-KA-823-A05: Cancelled session does not emit phantom completed/failed", func() {
		It("should only emit started and cancelled, not completed or failed", func() {
			metadata := map[string]string{"remediation_id": "rr-phantom"}
			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return nil, ctx.Err()
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(mgr.CancelInvestigation(id)).To(Succeed())
			close(proceed)

			// Wait for goroutine to finish
			Eventually(func() bool {
				_, subErr := mgr.Subscribe(context.Background(), id)
				return errors.Is(subErr, session.ErrSessionTerminal)
			}, 2*time.Second, 10*time.Millisecond).Should(BeTrue())

			// Allow any pending audit events to be emitted
			time.Sleep(50 * time.Millisecond)

			events := spy.getEvents()
			for _, e := range events {
				Expect(e.EventType).NotTo(Equal(audit.EventTypeSessionCompleted),
					"cancelled session should not emit session.completed")
				Expect(e.EventType).NotTo(Equal(audit.EventTypeSessionFailed),
					"cancelled session should not emit session.failed")
			}

			Expect(spy.eventsOfType(audit.EventTypeSessionStarted)).To(HaveLen(1))
			Expect(spy.eventsOfType(audit.EventTypeSessionCancelled)).To(HaveLen(1))
		})
	})

	Describe("IT-KA-823-A06: Audit events carry SOC2 correlation fields", func() {
		It("should include remediation_id as CorrelationID and session_id in event data", func() {
			metadata := map[string]string{
				"remediation_id": "rr-soc2-456",
				"incident_id":    "inc-soc2-789",
			}
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "soc2-ok", nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() []*audit.AuditEvent {
				return spy.eventsOfType(audit.EventTypeSessionCompleted)
			}, 2*time.Second, 10*time.Millisecond).Should(HaveLen(1))

			for _, event := range spy.getEvents() {
				Expect(event.CorrelationID).To(Equal("rr-soc2-456"),
					"all events should have remediation_id as CorrelationID, got type: %s", event.EventType)
				Expect(event.Data).To(HaveKeyWithValue("session_id", id),
					"all events should carry session_id in data, got type: %s", event.EventType)
				Expect(event.EventCategory).To(Equal(audit.EventCategory),
					"all events should have EventCategory=aiagent, got type: %s", event.EventType)
			}
		})
	})

	Describe("IT-KA-823-A07: Nil AuditStore defaults to NopAuditStore", func() {
		It("should not panic when AuditStore is nil", func() {
			nilStore := session.NewStore(5 * time.Minute)
			nilMgr := session.NewManager(nilStore, slog.Default(), nil, nil)

			id, err := nilMgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "safe", nil
			}, map[string]string{"remediation_id": "rr-nil"})
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			Eventually(func() session.Status {
				sess, _ := nilMgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))
		})
	})

	Describe("IT-KA-823-A08: Audit store error does not abort investigation", func() {
		It("should complete the investigation even when audit store fails", func() {
			failStore := &failingAuditStore{}
			failMgr := session.NewManager(session.NewStore(5*time.Minute), slog.Default(), failStore, nil)

			id, err := failMgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "resilient", nil
			}, map[string]string{"remediation_id": "rr-fail-audit"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := failMgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			Expect(failStore.callCount()).To(BeNumerically(">", 0),
				"audit store should have been called at least once")
		})
	})
})
