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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("SSE Delivery Integration — #823 PR7", func() {

	Describe("IT-KA-823-D01: Subscribe triggers event sink — events flow to subscriber", func() {
		It("events emitted after Subscribe are received by subscriber", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			subscribed := make(chan struct{})
			proceed := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-subscribed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeReasoningDelta,
						Turn:  0,
						Phase: "rca",
					}
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeToolCallStart,
						Turn:  0,
						Phase: "rca",
					}
				}
				<-proceed
				return map[string]string{"rca_summary": "delivered"}, nil
			}, map[string]string{"remediation_id": "rr-sse-delivery"})
			Expect(err).NotTo(HaveOccurred())

			ch, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			close(subscribed)

			var events []session.InvestigationEvent
			Eventually(func() int {
				for {
					select {
					case ev, ok := <-ch:
						if !ok {
							return len(events)
						}
						events = append(events, ev)
					default:
						return len(events)
					}
				}
			}, 5*time.Second).Should(BeNumerically(">=", 2),
				"subscriber should receive events emitted after Subscribe triggers the sink")

			close(proceed)
		})
	})

	Describe("IT-KA-823-D02: Client disconnect does not block investigation", func() {
		It("investigation completes even if subscriber stops reading", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			subscribed := make(chan struct{})

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-subscribed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					for i := 0; i < 200; i++ {
						select {
						case sink <- session.InvestigationEvent{
							Type:  session.EventTypeReasoningDelta,
							Turn:  i,
							Phase: "rca",
						}:
						default:
						}
					}
				}
				return map[string]string{"rca_summary": "completed despite slow consumer"}, nil
			}, map[string]string{"remediation_id": "rr-disconnect"})
			Expect(err).NotTo(HaveOccurred())

			_, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			close(subscribed)

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return session.StatusPending
				}
				return sess.Status
			}, 10*time.Second).Should(Equal(session.StatusCompleted),
				"investigation must complete even when subscriber is slow/disconnected")
		})
	})
})
