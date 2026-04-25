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
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Lazy Event Sink — #823 PR7", func() {

	Describe("UT-KA-823-D05: Autonomous investigation receives no event sink", func() {
		It("EventSinkFromContext returns nil when no Subscribe has been called", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			sinkResult := make(chan bool, 1)
			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				sink := session.EventSinkFromContext(ctx)
				sinkResult <- (sink == nil)
				return map[string]string{"rca_summary": "autonomous"}, nil
			}, map[string]string{"remediation_id": "rr-lazy-test"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(sinkResult, 5*time.Second).Should(Receive(BeTrue()),
				"autonomous investigation (no Subscribe) must receive nil event sink")
		})
	})

	Describe("UT-KA-823-D06: Panic in investigation goroutine transitions session to Failed", func() {
		It("session reaches StatusFailed and goroutine does not crash the process", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				panic("simulated investigation panic")
			}, map[string]string{"remediation_id": "rr-panic-test"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return session.StatusPending
				}
				return sess.Status
			}, 5*time.Second).Should(Equal(session.StatusFailed),
				"panic in investigation should transition session to Failed")
		})
	})

	Describe("UT-KA-823-D07: EventTypeComplete emitted at investigation end", func() {
		It("subscriber receives a complete event before channel closure", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return map[string]string{"rca_summary": "done"}, nil
			}, map[string]string{"remediation_id": "rr-complete-evt"})
			Expect(err).NotTo(HaveOccurred())

			ch, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			close(proceed)

			var sawComplete bool
			Eventually(func() bool {
				for {
					select {
					case ev, ok := <-ch:
						if !ok {
							return sawComplete
						}
						if ev.Type == session.EventTypeComplete {
							sawComplete = true
						}
					default:
						return sawComplete
					}
				}
			}, 5*time.Second).Should(BeTrue(),
				"subscriber must receive EventTypeComplete before channel closure")
		})
	})

	Describe("UT-KA-823-D08: EventTypeSessionObserved emitted on Subscribe", func() {
		It("audit store receives session.observed event when Subscribe is called", func() {
			store := session.NewStore(30 * time.Minute)
			recorder := &auditRecorder{}
			mgr := session.NewManager(store, slog.Default(), recorder)

			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return nil, nil
			}, map[string]string{"remediation_id": "rr-observed"})
			Expect(err).NotTo(HaveOccurred())

			_, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())

			close(proceed)

			Eventually(func() bool {
				for _, evt := range recorder.Events() {
					if evt.EventType == audit.EventTypeSessionObserved {
						return true
					}
				}
				return false
			}, 5*time.Second).Should(BeTrue(),
				"Subscribe must emit aiagent.session.observed audit event")
		})
	})
})

type auditRecorder struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (r *auditRecorder) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *auditRecorder) Events() []*audit.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(r.events))
	copy(cp, r.events)
	return cp
}
