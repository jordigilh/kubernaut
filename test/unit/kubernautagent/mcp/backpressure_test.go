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

var _ = Describe("CP-5 UX-005: NotificationBus Backpressure Behavior", func() {

	// ---------------------------------------------------------------
	// UT-KA-UX005: Backpressure drops excess notifications without blocking
	// Reclassified from E2E UX-005 (BR-INTERACTIVE-003)
	// Validates: when subscriber buffer is full, publisher drops silently
	// and neither blocks nor corrupts other subscribers.
	// ---------------------------------------------------------------
	Describe("UT-KA-UX005: Backpressure drops excess notifications without blocking or corruption", func() {
		It("should drop messages exceeding buffer capacity without blocking publisher", func() {
			bufferSize := 3
			bus := mcpinternal.NewInMemoryNotificationBus(bufferSize)

			By("Creating a subscriber with small buffer (3)")
			ch := bus.Subscribe("rr-ux005", "sess-ux005")
			Expect(ch).NotTo(BeNil())

			By("Publishing more messages than buffer capacity (10 > 3)")
			publishCount := 10
			done := make(chan struct{})
			go func() {
				defer close(done)
				for i := 0; i < publishCount; i++ {
					bus.Publish("rr-ux005", mcpinternal.Notification{
						Type:          mcpinternal.NotificationAuditEvent,
						CorrelationID: "rr-ux005",
						SessionID:     "sess-ux005",
						Payload:       i,
						Timestamp:     time.Now(),
					})
				}
			}()

			By("Verifying publisher does not block (completes within 2s)")
			Eventually(done, 2*time.Second).Should(BeClosed(),
				"publisher must not block even when subscriber buffer is full")

			By("Verifying subscriber received exactly bufferSize messages (rest dropped)")
			received := 0
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						goto counted
					}
					received++
				default:
					goto counted
				}
			}
		counted:
			Expect(received).To(Equal(bufferSize),
				"subscriber should receive exactly bufferSize=%d messages; %d were dropped",
				bufferSize, publishCount-bufferSize)
			dropped := publishCount - received
			Expect(dropped).To(Equal(publishCount - bufferSize),
				"excess messages should be silently dropped")
			GinkgoWriter.Printf("  Published: %d, Received: %d, Dropped: %d\n",
				publishCount, received, dropped)

			GinkgoWriter.Println("✅ UT-KA-UX005: Backpressure correctly drops excess notifications")
		})

		It("should not corrupt other subscribers when one is slow", func() {
			bus := mcpinternal.NewInMemoryNotificationBus(2)

			By("Creating fast and slow subscribers for same correlationID")
			slowCh := bus.Subscribe("rr-ux005-multi", "sess-slow")
			fastCh := bus.Subscribe("rr-ux005-multi", "sess-fast")

			By("Publishing 5 messages (exceeds slow subscriber buffer)")
			for i := 0; i < 5; i++ {
				bus.Publish("rr-ux005-multi", mcpinternal.Notification{
					Type:          mcpinternal.NotificationAuditEvent,
					CorrelationID: "rr-ux005-multi",
					Payload:       i,
				})
			}

			By("Fast consumer drains immediately")
			fastReceived := 0
			for {
				select {
				case <-fastCh:
					fastReceived++
				default:
					goto fastDone
				}
			}
		fastDone:
			Expect(fastReceived).To(Equal(2), "fast consumer also limited by buffer=2")

			By("Slow consumer also receives buffer-limited messages")
			slowReceived := 0
			for {
				select {
				case <-slowCh:
					slowReceived++
				default:
					goto slowDone
				}
			}
		slowDone:
			Expect(slowReceived).To(Equal(2), "slow consumer receives exactly buffer=2")

			By("Verifying no data corruption: messages are valid ints")
			bus2 := mcpinternal.NewInMemoryNotificationBus(5)
			ch := bus2.Subscribe("rr-ux005-validate", "sess-validate")
			bus2.Publish("rr-ux005-validate", mcpinternal.Notification{
				Type:    mcpinternal.NotificationAuditEvent,
				Payload: 42,
			})
			Eventually(ch).Should(Receive(WithTransform(
				func(n mcpinternal.Notification) int { return n.Payload.(int) },
				Equal(42),
			)))

			GinkgoWriter.Println("✅ UT-KA-UX005: Slow consumer doesn't corrupt other subscribers")
		})
	})
})
