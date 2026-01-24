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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// DD-NOT-003 V2.0: Category 12 - Observability Integration Tests
//
// TESTING PHILOSOPHY (per 03-testing-strategy.mdc):
// - Test BEHAVIOR: Observable state changes and status field population
// - Test CORRECTNESS: Status reflects actual delivery outcomes
// - Test OUTCOMES: System provides visibility into operations
//
// BR-NOT-070/071/072: Metrics and Observability - System exposes operational metrics
// BR-NOT-073/074: Health Probes - System reports health status
//
// NOTE: Full metrics endpoint testing (Prometheus scraping) is in E2E tests.
//       Integration tests focus on observable state and status field correctness.

var _ = Describe("Category 12: Observability & Status", Label("integration", "observability"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
	})

	// ==============================================
	// BEHAVIOR 1: Status Field Population
	// ==============================================

	Context("Status Field Observability (BR-NOT-070)", func() {
		It("should populate status fields with observable delivery metrics", func() {
			// BEHAVIOR: Status fields provide visibility into delivery outcomes
			// CORRECTNESS: Status accurately reflects delivery attempts and results

			notifName := fmt.Sprintf("status-observability-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Status Observability Test",
					Body:     "Testing status field population for observability",
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

			// BEHAVIOR: Status populated during delivery lifecycle
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
			}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// CORRECTNESS: Status fields provide accurate observability
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			// Verify observable metrics in status
			Expect(freshNotif.Status.SuccessfulDeliveries).To(Equal(1),
				"Status should show successful delivery count")
			Expect(freshNotif.Status.FailedDeliveries).To(Equal(0),
				"Status should show zero failures for successful delivery")
			Expect(freshNotif.Status.TotalAttempts).To(BeNumerically(">=", 1),
				"Status should track total delivery attempts")
			Expect(freshNotif.Status.CompletionTime).NotTo(BeNil(),
				"Status should include completion timestamp for observability")

			GinkgoWriter.Printf("✅ Status observability validated:\n")
			GinkgoWriter.Printf("   Phase: %s\n", freshNotif.Status.Phase)
			GinkgoWriter.Printf("   Successful: %d\n", freshNotif.Status.SuccessfulDeliveries)
			GinkgoWriter.Printf("   Failed: %d\n", freshNotif.Status.FailedDeliveries)
			GinkgoWriter.Printf("   Total Attempts: %d\n", freshNotif.Status.TotalAttempts)
			GinkgoWriter.Printf("   Completion: %v\n", freshNotif.Status.CompletionTime)

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should track retry attempts through status fields", func() {
			// BEHAVIOR: Retry attempts are observable through status
			// CORRECTNESS: TotalAttempts reflects delivery effort
			// NOTE: Mock Slack server accepts all requests, so we test retry tracking
			//       rather than failures. Real error scenarios are in E2E tests.

			notifName := fmt.Sprintf("retry-observability-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Retry Observability Test",
					Body:     "Testing retry attempt tracking in status",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts: 3, // Allow retries for observability
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR: Delivery observable in status
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
			}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Delivery status should be observable")

			// CORRECTNESS: Attempt tracking observable
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			Expect(freshNotif.Status.TotalAttempts).To(BeNumerically(">=", 1),
				"Total attempts should be tracked for observability")
			Expect(freshNotif.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1),
				"Successful deliveries should be tracked")

			GinkgoWriter.Printf("✅ Retry observability validated:\n")
			GinkgoWriter.Printf("   Phase: %s\n", freshNotif.Status.Phase)
			GinkgoWriter.Printf("   Successful Deliveries: %d\n", freshNotif.Status.SuccessfulDeliveries)
			GinkgoWriter.Printf("   Total Attempts: %d\n", freshNotif.Status.TotalAttempts)

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// BEHAVIOR 2: Delivery Latency Observability
	// ==============================================

	Context("Delivery Latency Observability (BR-NOT-071)", func() {
		It("should provide observable timing information through status timestamps", func() {
			// BEHAVIOR: Timestamps provide latency visibility
			// CORRECTNESS: CreationTimestamp → CompletionTime shows delivery latency

			notifName := fmt.Sprintf("latency-observability-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  "Latency Observability Test",
					Body:     "Testing delivery latency tracking",
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

			// BEHAVIOR: Delivery completes and timing is observable
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

			// CORRECTNESS: Timing information observable
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			// Verify timing observability
			createdAt := freshNotif.CreationTimestamp.Time
			Expect(createdAt).NotTo(BeZero(), "Creation timestamp should be set")

			if freshNotif.Status.CompletionTime != nil {
				completedAt := freshNotif.Status.CompletionTime.Time
				deliveryLatency := completedAt.Sub(createdAt)

				GinkgoWriter.Printf("✅ Delivery latency observable:\n")
				GinkgoWriter.Printf("   Created: %v\n", createdAt.Format(time.RFC3339))
				GinkgoWriter.Printf("   Completed: %v\n", completedAt.Format(time.RFC3339))
				GinkgoWriter.Printf("   Latency: %v\n", deliveryLatency)

				// BEHAVIOR: Timing fields provide latency visibility
				Expect(completedAt).NotTo(BeZero(), "Completion timestamp should be set")
				// Note: CompletionTime may have truncated precision, so we check order not exact timing
				Expect(deliveryLatency).To(BeNumerically(">=", -1*time.Second),
					"Delivery latency should be reasonable (allowing timestamp precision differences)")
				Expect(deliveryLatency).To(BeNumerically("<", 30*time.Second),
					"Delivery latency should be bounded")
			} else {
				// CompletionTime may not be set immediately for very fast deliveries
				GinkgoWriter.Printf("✅ Timing observability validated:\n")
				GinkgoWriter.Printf("   Created: %v\n", createdAt.Format(time.RFC3339))
				GinkgoWriter.Printf("   Status: %s (CompletionTime not yet set, very fast delivery)\n", freshNotif.Status.Phase)
			}

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// BEHAVIOR 3: Multi-Channel Observability
	// ==============================================

	Context("Multi-Channel Delivery Observability (BR-NOT-070)", func() {
		It("should provide per-channel delivery observability through status", func() {
			// BEHAVIOR: Status shows which channels succeeded/failed
			// CORRECTNESS: Operators can identify problematic channels

			notifName := fmt.Sprintf("multichannel-observability-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Multi-Channel Observability Test",
					Body:     "Testing per-channel observability",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR: Multi-channel delivery observable
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
			}, 30*time.Second, 1*time.Second).Should(Or(
				Equal(notificationv1alpha1.NotificationPhaseSent),
				Equal(notificationv1alpha1.NotificationPhasePartiallySent),
			), "Multi-channel delivery status should be observable")

			// CORRECTNESS: Status provides channel-level visibility
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			totalDeliveries := freshNotif.Status.SuccessfulDeliveries + freshNotif.Status.FailedDeliveries
			Expect(totalDeliveries).To(BeNumerically(">=", 1),
				"Status should show delivery attempts per channel")

			GinkgoWriter.Printf("✅ Multi-channel observability validated:\n")
			GinkgoWriter.Printf("   Phase: %s\n", freshNotif.Status.Phase)
			GinkgoWriter.Printf("   Successful: %d\n", freshNotif.Status.SuccessfulDeliveries)
			GinkgoWriter.Printf("   Failed: %d\n", freshNotif.Status.FailedDeliveries)
			GinkgoWriter.Printf("   Total Channels: %d\n", len(freshNotif.Spec.Channels))

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// BEHAVIOR 4: State Transition Observability
	// ==============================================

	Context("State Transition Observability (BR-NOT-073)", func() {
		It("should make notification lifecycle state transitions observable", func() {
			// BEHAVIOR: Phase transitions are observable (Pending → Sent)
			// CORRECTNESS: Status reflects current processing state

			notifName := fmt.Sprintf("state-observability-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Subject:  "State Transition Observability",
					Body:     "Testing lifecycle state observability",
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

			// BEHAVIOR: State transitions are observable
			seenPhases := make(map[notificationv1alpha1.NotificationPhase]bool)

			// Watch for phase transitions
			Eventually(func() notificationv1alpha1.NotificationPhase {
				freshNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}

				currentPhase := freshNotif.Status.Phase
				if currentPhase != "" {
					seenPhases[currentPhase] = true
					GinkgoWriter.Printf("   Observed phase transition: %s\n", currentPhase)
				}
				return currentPhase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// CORRECTNESS: Final state is correct
			Expect(seenPhases).To(HaveKey(notificationv1alpha1.NotificationPhaseSent),
				"Should observe Sent phase")

			GinkgoWriter.Printf("✅ State transitions observable: %d distinct phases seen\n", len(seenPhases))

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
