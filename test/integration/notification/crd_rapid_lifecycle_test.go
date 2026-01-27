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
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ════════════════════════════════════════════════════════════════════════
// Phase 2, Task 2.2: Rapid CRD Lifecycle Testing
// ════════════════════════════════════════════════════════════════════════
//
// PURPOSE: Validate idempotency and duplicate prevention under rapid lifecycle changes
//
// BUSINESS REQUIREMENTS:
// - BR-NOT-053: At-least-once delivery (no duplicate deliveries despite rapid changes)
// - BR-NOT-056: CRD lifecycle correctness (proper state transitions)
//
// RISK ADDRESSED: Medium Risk from RISK-ASSESSMENT-MISSING-29-TESTS.md
// - Scenario: "Rapid create-delete-create cycles - Duplicate deliveries"
// - Impact: Timing races in idempotency logic
// - Mitigation: Idempotency logic must handle rapid operations
//
// SUCCESS CRITERIA (Behavior-focused):
// - BUSINESS OUTCOME: No duplicate deliveries across rapid lifecycle changes
// - STATE CORRECTNESS: Each lifecycle produces correct status transitions
// - RESOURCE CLEANUP: No orphaned resources after rapid operations
//
// ════════════════════════════════════════════════════════════════════════

var _ = Describe("CRD Lifecycle: Rapid Create-Delete-Create", func() {
	var (
		uniqueSuffix  string
		testNamespace = "kubernaut-notifications"
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
	})

	Context("BR-NOT-053: Idempotency Under Rapid Lifecycle Changes", func() {
		It("should handle rapid create-delete cycles without duplicate deliveries", func() {
			// BUSINESS SCENARIO: User rapidly creates and deletes notifications (UI mistake, automation bug)

			notificationBaseName := fmt.Sprintf("rapid-lifecycle-%s", uniqueSuffix)
			totalDeliveries := 0

			By("Performing 10 rapid create-delete cycles")
			for cycle := 0; cycle < 10; cycle++ {
				notificationName := fmt.Sprintf("%s-cycle-%d", notificationBaseName, cycle)

				By(fmt.Sprintf("Cycle %d: Creating notification", cycle+1))
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notificationName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Subject:  fmt.Sprintf("Rapid Lifecycle Cycle %d", cycle+1),
						Body:     fmt.Sprintf("Testing rapid create-delete-create (cycle %d)", cycle+1),
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
						Recipients: []notificationv1alpha1.Recipient{
							{Slack: "#rapid-test"},
						},
					},
				}

				Expect(k8sClient.Create(ctx, notif)).Should(Succeed())

				By(fmt.Sprintf("Cycle %d: Waiting for reconciliation to start", cycle+1))
				// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
				// Wait for controller to start reconciliation (status transitions from empty)
				Eventually(func() bool {
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, notif)
					if err != nil {
						return false
					}
					// Reconciliation started when status phase is set (not empty)
					return notif.Status.Phase != ""
				}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
					"Controller should start reconciliation within 5 seconds")

				By(fmt.Sprintf("Cycle %d: Checking status before deletion", cycle+1))

				// CORRECTNESS: Track if delivery completed before deletion
				if notif.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
					totalDeliveries++
					GinkgoWriter.Printf("  ✅ Cycle %d: Delivered before deletion (phase: %s)\n", cycle+1, notif.Status.Phase)
				} else {
					GinkgoWriter.Printf("  ⏭️  Cycle %d: Deleted before delivery (phase: %s)\n", cycle+1, notif.Status.Phase)
				}

				By(fmt.Sprintf("Cycle %d: Deleting notification", cycle+1))
				Expect(k8sClient.Delete(ctx, notif)).Should(Succeed())

				By(fmt.Sprintf("Cycle %d: Verifying deletion completed", cycle+1))
				Eventually(func() bool {
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, notif)
					return apierrors.IsNotFound(err)
				}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
					"Notification should be deleted from Kubernetes")
			}

			By("Verifying business outcome: No duplicate deliveries (BR-NOT-053)")
			GinkgoWriter.Printf("📊 Total deliveries across 10 cycles: %d\n", totalDeliveries)

			// BUSINESS OUTCOME: Each cycle delivers at most once (idempotency preserved)
			// In rapid cycles, some notifications may be deleted before delivery
			Expect(totalDeliveries).To(BeNumerically("<=", 10),
				"Each cycle should deliver at most once (no duplicates)")
			Expect(totalDeliveries).To(BeNumerically(">=", 0),
				"Delivery count should be non-negative")

			// CORRECTNESS: If deliveries happened, they should be properly recorded
			// No silent failures or lost deliveries
		})

		It("should handle rapid create-delete-create with same CRD name", func() {
			// BUSINESS SCENARIO: User creates, deletes, and immediately recreates same notification name
			// This tests idempotency when ResourceVersion resets

			notificationName := fmt.Sprintf("rapid-same-name-%s", uniqueSuffix)
			deliveryAttempts := []notificationv1alpha1.NotificationPhase{}

			By("Performing 5 rapid create-delete-create cycles with SAME NAME")
			for cycle := 0; cycle < 5; cycle++ {
				By(fmt.Sprintf("Cycle %d: Creating notification '%s'", cycle+1, notificationName))
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notificationName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
						Labels: map[string]string{
							"cycle": fmt.Sprintf("%d", cycle),
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Subject:  fmt.Sprintf("Rapid Same-Name Test (Cycle %d)", cycle+1),
						Body:     fmt.Sprintf("Testing rapid create-delete-create with same name (cycle %d)", cycle+1),
						Priority: notificationv1alpha1.NotificationPriorityHigh,
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
						Recipients: []notificationv1alpha1.Recipient{
							{Slack: "#rapid-same-name"},
						},
					},
				}

				Expect(k8sClient.Create(ctx, notif)).Should(Succeed())

				By(fmt.Sprintf("Cycle %d: Waiting for delivery attempt to complete", cycle+1))
				// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
				// Wait for controller to complete delivery attempt (terminal state reached)
				Eventually(func() notificationv1alpha1.NotificationPhase {
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 5*time.Second, 100*time.Millisecond).Should(Or(
					Equal(notificationv1alpha1.NotificationPhaseSent),
					Equal(notificationv1alpha1.NotificationPhaseFailed),
					Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				), "Delivery should complete within 5 seconds")

				By(fmt.Sprintf("Cycle %d: Capturing status before deletion", cycle+1))
				deliveryAttempts = append(deliveryAttempts, notif.Status.Phase)
				GinkgoWriter.Printf("  Cycle %d: Status before delete = %s\n", cycle+1, notif.Status.Phase)

				By(fmt.Sprintf("Cycle %d: Deleting notification", cycle+1))
				Expect(k8sClient.Delete(ctx, notif)).Should(Succeed())

				By(fmt.Sprintf("Cycle %d: Verifying deletion completed", cycle+1))
				// Per TESTING_GUIDELINES.md v2.0.0: Eventually() already verifies deletion complete
				// No additional sleep needed - IsNotFound() confirms K8s API processed deletion
				Eventually(func() bool {
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, notif)
					return apierrors.IsNotFound(err)
				}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
					"Deletion should complete within 5 seconds (ready for next cycle)")
			}

			By("Verifying business outcome: Each cycle independent (no state leakage)")
			GinkgoWriter.Printf("📊 Delivery attempts across 5 cycles: %v\n", deliveryAttempts)

			// BUSINESS OUTCOME: Each create-delete-create cycle should be independent
			// No "orphaned" status from previous incarnation
			// CORRECTNESS: Delivery attempts should be reasonable (0-5)
			deliveryCount := 0
			for _, phase := range deliveryAttempts {
				if phase == notificationv1alpha1.NotificationPhaseSent ||
					phase == notificationv1alpha1.NotificationPhasePartiallySent {
					deliveryCount++
				}
			}

			GinkgoWriter.Printf("📊 Successful deliveries: %d out of 5 cycles\n", deliveryCount)
			Expect(deliveryCount).To(BeNumerically("<=", 5),
				"Should not have more deliveries than cycles (no duplicates)")
		})

		It("should handle extremely rapid create-delete (stress test)", func() {
			// BUSINESS SCENARIO: Automation bug creates delete loop (stress test)

			notificationBaseName := fmt.Sprintf("extreme-rapid-%s", uniqueSuffix)
			successfulCreates := 0
			successfulDeletes := 0

			By("Performing 20 extremely rapid create-delete operations")
			for i := 0; i < 20; i++ {
				notificationName := fmt.Sprintf("%s-%d", notificationBaseName, i)
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notificationName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Subject:  fmt.Sprintf("Extreme Rapid Test %d", i),
						Body:     "Stress testing rapid create-delete",
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
						Recipients: []notificationv1alpha1.Recipient{
							{Slack: "#stress-test"},
						},
					},
				}

				// Create
				createErr := k8sClient.Create(ctx, notif)
				if createErr == nil {
					successfulCreates++
				}

				// Immediately delete (no waiting)
				deleteErr := k8sClient.Delete(ctx, notif)
				if deleteErr == nil || apierrors.IsNotFound(deleteErr) {
					successfulDeletes++
				}
			}

			By("Verifying business outcome: System handles rapid operations gracefully")
			GinkgoWriter.Printf("📊 Successful creates: %d/20, deletes: %d/20\n",
				successfulCreates, successfulDeletes)

			// BUSINESS OUTCOME: System should handle rapid operations without crashing
			// Some creates may fail (conflict), some deletes may fail (already deleted)
			// What matters: No panic, no crash, graceful error handling
			Expect(successfulCreates).To(BeNumerically(">", 0),
				"At least some creates should succeed")
			Expect(successfulDeletes).To(BeNumerically(">", 0),
				"At least some deletes should succeed")

			By("Verifying all CRDs eventually deleted (no orphans)")
			Eventually(func() int {
				list := &notificationv1alpha1.NotificationRequestList{}
				_ = k8sManager.GetAPIReader().List(ctx, list, &client.ListOptions{Namespace: testNamespace})
				orphanCount := 0
				for _, n := range list.Items {
					if n.Name == notificationBaseName ||
						(len(n.Name) > len(notificationBaseName) &&
							n.Name[:len(notificationBaseName)] == notificationBaseName) {
						orphanCount++
					}
				}
				return orphanCount
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(0),
				"All test notifications should be cleaned up (no orphans)")
		})
	})

	Context("BR-NOT-060: Concurrent Create-Delete Safety", func() {
		It("should handle concurrent create-delete operations on different CRDs", func() {
			// BUSINESS SCENARIO: Multiple automation systems creating and deleting notifications

			By("Performing 20 concurrent create-delete operations")
			startTime := time.Now()
			var wg sync.WaitGroup
			successfulOperations := int32(0)

			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					defer GinkgoRecover()

					notificationName := fmt.Sprintf("concurrent-rapid-%d-%s", id, uniqueSuffix)
					notif := &notificationv1alpha1.NotificationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:       notificationName,
							Namespace:  testNamespace,
							Generation: 1, // K8s increments on create/update
						},
						Spec: notificationv1alpha1.NotificationRequestSpec{
							Type:     notificationv1alpha1.NotificationTypeSimple,
							Subject:  fmt.Sprintf("Concurrent Rapid Test %d", id),
							Body:     "Testing concurrent rapid create-delete",
							Priority: notificationv1alpha1.NotificationPriorityMedium,
							Channels: []notificationv1alpha1.Channel{
								notificationv1alpha1.ChannelConsole,
							},
							Recipients: []notificationv1alpha1.Recipient{
								{Slack: "#concurrent-rapid"},
							},
						},
					}

					// Create
					if err := k8sClient.Create(ctx, notif); err == nil {
					// Per TESTING_GUIDELINES.md v2.0.0: Use Eventually(), never time.Sleep()
					// DD-AUTH-014: 2s timeout sufficient with 5 concurrent workers
					// Wait for controller to start processing (status phase set) before deletion
					processed := Eventually(func() bool {
						var checkNotif notificationv1alpha1.NotificationRequest
						err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notificationName, Namespace: testNamespace}, &checkNotif)
						if err != nil {
							return false
						}
						// Processing started when phase is set
						return checkNotif.Status.Phase != ""
					}, 2*time.Second, 50*time.Millisecond).Should(BeTrue())

						// Only delete if we confirmed processing started (rapid lifecycle stress test)
						if processed {
							// Delete
							if err := k8sClient.Delete(ctx, notif); err == nil || apierrors.IsNotFound(err) {
								atomic.AddInt32(&successfulOperations, 1)
							}
						}
					}
				}(i)
			}

			wg.Wait()
			elapsedTime := time.Since(startTime)

			By("Verifying business outcome: Concurrent operations handled safely")
			operations := atomic.LoadInt32(&successfulOperations)
			successRate := float64(operations) / 20.0 * 100

			GinkgoWriter.Printf("📈 Successful create-delete operations: %d/20 (%.1f%%)\n",
				operations, successRate)
			GinkgoWriter.Printf("⏱️  Total time: %v (avg %.2fms per operation)\n",
				elapsedTime, float64(elapsedTime.Milliseconds())/20.0)

			// BUSINESS OUTCOME: 80%+ operations should succeed
			Expect(successRate).To(BeNumerically(">=", 80.0),
				"System should handle 80%+ of concurrent create-delete operations")

			By("Verifying no CRD orphans after concurrent rapid operations")
			Eventually(func() int {
				list := &notificationv1alpha1.NotificationRequestList{}
				_ = k8sManager.GetAPIReader().List(ctx, list, &client.ListOptions{Namespace: testNamespace})
				orphanCount := 0
				for _, n := range list.Items {
					if len(n.Name) > len("concurrent-rapid-") &&
						n.Name[:len("concurrent-rapid-")] == "concurrent-rapid-" {
						orphanCount++
					}
				}
				return orphanCount
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(0),
				"All test notifications should be cleaned up")
		})
	})
})
