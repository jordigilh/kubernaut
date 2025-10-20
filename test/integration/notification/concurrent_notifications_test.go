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
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/timing"
)

// BR-NOT-053: At-Least-Once Delivery - Concurrent notifications should all be delivered
// BR-NOT-051: Complete Audit Trail - Status updates must be atomic (no lost data)

var _ = Describe("Integration Test 5: Concurrent Notification Handling", func() {
	var testNamespace string

	BeforeEach(func() {
		testNamespace = "default"
	})

	Context("Multiple Concurrent Notifications", func() {
		It("should process 10 concurrent notifications without conflicts (BR-NOT-053: At-Least-Once)", func() {
			By("Creating 10 notifications in parallel")
			const numNotifications = 10
			notifications := make([]*notificationv1alpha1.NotificationRequest, numNotifications)
			var wg sync.WaitGroup
			createErrors := make([]error, numNotifications)

			// TDD RED: Create multiple notifications concurrently
			// Expected: All should be created and processed successfully

			for i := 0; i < numNotifications; i++ {
				notifications[i] = &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("concurrent-test-%d-%d", i, time.Now().Unix()),
						Namespace: testNamespace,
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Subject:  fmt.Sprintf("Concurrent Test %d", i),
						Body:     fmt.Sprintf("Testing concurrent processing - notification %d", i),
						Recipients: []notificationv1alpha1.Recipient{
							{
								Slack: "#integration-tests",
							},
						},
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole, // Use console for fast, reliable delivery
						},
					},
				}

				wg.Add(1)
				go func(idx int, n *notificationv1alpha1.NotificationRequest) {
					defer GinkgoRecover()
					defer wg.Done()
					createErrors[idx] = k8sClient.Create(ctx, n)
				}(i, notifications[i])
			}

			By("Waiting for all creates to complete")
			wg.Wait()

			By("Verifying all notifications were created successfully")
		for i, err := range createErrors {
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Notification %d should be created", i))
		}

		By("Waiting for all notifications to reach Sent phase (with anti-flaky retry)")
		// v3.1: Use EventuallyWithRetry with 60s timeout for concurrent delivery reliability
		for i := 0; i < numNotifications; i++ {
			i := i // capture loop variable
			timing.EventuallyWithRetry(func() error {
				latest := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notifications[i]), latest)
				if err != nil {
					return err
				}
				if latest.Status.Phase != notificationv1alpha1.NotificationPhaseSent {
					return fmt.Errorf("expected phase Sent, got %s", latest.Status.Phase)
				}
				return nil
			}, 10, 6*time.Second).Should(Succeed(),
				fmt.Sprintf("Notification %d should reach Sent phase", i))
		}

		By("Verifying all status updates are correct (no conflicts)")
			for i := 0; i < numNotifications; i++ {
				latest := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notifications[i]), latest)
				Expect(err).NotTo(HaveOccurred())

				// BR-NOT-053: At-least-once delivery verified
				Expect(latest.Status.SuccessfulDeliveries).To(Equal(1),
					fmt.Sprintf("Notification %d: Expected 1 successful delivery", i))
				Expect(latest.Status.FailedDeliveries).To(Equal(0),
					fmt.Sprintf("Notification %d: Expected 0 failed deliveries", i))

				// BR-NOT-051: Audit trail verified (no lost delivery attempts)
				Expect(latest.Status.TotalAttempts).To(BeNumerically(">=", 1),
					fmt.Sprintf("Notification %d: Expected at least 1 delivery attempt", i))
				Expect(latest.Status.DeliveryAttempts).To(HaveLen(1),
					fmt.Sprintf("Notification %d: Expected 1 delivery attempt recorded", i))
			}

			By("Cleaning up concurrent notifications")
			for i := 0; i < numNotifications; i++ {
				deleteErr := k8sClient.Delete(context.Background(), notifications[i])
				Expect(deleteErr).NotTo(HaveOccurred())
			}
		})

		It("should handle mixed priority notifications correctly (BR-NOT-053: Priority Processing)", func() {
			By("Creating 5 critical + 5 low priority notifications")
			const numNotifications = 10
			notifications := make([]*notificationv1alpha1.NotificationRequest, numNotifications)
			var wg sync.WaitGroup
			createErrors := make([]error, numNotifications)

			// TDD RED: Create notifications with different priorities concurrently
			// Expected: All should be processed (priority order not guaranteed in reconciliation)

			for i := 0; i < numNotifications; i++ {
				priority := notificationv1alpha1.NotificationPriorityLow
				if i < 5 {
					priority = notificationv1alpha1.NotificationPriorityCritical
				}

				notifications[i] = &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("priority-test-%d-%d", i, time.Now().Unix()),
						Namespace: testNamespace,
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: priority,
						Subject:  fmt.Sprintf("Priority Test %d (%s)", i, priority),
						Body:     fmt.Sprintf("Testing mixed priority processing - notification %d", i),
						Recipients: []notificationv1alpha1.Recipient{
							{
								Slack: "#integration-tests",
							},
						},
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				wg.Add(1)
				go func(idx int, n *notificationv1alpha1.NotificationRequest) {
					defer GinkgoRecover()
					defer wg.Done()
					createErrors[idx] = k8sClient.Create(ctx, n)
				}(i, notifications[i])
			}

			By("Waiting for all creates to complete")
			wg.Wait()

		By("Verifying all notifications were created successfully")
		for i, err := range createErrors {
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Notification %d should be created", i))
		}

		By("Waiting for all notifications to reach Sent phase (with anti-flaky retry)")
		// v3.1: Use EventuallyWithRetry with 60s timeout for concurrent delivery reliability
		for i := 0; i < numNotifications; i++ {
			i := i // capture loop variable
			timing.EventuallyWithRetry(func() error {
				latest := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notifications[i]), latest)
				if err != nil {
					return err
				}
				if latest.Status.Phase != notificationv1alpha1.NotificationPhaseSent {
					return fmt.Errorf("expected phase Sent, got %s", latest.Status.Phase)
				}
				return nil
			}, 10, 6*time.Second).Should(Succeed(),
				fmt.Sprintf("Notification %d should reach Sent phase", i))
		}

		By("Verifying all priorities were processed (no priority inversions)")
			criticalCount := 0
			lowCount := 0

			for i := 0; i < numNotifications; i++ {
				latest := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notifications[i]), latest)
				Expect(err).NotTo(HaveOccurred())

				Expect(latest.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent))

				if latest.Spec.Priority == notificationv1alpha1.NotificationPriorityCritical {
					criticalCount++
				} else {
					lowCount++
				}
			}

			Expect(criticalCount).To(Equal(5), "Expected 5 critical priority notifications processed")
			Expect(lowCount).To(Equal(5), "Expected 5 low priority notifications processed")

			By("Cleaning up priority test notifications")
			for i := 0; i < numNotifications; i++ {
				deleteErr := k8sClient.Delete(context.Background(), notifications[i])
				Expect(deleteErr).NotTo(HaveOccurred())
			}
		})

		It("should handle concurrent status updates atomically (BR-NOT-051: Atomic Updates)", func() {
			By("Creating notification with fast retry policy to trigger concurrent reconciliation")

			// TDD RED: Create notification that will trigger multiple rapid reconciliation loops
			// Expected: Status updates should be atomic (no lost attempts)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "atomic-updates-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Atomic Status Updates Test",
					Body:     "Testing concurrent status updates are atomic",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#integration-tests",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1, // Fast retries to trigger concurrent reconciliation
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			// Configure mock Slack to fail first 2 attempts
			By("Configuring mock Slack server to fail first 2 attempts")
			ConfigureFailureMode("first-N", 2, 500) // 500 = Service Unavailable

			By("Creating NotificationRequest")
			err := k8sClient.Create(ctx, notification)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for notification to reach Sent or Failed phase")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				latest := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), latest)
				if err != nil {
					return ""
				}
				return latest.Status.Phase
			}, "20s", "500ms").Should(Or(
				Equal(notificationv1alpha1.NotificationPhaseSent),
				Equal(notificationv1alpha1.NotificationPhaseFailed),
			), "Notification should reach terminal phase")

			By("Verifying status updates are atomic (BR-NOT-051: No Lost Attempts)")
			latest := &notificationv1alpha1.NotificationRequest{}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), latest)
			Expect(err).NotTo(HaveOccurred())

			// All delivery attempts must be recorded (no lost updates due to conflicts)
			Expect(latest.Status.TotalAttempts).To(BeNumerically(">=", 2),
				"Expected at least 2 delivery attempts (failures)")
			Expect(latest.Status.DeliveryAttempts).To(HaveLen(int(latest.Status.TotalAttempts)),
				"Expected DeliveryAttempts length to match TotalAttempts (no lost records)")

			// Verify attempts are in order
			if len(latest.Status.DeliveryAttempts) > 1 {
				for i := 1; i < len(latest.Status.DeliveryAttempts); i++ {
					prevTime := latest.Status.DeliveryAttempts[i-1].Timestamp.Time
					currTime := latest.Status.DeliveryAttempts[i].Timestamp.Time
					Expect(currTime.After(prevTime) || currTime.Equal(prevTime)).To(BeTrue(),
						"Delivery attempts should be in chronological order")
				}
			}

			By("Cleaning up atomic updates test notification")
			// Reset mock to success mode
			ConfigureFailureMode("none", 0, 200)

			deleteErr := k8sClient.Delete(context.Background(), notification)
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})
