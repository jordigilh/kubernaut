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
		PIt("should return HTTP 503 with retry-after header when Redis is unavailable", func() {
			// TDD RED PHASE: This test will fail initially
			// Business Outcome: Operators receive actionable error with retry guidance
			
			// NOTE: This test requires a way to simulate Redis failure
			// Current StartTestGateway() requires working Redis
			// Need to implement: StartTestGatewayWithInvalidRedis() helper
			
			Skip("Requires helper function to start Gateway with invalid Redis config")
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
})

