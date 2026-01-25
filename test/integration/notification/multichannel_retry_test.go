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

// DD-NOT-003 V2.1: Category 2 & 3 - Multi-Channel Delivery and Retry/Circuit Breaker Integration Tests
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Integration tests (>50%): Multi-channel coordination, retry behavior, circuit breaker states
//
// These integration tests validate multi-channel delivery and retry logic with REAL Kubernetes API (envtest)

var _ = Describe("Category 2 & 3: Multi-Channel Delivery and Retry/Circuit Breaker", Label("integration", "multi-channel", "retry"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())

		// Reset mock Slack server state
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	// ============================================================================
	// Category 2: Multi-Channel Delivery (7 tests)
	// ============================================================================

	Context("Category 2: Multi-Channel Delivery Integration Tests", func() {

		// Test 18: MOVED TO E2E - Slack delivery success
		// BR-NOT-020: Slack Integration
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go - "should successfully deliver notification to single Slack channel"
		// MIGRATION REASON: Timing-sensitive test had race conditions in fast envtest reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing

		// Test 19 (was 30): Console delivery success
		// BR-NOT-021: Console Logging
		It("should successfully deliver notification via Console", func() {
			notifName := fmt.Sprintf("console-success-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Subject:  "Console Delivery Test",
					Body:     "Testing console delivery",
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
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for successful delivery
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Console delivery should complete successfully")

			// Verify status
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1))
			Expect(notif.Status.FailedDeliveries).To(Equal(0))

			GinkgoWriter.Println("✅ Console delivery successful")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// NOTE: File delivery is comprehensively tested in E2E tests
		// See: test/e2e/notification/03_file_delivery_validation_test.go (5 scenarios)
		// - Scenario 1: Complete message content validation
		// - Scenario 2: Data sanitization in file output
		// - Scenario 3: Priority field preservation
		// - Scenario 4: Concurrent file delivery (thread safety)
		// - Scenario 5: Non-blocking behavior when FileService fails (CRITICAL)
		//
		// Integration tests focus on Slack/Console multi-channel orchestration.
		// File delivery requires filesystem operations best tested in E2E environment.

		// Test 21: MOVED TO E2E - Multi-channel delivery
		// BR-NOT-010: Multi-Channel Notification Delivery
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go - "should coordinate retries across multiple channels independently"
		// MIGRATION REASON: Timing-sensitive test had race conditions with concurrent reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing

		// Test 22 (was 33): Partial channel failure
		// BR-NOT-058: Graceful Degradation
		It("should handle partial channel failure gracefully (Slack fails, Console succeeds)", func() {
			notifName := fmt.Sprintf("partial-failure-%s", uniqueSuffix)

			// Configure Slack to fail
			ConfigureFailureMode("always", 0, 503)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Partial Failure Test",
					Body:     "Testing partial channel failure",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
						notificationv1alpha1.ChannelConsole,
					},
					// NT-BUG-005 Fix: Use fast retry policy so test completes within 20s
					// Default policy has 30s initial backoff, which exceeds test timeout
					// 3 attempts with 1s initial + 2x multiplier = 1s + 2s = 3s total (well within 20s)
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3, // Reduced from 5 for faster test completion
						InitialBackoffSeconds: 1, // Fast retries for testing
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // CRD minimum validation
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for delivery to complete (partial success = PartiallySent)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				"Should mark as PartiallySent when one channel fails")

			// BEHAVIOR VALIDATION: Verify partial success (Console succeeds, Slack fails)
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1),
				"Console should succeed (Slack configured to always fail)")

			// DD-E2E-003: Counter Semantics - FailedDeliveries counts UNIQUE CHANNELS, not attempts
			// NT-BUG-005 Fix: MaxAttempts = 3 (reduced for faster test)
			Expect(notif.Status.FailedDeliveries).To(Equal(1),
				"Slack channel fails (1 failed channel, not 3 attempts) - DD-E2E-003")

			// BEHAVIOR VALIDATION: Total attempts = Console (1 success) + Slack (3 failed retries)
			Expect(notif.Status.DeliveryAttempts).To(HaveLen(4),
				"Should have exactly 4 attempts: Console (1) + Slack retries (3)")

			// CORRECTNESS VALIDATION: Verify delivery attempt details
			consoleAttempts := 0
			slackAttempts := 0
			for _, attempt := range notif.Status.DeliveryAttempts {
				if attempt.Channel == "console" {
					consoleAttempts++
					Expect(attempt.Status).To(Equal("success"), "Console attempt should succeed")
				}
				if attempt.Channel == "slack" {
					slackAttempts++
					Expect(attempt.Status).To(Equal("failed"), "Slack attempts should all fail")
				}
			}
			Expect(consoleAttempts).To(Equal(1), "Exactly 1 console delivery attempt")
			Expect(slackAttempts).To(Equal(3), "Exactly 3 slack retry attempts (NT-BUG-005)")

			GinkgoWriter.Printf("✅ Partial failure handled: %d successful, %d failed\n",
				notif.Status.SuccessfulDeliveries, notif.Status.FailedDeliveries)

			// Reset mock Slack
			ConfigureFailureMode("none", 0, 0)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 23 (was 34): All channels fail
		// BR-NOT-058: Graceful Degradation
		It("should handle all channels failing gracefully", func() {
			notifName := fmt.Sprintf("all-fail-%s", uniqueSuffix)

			// Configure Slack to always fail
			ConfigureFailureMode("always", 0, 503)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "All Channels Fail Test",
					Body:     "Testing all channels failing",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					// NT-BUG-005 Fix: Use fast retry policy so test completes within 20s
					// Default policy has 30s initial backoff, which exceeds test timeout
					// 3 attempts with 1s initial + 2x multiplier = 1s + 2s = 3s total (well within 20s)
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3, // Reduced from 5 for faster test completion
						InitialBackoffSeconds: 1, // Fast retries for testing
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60, // CRD minimum validation
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// NT-BUG-005 Fix: Wait for ALL retries to exhaust (not just first failure)
			// DD-STATUS-001: Use API reader to bypass cache in parallel execution
			Eventually(func() bool {
				err := k8sAPIReader.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return false
				}
				// Wait for Failed phase AND slack channel exhausted (1 unique channel)
				// Counter semantics: FailedDeliveries = unique channels that ultimately failed
				return notif.Status.Phase == notificationv1alpha1.NotificationPhaseFailed &&
					notif.Status.FailedDeliveries == 1
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"Should mark as Failed when all channels fail after exhausting retry attempts")

			// Verify all 3 delivery attempts were recorded
			Eventually(func() int {
				err := k8sAPIReader.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return -1
				}
				return len(notif.Status.DeliveryAttempts)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(3),
				"Should have exactly 3 delivery attempts (1 initial + 2 retries)")

			// CORRECTNESS VALIDATION: Verify exact failure count
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(0),
				"No successful deliveries when all channels fail")
			Expect(notif.Status.FailedDeliveries).To(Equal(1),
				"Should have exactly 1 failed channel (slack)")

			GinkgoWriter.Printf("✅ All channels failed as expected: %d failed deliveries\n",
				notif.Status.FailedDeliveries)

			// Reset mock Slack
			ConfigureFailureMode("none", 0, 0)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 24: DELETED - Channel-specific retry behavior
		// BR-NOT-054: Channel-Specific Retry
		// RATIONALE: This behavior is already tested in Category 3 (Retry/Circuit Breaker)
		// Per "NO SKIPPED TESTS" rule, redundant placeholder deleted
	})

	// ============================================================================
	// Category 3: Retry/Circuit Breaker (7 tests)
	// ============================================================================

	Context("Category 3: Retry and Circuit Breaker Integration Tests", func() {

		// Test 25 (was 36): Transient failure → Retry with backoff
		// BR-NOT-052: Retry Policy
		// Test: "should retry transient failures with exponential backoff" - MOVED TO E2E
		// BR-NOT-052: Retry Policy Configuration
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go (Test 8)
		// MIGRATION REASON: Timing-sensitive test failed in parallel runs due to envtest's fast reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing (Kind cluster, ~500ms reconciliation)

		// NOTE: Permanent failure detection (4xx errors) is fully implemented and tested
		// See: test/integration/notification/delivery_errors_test.go
		// - Tests 1-4: HTTP 400, 403, 404, 410 classified as permanent (no retry)
		// - Controller: hasChannelPermanentError() function (line 472)
		// - BR-NOT-055: Permanent Error Classification
		//
		// Implementation verified in controller:
		// - 4xx errors marked with "permanent failure" error string
		// - Channel skipped for subsequent retries
		// - attemptCount set to max to prevent future retries

		// Remaining tests DELETED per "NO SKIPPED TESTS" rule:
		// - Max retry limit testing - already covered by existing retry tests
		// - Circuit breaker state transitions - tested in unit tests (test/unit/notification/retry_test.go)
		// - Circuit breaker half-open probing - tested in unit tests
		// - Circuit breaker closure - tested in unit tests
		// - Exponential backoff calculation - tested in unit tests
		//
		// RATIONALE: These behaviors are already validated in unit tests with full coverage.
		// Integration tests focus on end-to-end business workflows, not granular timing/state.
	})
})
