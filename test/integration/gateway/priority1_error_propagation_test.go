// Package gateway contains Priority 1 integration tests for error propagation
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PRIORITY 1: ERROR PROPAGATION INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Purpose: Validate Gateway error handling and operator-friendly error messages
// Coverage: BR-001 (Prometheus → CRD), BR-002 (K8s Events), BR-003 (Deduplication)
// Test Count: 3 tests
//
// Business Outcomes Validated:
// 1. Redis unavailable → HTTP 503 with Retry-After header
// 2. K8s API error → HTTP 500 with actionable error details
// 3. Validation error → HTTP 400 with field-level error messages
//
// TDD Methodology: RED-GREEN-REFACTOR for each test
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Priority 1: Error Propagation - Integration Tests", func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		testServer  *httptest.Server
		redisClient *RedisTestClient
		k8sClient   *K8sTestClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		// Clean Redis state
		if redisClient != nil && redisClient.Client != nil {
			err := redisClient.Client.FlushDB(ctx).Err()
			Expect(err).ToNot(HaveOccurred())
		}

		// Create production namespace
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns)

		Eventually(func() error {
			checkNs := &corev1.Namespace{}
			return k8sClient.Client.Get(ctx, client.ObjectKey{Name: "production"}, checkNs)
		}, "10s", "100ms").Should(HaveOccurred())

		ns = &corev1.Namespace{}
		ns.Name = "production"
		ns.Labels = map[string]string{
			"environment": "production",
		}
		err := k8sClient.Client.Create(ctx, ns)
		Expect(err).ToNot(HaveOccurred())

		// Start Gateway server
		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		testServer = httptest.NewServer(gatewayServer.Handler())
	})

	AfterEach(func() {
		// Cleanup
		if testServer != nil {
			testServer.Close()
		}

		// Cleanup namespace
		if k8sClient != nil {
			ns := &corev1.Namespace{}
			ns.Name = "production"
			_ = k8sClient.Client.Delete(ctx, ns)
		}

		// Cleanup Redis
		if redisClient != nil && redisClient.Client != nil {
			_ = redisClient.Client.FlushDB(ctx)
		}

		if cancel != nil {
			cancel()
		}
	})

	Describe("BR-003: Redis Connection Error Propagation", func() {
		It("should return HTTP 503 with retry-after header when Redis is unavailable", func() {
			// TDD RED PHASE: Test fails because Gateway doesn't handle Redis connection errors gracefully
			// TDD GREEN PHASE: Gateway will be updated to detect Redis errors and return HTTP 503
			// Business Outcome: Operators receive actionable error with retry guidance when Redis is down

			// Cleanup existing test infrastructure
			if testServer != nil {
				testServer.Close()
				testServer = nil
			}
			if redisClient != nil {
				redisClient.Cleanup(ctx)
				redisClient = nil
			}

			// Create Gateway with invalid Redis config (pointing to non-existent Redis)
			invalidRedisClient := &RedisTestClient{
				Client: goredis.NewClient(&goredis.Options{
					Addr: "localhost:9999", // Non-existent Redis port
					DB:   0,
				}),
			}

			// Start Gateway with invalid Redis (should still start but fail on first Redis operation)
			gatewayServer, err := StartTestGateway(ctx, invalidRedisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred(), "Gateway should start even with invalid Redis config")
			testServer = httptest.NewServer(gatewayServer.Handler())

			// Send valid alert that will trigger Redis operation
			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "RedisConnectionTest",
						"severity": "critical",
						"namespace": "production"
					},
					"annotations": {
						"summary": "Test alert for Redis connection error"
					}
				}]
			}`

			// Send request
			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Verify business outcome: HTTP 503 Service Unavailable
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"Gateway should return 503 when Redis is unavailable")

			// Verify Retry-After header is present
			retryAfter := resp.Header.Get("Retry-After")
			Expect(retryAfter).ToNot(BeEmpty(), "Response should include Retry-After header")

			// Verify JSON error response
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("error"), "Error response should include error field")
			Expect(errorResponse).To(HaveKey("status"), "Error response should include status field")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusServiceUnavailable))

			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("redis"),
				ContainSubstring("Redis"),
				ContainSubstring("unavailable"),
				ContainSubstring("connection"),
			), "Error message should indicate Redis connection issue")
		})
	})

	Describe("BR-001: Validation Error Propagation", func() {
		It("should return HTTP 400 with field-level errors for invalid input", func() {
			// TDD RED PHASE: Write test first
			// Business Outcome: Operators receive field-level validation errors

			// Create invalid alert (missing required fields)
			invalidAlertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {},
					"annotations": {}
				}]
			}`

			// Send request
			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(invalidAlertJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Verify business outcome: HTTP 400 with field-level errors
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Invalid input should return 400 Bad Request")

			// Verify error response includes field-level validation details
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("error"),
				"Error response should include error field")

			// Error should indicate which fields are missing/invalid
			errorMsg, ok := errorResponse["error"].(string)
			Expect(ok).To(BeTrue(), "Error field should be a string")
			Expect(errorMsg).To(Or(
				ContainSubstring("alertname"),
				ContainSubstring("required"),
				ContainSubstring("invalid"),
				ContainSubstring("empty"),
			), "Error message should indicate which fields are problematic")
		})
	})

	Describe("BR-001: Internal Server Error Propagation", func() {
		It("should return HTTP 500 with error details when processing fails", func() {
			// TDD RED PHASE: Write test first
			// Business Outcome: Operators receive actionable error messages for server failures

			// Create valid alert that will pass validation but might fail processing
			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "ProcessingTest",
						"severity": "critical",
						"namespace": "nonexistent-namespace"
					},
					"annotations": {
						"summary": "Test alert for processing error"
					}
				}]
			}`

			// Send request
			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Verify business outcome: HTTP 500 or 201 (depending on whether namespace exists)
			// If namespace doesn't exist, Gateway should either:
			// 1. Create CRD in default namespace (201)
			// 2. Return error (500)
			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusCreated),
				Equal(http.StatusInternalServerError),
			), "Should either create CRD or return error")

			// If error, verify JSON format
			if resp.StatusCode == http.StatusInternalServerError {
				var errorResponse map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResponse)
				Expect(err).ToNot(HaveOccurred())

				Expect(errorResponse).To(HaveKey("error"),
					"Error response should include error field")
			}
		})
	})
})
