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
// TDD Methodology: RED → GREEN → REFACTOR
// Business Outcome Focus: Validate WHAT the system achieves, not HOW
//
// Purpose: Validate Gateway handles extreme/malformed inputs gracefully
// Coverage: BR-001 (Validation), BR-008 (Deduplication)
//
// Business Outcomes:
// - BR-001: Gateway rejects invalid inputs with clear, actionable error messages
// - BR-008: Invalid fingerprints never enter deduplication system
// - Operators receive structured feedback they can parse and act on
// - System remains stable and secure under edge case conditions
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

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 1: Empty Fingerprint Handling (BR-001, BR-008)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: Operators receive clear, actionable error when alertname is missing
	// Security Outcome: Invalid fingerprints never enter deduplication system (BR-008)
	// User Experience: Structured JSON error response that can be parsed and acted upon
	//
	// TDD RED PHASE: This test will FAIL because we're validating business outcome
	// Expected Failure: Gateway should reject with HTTP 400 and clear error message
	//
	Describe("BR-001 & BR-008: Empty Fingerprint Rejection", func() {
		It("should reject alerts with missing alertname and provide clear error guidance", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Operator misconfigures Prometheus AlertManager rule
			// Missing: alertname label (required for fingerprint generation)
			// Expected: Gateway rejects immediately with clear guidance
			// Why: Prevents invalid data from entering deduplication system
			//      Provides fast feedback to operator (fail-fast principle)
			//      Maintains data integrity in Redis and Kubernetes

			// Create alert without alertname (will generate empty/invalid fingerprint)
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

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// We validate WHAT happens, not HOW it's implemented:
			// 1. Request is rejected (HTTP 400)
			// 2. Error response is structured JSON (parseable)
			// 3. Error message clearly indicates the problem
			// 4. Operator can immediately fix the issue

			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: Request is rejected with client error
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Gateway MUST reject invalid input with 400 Bad Request (BR-001)")

			// BUSINESS OUTCOME 2: Error response is structured JSON
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(),
				"Error response MUST be valid JSON for operator parsing (BR-001)")

			Expect(errorResponse).To(HaveKey("error"),
				"Error response MUST include 'error' field with description (BR-001)")
			Expect(errorResponse).To(HaveKey("status"),
				"Error response MUST include 'status' field with HTTP code (BR-001)")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest),
				"Status field MUST match HTTP status code (BR-001)")

			// BUSINESS OUTCOME 3: Error message is clear and actionable
			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("alertname"),
				ContainSubstring("required"),
				ContainSubstring("fingerprint"),
				ContainSubstring("empty"),
			), "Error message MUST clearly indicate missing alertname so operator can fix it (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// SECURITY OUTCOME VALIDATION (BR-008)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Verify invalid fingerprint never entered deduplication system
			// This is a BUSINESS outcome: data integrity maintained

			// Check Redis: should have NO keys (invalid data rejected before storage)
			keys, err := redisClient.Client.Keys(ctx, "gateway:dedup:*").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(keys).To(BeEmpty(),
				"Invalid fingerprint MUST NOT enter deduplication system (BR-008 data integrity)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ Operator receives immediate, clear feedback
			// ✅ Invalid data never enters system (data integrity)
			// ✅ Deduplication system remains clean (BR-008)
			// ✅ Error is parseable by monitoring tools (structured JSON)
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 2: Empty Alerts Array Handling (BR-001)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: Operators receive clear error for empty webhook payloads
	// Operational Outcome: Prevents wasted processing cycles on empty webhooks
	// User Experience: Fast feedback loop - fail immediately, not after processing
	//
	// TDD RED PHASE: This test validates business outcome
	// Expected: Gateway rejects empty payloads with HTTP 400
	//
	Describe("BR-001: Empty Webhook Payload Rejection", func() {
		It("should reject empty alerts array and guide operator to fix webhook configuration", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Prometheus AlertManager sends webhook with no alerts
			// Cause: Misconfigured routing, resolved alerts only, or test webhook
			// Expected: Gateway rejects immediately (no processing overhead)
			// Why: Prevents wasted resources, provides clear feedback to operator

			// Create webhook with empty alerts array
			emptyAlertsJSON := `{
				"receiver": "kubernaut-gateway",
				"status": "firing",
				"alerts": []
			}`

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(emptyAlertsJSON),
			)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: Empty payload rejected immediately
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Gateway MUST reject empty webhook payload with 400 Bad Request (BR-001)")

			// BUSINESS OUTCOME 2: Error response is structured and parseable
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(),
				"Error response MUST be valid JSON (BR-001)")

			Expect(errorResponse).To(HaveKey("error"),
				"Error response MUST include 'error' field (BR-001)")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest),
				"Status field MUST match HTTP status code (BR-001)")

			// BUSINESS OUTCOME 3: Error message clearly indicates empty payload
			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("no alerts"),
				ContainSubstring("empty"),
				ContainSubstring("alerts"),
			), "Error message MUST indicate empty alerts array (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// OPERATIONAL OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Verify no processing overhead (no Redis keys, no K8s API calls)

			// Check Redis: should have NO keys (no processing occurred)
			keys, err := redisClient.Client.Keys(ctx, "gateway:*").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(keys).To(BeEmpty(),
				"Empty payload MUST NOT trigger any processing (operational efficiency)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ Operator receives immediate feedback (fail-fast)
			// ✅ No wasted processing cycles (operational efficiency)
			// ✅ Clear guidance to fix webhook configuration
			// ✅ System resources preserved for valid alerts
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TEST 3: Malformed Timestamp Handling (BR-001)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// Business Outcome: Gateway remains operational even with malformed timestamps
	// Resilience Outcome: System gracefully handles data quality issues
	// User Experience: Alerts are processed despite timestamp problems
	//
	// TDD RED PHASE: This test validates graceful degradation
	// Expected: HTTP 201 (uses current time) OR HTTP 400 (clear validation error)
	//
	Describe("BR-001: Malformed Timestamp Graceful Handling", func() {
		It("should process alert with malformed timestamp using fallback strategy", func() {
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS CONTEXT
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Scenario: Prometheus sends alert with invalid timestamp format
			// Cause: Clock skew, timezone issues, or malformed webhook
			// Expected: Gateway handles gracefully (doesn't crash or reject valid alert)
			// Why: Resilience - timestamp is metadata, not core alert data
			//      Better to process alert with fallback time than reject entirely

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

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Gateway should handle gracefully - either process with fallback or reject clearly

			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME: Gateway handles gracefully (doesn't crash)
			// Two acceptable outcomes:
			// 1. Process with fallback time (HTTP 201) - resilience approach
			// 2. Reject with clear error (HTTP 400) - strict validation approach
			Expect(resp.StatusCode).To(Or(
				Equal(http.StatusCreated),
				Equal(http.StatusBadRequest),
			), "Gateway MUST handle malformed timestamp gracefully (BR-001)")

			if resp.StatusCode == http.StatusCreated {
				// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
				// RESILIENCE APPROACH: Process with fallback time
				// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("remediationRequestName"),
					"Response should include CRD name (alert processed)")
				Expect(response["status"]).To(Equal("created"),
					"Status should be 'created' (alert processed with fallback time)")

				// BUSINESS VALUE: Alert was processed despite timestamp issue
				// System prioritizes alert handling over timestamp perfection
			} else {
				// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
				// STRICT VALIDATION APPROACH: Reject with clear error
				// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
				var errorResponse map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResponse)
				Expect(err).ToNot(HaveOccurred())

				Expect(errorResponse).To(HaveKey("error"),
					"Error response should include error field")
				errorMsg := errorResponse["error"].(string)
				Expect(errorMsg).To(Or(
					ContainSubstring("timestamp"),
					ContainSubstring("time"),
					ContainSubstring("invalid"),
				), "Error message should indicate timestamp issue (BR-001)")

				// BUSINESS VALUE: Operator receives clear feedback about timestamp problem
				// Can fix webhook configuration to prevent future issues
			}

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS VALUE ACHIEVED
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// ✅ Gateway remains operational (doesn't crash)
			// ✅ Alert is processed OR operator receives clear guidance
			// ✅ System demonstrates resilience to data quality issues
			// ✅ Business continuity maintained despite timestamp problems
		})
	})
})
