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
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// PARTIAL FAILURE HANDLING INTEGRATION TESTS
// 📋 Business Requirement: BR-NOT-053 (Multi-Channel Fanout)
// 📋 Migrated From: test/e2e/notification/06_multi_channel_fanout_test.go (Scenario 2)
// ========================================
//
// WHY INTEGRATION TIER IS BETTER:
// - ✅ Deterministic channel failure simulation (mock services)
// - ✅ Fast execution (~seconds instead of ~minutes in E2E)
// - ✅ Can test all failure combinations (any channel fails)
// - ✅ No file system or cluster infrastructure dependencies
//
// MIGRATION RATIONALE:
// The E2E test was pending (PIt) because it required simulating file delivery
// failures, which was infeasible after FileDeliveryConfig removal (DD-NOT-006 v2).
// Integration tests with mock services provide comprehensive coverage of partial
// failure scenarios without infrastructure complexity.
//
// ========================================

var _ = Describe("Controller Partial Failure Handling (BR-NOT-053)", func() {
	Context("When file channel fails but console/log channels succeed", func() {
		It("should mark notification as PartiallySent (not Sent, not Failed)", func() {
			// ========================================
			// PARALLEL EXECUTION FIX: Lock orchestrator mock sequence
			// sync.Map provides thread-safety, but we need to serialize:
			// register mocks → test execution → restore original services
			// ========================================
			orchestratorMockLock.Lock()
			DeferCleanup(func() {
				orchestratorMockLock.Unlock()
			})

			// ========================================
			// TEST SETUP: Mock services with controlled failure
			// ========================================
			mockConsoleService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil // Success
				},
			}

			mockLogService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil // Success
				},
			}

			mockFileService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return fmt.Errorf("disk full (simulated)") // Permanent failure
				},
			}

			// Register mock services (will be cleaned up by DeferCleanup)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockConsoleService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), mockLogService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), mockFileService)
			DeferCleanup(func() {
				// Restore original services (console/slack exist, file/log don't in suite)
				deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), originalConsoleService)
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelLog))  // Not in suite
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelFile)) // Not in suite
			})

			// ========================================
			// CREATE TEST NOTIFICATION WITH MULTIPLE CHANNELS
			// ========================================
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-partial-failure-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test-scenario": "partial-failure",
						"test-tier":     "integration",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Integration Test: Partial Failure Handling",
					Body:     "Testing PartiallySent phase when file fails but console/log succeed",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Will succeed
						notificationv1alpha1.ChannelLog,     // Will succeed
						notificationv1alpha1.ChannelFile,    // Will fail
					},
					// Disable retries for this test (we want to test partial failure, not retry logic)
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1, // No retries
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     1,
						MaxBackoffSeconds:     60, // Minimum allowed by CRD validation
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// PHASE 1: Wait for delivery attempts
			// ========================================
			By("Waiting for controller to attempt delivery to all channels")

			// RACE FIX: First ensure all delivery attempts are recorded
			// In CI's faster environment, the phase transition might happen before
			// all delivery attempts are persisted, causing the test to see an
			// inconsistent state (e.g., phase=PartiallySent but attempts < 3)
			Eventually(func() int {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return -1
				}
				return len(notification.Status.DeliveryAttempts)
			}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 3),
				"All 3 delivery attempts must be recorded before checking phase")

			// Now check the phase (should be PartiallySent)
			// DD-STATUS-001: Increased timeout for parallel execution (12 procs)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"DD-E2E-003: Permanent error (not retryable) → immediate PartiallySent")

			// Ensure phase is stable (no more reconciles) before checking call counts
			Consistently(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 1*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"Phase should remain stable (no more reconciles)")

			// ========================================
			// ASSERTIONS: Partial Failure State Validation
			// ========================================
			// DD-STATUS-001: Wait for all delivery attempts to propagate
			// Increased timeout for parallel execution (12 procs)
			Eventually(func() int {
				err := k8sAPIReader.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return -1
				}
				return len(notification.Status.DeliveryAttempts)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(3),
				"DD-STATUS-001: Wait for all 3 channel attempts")

			By("Validating partial failure statistics (BR-NOT-053)")
			Expect(notification.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"Phase must be PartiallySent (not Sent, not Failed)")
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(2),
				"Console and log should succeed (2 successful)")
			Expect(notification.Status.FailedDeliveries).To(Equal(1),
				"File delivery should fail (1 failed)")
			Expect(len(notification.Status.DeliveryAttempts)).To(Equal(3),
				"Should attempt delivery to all 3 channels")

			By("Validating mock service call counts")
			// DD-STATUS-001: Cache-bypassed reads + rapid reconciliation can cause 1-2 calls
			// The controller's status update may not propagate before the next reconcile,
			// causing the delivery orchestrator to attempt delivery again (but dedupe via audit)
			Expect(mockConsoleService.GetCallCount()).To(BeNumerically("<=", 2),
				"Console service called 1-2 times (rapid reconciliation)")
			Expect(mockLogService.GetCallCount()).To(BeNumerically("<=", 2),
				"Log service called 1-2 times (rapid reconciliation)")
			Expect(mockFileService.GetCallCount()).To(Equal(1),
				"File service called once (no retries, permanent failure)")

			// ========================================
			// CLEANUP
			// ========================================
			err = k8sClient.Delete(ctx, notification)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When console channel fails but file/log channels succeed", func() {
		It("should mark notification as PartiallySent", func() {
			// ========================================
			// TEST SETUP: Different failure pattern (console fails instead of file)
			// ========================================
			mockConsoleService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return fmt.Errorf("stdout write error (simulated)") // Failure
				},
			}

			mockLogService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil // Success
				},
			}

			mockFileService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil // Success
				},
			}

			// Register mock services
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockConsoleService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), mockLogService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), mockFileService)
			DeferCleanup(func() {
				// Restore original services
				deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), originalConsoleService)
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelLog))
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelFile))
			})

			// ========================================
			// CREATE TEST NOTIFICATION
			// ========================================
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-console-failure-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test-scenario": "console-failure",
						"test-tier":     "integration",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Integration Test: Console Failure",
					Body:     "Testing PartiallySent when console fails but file/log succeed",
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Will fail
						notificationv1alpha1.ChannelLog,     // Will succeed
						notificationv1alpha1.ChannelFile,    // Will succeed
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1, // No retries
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     1,
						MaxBackoffSeconds:     60, // Minimum allowed by CRD validation
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// WAIT AND VALIDATE
			// ========================================
			By("Waiting for PartiallySent phase")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent))

			// DD-STATUS-001: Wait for all delivery attempts to propagate
			Eventually(func() int {
				err := k8sAPIReader.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return -1
				}
				return len(notification.Status.DeliveryAttempts)
			}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 3),
				"DD-STATUS-001: Wait for delivery attempts")

			By("Validating statistics")
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(2),
				"File and log should succeed")
			Expect(notification.Status.FailedDeliveries).To(Equal(1),
				"Console should fail")

			// ========================================
			// CLEANUP
			// ========================================
			err = k8sClient.Delete(ctx, notification)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When all channels fail", func() {
		It("should mark notification as Failed (not PartiallySent)", func() {
			// ========================================
			// TEST SETUP: All channels fail scenario
			// ========================================
			mockConsoleService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return fmt.Errorf("console failure")
				},
			}

			mockLogService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return fmt.Errorf("log failure")
				},
			}

			mockFileService := &mocks.MockDeliveryService{
				DeliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return fmt.Errorf("file failure")
				},
			}

			// Register mock services
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockConsoleService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), mockLogService)
			deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), mockFileService)
			DeferCleanup(func() {
				// Restore original services
				deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), originalConsoleService)
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelLog))
				deliveryOrchestrator.UnregisterChannel(string(notificationv1alpha1.ChannelFile))
			})

			// ========================================
			// CREATE TEST NOTIFICATION
			// ========================================
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "integration-all-channels-fail-test",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test-scenario": "all-channels-fail",
						"test-tier":     "integration",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Integration Test: All Channels Fail",
					Body:     "Testing Failed phase when all channels fail",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelLog,
						notificationv1alpha1.ChannelFile,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           1, // No retries
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     1,
						MaxBackoffSeconds:     60, // Minimum allowed by CRD validation
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// ========================================
			// WAIT AND VALIDATE
			// ========================================
			By("Waiting for Failed phase (all channels failed)")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed),
				"Should transition to Failed when ALL channels fail")

			// DD-STATUS-001: Wait for all delivery attempts to propagate
			// Increased timeout for parallel execution (12 procs)
			Eventually(func() int {
				err := k8sAPIReader.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return -1
				}
				return len(notification.Status.DeliveryAttempts)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(3),
				"DD-STATUS-001: Wait for all 3 channel attempts")

			By("Validating failure statistics")
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(0),
				"No channels should succeed")
			Expect(notification.Status.FailedDeliveries).To(Equal(3),
				"All 3 channels should fail")
			Expect(len(notification.Status.DeliveryAttempts)).To(Equal(3),
				"Should attempt delivery to all 3 channels")

			// ========================================
			// CLEANUP
			// ========================================
			err = k8sClient.Delete(ctx, notification)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
