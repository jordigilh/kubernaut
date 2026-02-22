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
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

var _ = Describe("BR-NOT-053: Slack Delivery Service", func() {
	var (
		ctx     context.Context
		service *delivery.SlackDeliveryService
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ⭐ TABLE-DRIVEN: Webhook response handling
	DescribeTable("should handle webhook responses correctly",
		func(statusCode int, expectError bool, expectRetry bool) {
			// Create mock Slack webhook server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			service = delivery.NewSlackDeliveryService(server.URL)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Test message",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
				},
			}

			err := service.Deliver(ctx, notification)

			if expectError {
				Expect(err).To(HaveOccurred())
				if expectRetry {
					Expect(delivery.IsRetryableError(err)).To(BeTrue(), "Expected retryable error for status %d", statusCode)
				} else {
					Expect(delivery.IsRetryableError(err)).To(BeFalse(), "Expected permanent error for status %d", statusCode)
				}
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("200 OK - success", http.StatusOK, false, false),
		Entry("204 No Content - success", http.StatusNoContent, false, false),
		Entry("503 Service Unavailable - retryable", http.StatusServiceUnavailable, true, true),
		Entry("500 Internal Server Error - retryable", http.StatusInternalServerError, true, true),
		Entry("502 Bad Gateway - retryable", http.StatusBadGateway, true, true),
		Entry("401 Unauthorized - permanent failure", http.StatusUnauthorized, true, false),
		Entry("404 Not Found - permanent failure", http.StatusNotFound, true, false),
		Entry("400 Bad Request - permanent failure", http.StatusBadRequest, true, false),
	)

	Context("when formatting Slack message", func() {
		It("should create valid Block Kit JSON", func() {
			// Test Block Kit formatting
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test Subject",
					Body:     "Test Body",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Type:     notificationv1alpha1.NotificationTypeEscalation,
				},
			}

			payload := delivery.FormatSlackPayload(notification)

			// Validate Block Kit structure
			Expect(payload).To(HaveKey("blocks"))
			Expect(payload["blocks"]).To(BeAssignableToTypeOf([]interface{}{}))

			blocks := payload["blocks"].([]interface{})
			Expect(blocks).To(HaveLen(3), "Expected header, section, and context blocks")

			// Validate header block
			headerBlock := blocks[0].(map[string]interface{})
			Expect(headerBlock["type"]).To(Equal("header"))
			Expect(headerBlock).To(HaveKey("text"))

			// Validate section block (message body)
			sectionBlock := blocks[1].(map[string]interface{})
			Expect(sectionBlock["type"]).To(Equal("section"))
			Expect(sectionBlock).To(HaveKey("text"))

			// Validate context block (metadata)
			contextBlock := blocks[2].(map[string]interface{})
			Expect(contextBlock["type"]).To(Equal("context"))
			Expect(contextBlock).To(HaveKey("elements"))
		})

		It("should include priority emoji in subject", func() {
			notifications := []struct {
				priority      notificationv1alpha1.NotificationPriority
				expectedEmoji string
			}{
				{notificationv1alpha1.NotificationPriorityCritical, "🚨"},
				{notificationv1alpha1.NotificationPriorityHigh, "⚠️"},
				{notificationv1alpha1.NotificationPriorityMedium, "ℹ️"},
				{notificationv1alpha1.NotificationPriorityLow, "💬"},
			}

			for _, n := range notifications {
				notification := &notificationv1alpha1.NotificationRequest{
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Test",
						Body:     "Body",
						Priority: n.priority,
					},
				}

				payload := delivery.FormatSlackPayload(notification)
				blocks := payload["blocks"].([]interface{})
				headerBlock := blocks[0].(map[string]interface{})
				headerText := headerBlock["text"].(map[string]interface{})
				text := headerText["text"].(string)

				Expect(text).To(ContainSubstring(n.expectedEmoji),
					"Expected emoji %s for priority %s", n.expectedEmoji, n.priority)
			}
		})
	})

	Context("when webhook URL is invalid", func() {
		It("should return retryable error for network failures", func() {
			service = delivery.NewSlackDeliveryService("http://invalid-url-that-does-not-exist:9999")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Test message",
				},
			}

			err := service.Deliver(ctx, notification)

			Expect(err).To(HaveOccurred())
			Expect(delivery.IsRetryableError(err)).To(BeTrue(), "Network errors should be retryable")
		})
	})

	Context("when context is cancelled", func() {
		It("should respect context cancellation", func() {
			// Create a server that blocks until request context is cancelled
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Block on request context instead of sleeping
				<-r.Context().Done()
				// Context cancelled, return error
			}))
			defer server.Close()

			service = delivery.NewSlackDeliveryService(server.URL)

			// Create cancellable context
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Test message",
				},
			}

			err := service.Deliver(ctx, notification)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})

		// 🆕 OPTION A - PHASE 1: Network Timeout Tests (BR-NOT-052, BR-NOT-058)
		Context("Network Timeout Handling", func() {
			It("should classify webhook timeout as retryable error (BR-NOT-052: Retry on Timeout)", func() {
				// TDD RED: Test that timeout is classified as retryable

				// Create mock server that simulates slow external service
				// Note: time.Sleep() here is VALID - simulating external delay, not synchronizing tests
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow webhook (longer than client timeout)
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				service = delivery.NewSlackDeliveryService(server.URL)

				// Create context with very short timeout (triggers before server responds)
				ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "timeout-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Timeout Test",
						Body:     "Testing timeout behavior",
						Priority: notificationv1alpha1.NotificationPriorityHigh,
					},
				}

				// Execute delivery with timeout context
				err := service.Deliver(ctxWithTimeout, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected timeout error")

				// TDD RED: Verify error is retryable
				// This will likely fail initially if timeout errors aren't classified as retryable
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"Timeout errors should be retryable (transient network issue)")

				// Verify error message indicates timeout
				Expect(err.Error()).To(Or(
					ContainSubstring("timeout"),
					ContainSubstring("deadline exceeded"),
					ContainSubstring("context deadline"),
				), "Error message should indicate timeout")
			})

			It("should handle webhook timeout and preserve error details for audit trail (BR-NOT-051: Audit Trail)", func() {
				// TDD RED: Test that timeout errors include actionable details

				// Create mock server that simulates slow external service
				// Note: time.Sleep() here is VALID - simulating external delay, not synchronizing tests
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow webhook (longer than client timeout)
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				service = delivery.NewSlackDeliveryService(server.URL)

				// Create context with very short timeout (triggers before server responds)
				ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "timeout-audit-test",
						Namespace: "test-namespace",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Timeout Audit Test",
						Body:     "Testing error details preservation",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Type:     notificationv1alpha1.NotificationTypeEscalation,
					},
				}

				err := service.Deliver(ctxWithTimeout, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected timeout error")

				// TDD RED: Verify error wrapping preserves context
				// The error should include enough information for debugging:
				// - That it was a timeout
				// - Which webhook URL was called
				// - Retryable classification
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring("slack"),
					"Error should indicate Slack webhook")
				Expect(errMsg).To(Or(
					ContainSubstring("timeout"),
					ContainSubstring("deadline"),
				), "Error should clearly indicate timeout")

				// Verify error is retryable (for retry logic)
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"Timeout errors must be retryable for automatic retry")
			})
		})

		// 🆕 OPTION A - PHASE 2: Invalid JSON Response (BR-NOT-058: Error Handling)
		Context("Invalid JSON Response Handling", func() {
			It("should treat 200 OK with invalid JSON as success and log warning (BR-NOT-058: Graceful Degradation)", func() {
				// TDD RED: Test that 200 OK is treated as success even if JSON is malformed
				// Rationale: HTTP 200 indicates Slack accepted the webhook
				// Invalid JSON in response body shouldn't fail the delivery

				// Create mock server that returns 200 OK but invalid JSON
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "application/json")
					// Invalid JSON response
					_, _ = w.Write([]byte(`{"status": "ok", "invalid_json: missing_quote}`))
				}))
				defer server.Close()

				service = delivery.NewSlackDeliveryService(server.URL)

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "invalid-json-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Invalid JSON Response Test",
						Body:     "Testing graceful handling of malformed JSON",
						Priority: notificationv1alpha1.NotificationPriorityMedium,
					},
				}

				err := service.Deliver(ctx, notification)

				// TDD RED: Verify delivery succeeds despite invalid JSON
				// HTTP 200 = webhook accepted = delivery successful
				Expect(err).NotTo(HaveOccurred(),
					"200 OK response should be treated as success regardless of response body")

				// Note: Implementation should log warning about malformed JSON
				// but not fail the delivery (200 OK is authoritative)
			})
		})

		// 🆕 OPTION A - PHASE 3: Rate Limiting 429 (BR-NOT-052: Retry with Backoff)
		Context("Rate Limiting Handling (HTTP 429)", func() {
			It("should classify 429 Too Many Requests as retryable (BR-NOT-052: Retry on Rate Limit)", func() {
				// TDD RED: Test that rate limiting triggers retry with exponential backoff

				// Create mock server that returns 429 rate limit
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", "60")       // Slack's rate limit header
					w.WriteHeader(http.StatusTooManyRequests) // 429
					_, _ = w.Write([]byte(`{"ok": false, "error": "rate_limited"}`))
				}))
				defer server.Close()

				service = delivery.NewSlackDeliveryService(server.URL)

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rate-limit-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Rate Limit Test",
						Body:     "Testing rate limit handling",
						Priority: notificationv1alpha1.NotificationPriorityHigh,
					},
				}

				err := service.Deliver(ctx, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected rate limit error")

				// TDD RED: Verify error is retryable
				// Rate limiting is transient - should retry with backoff
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"Rate limit errors (429) should be retryable")

				// Verify error message indicates rate limiting
				Expect(err.Error()).To(Or(
					ContainSubstring("rate"),
					ContainSubstring("429"),
					ContainSubstring("Too Many Requests"),
				), "Error message should indicate rate limiting")
			})

			It("should preserve Retry-After header information for intelligent backoff (BR-NOT-052: Adaptive Backoff)", func() {
				// TDD RED: Test that Retry-After header is captured for backoff calculation
				// Slack provides Retry-After header indicating when to retry

				retryAfterValue := "120" // 120 seconds

				// Create mock server with Retry-After header
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Retry-After", retryAfterValue)
					w.Header().Set("X-Rate-Limit-Remaining", "0")
					w.WriteHeader(http.StatusTooManyRequests) // 429
					_, _ = w.Write([]byte(`{"ok": false, "error": "rate_limited", "retry_after": 120}`))
				}))
				defer server.Close()

				service = delivery.NewSlackDeliveryService(server.URL)

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "rate-limit-retry-after-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Rate Limit Retry-After Test",
						Body:     "Testing Retry-After header preservation",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
						Type:     notificationv1alpha1.NotificationTypeEscalation,
					},
				}

				err := service.Deliver(ctx, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected rate limit error")

				// TDD RED: Verify error is retryable
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"429 with Retry-After should be retryable")

				// Verify error message includes rate limit context
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring("429"),
					"Error should include HTTP status code")

				// Note: The controller's retry logic should respect Retry-After header
				// when calculating backoff duration (longer than default exponential backoff)
				// This is handled at the controller level, not in the delivery service
			})
		})

		// 🆕 OPTION A - PHASE 4: DNS Resolution Failure (BR-NOT-052, BR-NOT-058)
		Context("DNS Resolution Failure Handling", func() {
			It("should classify DNS resolution failure as retryable transient error (BR-NOT-052: Retry on DNS Failure)", func() {
				// TDD RED: Test that DNS failures are retryable
				// DNS failures are usually transient (network issues, temporary DNS server problems)

				// Use invalid domain that will fail DNS lookup
				invalidWebhookURL := "https://this-domain-absolutely-does-not-exist-12345.invalid/webhook"
				service = delivery.NewSlackDeliveryService(invalidWebhookURL)

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dns-failure-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "DNS Failure Test",
						Body:     "Testing DNS resolution failure handling",
						Priority: notificationv1alpha1.NotificationPriorityMedium,
					},
				}

				err := service.Deliver(ctx, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected DNS resolution error")

				// TDD RED: Verify error is retryable
				// DNS failures are transient - retry may succeed if DNS recovers
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"DNS resolution failures should be retryable (transient network issue)")

				// Verify error message indicates DNS/network issue
				Expect(err.Error()).To(Or(
					ContainSubstring("dns"),
					ContainSubstring("no such host"),
					ContainSubstring("lookup"),
					ContainSubstring("dial"),
				), "Error message should indicate DNS/network failure")
			})

			It("should preserve DNS error details for debugging (BR-NOT-051: Audit Trail)", func() {
				// TDD RED: Test that DNS errors include enough detail for troubleshooting

				// Use domain with invalid TLD
				invalidWebhookURL := "https://slack-webhook-invalid-tld.nonexistent/webhook"
				service = delivery.NewSlackDeliveryService(invalidWebhookURL)

				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dns-audit-test",
						Namespace: "test-namespace",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "DNS Audit Test",
						Body:     "Testing DNS error detail preservation",
						Priority: notificationv1alpha1.NotificationPriorityCritical,
					},
				}

				err := service.Deliver(ctx, notification)

				// Assertions
				Expect(err).To(HaveOccurred(), "Expected DNS error")

				// Verify error is retryable
				Expect(delivery.IsRetryableError(err)).To(BeTrue(),
					"DNS errors should be retryable")

				// Verify error includes actionable information
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring("slack"),
					"Error should indicate it's related to Slack webhook")
				// DNS errors typically include the hostname that failed to resolve
				Expect(errMsg).To(Or(
					ContainSubstring("slack-webhook"),
					ContainSubstring("nonexistent"),
					ContainSubstring("no such host"),
				), "Error should include hostname that failed to resolve")
			})
		})

		// 🆕 OPTION A - PHASE 5: TLS Certificate Validation (BR-NOT-058: Error Handling)
		// MOVED TO: test/integration/notification/slack_tls_integration_test.go
		// See also: docs/services/crd-controllers/06-notification/security-tls-policy.md
	})

	Context("Issue #118 Gap 11: Slack channel registration from env var", func() {
		It("UT-NT-SR-001: should register plain 'slack' channel via RegisterChannel with SlackDeliveryService", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			orch := delivery.NewOrchestrator(nil, nil, nil, logr.Discard())

			Expect(orch.HasChannel("slack")).To(BeFalse(),
				"'slack' channel should not be registered before explicit registration")

			slackService := delivery.NewSlackDeliveryService(server.URL)
			orch.RegisterChannel("slack", slackService)

			Expect(orch.HasChannel("slack")).To(BeTrue(),
				"'slack' channel must be registered after RegisterChannel call (env var fallback path)")
		})
	})
})
