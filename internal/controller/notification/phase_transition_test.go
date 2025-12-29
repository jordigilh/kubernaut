package notification

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/metrics"
)

func TestPhaseTransition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase Transition Logic Test Suite")
}

var _ = Describe("Phase Transition Logic - Retrying vs PartiallySent", func() {
	var (
		reconciler   *NotificationRequestReconciler
		ctx          context.Context
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Mock dependencies (minimal setup for unit test)
		reconciler = &NotificationRequestReconciler{
			Metrics: &metrics.NoOpRecorder{}, // No-op metrics for test
		}
	})

	// ========================================
	// NT-BUG-006: PartiallySent vs Retrying Phase Confusion
	// ========================================
	// This test reproduces the exact scenario from failing E2E tests:
	// - Console channel: Succeeds on first attempt
	// - File channel: Fails on first attempt (1/5 attempts used)
	// - Expected: Transition to Retrying (4 more attempts available)
	// - Actual (BUG): Transitions to PartiallySent (terminal phase)
	//
	// Root Cause: allChannelsExhausted logic incorrectly returns true
	// when hasSuccess=true for console channel, even though file channel
	// has 4 remaining retry attempts.
	Context("NT-BUG-006: Partial Success with Retries Remaining", func() {
		It("should transition to Retrying when one channel succeeds and another fails with retries remaining", func() {
			// ===== ARRANGE =====
			// Simulate E2E test scenario:
			// - 2 channels: Console (success), File (failed, 1 attempt)
			// - MaxAttempts: 5 (4 more retries available for file channel)
			notification = &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-retry-transition",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Unit Test: Phase Transition Bug",
					Body:     "Testing Retrying vs PartiallySent transition",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Succeeds
						notificationv1alpha1.ChannelFile,    // Fails, but has retries
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,  // 5 attempts allowed
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
					// Console: 1 successful delivery
					// File: 1 failed delivery (4 more attempts available)
					DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
						{
							Channel:   "console",
							Timestamp: metav1.Now(),
							Status:    "success",
							Attempt:   1,
						},
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "permission denied: read-only directory",
							Attempt:   1, // Only 1 attempt, 4 more available
						},
					},
					SuccessfulDeliveries: 1, // Console succeeded
					FailedDeliveries:     1, // File failed (but retries remain)
				},
			}

			// Create delivery result matching the status
			// deliveryResults is map[string]error where key=channel, value=error (nil if success)
			result := &deliveryLoopResult{
				deliveryResults: map[string]error{
					"console": nil,                                           // Success
					"file":    fmt.Errorf("permission denied: read-only directory"), // Failure
				},
				failureCount: 1,
				deliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
					notification.Status.DeliveryAttempts[0],
					notification.Status.DeliveryAttempts[1],
				},
			}

			// ===== ACT =====
			ctrlResult, err := reconciler.determinePhaseTransition(ctx, notification, result)

			// ===== ASSERT =====
			Expect(err).ToNot(HaveOccurred(), "Phase transition should not return error")

			// CRITICAL ASSERTION: Should transition to Retrying, NOT PartiallySent
			Expect(notification.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseRetrying),
				"Should transition to Retrying when partial success occurs with retries remaining.\n"+
					"Current bug: Transitions to PartiallySent (terminal) prematurely.\n"+
					"Expected: Retrying (non-terminal, allows 4 more retry attempts)\n"+
					"Console: SUCCESS (1 attempt)\n"+
					"File: FAILED (1/5 attempts, 4 remaining)")

			// Should schedule retry with backoff
			Expect(ctrlResult.RequeueAfter).To(BeNumerically(">", 0),
				"Should schedule next retry with exponential backoff")

			// Verify status counters are preserved
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(1),
				"Should maintain successful delivery count")
			Expect(notification.Status.FailedDeliveries).To(Equal(1),
				"Should maintain failed delivery count")
		})

		It("should only transition to PartiallySent when retries are EXHAUSTED", func() {
			// ===== ARRANGE =====
			// All retry attempts exhausted for failed channel
			notification = &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-exhausted-transition",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Unit Test: Exhausted Retries",
					Body:     "Testing PartiallySent when retries exhausted",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Succeeds
						notificationv1alpha1.ChannelFile,    // Failed all 5 attempts
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseRetrying,
					// Console: 1 successful delivery
					// File: 5 failed deliveries (all attempts exhausted)
					DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
						{
							Channel:   "console",
							Timestamp: metav1.Now(),
							Status:    "success",
							Attempt:   1,
						},
						// File channel: All 5 attempts failed
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "attempt 1: permission denied",
							Attempt:   1,
						},
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "attempt 2: permission denied",
							Attempt:   2,
						},
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "attempt 3: permission denied",
							Attempt:   3,
						},
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "attempt 4: permission denied",
							Attempt:   4,
						},
						{
							Channel:   "file",
							Timestamp: metav1.Now(),
							Status:    "failed",
							Error:     "attempt 5: permission denied",
							Attempt:   5, // Final attempt
						},
					},
					SuccessfulDeliveries: 1, // Console succeeded
					FailedDeliveries:     5, // File failed all attempts
				},
			}

			// Create delivery result for final failed attempt
			result := &deliveryLoopResult{
				deliveryResults: map[string]error{
					"file": fmt.Errorf("attempt 5: permission denied"),
				},
				failureCount: 1,
				deliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
					notification.Status.DeliveryAttempts[5], // Final attempt
				},
			}

			// ===== ACT =====
			ctrlResult, err := reconciler.determinePhaseTransition(ctx, notification, result)

			// ===== ASSERT =====
			Expect(err).ToNot(HaveOccurred(), "Phase transition should not return error")

			// NOW PartiallySent is correct (all retries exhausted)
			Expect(notification.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"Should transition to PartiallySent when retries are EXHAUSTED.\n"+
					"Console: SUCCESS (1 attempt)\n"+
					"File: FAILED (5/5 attempts, 0 remaining)")

			// Should NOT schedule retry (terminal phase)
			Expect(ctrlResult.RequeueAfter).To(Equal(ctrl.Result{}.RequeueAfter),
				"Should NOT requeue - terminal phase reached")

			// Verify completion time is set for terminal phase
			Expect(notification.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime should be set for terminal phase")
		})
	})

	Context("Edge Cases - Phase Transition Logic", func() {
		It("should transition to Retrying when ALL channels fail but retries remain", func() {
			// ===== ARRANGE =====
			notification = &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-all-failed-retries-remain",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelFile,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
					// Both channels failed on first attempt
					DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
						{
							Channel: "console",
							Status:  "failed",
							Error:   "network timeout",
							Attempt: 1,
						},
						{
							Channel: "file",
							Status:  "failed",
							Error:   "permission denied",
							Attempt: 1,
						},
					},
					SuccessfulDeliveries: 0,
					FailedDeliveries:     2,
				},
			}

			result := &deliveryLoopResult{
				deliveryResults: map[string]error{
					"console": fmt.Errorf("network timeout"),
					"file":    fmt.Errorf("permission denied"),
				},
				failureCount: 2,
				deliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
					notification.Status.DeliveryAttempts[0],
					notification.Status.DeliveryAttempts[1],
				},
			}

			// ===== ACT =====
			ctrlResult, err := reconciler.determinePhaseTransition(ctx, notification, result)

			// ===== ASSERT =====
			Expect(err).ToNot(HaveOccurred())

			// Should transition to Retrying (not Failed)
			Expect(notification.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseRetrying),
				"Should transition to Retrying when all channels fail but retries remain.\n"+
					"Console: FAILED (1/5 attempts, 4 remaining)\n"+
					"File: FAILED (1/5 attempts, 4 remaining)")

			Expect(ctrlResult.RequeueAfter).To(BeNumerically(">", 0),
				"Should schedule retry with backoff")
		})

		It("should transition to Sent when all channels succeed", func() {
			// ===== ARRANGE =====
			notification = &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-all-succeeded",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelFile,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
					DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
						{
							Channel: "console",
							Status:  "success",
							Attempt: 1,
						},
						{
							Channel: "file",
							Status:  "success",
							Attempt: 1,
						},
					},
					SuccessfulDeliveries: 2,
					FailedDeliveries:     0,
				},
			}

			result := &deliveryLoopResult{
				deliveryResults: map[string]error{
					"console": nil,
					"file":    nil,
				},
				failureCount: 0,
				deliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
					notification.Status.DeliveryAttempts[0],
					notification.Status.DeliveryAttempts[1],
				},
			}

			// ===== ACT =====
			ctrlResult, err := reconciler.determinePhaseTransition(ctx, notification, result)

			// ===== ASSERT =====
			Expect(err).ToNot(HaveOccurred())

			Expect(notification.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Should transition to Sent when all channels succeed")

			Expect(ctrlResult.RequeueAfter).To(Equal(ctrl.Result{}.RequeueAfter),
				"Should NOT requeue - terminal phase reached")
		})
	})
})

