// Package gateway contains Priority 1 integration tests for edge cases
package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	var testCtx *Priority1TestContext

	// REFACTORED: Use shared test infrastructure helpers (TDD REFACTOR phase)
	BeforeEach(func() {
		testCtx = SetupPriority1Test()
	})

	AfterEach(func() {
		testCtx.Cleanup()
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
			alertJSON := fmt.Sprintf(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"severity": "critical",
					"namespace": "%s"
				},
				"annotations": {
					"summary": "Test alert without alertname"
				}
			}]
		}`, testCtx.TestNamespace)

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// We validate WHAT happens, not HOW it's implemented:
			// 1. Request is rejected (HTTP 400)
			// 2. Error response is structured JSON (parseable)
			// 3. Error message clearly indicates the problem
			// 4. Operator can immediately fix the issue

			url := testCtx.TestServer.URL + "/api/v1/signals/prometheus"
			req, err := http.NewRequest("POST", url, strings.NewReader(alertJSON))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: Request is rejected with client error
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Gateway MUST reject invalid input with 400 Bad Request (BR-001)")

			// BUSINESS OUTCOME 2: Error response is RFC 7807 structured JSON
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(),
				"Error response MUST be valid JSON for operator parsing (BR-001)")

			// BR-041: RFC 7807 format validation
			Expect(errorResponse).To(HaveKey("detail"),
				"Error response MUST include 'detail' field with description (BR-041)")
			Expect(errorResponse).To(HaveKey("status"),
				"Error response MUST include 'status' field with HTTP code (BR-041)")
			Expect(errorResponse).To(HaveKey("type"),
				"Error response MUST include 'type' field with error type URI (BR-041)")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest),
				"Status field MUST match HTTP status code (BR-001)")

			// BUSINESS OUTCOME 3: Error message is clear and actionable
			errorDetail := errorResponse["detail"].(string)
			Expect(errorDetail).To(Or(
				ContainSubstring("alertname"),
				ContainSubstring("required"),
				ContainSubstring("fingerprint"),
				ContainSubstring("empty"),
			), "Error detail MUST clearly indicate missing alertname so operator can fix it (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// SECURITY OUTCOME VALIDATION (BR-008)
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Verify invalid fingerprint never entered deduplication system
			// This is a BUSINESS outcome: data integrity maintained

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Deduplication validation now via K8s CRD check
			// No CRD should be created for invalid data (HTTP 400 rejection)
			crds := ListRemediationRequests(testCtx.Ctx, testCtx.K8sClient, testCtx.TestNamespace)
			Expect(crds).To(BeEmpty(),
				"Invalid fingerprint MUST NOT create CRD (BR-008 data integrity)")

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

			url := testCtx.TestServer.URL + "/api/v1/signals/prometheus"
			req, err := http.NewRequest("POST", url, strings.NewReader(emptyAlertsJSON))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// BUSINESS OUTCOME 1: Empty payload rejected immediately
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Gateway MUST reject empty webhook payload with 400 Bad Request (BR-001)")

			// BUSINESS OUTCOME 2: Error response is RFC 7807 structured and parseable
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(),
				"Error response MUST be valid JSON (BR-001)")

			// BR-041: RFC 7807 format validation
			Expect(errorResponse).To(HaveKey("detail"),
				"Error response MUST include 'detail' field (BR-041)")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest),
				"Status field MUST match HTTP status code (BR-001)")

			// BUSINESS OUTCOME 3: Error message clearly indicates empty payload
			errorDetail := errorResponse["detail"].(string)
			Expect(errorDetail).To(Or(
				ContainSubstring("no alerts"),
				ContainSubstring("empty"),
				ContainSubstring("alerts"),
			), "Error detail MUST indicate empty alerts array (BR-001)")

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// OPERATIONAL OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Verify no processing overhead (no CRDs created, no K8s API writes)

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: Verify no CRDs created (empty payload rejected early)
			crds := ListRemediationRequests(testCtx.Ctx, testCtx.K8sClient, testCtx.TestNamespace)
			Expect(crds).To(BeEmpty(),
				"Empty payload MUST NOT create CRD (operational efficiency)")

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
			alertJSON := fmt.Sprintf(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"alertname": "MalformedTimestampTest",
					"severity": "critical",
					"namespace": "%s"
				},
				"annotations": {
					"summary": "Test alert with malformed timestamp"
				},
				"startsAt": "invalid-timestamp-format"
			}]
		}`, testCtx.TestNamespace)

			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// BUSINESS OUTCOME VALIDATION
			// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
			// Gateway should handle gracefully - either process with fallback or reject clearly

			url := testCtx.TestServer.URL + "/api/v1/signals/prometheus"
			req, err := http.NewRequest("POST", url, strings.NewReader(alertJSON))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
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
				// STRICT VALIDATION APPROACH: Reject with clear RFC 7807 error
				// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
				var errorResponse map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&errorResponse)
				Expect(err).ToNot(HaveOccurred())

				// BR-041: RFC 7807 format validation
				Expect(errorResponse).To(HaveKey("detail"),
					"Error response should include detail field (BR-041)")
				errorDetail := errorResponse["detail"].(string)
				Expect(errorDetail).To(Or(
					ContainSubstring("timestamp"),
					ContainSubstring("time"),
					ContainSubstring("invalid"),
				), "Error detail should indicate timestamp issue (BR-001)")

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
