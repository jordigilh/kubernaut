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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// ========================================
// RETRY LOGIC INTEGRATION TESTS
// 📋 Business Requirement: BR-NOT-054 (Exponential Backoff Retry)
// 📋 Migrated From: test/e2e/notification/05_retry_exponential_backoff_test.go
// ========================================
//
// WHY INTEGRATION TIER IS BETTER:
// - ✅ Deterministic failure simulation (mock services)
// - ✅ Fast execution (~seconds instead of ~minutes in E2E)
// - ✅ Can verify exact retry intervals and counts
// - ✅ No file system or cluster infrastructure dependencies
//
// MIGRATION RATIONALE:
// The E2E test was pending (PIt) because it required specifying a read-only
// directory to simulate file write failures, which is no longer possible after
// FileDeliveryConfig removal (DD-NOT-006 v2). Integration tests with mock
// services provide the same coverage without infrastructure complexity.
//
// ========================================

var _ = Describe("Controller Retry Logic (BR-NOT-054)", func() {
	Context("When file delivery fails repeatedly", func() {
		It("should retry with exponential backoff up to max attempts", func() {
			// ========================================
			// TEST SETUP: Mock file service that always fails with RETRYABLE error
			// ========================================
			mockFileService := &testutil.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					// Return retryable error so controller will retry
					return delivery.NewRetryableError(fmt.Errorf("simulated file write failure"))
				},
			}

			// ========================================
			// TEST SETUP: Mock console service that always succeeds
			// ========================================
			mockConsoleService := &testutil.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil // Success
				},
			}

			// Register mock services
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockConsoleService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), mockFileService)
			DeferCleanup(func() {
				// Restore original services
				deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), originalConsoleService)
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelFile))
			})

			// ========================================
			// CREATE TEST NOTIFICATION WITH RETRY POLICY
			// ========================================
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-retry-backoff-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test-scenario": "retry-exponential-backoff",
						"test-tier":     "integration",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Integration Test: Retry with Exponential Backoff",
					Body:     "Testing automatic retry logic with mock file delivery",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Should succeed
						notificationv1alpha1.ChannelFile,    // Will fail
					},
					// Integration Test Optimization: Use shorter backoff for faster tests
					// Production default: 30s initial, 480s max
					// Integration override: 1s initial, 10s max (~20s total for 5 attempts)
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 1,  // 1s instead of 30s for fast tests
						BackoffMultiplier:     2,  // Same as production (exponential 2x)
						MaxBackoffSeconds:     60, // Minimum allowed by CRD validation
					},
				},
			}

			startTime := time.Now()
			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			// ========================================
			// PHASE 1: Wait for initial delivery attempts
			// ========================================
			By("Waiting for controller to attempt initial delivery")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 5*time.Second, 200*time.Millisecond).Should(Or(
				Equal(notificationv1alpha1.NotificationPhaseSending),
				Equal(notificationv1alpha1.NotificationPhaseRetrying)),
				"DD-E2E-003: With instant mocks, may skip Sending and jump to Retrying")

			// ========================================
			// PHASE 2: Wait for retry logic to execute
			// ========================================
			By("Waiting for exponential backoff retries (up to 5 attempts)")
			// Expected retry intervals: 1s, 2s, 4s, 8s, 10s (capped at max) = ~25s total
			Eventually(func() int {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return 0
				}
				return len(notification.Status.DeliveryAttempts)
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(5), "Should attempt delivery 5 times (initial + 4 retries)")

			// ========================================
			// PHASE 3: Verify final state after max attempts
			// ========================================
			By("Verifying notification marked as PartiallySent after max retries")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"DD-E2E-003: After retry exhaustion (1s+2s+4s+8s=~15s) → PartiallySent")

			// ========================================
			// ASSERTIONS: Retry Logic Validation
			// ========================================
			elapsedTime := time.Since(startTime)

			By("Validating retry statistics (BR-NOT-054)")
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(1),
				"Console delivery should succeed (1 successful)")
			Expect(notification.Status.FailedDeliveries).To(Equal(1),
				"File delivery should fail after max retries (1 failed)")
			Expect(len(notification.Status.DeliveryAttempts)).To(Equal(5),
				"Should record all 5 delivery attempts (initial + 4 retries)")

			By("Validating exponential backoff timing")
			// Expected minimum time: 1s + 2s + 4s + 8s + 10s = 25s
			// Allow some buffer for controller processing
			Expect(elapsedTime.Seconds()).To(BeNumerically(">=", 20),
				"Retry logic should take at least 20s (exponential backoff)")
			Expect(elapsedTime.Seconds()).To(BeNumerically("<", 40),
				"Retry logic should not exceed 40s (reasonable upper bound)")

			By("Validating mock service call counts")
			Expect(mockFileService.GetCallCount()).To(Equal(5),
				"File service should be called 5 times (initial + 4 retries)")
			Expect(mockConsoleService.GetCallCount()).To(Equal(1),
				"Console service should be called once (no retries needed)")

			// ========================================
			// CLEANUP: Remove test notification
			// ========================================
			err = k8sClient.Delete(ctx, notification)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When delivery succeeds on retry", func() {
		It("should stop retrying after first success", func() {
			attemptCount := 0

			// ========================================
			// TEST SETUP: Mock file service that fails twice, then succeeds
			// ========================================
			mockFileService := &testutil.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					attemptCount++
					if attemptCount <= 2 {
						// Return retryable error so controller will retry
						return delivery.NewRetryableError(fmt.Errorf("simulated transient failure (attempt %d)", attemptCount))
					}
					return nil // Success on 3rd attempt
				},
			}

			// Register mock service
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), mockFileService)
			DeferCleanup(func() {
				// Restore original state (file service not registered in suite)
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelFile))
			})

			// ========================================
			// CREATE TEST NOTIFICATION WITH RETRY POLICY
			// ========================================
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-retry-success-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test-scenario": "retry-until-success",
						"test-tier":     "integration",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Integration Test: Retry Until Success",
					Body:     "Testing that retries stop after first success",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelFile,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // Minimum allowed by CRD validation
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// PHASE 1: Wait for successful delivery after retries
			// ========================================
			By("Waiting for delivery to succeed on 3rd attempt")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Should transition to Sent after successful retry")

			// ========================================
			// ASSERTIONS: Verify retry stopped after success
			// ========================================
			By("Validating retry logic stopped after success (BR-NOT-054)")
			Expect(len(notification.Status.DeliveryAttempts)).To(Equal(3),
				"Should stop retrying after first success (3 attempts: 2 failures + 1 success)")
			Expect(mockFileService.GetCallCount()).To(Equal(3),
				"File service should be called exactly 3 times")
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(1),
				"Should record 1 successful delivery")
			Expect(notification.Status.FailedDeliveries).To(Equal(0),
				"Should record 0 failed deliveries (final attempt succeeded)")

			// ========================================
			// CLEANUP
			// ========================================
			err = k8sClient.Delete(ctx, notification)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
