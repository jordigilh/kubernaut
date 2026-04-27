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

var _ = Describe("Session Manager Stream Integration — #823 PR4", func() {

	Describe("IT-KA-823-S04: Lazy event sink activated by Subscribe", func() {
		It("investigation function receives non-nil event sink after Subscribe", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			subscribed := make(chan struct{})
			sinkReceived := make(chan bool, 1)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-subscribed
				sink := session.EventSinkFromContext(ctx)
				sinkReceived <- (sink != nil)
				return map[string]string{"rca_summary": "test"}, nil
			}, map[string]string{"remediation_id": "rr-stream-test"})

			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			_, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			close(subscribed)

			Eventually(sinkReceived, 5*time.Second).Should(Receive(BeTrue()),
				"investigation function must receive a non-nil event sink after Subscribe activates the lazy sink")
		})
	})

	Describe("IT-KA-823-S02: Investigation completes — event channel closed", func() {
		It("subscriber channel is closed when investigation finishes", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return map[string]string{"rca_summary": "done"}, nil
			}, map[string]string{"remediation_id": "rr-stream-close"})
			Expect(err).NotTo(HaveOccurred())

			ch, subErr := mgr.Subscribe(context.Background(), id)
			Expect(subErr).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			close(proceed)

			Eventually(func() bool {
				_, ok := <-ch
				return ok
			}, 5*time.Second).Should(BeFalse(), "channel should be closed after investigation completes")
		})
	})

	Describe("IT-KA-823-S01: Full event flow — events arrive from investigation via event sink", func() {
		It("events emitted by investigation function are received by subscriber", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			subscribed := make(chan struct{})
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
				return map[string]string{"rca_summary": "test result"}, nil
			}, map[string]string{"remediation_id": "rr-flow-test"})
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
				"subscriber should receive at least 2 events emitted by the investigation")
		})
	})
})
