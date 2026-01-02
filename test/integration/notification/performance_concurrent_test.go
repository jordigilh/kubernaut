/*
Copyright 2025 Jordi Gil.

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

package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/retry"
)

// P0 TESTS: Concurrent Deliveries + Circuit Breaker (6 tests)
// BR-NOT-060: Concurrent delivery safety (10+ simultaneous)
// BR-NOT-061: Circuit breaker protection
//
// BEHAVIOR FOCUS: Does the system handle concurrent operations correctly?
// CORRECTNESS FOCUS: Does circuit breaker protect from cascade failures?

var _ = Describe("P0: Concurrent Deliveries + Circuit Breaker", Label("p0", "concurrent", "circuit-breaker"), func() {
	var (
		ctx          context.Context
		uniqueSuffix string
	)

	BeforeEach(func() {
		ctx = context.Background()
		uniqueSuffix = fmt.Sprintf("%d", time.Now().UnixNano())
	})

	// ==============================================
	// CATEGORY 1: Concurrent Delivery Safety (3 tests)
	// BR-NOT-060: System must handle 10+ concurrent deliveries
	// ==============================================

	Context("BR-NOT-060: Concurrent Delivery Safety", func() {
		It("should handle 10 concurrent notification deliveries without race conditions", func() {
			// BEHAVIOR: Concurrent deliveries succeed independently
			// CORRECTNESS: No data races, all notifications delivered

			const concurrentCount = 10
			notifications := make([]*notificationv1alpha1.NotificationRequest, concurrentCount)
			var wg sync.WaitGroup

			// Create 10 notifications concurrently
			for i := 0; i < concurrentCount; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()

					notif := &notificationv1alpha1.NotificationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("concurrent-test-%s-%d", uniqueSuffix, idx),
							Namespace: testNamespace,
							Generation: 1, // K8s increments on create/update
						},
						Spec: notificationv1alpha1.NotificationRequestSpec{
							Type:     notificationv1alpha1.NotificationTypeSimple,
							Priority: notificationv1alpha1.NotificationPriorityMedium,
							Subject:  fmt.Sprintf("Concurrent Test %d - %s", idx, uniqueSuffix),
							Body:     fmt.Sprintf("Testing concurrent delivery %d", idx),
							Channels: []notificationv1alpha1.Channel{
								notificationv1alpha1.ChannelSlack,
							},
							Recipients: []notificationv1alpha1.Recipient{
								{Slack: slackWebhookURL},
							},
						},
					}

					Expect(k8sClient.Create(ctx, notif)).To(Succeed(),
						"Should create notification %d", idx)
					notifications[idx] = notif
				}(i)
			}

			wg.Wait()

			// Wait for all notifications to be delivered
			for i, notif := range notifications {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					key := types.NamespacedName{
						Name:      notif.Name,
						Namespace: notif.Namespace,
					}
					Expect(k8sClient.Get(ctx, key, notif)).To(Succeed())
					return notif.Status.Phase
				}, "30s", "500ms").Should(Equal(notificationv1alpha1.NotificationPhaseSent),
					"Notification %d should reach Sent phase", i)
			}

			// BEHAVIOR VALIDATION: All notifications delivered successfully
			slackCalls := getSlackRequestsCopy(uniqueSuffix)
			Expect(slackCalls).To(HaveLen(concurrentCount),
				"Should have exactly %d Slack webhook calls", concurrentCount)

			// CORRECTNESS VALIDATION: Each notification delivered once
			// All requests should have the same TestID (uniqueSuffix) for correlation
			// The fact we have exactly concurrentCount calls means each was delivered once
			for i, call := range slackCalls {
				Expect(call.TestID).To(ContainSubstring(uniqueSuffix),
					"Request %d should have correct TestID for correlation", i)
			}

			// Cleanup
			for _, notif := range notifications {
				deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			}
		})

		It("should handle rapid successive CRD creations (stress test)", Serial, func() {
			// BEHAVIOR: Rapid creation doesn't cause controller failures
			// CORRECTNESS: All CRDs processed in correct order
			// NOTE: Marked Serial to prevent resource contention with parallel tests

			const rapidCount = 20
			notifications := make([]*notificationv1alpha1.NotificationRequest, rapidCount)

			// Create 20 notifications as fast as possible (no goroutines - sequential stress)
			for i := 0; i < rapidCount; i++ {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("rapid-test-%s-%d", uniqueSuffix, i),
						Namespace: testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  fmt.Sprintf("Rapid Test %d - %s", i, uniqueSuffix),
						Body:     "Testing rapid creation",
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelSlack,
						},
						Recipients: []notificationv1alpha1.Recipient{
							{Slack: slackWebhookURL},
						},
					},
				}
				Expect(k8sClient.Create(ctx, notif)).To(Succeed())
				notifications[i] = notif
			}

			// Wait for all to be delivered
			for i, notif := range notifications {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					key := types.NamespacedName{
						Name:      notif.Name,
						Namespace: notif.Namespace,
					}
					Expect(k8sClient.Get(ctx, key, notif)).To(Succeed())
					return notif.Status.Phase
				}, "60s", "1s").Should(Equal(notificationv1alpha1.NotificationPhaseSent),
					"Rapid notification %d should reach Sent phase", i)
			}

			// BEHAVIOR VALIDATION: All notifications eventually delivered
			slackCalls := getSlackRequestsCopy(uniqueSuffix)
			Expect(slackCalls).To(HaveLen(rapidCount),
				"Should process all %d rapid notifications", rapidCount)

			// Cleanup
			for _, notif := range notifications {
				deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			}
		})

		It("should handle concurrent status updates without conflicts", func() {
			// BEHAVIOR: Status updates don't interfere with each other
			// CORRECTNESS: Optimistic locking prevents status corruption

			const concurrentCount = 5
			var wg sync.WaitGroup

			// Create 5 notifications that will update concurrently
			for i := 0; i < concurrentCount; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()

					notif := &notificationv1alpha1.NotificationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("status-concurrent-%s-%d", uniqueSuffix, idx),
							Namespace: testNamespace,
							Generation: 1, // K8s increments on create/update
						},
						Spec: notificationv1alpha1.NotificationRequestSpec{
							Type:     notificationv1alpha1.NotificationTypeSimple,
							Priority: notificationv1alpha1.NotificationPriorityHigh,
							Subject:  fmt.Sprintf("Status Test %d - %s", idx, uniqueSuffix),
							Body:     "Testing concurrent status updates",
							Channels: []notificationv1alpha1.Channel{
								notificationv1alpha1.ChannelSlack,
							},
							Recipients: []notificationv1alpha1.Recipient{
								{Slack: slackWebhookURL},
							},
						},
					}

					Expect(k8sClient.Create(ctx, notif)).To(Succeed())

					// Wait for delivery
					Eventually(func() notificationv1alpha1.NotificationPhase {
						key := types.NamespacedName{
							Name:      notif.Name,
							Namespace: notif.Namespace,
						}
						Expect(k8sClient.Get(ctx, key, notif)).To(Succeed())
						return notif.Status.Phase
					}, "30s", "500ms").Should(Equal(notificationv1alpha1.NotificationPhaseSent))

					// CORRECTNESS VALIDATION: Status must be consistent
					key := types.NamespacedName{
						Name:      notif.Name,
						Namespace: notif.Namespace,
					}
					Expect(k8sClient.Get(ctx, key, notif)).To(Succeed())

					// Validate status consistency
					Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent))
					Expect(notif.Status.CompletionTime).ToNot(BeNil(),
						"Sent notification must have CompletionTime")
					Expect(notif.Status.SuccessfulDeliveries).To(Equal(1),
						"Should have exactly 1 successful delivery")

					// Cleanup
					deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
				}(i)
			}

			wg.Wait()
		})
	})

	// ==============================================
	// CATEGORY 2: Circuit Breaker Protection (3 tests)
	// BR-NOT-061: Circuit breaker prevents cascade failures
	// ==============================================

	Context("BR-NOT-061: Circuit Breaker Protection", func() {
		It("should block requests after threshold failures (BR-NOT-061: Cascade failure prevention)", func() {
			// BEHAVIOR: Circuit breaker blocks requests to protect failing service
			// BUSINESS CONTEXT: Prevents overwhelming unhealthy service with more requests

			// Configure circuit breaker with low threshold for testing
			circuitBreaker := retry.NewCircuitBreaker(&retry.CircuitBreakerConfig{
				FailureThreshold: 3, // Open after 3 failures
				SuccessThreshold: 2, // Close after 2 successes in half-open
				Timeout:          5 * time.Second,
			})

			// BEHAVIOR VALIDATION: Initial state allows requests
			Expect(circuitBreaker.AllowRequest("slack")).To(BeTrue(),
				"Circuit should allow requests initially (normal operation)")

			// Record failures up to threshold - 1
			for i := 0; i < 2; i++ {
				circuitBreaker.RecordFailure("slack")

				// BEHAVIOR VALIDATION: Requests still allowed before threshold
				Expect(circuitBreaker.AllowRequest("slack")).To(BeTrue(),
					"Should allow requests before reaching threshold (failure %d)", i+1)
			}

			// 3rd failure triggers circuit breaker
			circuitBreaker.RecordFailure("slack")

			// BEHAVIOR VALIDATION: Circuit breaker now blocks requests
			Expect(circuitBreaker.AllowRequest("slack")).To(BeFalse(),
				"Should block requests after reaching failure threshold to prevent cascade failures")
		})

		It("should allow requests after successful recovery (BR-NOT-061: Service recovery)", func() {
			// BEHAVIOR: Circuit breaker allows requests again after service recovers
			// BUSINESS CONTEXT: Returns to normal operation once service proves it's healthy

			circuitBreaker := retry.NewCircuitBreaker(&retry.CircuitBreakerConfig{
				FailureThreshold: 3,
				SuccessThreshold: 2,
				Timeout:          100 * time.Millisecond,
			})

			// Trigger circuit breaker (open state)
			for i := 0; i < 3; i++ {
				circuitBreaker.RecordFailure("slack")
			}

			// BEHAVIOR VALIDATION: Circuit blocks requests when open
			Expect(circuitBreaker.AllowRequest("slack")).To(BeFalse(),
				"Circuit should block requests after failures")

			// Manually transition to HalfOpen (simulating timeout for recovery probe)
			circuitBreaker.TryReset("slack")

			// BEHAVIOR VALIDATION: Half-open allows probe requests
			Expect(circuitBreaker.AllowRequest("slack")).To(BeTrue(),
				"Should allow probe requests in recovery mode (half-open)")

			// Record successful probe requests
			circuitBreaker.RecordSuccess("slack")
			circuitBreaker.RecordSuccess("slack")

			// BEHAVIOR VALIDATION: Circuit allows all requests after recovery
			Expect(circuitBreaker.AllowRequest("slack")).To(BeTrue(),
				"Should allow all requests after successful recovery (circuit closed)")

			// CORRECTNESS: Verify circuit stays closed for subsequent requests
			for i := 0; i < 10; i++ {
				Expect(circuitBreaker.AllowRequest("slack")).To(BeTrue(),
					"Circuit should remain closed for normal operation (request %d)", i)
			}
		})

		// Test "should fail delivery when circuit breaker is open" - DELETED per "NO SKIPPED TESTS" rule
		//
		// This test was a Skip() placeholder for controller-level circuit breaker integration
		// that requires architectural changes (injectable CircuitBreaker interface).
		//
		// Current coverage:
		// - Circuit breaker logic: ✅ UNIT TESTED (above tests)
		// - Circuit breaker behavior: ✅ E2E TESTED (test/e2e/notification/)
		//
		// Per project rule: No Skip() placeholders allowed. Implement properly when architecture supports it.
	})
})
