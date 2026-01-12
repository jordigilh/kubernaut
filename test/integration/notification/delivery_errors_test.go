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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// DD-NOT-003 V2.1: Category 4 - Delivery Service Error Handling Integration Tests
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Integration tests (>50%): HTTP error classification, network failure handling
//
// These integration tests validate delivery service error handling with REAL Kubernetes API (envtest)

var _ = Describe("Category 4: Delivery Service Error Handling", Label("integration", "delivery-errors"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())

		// Reset mock Slack server state
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	AfterEach(func() {
		// Always reset mock Slack to normal operation after each test
		ConfigureFailureMode("none", 0, 0)
	})

	Context("HTTP 4xx Permanent Errors (Should NOT Retry)", func() {

		// Test 32: Slack 400 Bad Request (permanent error)
		// BR-NOT-055: Permanent Error Classification
		It("should classify HTTP 400 as permanent error and not retry", FlakeAttempts(3), func() {
			notifName := fmt.Sprintf("slack-400-%s", uniqueSuffix)

			// Configure mock to return 400
			ConfigureFailureMode("always", 0, http.StatusBadRequest)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "HTTP 400 Test",
					Body:     "Testing permanent error",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for failure
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed),
				"Should fail immediately for permanent error")

			// Verify NO retries for permanent error (only 1 attempt)
			// FIX: Refetch to avoid stale status in parallel execution
			var totalAttempts int
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				totalAttempts = refetchedNotif.Status.TotalAttempts
				return totalAttempts
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1),
				"Should NOT retry permanent 4xx errors")

			GinkgoWriter.Printf("✅ HTTP 400 classified as permanent: %d attempts\n", totalAttempts)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 33: Slack 403 Forbidden (permanent error)
		// BR-NOT-055: Permanent Error Classification
		It("should classify HTTP 403 as permanent error and not retry", FlakeAttempts(3), func() {
			notifName := fmt.Sprintf("slack-403-%s", uniqueSuffix)

			ConfigureFailureMode("always", 0, http.StatusForbidden)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "HTTP 403 Test",
					Body:     "Testing forbidden error",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

			// FIX: Refetch to avoid stale status in parallel execution
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				return refetchedNotif.Status.TotalAttempts
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1), "Should NOT retry 403 errors")

			GinkgoWriter.Printf("✅ HTTP 403 classified as permanent\n")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 34: Slack 404 Not Found (permanent error)
		// BR-NOT-055: Permanent Error Classification
		It("should classify HTTP 404 as permanent error and not retry", FlakeAttempts(3), func() {
			notifName := fmt.Sprintf("slack-404-%s", uniqueSuffix)

			ConfigureFailureMode("always", 0, http.StatusNotFound)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "HTTP 404 Test",
					Body:     "Testing not found error",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#nonexistent"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

			// FIX: Refetch to avoid stale status in parallel execution
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				return refetchedNotif.Status.TotalAttempts
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

			GinkgoWriter.Printf("✅ HTTP 404 classified as permanent\n")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 35: Slack 410 Gone (permanent error)
		// BR-NOT-055: Permanent Error Classification
		It("should classify HTTP 410 as permanent error and not retry", FlakeAttempts(3), func() {
			notifName := fmt.Sprintf("slack-410-%s", uniqueSuffix)

			ConfigureFailureMode("always", 0, http.StatusGone)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "HTTP 410 Test",
					Body:     "Testing gone error",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#deprecated"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

			// FIX: Refetch to avoid stale status in parallel execution
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				return refetchedNotif.Status.TotalAttempts
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

			GinkgoWriter.Printf("✅ HTTP 410 classified as permanent\n")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})
	})

	Context("HTTP 5xx Retryable Errors (Should Retry)", func() {

		// Test 36: Slack 500 Internal Server Error (retryable)
		// BR-NOT-052: Retry on Transient Errors
		// Test: "should classify HTTP 500 as retryable and retry with backoff" - MOVED TO E2E
		// BR-NOT-052: Retry Policy Configuration
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go (Test 7)
		// MIGRATION REASON: Timing-sensitive test failed in parallel runs due to envtest's fast reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing (Kind cluster, ~500ms reconciliation)

		// Test 37: Slack 502 Bad Gateway (retryable)
		// BR-NOT-052: Retry on Transient Errors
		It("should classify HTTP 502 as retryable and retry", FlakeAttempts(3), func() {
			notifName := fmt.Sprintf("slack-502-%s", uniqueSuffix)

			ConfigureFailureMode("first-N", 1, http.StatusBadGateway)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "HTTP 502 Test",
					Body:     "Testing bad gateway error",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           3,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Should succeed after retrying 502 Bad Gateway error")

			// BEHAVIOR VALIDATION: Verify 502 was treated as retryable and succeeded
			// DD-STATUS-001: Use API reader to bypass cache in parallel execution
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1 // Indicate error to retry
				}
				return refetchedNotif.Status.SuccessfulDeliveries
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1), "Should have exactly 1 successful delivery after retry")

			// Counter semantics (post BR-NOT-052 clarification): FailedDeliveries = unique channels that ultimately failed
			// Since slack channel succeeded after retry, FailedDeliveries should be 0
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				return refetchedNotif.Status.FailedDeliveries
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(0),
				"FailedDeliveries should be 0 since slack channel ultimately succeeded after retry")

			// Verify there was at least 1 failed attempt in DeliveryAttempts (proving 502 was retried)
			Eventually(func() int {
				refetchedNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sAPIReader.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
				if err != nil {
					return -1
				}
				failedCount := 0
				for _, attempt := range refetchedNotif.Status.DeliveryAttempts {
					if attempt.Status == "failed" {
						failedCount++
					}
				}
				return failedCount
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
				"Should have at least 1 failed attempt in DeliveryAttempts (HTTP 502 was retried)")

			// CORRECTNESS VALIDATION: Verify error classification
			// Refetch once more to get latest delivery attempts
			refetchedNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, refetchedNotif)
			Expect(err).NotTo(HaveOccurred())
			Expect(refetchedNotif.Status.DeliveryAttempts[0].Status).To(Equal("failed"))
			Expect(refetchedNotif.Status.DeliveryAttempts[0].Error).To(ContainSubstring("502"),
				"Error should indicate 502 Bad Gateway")
			Expect(refetchedNotif.Status.DeliveryAttempts[1].Status).To(Equal("success"))

			GinkgoWriter.Printf("✅ HTTP 502 retried successfully: %d attempts\n", notif.Status.TotalAttempts)

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 38: MOVED TO E2E - HTTP 503 Service Unavailable
		// BR-NOT-052: Retry on Transient Errors
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go - "should handle transient HTTP 502/503 errors in production-like environment"
		// MIGRATION REASON: Timing-sensitive test had race conditions with concurrent reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing
	})

	// Network-Level Errors Context - DELETED per "NO SKIPPED TESTS" rule
	//
	// All tests in this context were Skip() placeholders for future infrastructure.
	// Per project rule, placeholders are not allowed. If/when infrastructure is added,
	// implement these tests properly without Skip().
	//
	// Deferred (implement when infrastructure available):
	// - Slack timeout (context deadline exceeded) - requires custom HTTP client
	// - Network unreachable - requires DNS/routing simulation
	// - Connection refused - requires port-level control
	// - Partial response (TCP reset) - requires low-level network control
	//
	// NOTE: HTTP 200 with malformed JSON / empty body tests intentionally NOT implemented
	// RATIONALE: Slack webhook API treats HTTP 200 as success regardless of body content

	// NOTE: Panic recovery and nil pointer handling are covered in unit tests
	// See: pkg/notification/delivery/console_test.go (5/5 tests passing)
	// - BR-NOT-010: Console Delivery Service - Panic Recovery
	// - BR-NOT-011: Console Delivery Service - Nil Pointer Handling
	//
	// Integration tests focus on end-to-end delivery orchestration.
	// Console-specific error handling is unit-tested (faster, more isolated).

	// NOTE: FileService Error Handling is comprehensively covered in E2E tests
	// See: test/e2e/notification/03_file_delivery_validation_test.go
	// - Scenario 5: FileService Error Handling (CRITICAL)
	// - BR-NOT-058: Graceful Degradation - Non-blocking behavior
	//
	// E2E tests validate:
	// 1. Directory creation failure handling
	// 2. Write permission denied handling
	// 3. Disk full scenarios
	// 4. JSON marshaling failures
	// 5. CRITICAL: Production delivery succeeds even when FileService fails
	//
	// Integration tests focus on Slack/Console delivery orchestration.
	// FileService-specific error scenarios are E2E-tested (require filesystem manipulation).

	// ==============================================
	// Category: Rate Limiting (BR-NOT-054)
	// ==============================================

	// BR-NOT-054: Rate Limit Handling - TESTS MOVED TO E2E
	// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go
	// - "should retry after receiving 429 rate limit response"
	// - "should respect Retry-After header when handling 429 rate limits"
	// MIGRATION REASON: Timing-sensitive tests had race conditions in fast envtest reconciliation
	// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing
	// All tests for BR-NOT-054 have been moved to E2E tier for realistic timing control
})
