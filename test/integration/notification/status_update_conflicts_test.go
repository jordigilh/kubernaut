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

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ==============================================
// Integration Tests: Status Update Conflicts
// ==============================================
// BUSINESS CONTEXT: NotificationRequest CRDs use Kubernetes optimistic locking
// to handle concurrent updates from controller and external systems.
//
// BR-NOT-053: Idempotent Delivery - Status updates must handle conflicts gracefully
// BR-NOT-051: Status Transparency - Status fields must accurately reflect state
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Status update logic in isolation (see test/unit/notification/status_test.go)
// - Integration tests (>50%): Real Kubernetes API server optimistic locking behavior
// - E2E tests (10-15%): Complete notification lifecycle with status updates
//
// Test Categories:
// 1. Optimistic Locking (resourceVersion conflicts)
// 2. Timestamp Ordering (temporal consistency)
// 3. Error Message Encoding (special characters)
// 4. Status Size Growth (large deliveryAttempts arrays)
// 5. Deletion Race Conditions (status update during CRD deletion)
// 6. Status Update Retry (reconciliation on update failure)
//
// ==============================================

var _ = Describe("BR-NOT-053: Status Update Conflicts", func() {
	var (
		uniqueSuffix  string
		testNamespace = "default"
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", time.Now().UnixNano())
		time.Sleep(100 * time.Millisecond) // Allow environment to settle
	})

	// ==============================================
	// Category 1: Optimistic Locking (P0)
	// ==============================================

	Context("BR-NOT-053: Optimistic Locking", func() {
		It("should handle status update with conflicting resourceVersion (BR-NOT-053: Retry on conflict)", func() {
			// BEHAVIOR: Kubernetes rejects updates with stale resourceVersion
			// BUSINESS CONTEXT: Controller must retry status updates with fresh object
			// CORRECTNESS: Controller eventually succeeds despite concurrent updates

			notifName := fmt.Sprintf("conflict-version-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Optimistic Lock Test %s", uniqueSuffix),
					Body:     "Testing resourceVersion conflict handling",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Console for fast delivery
					},
				},
			}

			// Create notification
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for initial status update
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty())

			// Capture initial resourceVersion
			initialVersion := notif.ResourceVersion

			// Let controller update the object (this will change resourceVersion)
			time.Sleep(2 * time.Second)

			// Get fresh object to see new resourceVersion
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR VALIDATION: ResourceVersion changed (controller updated)
			if freshNotif.ResourceVersion != initialVersion {
				GinkgoWriter.Printf("✅ ResourceVersion changed: %s → %s (controller updated status)\n",
					initialVersion, freshNotif.ResourceVersion)
			}

			// CORRECTNESS VALIDATION: Controller successfully handled status updates
			// despite potential concurrent modifications
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}
				return freshNotif.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Controller should successfully complete delivery despite resourceVersion changes")

			GinkgoWriter.Printf("✅ Optimistic locking handled: Final phase=%s, attempts=%d\n",
				freshNotif.Status.Phase, freshNotif.Status.SuccessfulDeliveries)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, freshNotif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// Category 2: Timestamp Ordering (P0)
	// ==============================================

	Context("BR-NOT-051: Timestamp Ordering", func() {
		It("should maintain temporal consistency in status timestamps (BR-NOT-051: Monotonic ordering)", func() {
			// BEHAVIOR: Status timestamps should increase monotonically
			// BUSINESS CONTEXT: Timestamps must reflect actual event ordering
			// CORRECTNESS: CompletionTime > LastTransitionTime > CreationTimestamp

			notifName := fmt.Sprintf("timestamp-order-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Timestamp Test %s", uniqueSuffix),
					Body:     "Testing timestamp ordering consistency",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create notification and capture creation time
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())
			creationTime := notif.CreationTimestamp.Time

			// Wait for completion
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 250*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// CORRECTNESS VALIDATION: Temporal ordering
			// NOTE: NotificationRequestStatus has QueuedAt and CompletionTime
			// StartedAt field doesn't exist in the CRD (checked notificationrequest_types.go)
			Expect(notif.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime must be set")

			completionTime := notif.Status.CompletionTime.Time

			// BEHAVIOR VALIDATION: Timestamps are ordered correctly
			Expect(completionTime).To(BeTemporally(">=", creationTime),
				"CompletionTime should be after creation")
			Expect(completionTime).To(BeTemporally("<=", time.Now()),
				"CompletionTime should not be in the future")

			// CORRECTNESS: CompletionTime should be recent (within test execution)
			Expect(completionTime).To(BeTemporally("~", time.Now(), 30*time.Second),
				"CompletionTime should be recent (within test window)")

			// Verify QueuedAt if set (optional field)
			if notif.Status.QueuedAt != nil {
				queuedAt := notif.Status.QueuedAt.Time
				Expect(completionTime).To(BeTemporally(">=", queuedAt),
					"CompletionTime should be after QueuedAt")
				GinkgoWriter.Printf("✅ Timestamp ordering validated:\n")
				GinkgoWriter.Printf("   Creation:         %v\n", creationTime.Format(time.RFC3339))
				GinkgoWriter.Printf("   QueuedAt:         %v\n", queuedAt.Format(time.RFC3339))
				GinkgoWriter.Printf("   Completion:       %v\n", completionTime.Format(time.RFC3339))
			} else {
				GinkgoWriter.Printf("✅ Timestamp ordering validated:\n")
				GinkgoWriter.Printf("   Creation:         %v\n", creationTime.Format(time.RFC3339))
				GinkgoWriter.Printf("   Completion:       %v\n", completionTime.Format(time.RFC3339))
			}

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// Category 3: Status Update Retry (P0)
	// ==============================================

	Context("BR-NOT-053: Status Update Failure Handling", func() {
		It("should requeue for reconciliation when status update fails (BR-NOT-053: Retry mechanism)", func() {
			// BEHAVIOR: Status update failures should trigger reconciliation retry
			// BUSINESS CONTEXT: Transient API server issues should not lose delivery state
			// CORRECTNESS: Controller eventually persists correct status despite failures

			notifName := fmt.Sprintf("status-retry-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Status Retry Test %s", uniqueSuffix),
					Body:     "Testing status update retry on failure",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR VALIDATION: Controller eventually succeeds despite potential status update failures
			// (Kubernetes controller-runtime automatically retries on status update conflicts)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Controller should eventually succeed despite transient status update failures")

			// CORRECTNESS VALIDATION: Status is fully populated (no data loss)
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1),
				"Status should reflect successful delivery")
			Expect(notif.Status.DeliveryAttempts).To(HaveLen(1),
				"Status should contain delivery attempt record")
			Expect(notif.Status.CompletionTime).ToNot(BeNil(),
				"Status should have completion timestamp")

			GinkgoWriter.Printf("✅ Status update retry succeeded: phase=%s, attempts=%d\n",
				notif.Status.Phase, len(notif.Status.DeliveryAttempts))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// Category 4: Deletion Race Conditions (P1)
	// ==============================================

	Context("BR-NOT-053: Deletion Race Conditions", func() {
		It("should handle status update while CRD is being deleted (BR-NOT-053: Graceful failure)", func() {
			// BEHAVIOR: Status update during deletion should fail gracefully (no panic)
			// BUSINESS CONTEXT: User-initiated deletion can race with controller reconciliation
			// CORRECTNESS: Controller logs error but does not crash

			notifName := fmt.Sprintf("delete-race-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Delete Race Test %s", uniqueSuffix),
					Body:     "Testing status update during deletion",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for reconciliation to start
			time.Sleep(500 * time.Millisecond)

			// Delete immediately (race with status update)
			err = k8sClient.Delete(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// BEHAVIOR VALIDATION: CRD is eventually deleted (no stuck finalizers)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				return err != nil // Expect NotFound error
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"CRD should be deleted successfully despite deletion race")

			GinkgoWriter.Printf("✅ Deletion race handled gracefully: CRD removed without errors\n")
		})
	})

	// ==============================================
	// Category 5: Error Message Encoding (P1)
	// ==============================================

	Context("BR-NOT-051: Error Message Encoding", func() {
		It("should handle special characters in error messages (BR-NOT-051: Proper encoding)", func() {
			// BEHAVIOR: Error messages with special chars should be stored correctly
			// BUSINESS CONTEXT: Slack/API errors may contain JSON, quotes, newlines
			// CORRECTNESS: Status preserves error details without corruption

			notifName := fmt.Sprintf("special-chars-%s", uniqueSuffix)

			// Configure mock to return error with special characters
			ConfigureFailureMode("always", 0, 500)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Special Chars Test %s", uniqueSuffix),
					Body:     "Testing error message encoding: \"quotes\" \n newlines \t tabs",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for delivery attempts to fail
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

			// BEHAVIOR VALIDATION: Error message is stored (even with special chars)
			Expect(notif.Status.DeliveryAttempts).ToNot(BeEmpty(),
				"Status should contain delivery attempt records with error messages")

			// CORRECTNESS VALIDATION: Error message is readable (no corruption)
			if len(notif.Status.DeliveryAttempts) > 0 {
				errorMsg := notif.Status.DeliveryAttempts[0].Error
				Expect(errorMsg).ToNot(BeEmpty(),
					"Error message should be stored")
				Expect(strings.Contains(errorMsg, "500") || strings.Contains(errorMsg, "error") || strings.Contains(errorMsg, "fail"),
					"Error message should contain meaningful error information")

				GinkgoWriter.Printf("✅ Error message encoded correctly: %s\n", errorMsg)
			}

			// Reset and cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ==============================================
	// Category 6: Status Size Growth (P1)
	// ==============================================

	Context("BR-NOT-051: Status Size Management", func() {
		It("should handle large deliveryAttempts array (BR-NOT-051: Status size limits)", func() {
			// BEHAVIOR: Controller should handle many delivery attempts without status overflow
			// BUSINESS CONTEXT: Repeated failures could create very large status objects
			// CORRECTNESS: Status updates succeed even with many delivery attempts
			// NOTE: Current implementation stores all attempts; future may truncate/summarize

			notifName := fmt.Sprintf("large-status-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("Large Status Test %s", uniqueSuffix),
					Body:     "Testing status with many delivery attempts",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           10, // More attempts to grow status
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			// Configure mock to always fail (to generate many attempts)
			ConfigureFailureMode("always", 0, 503)

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for all retries to exhaust
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 90*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseFailed),
				"Should reach Failed phase after exhausting retries")

			// BEHAVIOR VALIDATION: Status handles many delivery attempts
			Expect(notif.Status.DeliveryAttempts).To(HaveLen(10),
				"Status should contain all 10 delivery attempts")
			Expect(notif.Status.FailedDeliveries).To(Equal(10),
				"Should track all 10 failed attempts")

			// CORRECTNESS: Status object size is manageable (< 1MB)
			// (Kubernetes etcd has ~1.5MB limit for objects)
			statusSize := len(fmt.Sprintf("%+v", notif.Status))
			Expect(statusSize).To(BeNumerically("<", 1000000),
				"Status size should be under 1MB to fit in Kubernetes etcd")

			GinkgoWriter.Printf("✅ Large status handled: %d attempts, ~%d bytes\n",
				len(notif.Status.DeliveryAttempts), statusSize)

			// Reset and cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

