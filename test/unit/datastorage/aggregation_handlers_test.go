package datastorage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// TDD RED PHASE: ADR-033 Aggregation Handlers Unit Tests
// 📋 Authority: IMPLEMENTATION_PLAN_V5.0.md Day 14.1
// 📋 Business Requirements:
//    - BR-STORAGE-031-01: Incident-Type Success Rate API
//    - BR-STORAGE-031-02: Playbook Success Rate API
// 📋 Testing Principle: Behavior + Correctness
// ========================================
//
// TESTING STRATEGY:
// - Mock ActionTraceRepository (external dependency)
// - Test HTTP request/response behavior
// - Validate query parameter parsing
// - Validate RFC 7807 error responses
// - Validate success response structure
//
// ========================================

var _ = Describe("ADR-033 Aggregation Handlers", func() {
	var (
		handler *server.Handler
		req     *http.Request
		rec     *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		// TODO: Create handler with mocked ActionTraceRepository
		// This will be implemented in TDD GREEN phase
		// For now, handler is nil to ensure tests fail (TDD RED)
		handler = nil
		rec = httptest.NewRecorder()
	})

	// ========================================
	// BR-STORAGE-031-01: Incident-Type Success Rate Handler
	// BEHAVIOR: Parse query params, call repository, return JSON
	// CORRECTNESS: Exact HTTP status codes and response structure
	// ========================================
	Describe("GET /api/v1/success-rate/incident-type", func() {
		Context("with valid query parameters", func() {
			It("should return 200 OK with incident-type success rate data", func() {
				// ARRANGE: Create HTTP request with valid params
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=7d&min_samples=5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Handler should return 200 OK for valid request")

				// ASSERT: Response is valid JSON
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate response structure
				Expect(response.IncidentType).To(Equal("HighCPUUsage"),
					"Response should contain requested incident type")
				Expect(response.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")
				Expect(response.SuccessRate).To(BeNumerically(">=", 0.0),
					"Success rate should be non-negative")
				Expect(response.SuccessRate).To(BeNumerically("<=", 100.0),
					"Success rate should be <= 100%")
				Expect(response.TotalExecutions).To(BeNumerically(">=", 0),
					"Total executions should be non-negative")
				Expect(response.SuccessfulExecutions).To(BeNumerically(">=", 0),
					"Successful executions should be non-negative")
				Expect(response.Confidence).To(BeElementOf("high", "medium", "low", "insufficient_data"),
					"Confidence should be one of the valid values")
			})

			It("should use default time_range=7d when not specified", func() {
				// ARRANGE: Request without time_range param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Default time range applied
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.TimeRange).To(Equal("7d"),
					"Handler should default to 7d time range")
			})

			It("should use default min_samples=5 when not specified", func() {
				// ARRANGE: Request without min_samples param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Response should be valid (min_samples=5 used internally)
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing required parameters", func() {
			It("should return 400 Bad Request when incident_type is missing", func() {
				// ARRANGE: Request without incident_type param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 when incident_type is missing")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"),
					"Error response should use RFC 7807 format")
			})
		})

		Context("with invalid query parameters", func() {
			It("should return 400 Bad Request when time_range format is invalid", func() {
				// ARRANGE: Request with invalid time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for invalid time_range format")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})

			It("should return 400 Bad Request when min_samples is not a number", func() {
				// ARRANGE: Request with non-numeric min_samples
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=abc",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for non-numeric min_samples")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})

		Context("when repository returns error", func() {
			It("should return 500 Internal Server Error", func() {
				// ARRANGE: Mock repository to return error
				// TODO: Configure mock to return error in TDD GREEN phase
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 500 Internal Server Error
				// (This test will be skipped until mock is configured)
				Skip("Requires mock repository configuration")
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-02: Playbook Success Rate Handler
	// BEHAVIOR: Parse query params, call repository, return JSON
	// CORRECTNESS: Exact HTTP status codes and response structure
	// ========================================
	Describe("GET /api/v1/success-rate/playbook", func() {
		Context("with valid query parameters", func() {
			It("should return 200 OK with playbook success rate data", func() {
				// ARRANGE: Create HTTP request with valid params
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&time_range=7d&min_samples=5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Handler should return 200 OK for valid request")

				// ASSERT: Response is valid JSON
				var response models.PlaybookSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate response structure
				Expect(response.PlaybookID).To(Equal("restart-pod-v1"),
					"Response should contain requested playbook ID")
				Expect(response.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")
				Expect(response.SuccessRate).To(BeNumerically(">=", 0.0),
					"Success rate should be non-negative")
				Expect(response.SuccessRate).To(BeNumerically("<=", 100.0),
					"Success rate should be <= 100%")
				Expect(response.TotalExecutions).To(BeNumerically(">=", 0),
					"Total executions should be non-negative")
				Expect(response.SuccessfulExecutions).To(BeNumerically(">=", 0),
					"Successful executions should be non-negative")
				Expect(response.Confidence).To(BeElementOf("high", "medium", "low", "insufficient_data"),
					"Confidence should be one of the valid values")
			})

			It("should accept optional playbook_version parameter", func() {
				// ARRANGE: Request with playbook_version
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&playbook_version=1.2.3&time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Version included in response
				var response models.PlaybookSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.PlaybookVersion).To(Equal("1.2.3"),
					"Response should include playbook version when specified")
			})
		})

		Context("with missing required parameters", func() {
			It("should return 400 Bad Request when playbook_id is missing", func() {
				// ARRANGE: Request without playbook_id param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 when playbook_id is missing")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})

		Context("with invalid query parameters", func() {
			It("should return 400 Bad Request when time_range format is invalid", func() {
				// ARRANGE: Request with invalid time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&time_range=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for invalid time_range format")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})
	})

	// ========================================
	// Edge Cases and Error Scenarios
	// ========================================
	Describe("Edge Cases", func() {
		Context("time range parsing", func() {
			It("should accept valid time range formats: 1h, 24h, 7d, 30d", func() {
				validRanges := []string{"1h", "24h", "7d", "30d"}

				for _, timeRange := range validRanges {
					req = httptest.NewRequest(
						http.MethodGet,
						"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range="+timeRange,
						nil,
					)
					rec = httptest.NewRecorder()

					handler.HandleGetSuccessRateByIncidentType(rec, req)

					Expect(rec.Code).To(Equal(http.StatusOK),
						"Handler should accept time_range=%s", timeRange)
				}
			})

			It("should reject invalid time range formats", func() {
				invalidRanges := []string{"1x", "abc", "7days", "-1d", "0d"}

				for _, timeRange := range invalidRanges {
					req = httptest.NewRequest(
						http.MethodGet,
						"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range="+timeRange,
						nil,
					)
					rec = httptest.NewRecorder()

					handler.HandleGetSuccessRateByIncidentType(rec, req)

					Expect(rec.Code).To(Equal(http.StatusBadRequest),
						"Handler should reject time_range=%s", timeRange)
				}
			})
		})

		Context("min_samples validation", func() {
			It("should accept positive integers for min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=10",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
			})

			It("should reject negative min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=-5",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should reject negative min_samples")
			})

			It("should reject zero min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=0",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should reject zero min_samples")
			})
		})
	})
})
