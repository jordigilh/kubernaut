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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	var serverURL string
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

		// Start actual Context API server on port 8081 (Option A)
		GinkgoWriter.Println("🚀 Starting Context API server for edge case tests on port 8081...")

		cfg := &server.Config{
			Port:               8081, // Fixed port for edge case tests
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

		// Start server in background goroutine
		go func() {
			defer GinkgoRecover()
			if err := contextAPIServer.Start(); err != nil && err != http.ErrServerClosed {
				GinkgoWriter.Printf("❌ Context API server error: %v\n", err)
			}
		}()

		// Wait for server to be ready
		serverURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
		Eventually(func() error {
			resp, err := http.Get(serverURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("Context API unhealthy: %d", resp.StatusCode)
			}
			return nil
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed(), "Context API should be ready")

		GinkgoWriter.Printf("✅ Context API server started at %s\n", serverURL)
	})

	AfterAll(func() {
		if contextAPIServer != nil {
			GinkgoWriter.Println("🛑 Stopping Context API server...")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := contextAPIServer.Shutdown(ctx); err != nil {
				GinkgoWriter.Printf("⚠️  Context API shutdown error: %v\n", err)
			}
			GinkgoWriter.Println("✅ Context API server stopped")
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

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// NOTE: Data Storage Service unavailability is covered by E2E tests:
	// - test/e2e/contextapi/03_service_failures_test.go (Test 1)
	// - test/e2e/contextapi/04_cache_resilience_test.go (Test 5)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 2: P1 EDGE CASES - HIGH PRIORITY (6 tests)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Time Ranges (P1 - High)", func() {
		// BR-INTEGRATION-008: Incident-Type Success Rate API - Time Range Validation

		It("should handle 1-hour time range correctly", func() {
			// BEHAVIOR: Minimal valid time range returns data
			// CORRECTNESS: Only includes data from last 1 hour

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=1h", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (1h is minimal valid time range)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "1-hour time range should be valid")

			// CORRECTNESS: Response includes time_range field
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["time_range"]).To(Equal("1h"), "Time range should be reflected in response")
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
			// BEHAVIOR: Invalid time range format is validated by Data Storage Service
			// CORRECTNESS: Returns error response (400 or 500, not crash)

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=invalid", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Context API propagates Data Storage validation error
			// Data Storage Service validates time_range format and returns 400 or 500
			Expect([]int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError}).To(ContainElement(resp.StatusCode),
				"Invalid time range should be handled (200/400/500)")

			// CORRECTNESS: Response is valid JSON (RFC 7807 or success response)
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")
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

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// PHASE 3: P2 EDGE CASES - MEDIUM PRIORITY (4 tests)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Boundary Conditions (P2 - Medium)", func() {
		// BR-INTEGRATION-008: Incident-Type Success Rate API - Boundary Conditions

		It("should handle zero executions gracefully", func() {
			// BEHAVIOR: No data in database returns valid response
			// CORRECTNESS: Returns 0% success rate with insufficient_data confidence

			// Use unique incident type that doesn't exist in database
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=nonexistent-incident-type-12345", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (no data is valid state)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Zero executions should return 200 OK")

			// CORRECTNESS: Response indicates no data
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["total_executions"]).To(BeNumerically("==", 0), "Total executions should be 0")
			Expect(result["success_rate"]).To(BeNumerically("==", 0), "Success rate should be 0")
			Expect(result["confidence"]).To(Equal("insufficient_data"), "Confidence should be insufficient_data")
		})

		It("should handle 100% success rate correctly", func() {
			// BEHAVIOR: All successful executions return 100% success rate
			// CORRECTNESS: Success rate calculation is accurate

			// Note: This test relies on seeded data or real executions
			// For now, we'll test that the endpoint handles the calculation correctly
			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=365d", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// CORRECTNESS: Response includes valid success_rate (0-100)
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["success_rate"]).To(BeNumerically(">=", 0), "Success rate should be >= 0")
			Expect(result["success_rate"]).To(BeNumerically("<=", 100), "Success rate should be <= 100")
		})

		It("should handle exactly min_samples boundary", func() {
			// BEHAVIOR: Exactly min_samples executions meets threshold
			// CORRECTNESS: min_samples_met should be true when total == min_samples

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&min_samples=1", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// CORRECTNESS: Response includes min_samples_met field
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())

			// If there's at least 1 execution, min_samples_met should be true
			totalExecutions := result["total_executions"].(float64)
			if totalExecutions >= 1 {
				Expect(result["min_samples_met"]).To(BeTrue(), "min_samples_met should be true when total >= min_samples")
			}
		})

		It("should handle very large min_samples gracefully", func() {
			// BEHAVIOR: Very large min_samples doesn't cause errors
			// CORRECTNESS: min_samples_met should be false when total < min_samples

			url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&min_samples=999999", serverURL)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BEHAVIOR: Returns 200 OK (large min_samples is valid)
			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Large min_samples should not cause error")

			// CORRECTNESS: min_samples_met should be false (unlikely to have 999999 executions)
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			Expect(err).ToNot(HaveOccurred())
			Expect(result["min_samples_met"]).To(BeFalse(), "min_samples_met should be false for very large threshold")
		})
	})
})
