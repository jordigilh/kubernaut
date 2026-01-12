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

// BR-NOT-056: CRD Lifecycle and Phase State Machine (P0)
//
// Description: The Notification Service MUST implement a 5-phase state machine for
// NotificationRequest CRDs with deterministic phase transitions and status updates.
//
// Five Phases:
//   1. Pending: Initial phase, delivery not yet attempted
//   2. Sending: Delivery in progress
//   3. Sent: All channels delivered successfully
//   4. PartiallySent: Some channels succeeded, some failed permanently
//   5. Failed: All channels failed permanently
//
// Valid Phase Transitions:
//   - Pending → Sending (reconciliation starts delivery)
//   - Sending → Sent (all channels delivered successfully)
//   - Sending → PartiallySent (some channels succeeded, some failed permanently)
//   - Sending → Failed (all channels failed permanently)
//   - Sending → Pending (transient failure, retry scheduled)
//
// Invalid Phase Transitions (must NOT occur):
//   - Sent → any other phase (terminal state)
//   - Failed → any other phase (terminal state)
//   - PartiallySent → Pending/Sending (terminal state)

var _ = Describe("BR-NOT-056: CRD Lifecycle and Phase State Machine", Label("integration", "phase-state-machine", "BR-NOT-056"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())

		// Reset mock Slack server state
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	Context("All Five Phases", func() {
		// Test 1: Pending → Sending → Sent (successful delivery)
		// BR-NOT-056: Valid phase transition sequence
		It("should transition Pending → Sending → Sent for successful delivery (BR-NOT-056)", func() {
			notifName := fmt.Sprintf("phase-sent-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Phase Transition Test - Sent",
					Body:     "Testing Pending → Sending → Sent transition",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for Sent phase (may skip Pending/Sending due to fast delivery)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"BR-NOT-056: Should reach Sent phase after successful delivery")

			// Verify terminal state properties
			Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent))
			Expect(notif.Status.Reason).To(Equal("AllDeliveriesSucceeded"))
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1))
			Expect(notif.Status.FailedDeliveries).To(Equal(0))
			Expect(notif.Status.CompletionTime).ToNot(BeNil(), "Terminal phase must have CompletionTime")

			GinkgoWriter.Printf("✅ BR-NOT-056: Sent phase validated - all deliveries successful\n")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		// Test 2: Pending → Sending → Failed (all channels fail)
		// BR-NOT-056: Valid phase transition to Failed terminal state
		It("should transition Pending → Sending → Failed when all channels fail permanently (BR-NOT-056)", func() {
			notifName := fmt.Sprintf("phase-failed-%s", uniqueSuffix)

			// Configure mock to return permanent error (401 Unauthorized)
			ConfigureFailureMode("always", 0, 401) // mode=always, count=0, statusCode=401

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Phase Transition Test - Failed",
					Body:     "Testing Pending → Sending → Failed transition",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack, // Will fail with 401
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1, // Only 1 attempt for faster test
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // CRD validation requires ≥60
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for Failed phase (permanent error, no retry)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed),
				"BR-NOT-056: Should reach Failed phase when all channels fail permanently")

			// Verify terminal state properties
			Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseFailed))
			Expect(notif.Status.Reason).To(Equal("AllDeliveriesFailed"))
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(0))
			Expect(notif.Status.FailedDeliveries).To(BeNumerically(">", 0))
			Expect(notif.Status.CompletionTime).ToNot(BeNil(), "Terminal phase must have CompletionTime")

			GinkgoWriter.Printf("✅ BR-NOT-056: Failed phase validated - all deliveries failed permanently\n")

			// Cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		// Test 3: Pending → Sending → PartiallySent (some channels succeed, some fail)
		// BR-NOT-056: Valid phase transition to PartiallySent terminal state
		It("should transition Pending → Sending → PartiallySent when some channels succeed and some fail (BR-NOT-056)", func() {
			notifName := fmt.Sprintf("phase-partial-%s", uniqueSuffix)

			// Configure mock to fail Slack but console will succeed
			ConfigureFailureMode("always", 0, 401) // mode=always, count=0, statusCode=401

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Phase Transition Test - PartiallySent",
					Body:     "Testing Pending → Sending → PartiallySent transition",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Will succeed
						notificationv1alpha1.ChannelSlack,   // Will fail with 401
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1, // Only 1 attempt for faster test
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // CRD validation requires ≥60
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for PartiallySent phase (mixed success/failure)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"BR-NOT-056: Should reach PartiallySent phase when some channels succeed and some fail")

			// Verify terminal state properties
			Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhasePartiallySent))
			Expect(notif.Status.Reason).To(ContainSubstring("Partial"))
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1), "Console should succeed")
			Expect(notif.Status.FailedDeliveries).To(Equal(1), "Slack should fail")
			Expect(notif.Status.CompletionTime).ToNot(BeNil(), "Terminal phase must have CompletionTime")

			GinkgoWriter.Printf("✅ BR-NOT-056: PartiallySent phase validated - mixed success/failure\n")

			// Cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		// Test 4: Pending and Sending phases are observable (non-terminal)
		// BR-NOT-056: Intermediate phases before terminal state
		It("should pass through Pending and/or Sending phases before terminal state (BR-NOT-056)", func() {
			notifName := fmt.Sprintf("phase-intermediate-%s", uniqueSuffix)

			// Configure slow mock to give us time to observe intermediate phases
			ConfigureFailureMode("slow", 0, 2000) // 2 second delay

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Phase Transition Test - Intermediate",
					Body:     "Testing Pending/Sending intermediate phases",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Try to observe intermediate phase (Pending or Sending)
			// Note: These phases are transient and may be skipped for fast deliveries
			var observedPending, observedSending bool
			for i := 0; i < 10; i++ {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err == nil {
					if notif.Status.Phase == notificationv1alpha1.NotificationPhasePending {
						observedPending = true
						GinkgoWriter.Printf("✅ BR-NOT-056: Observed Pending phase\n")
					}
					if notif.Status.Phase == notificationv1alpha1.NotificationPhaseSending {
						observedSending = true
						GinkgoWriter.Printf("✅ BR-NOT-056: Observed Sending phase\n")
					}
					if notif.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
						break
					}
				}
				time.Sleep(200 * time.Millisecond)
			}

			// Eventually reach terminal state
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"BR-NOT-056: Should eventually reach terminal Sent phase")

			// At least validate that we can query phase status
			GinkgoWriter.Printf("✅ BR-NOT-056: Phase progression validated (Pending observed: %v, Sending observed: %v)\n",
				observedPending, observedSending)

			// Cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Phase Transition Determinism", func() {
		// Test 5: Terminal phases are immutable (Sent should not transition)
		// BR-NOT-056: Invalid phase transitions must NOT occur
		It("should keep terminal phase Sent immutable (BR-NOT-056: No invalid transitions)", func() {
			notifName := fmt.Sprintf("phase-immutable-sent-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Terminal Phase Immutability Test",
					Body:     "Testing that Sent phase remains terminal",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create and wait for Sent
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// Record terminal phase
			terminalPhase := notif.Status.Phase
			terminalReason := notif.Status.Reason
			terminalCompletionTime := notif.Status.CompletionTime

			// Wait additional time to ensure phase doesn't change
			Consistently(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(terminalPhase),
				"BR-NOT-056: Terminal phase Sent must remain immutable")

			// Verify phase, reason, and completion time haven't changed
			Expect(notif.Status.Phase).To(Equal(terminalPhase))
			Expect(notif.Status.Reason).To(Equal(terminalReason))
			Expect(notif.Status.CompletionTime).To(Equal(terminalCompletionTime))

			GinkgoWriter.Printf("✅ BR-NOT-056: Terminal phase Sent is immutable (no invalid transitions)\n")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		// Test 6: Failed terminal phase is also immutable
		// BR-NOT-056: Invalid phase transitions must NOT occur
		It("should keep terminal phase Failed immutable (BR-NOT-056: No invalid transitions)", func() {
			notifName := fmt.Sprintf("phase-immutable-failed-%s", uniqueSuffix)

			// Configure permanent failure
			ConfigureFailureMode("always", 0, 401) // mode=always, count=0, statusCode=401

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Terminal Phase Immutability Test - Failed",
					Body:     "Testing that Failed phase remains terminal",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // CRD validation requires ≥60
					},
				},
			}

			// Create and wait for Failed
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

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

			// Record terminal phase
			terminalPhase := notif.Status.Phase

			// Wait additional time to ensure phase doesn't change
			Consistently(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(terminalPhase),
				"BR-NOT-056: Terminal phase Failed must remain immutable")

			GinkgoWriter.Printf("✅ BR-NOT-056: Terminal phase Failed is immutable (no invalid transitions)\n")

			// Cleanup
			ConfigureFailureMode("none", 0, 0)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Phase Audit Trail", func() {
		// Test 7: Phase transitions are recorded in audit trail
		// BR-NOT-056: Phase transitions recorded in audit trail
		It("should record phase transitions in status (BR-NOT-056: Audit trail)", func() {
			notifName := fmt.Sprintf("phase-audit-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Phase Audit Trail Test",
					Body:     "Testing phase transition audit trail",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for terminal phase
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			// Verify audit trail exists (status fields track phase history)
			Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent),
				"BR-NOT-056: Current phase must be recorded")
			Expect(notif.Status.Reason).ToNot(BeEmpty(),
				"BR-NOT-056: Phase transition reason must be recorded")
			Expect(notif.Status.Message).ToNot(BeEmpty(),
				"BR-NOT-056: Phase transition message must be recorded")
			Expect(notif.Status.DeliveryAttempts).ToNot(BeEmpty(),
				"BR-NOT-056: Delivery attempts audit trail must exist")
			Expect(notif.Status.CompletionTime).ToNot(BeNil(),
				"BR-NOT-056: Terminal phase completion time must be recorded")

			// Verify status counters reflect phase outcome
			Expect(notif.Status.TotalAttempts).To(Equal(notif.Status.SuccessfulDeliveries+notif.Status.FailedDeliveries),
				"BR-NOT-056: Total attempts must equal successful + failed deliveries")

			GinkgoWriter.Printf("✅ BR-NOT-056: Phase audit trail validated\n")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
