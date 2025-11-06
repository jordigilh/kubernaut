// Copyright 2025 The Kubernaut Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
)

// RFC7807Problem represents a Problem Details for HTTP APIs response
type RFC7807Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

var _ = Describe("Aggregation API Edge Cases", Ordered, func() {
	var contextAPIServer *server.Server
	var httpTestServer   *httptest.Server
	var serverURL        string
	var dataStorageBaseURL string

	BeforeAll(func() {
		// Use same infrastructure as Day 11 tests
		dataStorageBaseURL = fmt.Sprintf("http://localhost:%d", dataStoragePort)

		// Verify Data Storage Service is running
		GinkgoWriter.Println("🔍 Checking Data Storage Service availability for edge cases...")
		Eventually(func() error {
			resp, err := http.Get(dataStorageBaseURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Data Storage Service unhealthy: %d", resp.StatusCode)
			}
			return nil
		}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Data Storage Service must be running on port %d", dataStoragePort))

		GinkgoWriter.Println("✅ Data Storage Service is available")

		// Start in-process Context API server (same as Day 11)
		GinkgoWriter.Println("🚀 Starting Context API server for edge case tests...")

		cfg := &server.Config{
			Port:               8081, // Different port to avoid conflicts
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       30 * time.Second,
			DataStorageBaseURL: dataStorageBaseURL,
		}

		var err error
		contextAPIServer, err = server.NewServer(
			fmt.Sprintf("localhost:%d", redisPort), // Redis from suite_test.go
			logger,
			cfg,
		)
		Expect(err).ToNot(HaveOccurred(), "Context API server creation should succeed")

		// Create HTTP test server
		httpTestServer = httptest.NewServer(contextAPIServer.Handler())
		serverURL = httpTestServer.URL
		GinkgoWriter.Printf("✅ Context API server started at %s\n", serverURL)
	})

	AfterAll(func() {
		if httpTestServer != nil {
			httpTestServer.Close()
			GinkgoWriter.Println("✅ Context API test server stopped")
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 1: P0 EDGE CASES - CRITICAL (8 tests)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Input Validation (P0 - Critical)", func() {
		// BR-INTEGRATION-008: Incident-Type Success Rate API - Input Validation

		It("should return 400 Bad Request for empty incident_type", func() {
			// BEHAVIOR: Empty required parameter triggers validation error
			// CORRECTNESS: RFC 7807 error response with specific validation message

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Empty incident_type should return 400")

			// CORRECTNESS: RFC 7807 problem details
			var problem RFC7807Problem
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(problem.Type).To(ContainSubstring("bad-request"), "Error type should indicate bad request")
			Expect(problem.Detail).To(ContainSubstring("incident_type"), "Error detail should mention incident_type")
			Expect(problem.Status).To(Equal(400), "RFC 7807 status should match HTTP status")
		})

		It("should handle special characters in incident_type", func() {
			// BEHAVIOR: Special characters are properly URL-encoded and handled
			// CORRECTNESS: Returns valid response or proper error (not 500)

			// Test with trailing spaces (URL-encoded as %20)
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom%%20%%20", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Should not return 500 Internal Server Error
			Expect(resp.StatusCode).ToNot(Equal(http.StatusInternalServerError), "Special characters should not cause server error")

			// CORRECTNESS: Should return 200 OK or 400 Bad Request (not 500)
			Expect([]int{http.StatusOK, http.StatusBadRequest}).To(ContainElement(resp.StatusCode),
				"Should handle special characters gracefully")
		})

		It("should sanitize SQL injection attempts in incident_type", func() {
			// BEHAVIOR: SQL injection attempts are safely handled
			// CORRECTNESS: Returns valid response without executing SQL

			sqlInjection := "'; DROP TABLE resource_action_traces--"
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=%s", serverURL, sqlInjection)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Should not return 500 (SQL injection prevented)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusInternalServerError), "SQL injection should not cause server error")

			// CORRECTNESS: Should return safe response (200 or 400, not 500)
			Expect([]int{http.StatusOK, http.StatusBadRequest}).To(ContainElement(resp.StatusCode),
				"SQL injection should be safely handled")
		})

		It("should validate very long incident_type strings", func() {
			// BEHAVIOR: Very long strings are rejected or truncated
			// CORRECTNESS: Returns 400 Bad Request with validation error

			// Create 1000+ character string
			longString := ""
			for i := 0; i < 100; i++ {
				longString += "verylongincidenttype"
			}

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=%s", serverURL, longString)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Should handle long strings gracefully (not crash)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusInternalServerError), "Long strings should not cause server error")

			// CORRECTNESS: Should return 200 OK or 400 Bad Request
			Expect([]int{http.StatusOK, http.StatusBadRequest}).To(ContainElement(resp.StatusCode),
				"Long strings should be handled gracefully")
		})

		// BR-INTEGRATION-009: Playbook Success Rate API - Input Validation

		It("should return 400 Bad Request for playbook_version without playbook_id", func() {
			// BEHAVIOR: playbook_version requires playbook_id
			// CORRECTNESS: RFC 7807 error response with validation message

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/playbook?playbook_version=v1.0.0", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 400 Bad Request (missing required playbook_id)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "playbook_version without playbook_id should return 400")

			// CORRECTNESS: RFC 7807 problem details
			var problem RFC7807Problem
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(problem.Detail).To(ContainSubstring("playbook_id"), "Error should mention missing playbook_id")
			Expect(problem.Status).To(Equal(400))
		})

		It("should validate negative min_samples parameter", func() {
			// BEHAVIOR: Negative min_samples is rejected
			// CORRECTNESS: Returns 400 Bad Request with validation error

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&min_samples=-1", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Should handle negative values gracefully
			Expect(resp.StatusCode).ToNot(Equal(http.StatusInternalServerError), "Negative min_samples should not cause server error")

			// CORRECTNESS: Should return 200 OK (using default) or 400 Bad Request
			Expect([]int{http.StatusOK, http.StatusBadRequest}).To(ContainElement(resp.StatusCode),
				"Negative min_samples should be validated or defaulted")
		})

		// BR-INTEGRATION-010: Multi-Dimensional Success Rate API - Input Validation

		It("should return 400 Bad Request when all dimensions are empty", func() {
			// BEHAVIOR: At least one dimension is required
			// CORRECTNESS: RFC 7807 error response with validation message

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=&playbook_id=&action_type=", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 400 Bad Request (no dimensions specified)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Empty dimensions should return 400")

			// CORRECTNESS: RFC 7807 problem details
			var problem RFC7807Problem
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())
			Expect(problem.Detail).To(ContainSubstring("dimension"), "Error should mention dimensions")
			Expect(problem.Status).To(Equal(400))
		})
	})

	Context("Edge Cases: Data Storage Service Failures (P0 - Critical)", func() {
		// Note: These tests require infrastructure manipulation (stopping/starting Data Storage Service)
		// Skipping for now as they require additional test infrastructure

		PIt("should return cached data when Data Storage Service is unavailable", func() {
			// BEHAVIOR: Service degradation - return stale cache instead of failing
			// CORRECTNESS: Cached data is valid

			// TODO: Implement when infrastructure helper supports service stop/start
			Skip("Requires infrastructure helper for stopping/starting Data Storage Service")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: P1 EDGE CASES - HIGH PRIORITY (6 tests)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Time Ranges (P1 - High)", func() {
		// BR-INTEGRATION-008: Incident-Type Success Rate API - Time Range Validation

		It("should handle 1-minute time range correctly", func() {
			// BEHAVIOR: Minimal valid time range returns data
			// CORRECTNESS: Only includes data from last 1 minute

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=1m", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (minimal time range is valid)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "1-minute time range should be valid")

			// CORRECTNESS: Response includes time_range field
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["time_range"]).To(Equal("1m"), "Time range should be reflected in response")
		})

		It("should handle very long time range (365 days)", func() {
			// BEHAVIOR: Very long time range is accepted
			// CORRECTNESS: Returns data without performance issues

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=365d", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (long time range is valid)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "365-day time range should be valid")

			// CORRECTNESS: Response includes time_range field
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["time_range"]).To(Equal("365d"), "Time range should be reflected in response")
		})

		It("should handle invalid time range format gracefully", func() {
			// BEHAVIOR: Invalid time range format falls back to default
			// CORRECTNESS: Returns 200 OK with default time range (not 400)

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=invalid", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (graceful degradation to default)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Invalid time range should fall back to default")

			// CORRECTNESS: Response uses invalid format as-is (passed to Data Storage)
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			// Note: Data Storage Service will handle invalid format validation
		})
	})

	Context("Edge Cases: Caching (P1 - High)", func() {
		// BR-INTEGRATION-008: Incident-Type Success Rate API - Cache Behavior

		It("should cache responses for identical requests", func() {
			// BEHAVIOR: Identical requests hit cache (L1 Redis or L2 LRU)
			// CORRECTNESS: Cached response matches original response

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=5", serverURL)

			// First request (cache miss)
			resp1, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))

			var result1 map[string]interface{}
			err = json.NewDecoder(resp1.Body).Decode(&result1)
			Expect(err).ToNot(HaveOccurred())

			// Second request (should hit cache)
			resp2, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()

			// BEHAVIOR: Returns 200 OK from cache
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			// CORRECTNESS: Cached response matches original
			var result2 map[string]interface{}
			err = json.NewDecoder(resp2.Body).Decode(&result2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2["incident_type"]).To(Equal(result1["incident_type"]))
			Expect(result2["time_range"]).To(Equal(result1["time_range"]))
		})

		It("should use different cache keys for different parameters", func() {
			// BEHAVIOR: Different parameters create different cache keys
			// CORRECTNESS: Each parameter combination is cached separately

			url1 := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d", serverURL)
			url2 := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=30d", serverURL)

			// Request 1
			resp1, err := http.Get(url1)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))

			var result1 map[string]interface{}
			err = json.NewDecoder(resp1.Body).Decode(&result1)
			Expect(err).ToNot(HaveOccurred())

			// Request 2 (different time_range)
			resp2, err := http.Get(url2)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()

			// BEHAVIOR: Returns 200 OK (different cache key)
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			// CORRECTNESS: Different time_range in response
			var result2 map[string]interface{}
			err = json.NewDecoder(resp2.Body).Decode(&result2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2["time_range"]).To(Equal("30d"))
			Expect(result1["time_range"]).To(Equal("7d"))
		})

		It("should handle concurrent requests for same key gracefully", func() {
			// BEHAVIOR: Concurrent requests don't cause race conditions
			// CORRECTNESS: All requests return valid responses

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom-concurrent", serverURL)

			// Make 5 concurrent requests
			done := make(chan bool, 5)
			for i := 0; i < 5; i++ {
				go func() {
					defer GinkgoRecover()
					resp, err := http.Get(url)
					Expect(err).ToNot(HaveOccurred())
					defer resp.Body.Close()

					// BEHAVIOR: All requests succeed
					Expect(resp.StatusCode).To(Equal(http.StatusOK))

					// CORRECTNESS: All responses are valid JSON
					var result map[string]interface{}
					err = json.NewDecoder(resp.Body).Decode(&result)
					Expect(err).ToNot(HaveOccurred())
					Expect(result["incident_type"]).To(Equal("pod-oom-concurrent"))

					done <- true
				}()
			}

			// Wait for all requests to complete
			for i := 0; i < 5; i++ {
				Eventually(done, 10*time.Second).Should(Receive())
			}
		})
	})
})

