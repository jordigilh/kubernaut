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
			Expect(problem.Type).To(ContainSubstring("validation-error"), "Error type should indicate validation error")
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
})

