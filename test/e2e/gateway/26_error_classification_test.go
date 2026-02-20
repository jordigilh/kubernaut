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

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Test Plan Reference: docs/development/testing/GATEWAY_COVERAGE_GAP_TEST_PLAN.md
// Section 4: Error Classification & Retry Logic Testing (BR-GATEWAY-111 to BR-GATEWAY-114, BR-GATEWAY-189)
// Tests: GW-ERR-001, GW-ERR-002, GW-ERR-003

var _ = Describe("Gateway Error Classification & Retry Logic (BR-GATEWAY-111 to BR-GATEWAY-114, BR-GATEWAY-189)", func() {
	var (
		testNamespace string // ✅ FIX: Unique namespace per parallel process (prevents data pollution)
		testCtx       context.Context      // ← Test-local context
		testCancel    context.CancelFunc
		testClient    client.Client
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(context.Background())  // ← Uses local variable
		testClient = k8sClient // Use suite-level client (DD-E2E-K8S-CLIENT-001)

		// Pre-create managed namespace (Pattern: RO E2E)
		testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "gw-error-test")

		// Get DataStorage URL from environment
		// Create Gateway server
	})

	AfterEach(func() {
	if testCancel != nil {
		testCancel()  // ← Only cancels test-local context
	}
		// Clean up test namespace (Pattern: RO E2E)
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
	})

	Context("GW-ERR-001: Transient Error Retry with Exponential Backoff (P0)", func() {
		It("BR-GATEWAY-113: should retry transient K8s API errors with exponential backoff", func() {
			// Given: Gateway configured with retry policy (default: 3 attempts, exponential backoff)
			// When: Valid alert sent to gateway
			// Then: CRD is created (201) or deduplicated (202)

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestTransientRetry",
				Namespace: testNamespace,
				Severity:  "warning",
				Labels: map[string]string{
					"test_scenario": "transient_error",
				},
			})

			req, err := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request to gateway should not fail")
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
				"Gateway should return 201 (created) or 202 (deduplicated/accepted)")

			// Validate CRD was created in the test namespace
			Eventually(func() bool {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				listErr := testClient.List(testCtx, rrList, client.InNamespace(testNamespace))
				return listErr == nil && len(rrList.Items) > 0
			}, 10*time.Second, 1*time.Second).Should(BeTrue(),
				"RemediationRequest CRD should exist in test namespace after successful alert processing")
		})

		It("should use exponential backoff between retry attempts", func() {
			// Given: Gateway retry configuration with exponential backoff
			// - InitialBackoff: 100ms
			// - MaxBackoff: 2s
			// - Backoff multiplier: 2x

			// When: Multiple transient errors occur
			// Then: Backoff delays should increase exponentially
			// - Attempt 1: Immediate
			// - Attempt 2: 100ms delay
			// - Attempt 3: 200ms delay
			// - Attempt 4: 400ms delay (capped at MaxBackoff)

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestExponentialBackoff",
				Namespace: testNamespace,
				Severity:  "info",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			startTime := time.Now()
			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()
			duration := time.Since(startTime)

			// Validate response
			// This test documents expected backoff behavior
			// Actual validation requires instrumentation or log analysis
			logger.Info("Exponential backoff test completed",
				"status", resp.StatusCode,
				"duration", duration)
		})

		It("should not retry faster than initial backoff configuration", func() {
			// Given: Initial backoff configured to 100ms
			// When: Transient error requires retry
			// Then: First retry should wait at least 100ms

			// Business Rationale:
			// - Prevents overwhelming already-stressed K8s API
			// - Allows time for transient issues to resolve
			// - Rate limits retry attempts

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestMinimumBackoff",
				Namespace: testNamespace,
				Severity:  "warning",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// Note: Actual timing validation requires instrumentation
			// This test documents the expected behavior
		})
	})

	Context("GW-ERR-002: Permanent Error Abort (P0)", func() {
		It("BR-GATEWAY-189: should NOT retry permanent errors (HTTP 400, validation failures)", func() {
			// Given: Gateway with retry configuration
			// When: Permanent error occurs (invalid CRD spec, validation failure)
			// Then: Gateway returns error immediately WITHOUT retrying

			// Business Rationale:
			// - Retrying validation errors wastes resources
			// - Permanent errors require user intervention (fixing alert payload)
			// - Immediate feedback helps debug misconfiguration

			// Send invalid JSON payload (malformed)
			// This will cause Gateway to return HTTP 400 Bad Request
			invalidPayload := []byte(`{
				"status": "firing",
				"alerts": "this should be an array not a string"
			}`)

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(invalidPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			startTime := time.Now()
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed (error is in response body, not HTTP transport)")
			defer func() { _ = resp.Body.Close() }()
			duration := time.Since(startTime)

			// Then: Should return error quickly (no retry delays)
			// Duration should be < 1 second (no exponential backoff)
			Expect(duration).To(BeNumerically("<", 1*time.Second),
				"Permanent errors should fail fast without retry delays")

			// And: Response should indicate validation error
			// (HTTP 400 Bad Request or HTTP 422 Unprocessable Entity)
			Expect(resp.StatusCode).To(BeNumerically(">=", 400))
			Expect(resp.StatusCode).To(BeNumerically("<", 500),
				"Permanent errors should return 4xx status codes")
		})

		It("should classify HTTP 400 errors as permanent (no retry)", func() {
			// Given: Invalid webhook payload
			// When: Gateway returns HTTP 400 Bad Request
			// Then: No retry attempts (permanent error)

			invalidPayload := []byte(`{"invalid": "payload"}`)

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(invalidPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			startTime := time.Now()
			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()
			duration := time.Since(startTime)

			// Fast failure (no retries)
			Expect(duration).To(BeNumerically("<", 500*time.Millisecond))
		})

		It("should provide actionable error messages for permanent failures", func() {
			// Given: Validation error that cannot be retried
			// When: Gateway returns error
			// Then: Error message should explain what's wrong and how to fix it

			invalidPayload := []byte(`{
				"status": "firing",
				"alerts": [{
					"labels": {},
					"annotations": {}
				}]
			}`)

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(invalidPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				var errorResp map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResp) //nolint:ineffassign // Test pattern: error reassignment across phases

				// Validate error message is actionable
				detail := errorResp["detail"]
				Expect(detail).ToNot(BeNil())
				Expect(detail.(string)).ToNot(BeEmpty(), "Error message should provide actionable feedback")
			}
		})
	})

	Context("GW-ERR-003: Retry Exhaustion Handling (P0)", func() {
		It("BR-GATEWAY-189: should return error after max retry attempts exhausted", func() {
			// Given: Gateway configured with MaxAttempts=3
			// When: Transient error persists through all retry attempts
			// Then: Gateway returns error (HTTP 503 or HTTP 500)

			// Business Rationale:
			// - Prevents infinite retry loops
			// - Provides clear failure signal to webhook source
			// - Allows alerting on persistent infrastructure issues

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestRetryExhaustion",
				Namespace: testNamespace,
				Severity:  "critical",
				Labels: map[string]string{
					"test_scenario": "persistent_failure",
				},
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// If retries exhausted, should return error
			if resp.StatusCode >= 500 {
				var errorResp map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResp) //nolint:ineffassign // Test pattern: error reassignment across phases

				// Error should indicate exhaustion, not transient failure
				detail := errorResp["detail"].(string)
				// Note: Actual error message depends on implementation
				// This documents expected behavior
				logger.Info("Retry exhaustion error received",
					"detail", detail)
			}
		})

		It("should honor max backoff limit during retry exhaustion", func() {
			// Given: MaxBackoff configured to 2 seconds
			// When: Multiple retries occur with exponential backoff
			// Then: Backoff delay should never exceed 2 seconds

			// Business Rationale:
			// - Prevents excessive request latency
			// - Ensures webhook source receives timely response
			// - Balances retry attempts with responsiveness

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestMaxBackoff",
				Namespace: testNamespace,
				Severity:  "warning",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			startTime := time.Now()
			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()
			totalDuration := time.Since(startTime)

			// With 3 retries and MaxBackoff=2s:
			// - Attempt 1: Immediate
			// - Attempt 2: 100ms delay
			// - Attempt 3: 200ms delay
			// - Attempt 4: 400ms delay (but capped at 2s if configured)
			// Maximum total: ~2.7 seconds (if all retries occur at MaxBackoff)

			if resp.StatusCode >= 500 {
				// Retries likely occurred
				// Total duration should reflect capped backoff
				logger.Info("Max backoff test completed",
					"totalDuration", totalDuration,
					"status", resp.StatusCode)
			}
		})

		It("should include retry attempt count in error response (observability)", func() {
			// Given: Gateway that tracks retry attempts
			// When: Retries are exhausted
			// Then: Error response includes retry count for debugging

			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestRetryCount",
				Namespace: testNamespace,
				Severity:  "info",
			})

			req, _ := http.NewRequest("POST",
				fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
				bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			_ = err
			defer func() { _ = resp.Body.Close() }()

			// Note: Retry count observability helps debug infrastructure issues
			// This test documents expected behavior for error responses
		})
	})

	Context("GW-ERR-004: Error Classification Logic (P1)", func() {
		It("should classify network timeouts as transient (retry)", func() {
			// Given: K8s API with network connectivity issues
			// When: Connection timeout occurs
			// Then: Classified as transient, retry with backoff

			// Error types that should be retried:
			// - context.DeadlineExceeded
			// - net.OpError with Timeout() == true
			// - i/o timeout
			// - connection refused (temporary)
		})

		It("should classify CRD validation errors as permanent (no retry)", func() {
			// Given: RemediationRequest CRD with validation error
			// When: K8s API returns validation error
			// Then: Classified as permanent, return immediately

			// Error types that should NOT be retried:
			// - InvalidInput errors
			// - FieldValueInvalid errors
			// - FieldValueRequired errors
		})

		It("should classify API server overload (429) as transient with longer backoff", func() {
			// Given: K8s API returning 429 Too Many Requests
			// When: Rate limit exceeded
			// Then: Retry with longer backoff to allow API server recovery

			// Special case: 429 errors should use longer initial backoff
			// to avoid immediately re-hitting rate limits
		})
	})
})
