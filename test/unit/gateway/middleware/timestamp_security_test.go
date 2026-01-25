package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Timestamp Security for Attack Prevention
// ============================================================================
//
// PURPOSE: Validate Gateway prevents security attacks via timestamp validation:
// - Replay attacks (re-sending old alerts)
// - Clock skew attacks (alerts with future timestamps)
// - Missing security headers (bypass timestamp checks)
//
// BUSINESS VALUE:
// - Prevents attackers from replaying old alerts to trigger unwanted remediations
// - Prevents clock manipulation attacks
// - Ensures all webhook requests include required security headers
//
// REFACTORED: December 27, 2025 - Reduced from 21 tests to 6 tests
// Previous version tested input validation details (TESTING_GUIDELINES.md violation)
// New version tests security threats only
//
// BR-GATEWAY-074: X-Timestamp header mandatory for write operations
// BR-GATEWAY-075: Replay attack prevention via timestamp validation
// BR-GATEWAY-101: RFC 7807 error responses for client errors
// ============================================================================

var _ = Describe("BR-GATEWAY-074, BR-GATEWAY-075: Timestamp Security", func() {
	var (
		handler http.Handler
		rr      *httptest.ResponseRecorder
		req     *http.Request
	)

	BeforeEach(func() {
		// Setup: timestamp middleware with 5-minute tolerance (production default)
		handler = middleware.TimestampValidator(5 * time.Minute)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		)
		rr = httptest.NewRecorder()
		req = httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
		req.Header.Set("Content-Type", "application/json")
	})

	Context("Security Threat Prevention", func() {
		It("BR-GATEWAY-075: should prevent replay attacks by rejecting old timestamps", func() {
			// BUSINESS OUTCOME: Attackers cannot replay old alerts to trigger remediations
			// THREAT: Attacker captures legitimate alert at T0, replays at T0+15min to cause unwanted remediation
			// MITIGATION: Gateway rejects timestamps older than 5 minutes

			// Simulate replay attack: 10-minute-old timestamp
			oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(oldTimestamp, 10))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"Replay attacks (old timestamps) must be rejected to prevent unwanted remediations")

			// Verify error response explains the security issue
			var errorResp gwerrors.RFC7807Error
			err := json.NewDecoder(rr.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp.Type).To(Equal(gwerrors.ErrorTypeValidationError))
			Expect(errorResp.Detail).To(ContainSubstring("timestamp"),
				"Error message must explain replay attack rejection")
		})

		It("BR-GATEWAY-075: should prevent clock skew attacks by rejecting future timestamps", func() {
			// BUSINESS OUTCOME: Attackers cannot manipulate timestamps to bypass deduplication
			// THREAT: Attacker sends alerts with future timestamps to bypass TTL-based deduplication
			// MITIGATION: Gateway rejects timestamps from the future

			// Simulate clock skew attack: 2-hour future timestamp
			futureTimestamp := time.Now().Add(2 * time.Hour).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(futureTimestamp, 10))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"Future timestamps indicate clock skew attack and must be rejected")

			var errorResp gwerrors.RFC7807Error
			err := json.NewDecoder(rr.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp.Detail).To(ContainSubstring("timestamp"),
				"Error must identify timestamp issue")
		})

		It("BR-GATEWAY-074: should reject requests missing X-Timestamp header", func() {
			// BUSINESS OUTCOME: All write operations require timestamp for replay attack prevention
			// THREAT: Attacker bypasses timestamp validation by not including header
			// MITIGATION: Gateway requires X-Timestamp header for all write operations

			// Request without X-Timestamp header (security bypass attempt)
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"Missing X-Timestamp header allows replay attacks - must be rejected")

			var errorResp gwerrors.RFC7807Error
			err := json.NewDecoder(rr.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp.Detail).To(ContainSubstring("timestamp"),
				"Error must identify missing required security header")
		})

		It("BR-GATEWAY-074: should reject malformed timestamp values", func() {
			// BUSINESS OUTCOME: Gateway validates timestamp format to prevent validation bypass
			// THREAT: Attacker sends malformed timestamps to bypass validation logic
			// MITIGATION: Gateway rejects non-numeric and invalid timestamp formats

			malformedTimestamps := []string{
				"not-a-timestamp", // Non-numeric
				"-1",              // Negative
				"1234567890.5",    // Fractional seconds
				"123abc456",       // Special characters
				"",                // Empty
			}

			for _, malformed := range malformedTimestamps {
				rr = httptest.NewRecorder()
				req = httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Timestamp", malformed)

				handler.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusBadRequest),
					"Malformed timestamp '%s' must be rejected to prevent validation bypass", malformed)
			}
		})
	})

	Context("Legitimate Request Acceptance", func() {
		It("should accept valid recent timestamps within tolerance window", func() {
			// BUSINESS OUTCOME: Gateway accepts legitimate alerts from Prometheus/K8s
			// Valid timestamps allow normal business operations (alert ingestion, remediation)

			validTimestamps := []time.Duration{
				-1 * time.Second,                // Very recent
				-30 * time.Second,               // 30 seconds ago
				-2 * time.Minute,                // 2 minutes ago (well within 5min tolerance)
				-4*time.Minute - 59*time.Second, // Edge of tolerance (4:59)
			}

			for _, offset := range validTimestamps {
				rr = httptest.NewRecorder()
				req = httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
				req.Header.Set("Content-Type", "application/json")
				validTimestamp := time.Now().Add(offset).Unix()
				req.Header.Set("X-Timestamp", strconv.FormatInt(validTimestamp, 10))

				handler.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK),
					"Valid timestamp within tolerance (%s) must be accepted for normal operations", offset)
			}
		})

		It("BR-GATEWAY-101: should return RFC 7807 compliant error responses for security violations", func() {
			// BUSINESS OUTCOME: Clients receive standardized error responses for debugging
			// RFC 7807 provides structured error format with actionable information

			// Trigger security violation (missing timestamp)
			handler.ServeHTTP(rr, req)

			// Verify RFC 7807 compliance
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"RFC 7807 requires application/problem+json Content-Type")

			var errorResp gwerrors.RFC7807Error
			err := json.NewDecoder(rr.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Response must be valid JSON")

			// Validate RFC 7807 required fields
			Expect(errorResp.Type).ToNot(BeEmpty(), "RFC 7807 'type' is required")
			Expect(errorResp.Title).ToNot(BeEmpty(), "RFC 7807 'title' is required")
			Expect(errorResp.Status).To(Equal(http.StatusBadRequest), "RFC 7807 'status' must match HTTP status")
			Expect(errorResp.Detail).ToNot(BeEmpty(), "RFC 7807 'detail' is required")
			Expect(len(errorResp.Detail)).To(BeNumerically(">", 10), "Detail message must be actionable, not just a code")
		})
	})
})
