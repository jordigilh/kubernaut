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

package contextapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DAY 11 TDD RED: INTEGRATION TESTS FOR ADR-033 AGGREGATION API
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// ========================================
//
// **OBJECTIVE**: Write failing integration tests that validate behavior and correctness
//
// **BEHAVIOR VALIDATION**: Tests verify the system behaves correctly under various conditions
// **CORRECTNESS VALIDATION**: Tests verify the data returned is accurate and complete
//
// **TDD RED Phase**: All tests should FAIL initially (no routes registered yet)
// ========================================

var _ = Describe("Aggregation API Integration Tests", Ordered, func() {
	var contextAPIServer *server.Server
	var httpTestServer *httptest.Server
	var dataStorageBaseURL string

	BeforeAll(func() {
		// Infrastructure requirements (must be running):
		// 1. PostgreSQL: localhost:5432 (datastorage-postgres container)
		// 2. Redis: localhost:6379 (contextapi-redis-test container - started by suite_test.go)
		// 3. Data Storage Service: localhost:8085 (datastorage-service-test container)
		//
		// Start infrastructure with: make bootstrap-dev (from workspace root)
		// Or manually: see test/integration/datastorage/suite_test.go for Podman commands

		dataStorageBaseURL = fmt.Sprintf("http://localhost:%d", dataStoragePort)

		// Verify Data Storage Service is running
		GinkgoWriter.Println("🔍 Checking Data Storage Service availability...")
		Eventually(func() error {
			resp, err := http.Get(dataStorageBaseURL + "/health")
			if err != nil {
				return fmt.Errorf("Data Storage Service not reachable: %w (start with: make bootstrap-dev)", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Data Storage Service unhealthy: %d", resp.StatusCode)
			}
			return nil
		}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Data Storage Service must be running on port %d", dataStoragePort))

		GinkgoWriter.Println("✅ Data Storage Service is available")

		// Start in-process Context API server
		GinkgoWriter.Println("🚀 Starting Context API server...")
		cfg := &server.Config{
			Port:               8080,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			DataStorageBaseURL: dataStorageBaseURL,
		}

		// Create custom Prometheus registry for this test suite to avoid duplicate registration panic
		// This allows multiple test suites to create servers without conflicts
		customRegistry := prometheus.NewRegistry()
		customMetrics := metrics.NewMetricsWithRegistry("context_api", "server", customRegistry)

		var err error
		contextAPIServer, err = server.NewServerWithMetrics(
			fmt.Sprintf("localhost:%d", redisPort), // Redis from suite_test.go
			logger,
			cfg,
			customMetrics, // Use custom metrics with isolated registry
		)
		Expect(err).ToNot(HaveOccurred(), "Context API server creation should succeed")

		// Create HTTP test server
		httpTestServer = httptest.NewServer(contextAPIServer.Handler())
		GinkgoWriter.Printf("✅ Context API server started at %s\n", httpTestServer.URL)
	})

	AfterAll(func() {
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
			// CORRECTNESS: Response matches expected Data Storage Service format with all required fields

			// Make HTTP request
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=5", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid incident type")

			// CORRECTNESS: Response body is valid JSON matching Data Storage format
			var result dsmodels.IncidentTypeSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Response contains all required fields with correct values
			Expect(result.IncidentType).To(Equal("pod-oom"), "Should return requested incident type")
			Expect(result.TimeRange).To(Equal("7d"), "Should return requested time range")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
			Expect(result.SuccessRate).To(BeNumerically("<=", 100.0), "Success rate should be <= 100%")
			Expect(result.TotalExecutions).To(BeNumerically(">=", 0), "Total executions should be non-negative")
			Expect(result.SuccessfulExecutions).To(BeNumerically(">=", 0), "Successful executions should be non-negative")
			Expect(result.SuccessfulExecutions).To(BeNumerically("<=", result.TotalExecutions), "Successful <= Total")
			Expect(result.Confidence).ToNot(BeEmpty(), "Should include confidence level")
			Expect([]string{"low", "medium", "high", "insufficient_data"}).To(ContainElement(result.Confidence), "Confidence must be low/medium/high/insufficient_data")
		})

		It("should return 400 Bad Request when incident_type is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request with RFC 7807 error
			// CORRECTNESS: Error response follows RFC 7807 Problem Details format

			// Make HTTP request without incident_type
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?time_range=7d", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing parameter")

			// CORRECTNESS: RFC 7807 error response with correct structure
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Should return RFC 7807 error")

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			Expect(errorResp["type"]).ToNot(BeEmpty(), "Should include error type")
			Expect(errorResp["title"]).ToNot(BeEmpty(), "Should include error title")
			Expect(errorResp["status"]).To(Equal(float64(400)), "Should include status code 400")
			Expect(errorResp["detail"]).To(ContainSubstring("incident_type"), "Should mention missing parameter")
		})

		It("should use cache for repeated requests", func() {
			// BEHAVIOR: Second request for same data returns cached result (faster response)
			// CORRECTNESS: Cached response is identical to original response

			incidentType := "pod-oom-cache-test"
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=%s&time_range=7d&min_samples=5", httpTestServer.URL, incidentType)

			// First request (cache miss)
			start1 := time.Now()
			resp1, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			duration1 := time.Since(start1)

			Expect(resp1.StatusCode).To(Equal(http.StatusOK), "First request should succeed")
			var result1 dsmodels.IncidentTypeSuccessRateResponse
			json.NewDecoder(resp1.Body).Decode(&result1)

			// Second request (cache hit)
			start2 := time.Now()
			resp2, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()
			duration2 := time.Since(start2)

			Expect(resp2.StatusCode).To(Equal(http.StatusOK), "Second request should succeed")
			var result2 dsmodels.IncidentTypeSuccessRateResponse
			json.NewDecoder(resp2.Body).Decode(&result2)

			// CORRECTNESS: Results should be identical
			Expect(result2.IncidentType).To(Equal(result1.IncidentType), "Cached incident_type should match")
			Expect(result2.SuccessRate).To(Equal(result1.SuccessRate), "Cached success_rate should match")
			Expect(result2.TotalExecutions).To(Equal(result1.TotalExecutions), "Cached total_executions should match")
			Expect(result2.SuccessfulExecutions).To(Equal(result1.SuccessfulExecutions), "Cached successful_executions should match")

			// BEHAVIOR: Cache hit should be faster (or similar if Data Storage is very fast)
			GinkgoWriter.Printf("First request: %v, Second request (cached): %v\n", duration1, duration2)
		})
	})

	// ========================================
	// BR-INTEGRATION-009: Playbook Success Rate API
	// ========================================

	Context("GET /api/v1/aggregation/success-rate/playbook", func() {
		It("should return playbook success rate for valid playbook_id", func() {
			// BEHAVIOR: Valid playbook_id query returns 200 OK with playbook success rate
			// CORRECTNESS: Response matches expected Data Storage Service format

			// Make HTTP request
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1&playbook_version=1.0.0&time_range=7d&min_samples=5", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for valid playbook ID")

			// CORRECTNESS: Response body is valid JSON matching Data Storage format
			var result dsmodels.PlaybookSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Response contains all required fields with correct values
			Expect(result.PlaybookID).To(Equal("restart-pod-v1"), "Should return requested playbook ID")
			Expect(result.PlaybookVersion).To(Equal("1.0.0"), "Should return requested playbook version")
			Expect(result.TimeRange).To(Equal("7d"), "Should return requested time range")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
			Expect(result.SuccessRate).To(BeNumerically("<=", 100.0), "Success rate should be <= 100%")
			Expect(result.TotalExecutions).To(BeNumerically(">=", 0), "Total executions should be non-negative")
			Expect(result.SuccessfulExecutions).To(BeNumerically(">=", 0), "Successful executions should be non-negative")
			Expect(result.Confidence).ToNot(BeEmpty(), "Should include confidence level")
		})

		It("should return 400 Bad Request when playbook_id is missing", func() {
			// BEHAVIOR: Missing required parameter returns 400 Bad Request
			// CORRECTNESS: RFC 7807 error response

			// Make HTTP request without playbook_id
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_version=1.0.0&time_range=7d", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for missing parameter")

			// CORRECTNESS: RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Should return RFC 7807 error")

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			Expect(errorResp["detail"]).To(ContainSubstring("playbook_id"), "Should mention missing parameter")
		})

		It("should use default values for optional parameters", func() {
			// BEHAVIOR: Optional parameters (time_range, min_samples) use defaults if not provided
			// CORRECTNESS: Handler applies default values correctly (7d, 5 samples)

			// Make HTTP request with only required parameter
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_id=restart-pod-v1", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK with defaults")

			// CORRECTNESS: Response body is valid JSON
			var result dsmodels.PlaybookSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

			// CORRECTNESS: Default values applied
			Expect(result.TimeRange).To(Equal("7d"), "Should use default time_range of 7d")
			Expect(result.PlaybookID).To(Equal("restart-pod-v1"), "Should return requested playbook ID")
		})
	})

	// ========================================
	// BR-INTEGRATION-010: Multi-Dimensional Success Rate API
	// ========================================

	Context("GET /api/v1/aggregation/success-rate/multi-dimensional", func() {
		It("should return multi-dimensional data for all dimensions", func() {
			// BEHAVIOR: All dimensions specified returns combined success rate data
			// CORRECTNESS: Response includes all query dimensions with accurate aggregation

			// Make HTTP request with all dimensions
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&playbook_id=restart-pod-v1&playbook_version=1.0.0&action_type=restart&time_range=7d&min_samples=5", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for multi-dimensional query")

			// CORRECTNESS: Response includes all dimensions
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type dimension")
			Expect(result.Dimensions.PlaybookID).To(Equal("restart-pod-v1"), "Should include playbook_id dimension")
			Expect(result.Dimensions.PlaybookVersion).To(Equal("1.0.0"), "Should include playbook_version dimension")
			Expect(result.Dimensions.ActionType).To(Equal("restart"), "Should include action_type dimension")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be non-negative")
			Expect(result.SuccessRate).To(BeNumerically("<=", 100.0), "Success rate should be <= 100%")
			Expect(result.TotalExecutions).To(BeNumerically(">=", 0), "Total executions should be non-negative")
		})

		It("should return data for partial dimensions", func() {
			// BEHAVIOR: Partial dimensions (e.g., only incident_type) returns filtered data
			// CORRECTNESS: Response reflects only specified dimensions, others are empty

			// Make HTTP request with only incident_type
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&time_range=7d", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Should return 200 OK for partial dimensions")

			// CORRECTNESS: Response includes only specified dimension
			var result dsmodels.MultiDimensionalSuccessRateResponse
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
			Expect(result.Dimensions.IncidentType).To(Equal("pod-oom"), "Should include incident_type")
			Expect(result.Dimensions.PlaybookID).To(BeEmpty(), "Should not include playbook_id")
			Expect(result.Dimensions.ActionType).To(BeEmpty(), "Should not include action_type")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.0), "Success rate should be valid")
		})

		It("should return 400 Bad Request when no dimensions are specified", func() {
			// BEHAVIOR: At least one dimension is required
			// CORRECTNESS: RFC 7807 error response with clear message

			// Make HTTP request without any dimensions
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?time_range=7d", httpTestServer.URL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer resp.Body.Close()

			// CORRECTNESS: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should return 400 Bad Request for no dimensions")

			// CORRECTNESS: RFC 7807 error response
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"), "Should return RFC 7807 error")

			var errorResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
			Expect(errorResp["detail"]).To(ContainSubstring("at least one dimension"), "Should mention missing dimensions")
		})

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// NOTE: Data Storage Service unavailability (503 response) is covered by:
		// - Unit tests: test/unit/contextapi/aggregation_handlers_test.go (mocked)
		// - E2E tests: test/e2e/contextapi/03_service_failures_test.go (real infrastructure)
		// Integration tier doesn't need this test (sits between unit and E2E)
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	})
})
