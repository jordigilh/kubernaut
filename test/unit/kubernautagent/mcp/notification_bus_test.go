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

package mcp_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("NotificationBus — PR4 BR-INTERACTIVE-005 DD-INTERACTIVE-002", func() {

	Describe("UT-KA-BUS-001: Subscribe returns channel, receives published events", func() {
		It("should deliver published notifications to subscribers", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(10)
			ch := bus.Subscribe("rr-001", "sess-001")
			Expect(ch).NotTo(BeNil())

			bus.Publish("rr-001", mcpinternal.Notification{
				Type:          mcpinternal.NotificationAuditEvent,
				CorrelationID: "rr-001",
				SessionID:     "sess-001",
				Payload:       "event-1",
				Timestamp:     time.Now(),
			})

			Eventually(ch).Should(Receive(WithTransform(
				func(n mcpinternal.Notification) string { return n.Payload.(string) },
				Equal("event-1"),
			)))
		})
	})

	Describe("UT-KA-BUS-002: Unsubscribe stops delivery, channel closed", func() {
		It("should close the subscriber channel on unsubscribe", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(10)
			ch := bus.Subscribe("rr-002", "sess-002")

			bus.Unsubscribe("rr-002", "sess-002")

			Eventually(ch).Should(BeClosed())
		})
	})

	Describe("UT-KA-TAKE-008: NotificationBus ordering: events delivered in publish order", func() {
		It("should deliver events in the same order they were published", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(10)
			ch := bus.Subscribe("rr-003", "sess-003")

			for i := 0; i < 5; i++ {
				bus.Publish("rr-003", mcpinternal.Notification{
					Type:          mcpinternal.NotificationAuditEvent,
					CorrelationID: "rr-003",
					Payload:       i,
					Timestamp:     time.Now(),
				})
			}

			for i := 0; i < 5; i++ {
				Eventually(ch).Should(Receive(WithTransform(
					func(n mcpinternal.Notification) int { return n.Payload.(int) },
					Equal(i),
				)))
			}
		})
	})

	Describe("UT-KA-TAKE-009: NotificationBus slow consumer: publish does not block", func() {
		It("should not block the publisher when subscriber is slow", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(2) // small buffer
			_ = bus.Subscribe("rr-004", "sess-004")

			done := make(chan struct{})
			go func() {
				defer close(done)
				for i := 0; i < 10; i++ {
					bus.Publish("rr-004", mcpinternal.Notification{
						Type:          mcpinternal.NotificationAuditEvent,
						CorrelationID: "rr-004",
						Payload:       i,
					})
				}
			}()

			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})

	Describe("UT-KA-BUS-003: Multiple subscribers per correlationID", func() {
		It("should deliver to all subscribers for the same correlationID", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(10)
			ch1 := bus.Subscribe("rr-005", "sess-005a")
			ch2 := bus.Subscribe("rr-005", "sess-005b")

			bus.Publish("rr-005", mcpinternal.Notification{
				Type:          mcpinternal.NotificationAuditEvent,
				CorrelationID: "rr-005",
				Payload:       "shared-event",
			})

			Eventually(ch1).Should(Receive(WithTransform(
				func(n mcpinternal.Notification) string { return n.Payload.(string) },
				Equal("shared-event"),
			)))
			Eventually(ch2).Should(Receive(WithTransform(
				func(n mcpinternal.Notification) string { return n.Payload.(string) },
				Equal("shared-event"),
			)))
		})
	})
})
