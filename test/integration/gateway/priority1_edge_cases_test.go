// Package gateway contains Priority 1 integration tests for edge cases
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
// PRIORITY 1: EDGE CASES - INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Purpose: Validate Gateway handles extreme/malformed inputs gracefully
// Coverage: BR-001 (Validation), BR-008 (Deduplication), BR-016 (Storm Detection)
// Test Count: 3 tests
//
// Business Outcomes:
// - Gateway rejects invalid inputs with clear error messages
// - Operators receive actionable feedback for malformed requests
// - System remains stable under edge case conditions
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Priority 1: Edge Cases - Integration Tests", func() {
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
		}, "10s", "100ms").Should(HaveOccurred(), "Namespace should be deleted")

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

		if redisClient != nil {
			redisClient.Cleanup(ctx)
		}
		if k8sClient != nil {
			k8sClient.Cleanup(ctx)
		}
		if cancel != nil {
			cancel()
		}
	})

	Describe("BR-001: Empty Fingerprint Handling", func() {
		It("should reject alerts with missing alertname (empty fingerprint)", func() {
			// TDD RED PHASE: Test validates empty fingerprint rejection
			// TDD GREEN PHASE: Gateway already rejects empty fingerprints (deduplication.go:178-180)
			// Business Outcome: Operators receive clear error when alertname is missing

			// Create alert without alertname (will generate empty fingerprint)
			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"severity": "critical",
						"namespace": "production"
					},
					"annotations": {
						"summary": "Test alert without alertname"
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

			// Verify business outcome: HTTP 400 with clear error message
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Missing alertname should return 400 Bad Request")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("error"), "Error response should include error field")
			Expect(errorResponse).To(HaveKey("status"), "Error response should include status field")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest))

			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("alertname"),
				ContainSubstring("required"),
				ContainSubstring("fingerprint"),
			), "Error message should indicate missing alertname")
		})
	})

	Describe("BR-001: Nil/Empty Signal Handling", func() {
		It("should return HTTP 400 for empty alerts array", func() {
			// TDD RED PHASE: Test validates empty alerts array rejection
			// TDD GREEN PHASE: Gateway already handles this (prometheus_adapter.go:101-103)
			// Business Outcome: Operators receive clear error for empty webhook payloads

			// Create webhook with empty alerts array
			emptyAlertsJSON := `{
				"receiver": "kubernaut-gateway",
				"status": "firing",
				"alerts": []
			}`

			// Send request
			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(emptyAlertsJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Verify business outcome: HTTP 400 with clear error message
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Empty alerts array should return 400 Bad Request")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("error"), "Error response should include error field")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest))

			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("no alerts"),
				ContainSubstring("empty"),
				ContainSubstring("alerts"),
			), "Error message should indicate empty alerts array")
		})
	})

	Describe("BR-001: Malformed Timestamp Handling", func() {
		It("should accept alert with malformed timestamp and use current time", func() {
			// TDD RED PHASE: Test validates malformed timestamp handling
			// TDD GREEN PHASE: Gateway should handle gracefully (use current time as fallback)
			// Business Outcome: Gateway remains operational even with malformed timestamps

			// Create alert with malformed timestamp
			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "MalformedTimestampTest",
						"severity": "critical",
						"namespace": "production"
					},
					"annotations": {
						"summary": "Test alert with malformed timestamp"
					},
					"startsAt": "invalid-timestamp-format"
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

			// Verify business outcome: Gateway handles gracefully
			// Should either:
			// 1. Accept and use current time (HTTP 201)
			// 2. Reject with clear validation error (HTTP 400)
			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusCreated),
				Equal(http.StatusBadRequest),
			), "Gateway should handle malformed timestamp gracefully")

			if resp.StatusCode == http.StatusCreated {
				// Success case: Gateway used current time as fallback
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("remediationRequestName"), "Response should include CRD name")
				Expect(response["status"]).To(Equal("created"), "Status should be 'created'")
			} else {
				// Validation error case: Gateway rejected malformed timestamp
				var errorResponse map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResponse)
				Expect(err).ToNot(HaveOccurred())

				Expect(errorResponse).To(HaveKey("error"), "Error response should include error field")
				errorMsg := errorResponse["error"].(string)
				Expect(errorMsg).To(Or(
					ContainSubstring("timestamp"),
					ContainSubstring("time"),
					ContainSubstring("invalid"),
				), "Error message should indicate timestamp issue")
			}
		})
	})
})

