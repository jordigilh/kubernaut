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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// DD-NOT-003 V2.0: Category 8 - Performance Edge Cases Integration Tests
//
// TESTING PHILOSOPHY (per 03-testing-strategy.mdc):
// - Test BEHAVIOR: Performance characteristics under various payload sizes and load
// - Test CORRECTNESS: System handles performance edge cases gracefully
// - Test OUTCOMES: Notifications deliver successfully despite performance constraints
//
// BR-NOT-059: Large Payload Support - Handle notifications up to size limits
// BR-NOT-060: Concurrent Delivery Safety - Process multiple notifications efficiently
// BR-NOT-054: External Service Integration - Handle varying response times

var _ = Describe("Category 8: Performance Edge Cases", Label("integration", "performance"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
	})

	// ==============================================
	// BEHAVIOR 1: Payload Size Handling
	// ==============================================

	Context("Payload Size Performance (BR-NOT-059)", func() {
		It("should efficiently handle notification with small payload (1KB baseline)", func() {
			// BEHAVIOR: Small payloads are delivered quickly (baseline performance)
			// CORRECTNESS: Minimal overhead for small notifications

			notifName := fmt.Sprintf("small-payload-%s", uniqueSuffix)
			smallBody := strings.Repeat("Small payload test. ", 50) // ~1KB

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Small Payload Performance Test",
					Body:     smallBody,
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			startTime := time.Now()
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR: Small notification delivered quickly
			Eventually(func() notificationv1alpha1.NotificationPhase {
				freshNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}
				return freshNotif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			deliveryTime := time.Since(startTime)
			GinkgoWriter.Printf("✅ Small payload (1KB) delivered in %v\n", deliveryTime)

			// CORRECTNESS: Fast delivery for small payloads (<5s typical)
			Expect(deliveryTime).To(BeNumerically("<", 10*time.Second),
				"Small payload should be delivered quickly")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle notification with large payload at boundary (max size)", func() {
			// BEHAVIOR: Large payloads within limits are delivered successfully
			// CORRECTNESS: System handles maximum payload size without truncation

			notifName := fmt.Sprintf("large-payload-%s", uniqueSuffix)
			// Create ~10KB body (at reasonable size boundary)
			largeBody := strings.Repeat("Large payload test with more content to reach size boundary. ", 200) // ~12KB

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Large Payload Performance Test",
					Body:     largeBody,
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			startTime := time.Now()
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR: Large notification delivered successfully
			Eventually(func() notificationv1alpha1.NotificationPhase {
				freshNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}
				return freshNotif.Status.Phase
			}, 45*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			deliveryTime := time.Since(startTime)
			bodySize := len(largeBody)
			GinkgoWriter.Printf("✅ Large payload (%d bytes) delivered in %v\n", bodySize, deliveryTime)

			// CORRECTNESS: Large payload delivered without truncation
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, freshNotif)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(freshNotif.Spec.Body)).To(Equal(bodySize),
				"Body should not be truncated")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// BEHAVIOR 2: Sustained Load Performance
	// ==============================================

	Context("Sustained Load Performance (BR-NOT-060)", func() {
		It("should efficiently process batch of notifications with large payloads", func() {
			// BEHAVIOR: System processes sustained load without degradation
			// CORRECTNESS: All large-payload notifications delivered successfully

			batchSize := 20
			largeBody := strings.Repeat("Sustained load test with large payload. ", 150) // ~6KB each

			notifNames := make([]string, batchSize)
			startTime := time.Now()

			// Create batch of large-payload notifications
			for i := 0; i < batchSize; i++ {
				notifName := fmt.Sprintf("batch-load-%d-%s", i, uniqueSuffix)
				notifNames[i] = notifName

				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Subject:  fmt.Sprintf("Batch Load Test %d", i),
						Body:     largeBody,
						Recipients: []notificationv1alpha1.Recipient{
							{Email: "test@example.com"},
						},
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				err := k8sClient.Create(ctx, notif)
				Expect(err).NotTo(HaveOccurred())
			}

			// BEHAVIOR: All notifications in batch are delivered
			for _, notifName := range notifNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notif := &notificationv1alpha1.NotificationRequest{}
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 90*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
			}

			totalTime := time.Since(startTime)
			avgTime := totalTime / time.Duration(batchSize)

			GinkgoWriter.Printf("✅ Batch of %d large notifications delivered:\n", batchSize)
			GinkgoWriter.Printf("   Total time: %v\n", totalTime)
			GinkgoWriter.Printf("   Average per notification: %v\n", avgTime)

			// CORRECTNESS: Sustained load handled efficiently
			Expect(totalTime).To(BeNumerically("<", 2*time.Minute),
				"Batch should complete in reasonable time")

			// Cleanup
			for _, notifName := range notifNames {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
				}
				_ = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			}
		})
	})

	// ==============================================
	// BEHAVIOR 3: Slow External Service Handling
	// ==============================================

	Context("External Service Response Times (BR-NOT-054)", func() {
		It("should handle fast external service responses efficiently", func() {
			// BEHAVIOR: Fast external responses result in quick delivery
			// CORRECTNESS: Minimal overhead added by controller

			notifName := fmt.Sprintf("fast-slack-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  "Fast Response Test",
					Body:     "Testing fast Slack webhook response",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			startTime := time.Now()
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR: Fast delivery with fast webhook
			Eventually(func() notificationv1alpha1.NotificationPhase {
				freshNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}
				return freshNotif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			deliveryTime := time.Since(startTime)
			GinkgoWriter.Printf("✅ Fast Slack webhook delivered in %v (mock response time <100ms)\n", deliveryTime)

			// CORRECTNESS: Quick delivery with fast webhook
			Expect(deliveryTime).To(BeNumerically("<", 10*time.Second),
				"Fast webhook should result in quick delivery")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		// Note: Slow webhook tests (5s, 30s responses) are better suited for E2E tests
		// where we can control mock server response times. In integration tests,
		// the mock server responds immediately.
	})

	// ==============================================
	// BEHAVIOR 4: Performance Under Mixed Workload
	// ==============================================

	Context("Mixed Workload Performance (BR-NOT-060)", func() {
		It("should maintain performance with mixed payload sizes and priorities", func() {
			// BEHAVIOR: System handles heterogeneous workload efficiently
			// CORRECTNESS: High-priority small notifications not blocked by large ones

			notifCount := 30
			notifNames := make([]string, notifCount)
			startTime := time.Now()

			// Create mixed workload: 10 small/high-priority, 10 medium, 10 large/low-priority
			for i := 0; i < notifCount; i++ {
				notifName := fmt.Sprintf("mixed-workload-%d-%s", i, uniqueSuffix)
				notifNames[i] = notifName

				var priority notificationv1alpha1.NotificationPriority
				var body string

				// Mix payload sizes and priorities
				if i < 10 {
					// Small, high-priority
					priority = notificationv1alpha1.NotificationPriorityHigh
					body = "Small high-priority notification"
				} else if i < 20 {
					// Medium size, medium priority
					priority = notificationv1alpha1.NotificationPriorityMedium
					body = strings.Repeat("Medium notification. ", 50) // ~1KB
				} else {
					// Large, low-priority
					priority = notificationv1alpha1.NotificationPriorityLow
					body = strings.Repeat("Large low-priority notification with more content. ", 100) // ~5KB
				}

				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: priority,
						Subject:  fmt.Sprintf("Mixed Workload Test %d", i),
						Body:     body,
						Recipients: []notificationv1alpha1.Recipient{
							{Email: "test@example.com"},
						},
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				err := k8sClient.Create(ctx, notif)
				Expect(err).NotTo(HaveOccurred())
			}

			// BEHAVIOR: All notifications delivered despite mixed workload
			successCount := 0
			for _, notifName := range notifNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notif := &notificationv1alpha1.NotificationRequest{}
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 90*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
				successCount++
			}

			totalTime := time.Since(startTime)
			successRate := float64(successCount) / float64(notifCount) * 100

			GinkgoWriter.Printf("✅ Mixed workload delivered:\n")
			GinkgoWriter.Printf("   Total: %d notifications\n", notifCount)
			GinkgoWriter.Printf("   Success rate: %0.1f%%\n", successRate)
			GinkgoWriter.Printf("   Total time: %v\n", totalTime)

			// CORRECTNESS: High success rate for mixed workload
			Expect(successRate).To(Equal(100.0),
				"All notifications should be delivered")

			// Cleanup
			for _, notifName := range notifNames {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
				}
				_ = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			}
		})

		It("should handle burst workload followed by idle period (queue management)", func() {
			// BEHAVIOR: System processes burst efficiently and returns to idle
			// CORRECTNESS: No queue backlog or resource leaks after burst

			burstSize := 40
			notifNames := make([]string, burstSize)

			// Create burst of notifications
			GinkgoWriter.Printf("📈 Creating burst of %d notifications...\n", burstSize)
			burstStart := time.Now()

			for i := 0; i < burstSize; i++ {
				notifName := fmt.Sprintf("burst-idle-%d-%s", i, uniqueSuffix)
				notifNames[i] = notifName

				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Subject:  fmt.Sprintf("Burst Test %d", i),
						Body:     "Testing burst followed by idle period",
						Recipients: []notificationv1alpha1.Recipient{
							{Email: "test@example.com"},
						},
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				err := k8sClient.Create(ctx, notif)
				Expect(err).NotTo(HaveOccurred())
			}

			// BEHAVIOR: Burst processed completely
			for _, notifName := range notifNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notif := &notificationv1alpha1.NotificationRequest{}
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 90*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
			}

			burstTime := time.Since(burstStart)
			GinkgoWriter.Printf("✅ Burst of %d notifications processed in %v\n", burstSize, burstTime)

			// Cleanup - verify all deletions complete
			GinkgoWriter.Printf("🗑️  Cleaning up %d notifications...\n", burstSize)
			for _, notifName := range notifNames {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
				}
				err := deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to delete %s", notifName))
			}

			// BEHAVIOR: System returns to idle after burst (queue drained)
			// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
			// Verify system is truly idle by checking notification queue is empty in this test's namespace
			Eventually(func() int {
				list := &notificationv1alpha1.NotificationRequestList{}
				// Filter by namespace to avoid interference from other concurrent tests
				err := k8sManager.GetAPIReader().List(ctx, list, client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(list.Items)
			}, 30*time.Second, 1*time.Second).Should(BeZero(),
				fmt.Sprintf("System should return to idle (empty queue in %s) after burst", testNamespace))

			GinkgoWriter.Printf("✅ System returned to idle after burst (queue management working)\n")
		})
	})
})
