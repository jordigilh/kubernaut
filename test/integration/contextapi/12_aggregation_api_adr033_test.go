package contextapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DAY 11 INTEGRATION TESTS: ADR-033 Aggregation API
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// ========================================
//
// **OBJECTIVE**: Validate Context API aggregation endpoints with real Data Storage Service
//
// **Test Strategy**:
// - Use real Data Storage HTTP client (no mocks)
// - Use real Redis cache
// - Use real PostgreSQL database
// - Validate end-to-end flow: Context API → Data Storage → PostgreSQL
//
// **Test Coverage** (10 integration tests):
// 1. Incident-Type Endpoint (3 tests)
// 2. Playbook Endpoint (3 tests)
// 3. Multi-Dimensional Endpoint (4 tests)
// ========================================

var _ = Describe("ADR-033 Aggregation API Integration Tests", func() {
	var (
		contextAPIServer *server.Server
		httpTestServer   *httptest.Server
		dataStorageURL   string
	)

	BeforeEach(func() {
		// Data Storage Service URL (assumes it's running on localhost:8085)
		// In real integration tests, this would be started in BeforeSuite
		dataStorageURL = "http://localhost:8085"

		// Create Context API server with real components
		cfg := &server.Config{
			Port:               8080,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			DataStorageBaseURL: dataStorageURL,
		}

		var err error
		contextAPIServer, err = server.NewServer(
			"localhost:6379", // Redis from suite setup
			logger,
			cfg,
		)
		Expect(err).ToNot(HaveOccurred(), "Context API server creation should succeed")

		// Create HTTP test server
		httpTestServer = httptest.NewServer(contextAPIServer.Handler())
	})

	AfterEach(func() {
		if httpTestServer != nil {
			httpTestServer.Close()
		}
	})

	// ========================================
	// BR-INTEGRATION-008: Incident-Type Success Rate API
	// ========================================

	Context("GET /api/v1/aggregation/success-rate/incident-type", func() {
		It("should return success rate data for valid incident type", func() {
			// BEHAVIOR: Valid incident_type query returns 200 OK with success rate data
			// CORRECTNESS: Response matches Data Storage Service format

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid incident type")

			// CORRECTNESS: Response body is valid JSON
			var result dsmodels.IncidentTypeSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Response contains expected fields
			Expect(result.IncidentType).To(Equal("pod-oom"), "Should return requested incident type")
			Expect(result.TimeRange).To(Equal("7d"), "Should return requested time range")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
			Expect(result.SuccessRate).To(BeNumerically("<=", 100.0), "Success rate should be <= 100%")
			Expect(result.Confidence).ToNot(BeEmpty(), "Should include confidence level")
		})

		It("should return 400 Bad Request when incident_type is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request without incident_type
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing parameter")

			// CORRECTNESS: RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Should return RFC 7807 error")

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred())
			Expect(errorResp["type"]).ToNot(BeEmpty(), "Should include error type")
			Expect(errorResp["detail"]).To(ContainSubstring("incident_type"), "Should mention missing parameter")
		})

		It("should use cache for repeated requests", func() {
			// BEHAVIOR: Second request for same data returns cached result
			// CORRECTNESS: Response is identical, faster response time

			incidentType := "pod-oom-cache-test"
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5", httpTestServer.URL, incidentType)

			// First request (cache miss)
			start1 := time.Now()
			resp1, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			duration1 := time.Since(start1)

			Expect(resp1.StatusCode).To(Equal(http.StatusOK))
			var result1 dsmodels.IncidentTypeSuccessRateResponse
			json.NewDecoder(resp1.Body).Decode(&result1)

			// Second request (cache hit)
			start2 := time.Now()
			resp2, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()
			duration2 := time.Since(start2)

			Expect(resp2.StatusCode).To(Equal(http.StatusOK))
			var result2 dsmodels.IncidentTypeSuccessRateResponse
			json.NewDecoder(resp2.Body).Decode(&result2)

			// CORRECTNESS: Results should be identical
			Expect(result2.IncidentType).To(Equal(result1.IncidentType))
			Expect(result2.SuccessRate).To(Equal(result1.SuccessRate))

			// BEHAVIOR: Cache hit should be faster (or similar if Data Storage is very fast)
			// Note: This is a soft check - cache might not always be faster in tests
			GinkgoWriter.Printf("First request: %v, Second request (cached): %v\n", duration1, duration2)
		})
	})

	// ========================================
	// BR-INTEGRATION-009: Playbook Success Rate API
	// ========================================

	Context("GET /api/v1/aggregation/success-rate/playbook", func() {
		It("should return playbook success rate for valid playbook_id", func() {
			// BEHAVIOR: Valid playbook_id query returns 200 OK with playbook success rate
			// CORRECTNESS: Response matches Data Storage Service format

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1&playbook_version=1.0.0&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid playbook")

			// CORRECTNESS: Response body is valid JSON
			var result dsmodels.PlaybookSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Response contains expected fields
			Expect(result.PlaybookID).To(Equal("restart-pod-v1"), "Should return requested playbook ID")
			Expect(result.PlaybookVersion).To(Equal("1.0.0"), "Should return requested playbook version")
			Expect(result.TimeRange).To(Equal("7d"), "Should return requested time range")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
			Expect(result.SuccessRate).To(BeNumerically("<=", 100.0), "Success rate should be <= 100%")
		})

		It("should return 400 Bad Request when playbook_id is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request without playbook_id
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing parameter")

			// CORRECTNESS: RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			var errorResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(errorResp["detail"]).To(ContainSubstring("playbook_id"), "Should mention missing parameter")
		})

		It("should use default values for optional parameters", func() {
			// BEHAVIOR: Optional parameters (time_range, min_samples) use defaults if not provided
			// CORRECTNESS: Handler applies default values correctly

			// Make HTTP request with only required parameter
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK with defaults")

			// CORRECTNESS: Response uses default time_range
			var result dsmodels.PlaybookSuccessRateResponse
			json.NewDecoder(resp.Body).Decode(&result)
			Expect(result.TimeRange).To(Equal("7d"), "Should use default time_range=7d")
		})
	})

	// ========================================
	// BR-INTEGRATION-010: Multi-Dimensional Success Rate API
	// ========================================

	Context("GET /api/v1/aggregation/success-rate/multi-dimensional", func() {
		It("should return multi-dimensional data for all dimensions", func() {
			// BEHAVIOR: All dimensions specified returns combined success rate data
			// CORRECTNESS: Response includes all query dimensions

			// Make HTTP request with all dimensions
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&playbook_id=restart-pod-v1&playbook_version=1.0.0&action_type=restart&time_range=7d&min_samples=5", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for multi-dimensional query")

			// CORRECTNESS: Response includes all dimensions
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type dimension")
			Expect(result.Dimensions.PlaybookID).To(Equal("restart-pod-v1"), "Should include playbook_id dimension")
			Expect(result.Dimensions.ActionType).To(Equal("restart"), "Should include action_type dimension")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
		})

		It("should return data for partial dimensions", func() {
			// BEHAVIOR: Partial dimensions (e.g., only incident_type) returns filtered data
			// CORRECTNESS: Response reflects only specified dimensions

			// Make HTTP request with only incident_type
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for partial dimensions")

			// CORRECTNESS: Response includes only specified dimension
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type")
			Expect(result.Dimensions.PlaybookID).To(BeEmpty(), "Should not include playbook_id")
			Expect(result.Dimensions.ActionType).To(BeEmpty(), "Should not include action_type")
		})

		It("should return 400 Bad Request when no dimensions are specified", func() {
			// BEHAVIOR: At least one dimension is required
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request with no dimensions
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?time_range=7d", httpTestServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for no dimensions")

			// CORRECTNESS: RFC 7807 error response
			var errorResp map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(errorResp["detail"]).To(ContainSubstring("dimension"), "Should mention missing dimensions")
		})

		It("should handle Data Storage Service errors gracefully", func() {
			// BEHAVIOR: When Data Storage Service is unavailable, return appropriate error
			// CORRECTNESS: RFC 7807 error response with proper status code

			// Create Context API with invalid Data Storage URL
			cfg := &server.Config{
				Port:               8081,
				ReadTimeout:        30 * time.Second,
				WriteTimeout:       30 * time.Second,
				DataStorageBaseURL: "http://localhost:9999", // Invalid URL
			}

			testServer, err := server.NewServer("localhost:6379", logger, cfg)
			Expect(err).ToNot(HaveOccurred())

			testHTTPServer := httptest.NewServer(testServer.Handler())
			defer testHTTPServer.Close()

			// Make HTTP request
			resp, err := http.Get(fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d", testHTTPServer.URL))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// CORRECTNESS: Should return error status (500 or 503)
			Expect(resp.StatusCode).To(Or(Equal(http.StatusInternalServerError), Equal(http.StatusServiceUnavailable)), "Should return error status when Data Storage unavailable")

			// CORRECTNESS: RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
		})
	})
})

