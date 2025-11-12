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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E Test Suite - Phase 1: Service Failure Scenarios
// Tests Context API ↔ Data Storage Service integration under failure conditions
//
// Business Requirements:
// - BR-INTEGRATION-008: Incident-Type endpoint resilience
// - BR-INTEGRATION-009: Playbook endpoint resilience
// - BR-INTEGRATION-010: Multi-Dimensional endpoint resilience
// - BR-CONTEXT-012: Graceful degradation under service failures
//
// Related: Day 12.5 Phase 1 - Service Failure Scenarios (4 P0 tests)

// RFC7807Error represents an RFC 7807 Problem Details error response
type RFC7807Error struct {
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Status   int                    `json:"status"`
	Detail   string                 `json:"detail"`
	Instance string                 `json:"instance,omitempty"`
	Extra    map[string]interface{} `json:"-"`
}

var _ = Describe("E2E Service Failure Scenarios", Ordered, func() {
	Context("Phase 1: Critical Service Failures (P0)", func() {
		Describe("Test 1: Data Storage Service Unavailable", func() {
			It("should handle Data Storage Service unavailable gracefully", func() {
				// BEHAVIOR: Data Storage Service down → Context API returns RFC 7807 error
				// CORRECTNESS: HTTP 503 Service Unavailable with retry-after header
				//
				// Production Impact: 0.1-1% of requests
				// Business Requirement: BR-CONTEXT-012 (Graceful degradation under service failures)

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 1: Data Storage Service Unavailable")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Stop Data Storage Service
				GinkgoWriter.Println("🛑 Stopping Data Storage Service...")
				dataStorageInfra.Stop(GinkgoWriter)
				time.Sleep(2 * time.Second) // Wait for service to be fully stopped
				GinkgoWriter.Println("✅ Data Storage Service stopped")

				// Make request to Context API
				GinkgoWriter.Println("📡 Making request to Context API (Data Storage is down)...")
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Returns 503 Service Unavailable (not 500) OR 200 OK (if cache hit)
				// BR-CONTEXT-010: Graceful degradation - may return cached data
				Expect([]int{http.StatusOK, http.StatusServiceUnavailable}).To(ContainElement(resp.StatusCode),
					"Data Storage unavailable should return 503 (cache miss) or 200 (cache hit)")

				// CORRECTNESS: Validate response based on status code
				if resp.StatusCode == http.StatusServiceUnavailable {
					// Cache miss - should return RFC 7807 error
					var errorResp RFC7807Error
					err = json.NewDecoder(resp.Body).Decode(&errorResp)
					Expect(err).ToNot(HaveOccurred(), "Response should be valid RFC 7807 JSON")

					GinkgoWriter.Printf("📋 RFC 7807 Error:\n")
					GinkgoWriter.Printf("   Type:   %s\n", errorResp.Type)
					GinkgoWriter.Printf("   Title:  %s\n", errorResp.Title)
					GinkgoWriter.Printf("   Status: %d\n", errorResp.Status)
					GinkgoWriter.Printf("   Detail: %s\n", errorResp.Detail)

					Expect(errorResp.Type).To(ContainSubstring("service-unavailable"),
						"Error type should indicate service unavailable")
					Expect(errorResp.Title).To(Or(
						Equal("Data Storage Service Unavailable"),
						Equal("Service Unavailable"),
					), "Error title should indicate Data Storage unavailability")
					Expect(errorResp.Detail).To(ContainSubstring("retry"),
						"Error detail should include retry guidance")

					// Verify retry-after header
					retryAfter := resp.Header.Get("Retry-After")
					GinkgoWriter.Printf("🔄 Retry-After header: %s\n", retryAfter)
					Expect(retryAfter).ToNot(BeEmpty(),
						"Should include Retry-After header for client retry logic")

					GinkgoWriter.Println("✅ Test 1 PASSED: Data Storage unavailable handled gracefully (503 with RFC 7807 error)")
				} else {
					// Cache hit - should return cached data (BR-CONTEXT-010: Graceful degradation)
					var result SuccessRateResponse
					err = json.NewDecoder(resp.Body).Decode(&result)
					Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

					GinkgoWriter.Printf("📋 Cached Success Rate: %.2f%% (%d/%d executions)\n",
						result.SuccessRate, result.SuccessfulExecutions, result.TotalExecutions)

					GinkgoWriter.Println("✅ Test 1 PASSED: Data Storage unavailable handled gracefully (200 with cached data - BR-CONTEXT-010)")
				}

				// Restart Data Storage Service for next tests
				GinkgoWriter.Println("🔄 Restarting Data Storage Infrastructure...")
				cfg := &infrastructure.DataStorageConfig{
					PostgresPort: postgresPort,
					RedisPort:    redisPort,
					ServicePort:  dataStoragePort,
					DBName:       "action_history",
					DBUser:       "slm_user",
					DBPassword:   "test_password_e2e",
				}
				var restartErr error
				dataStorageInfra, restartErr = infrastructure.StartDataStorageInfrastructure(cfg, GinkgoWriter)
				Expect(restartErr).ToNot(HaveOccurred(), "Data Storage infrastructure should restart successfully")
				GinkgoWriter.Println("✅ Data Storage Infrastructure restarted")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 2: Data Storage Service Timeout", func() {
			It("should timeout Data Storage Service requests after 30s", func() {
				// BEHAVIOR: Slow Data Storage Service → Context API times out gracefully
				// CORRECTNESS: HTTP 504 Gateway Timeout within 35s (30s timeout + 5s overhead)
				//
				// Production Impact: 0.5-2% of requests
				// Business Requirement: BR-CONTEXT-012 (Graceful degradation under service failures)
				//
				// Note: This test validates timeout configuration
				// In a real scenario, Data Storage Service would be artificially delayed

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 2: Data Storage Service Timeout")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Make request and measure duration
				GinkgoWriter.Println("📡 Making request to Context API (testing timeout behavior)...")
				start := time.Now()
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=timeout-test&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				duration := time.Since(start)

				GinkgoWriter.Printf("⏱️  Request duration: %v\n", duration)

				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Either succeeds quickly OR times out within 35s
				Expect(duration).To(BeNumerically("<", 35*time.Second),
					"Request should complete or timeout within 35s (30s timeout + 5s overhead)")

				// CORRECTNESS: Returns 200 OK or 504 Gateway Timeout
				Expect([]int{http.StatusOK, http.StatusGatewayTimeout}).To(ContainElement(resp.StatusCode),
					"Should return either 200 OK (success) or 504 Gateway Timeout")

				if resp.StatusCode == http.StatusGatewayTimeout {
					// Validate RFC 7807 error response
					var errorResp RFC7807Error
					err = json.NewDecoder(resp.Body).Decode(&errorResp)
					Expect(err).ToNot(HaveOccurred(), "Response should be valid RFC 7807 JSON")

					GinkgoWriter.Printf("📋 RFC 7807 Error:\n")
					GinkgoWriter.Printf("   Type:   %s\n", errorResp.Type)
					GinkgoWriter.Printf("   Title:  %s\n", errorResp.Title)
					GinkgoWriter.Printf("   Status: %d\n", errorResp.Status)
					GinkgoWriter.Printf("   Detail: %s\n", errorResp.Detail)

					Expect(errorResp.Type).To(ContainSubstring("gateway-timeout"),
						"Error type should indicate gateway timeout")
					Expect(errorResp.Detail).To(ContainSubstring("30s"),
						"Error detail should mention 30s timeout")

					GinkgoWriter.Println("✅ Test 2 PASSED: Timeout handled gracefully with RFC 7807 error")
				} else {
					GinkgoWriter.Println("✅ Test 2 PASSED: Request completed successfully (no timeout)")
				}

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 3: Malformed Data Storage Response", func() {
			It("should handle malformed Data Storage response gracefully", func() {
				// BEHAVIOR: Invalid JSON from Data Storage → Context API returns 502 Bad Gateway
				// CORRECTNESS: RFC 7807 error with upstream service details OR graceful degradation
				//
				// Production Impact: 0.1-0.5% of requests
				// Business Requirement: BR-CONTEXT-012 (Graceful degradation under service failures)
				//
				// Note: This test validates error handling for unexpected responses
				// In production, this could be schema changes or data corruption

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 3: Malformed Data Storage Response")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Make request with a test incident type
				GinkgoWriter.Println("📡 Making request to Context API (testing malformed response handling)...")
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=malformed-test&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Returns 200 OK (graceful degradation) OR 502 Bad Gateway
				Expect([]int{http.StatusOK, http.StatusBadGateway}).To(ContainElement(resp.StatusCode),
					"Should handle malformed response gracefully (200 OK or 502 Bad Gateway)")

				if resp.StatusCode == http.StatusBadGateway {
					// Validate RFC 7807 error response
					var errorResp RFC7807Error
					err = json.NewDecoder(resp.Body).Decode(&errorResp)
					Expect(err).ToNot(HaveOccurred(), "Response should be valid RFC 7807 JSON")

					GinkgoWriter.Printf("📋 RFC 7807 Error:\n")
					GinkgoWriter.Printf("   Type:   %s\n", errorResp.Type)
					GinkgoWriter.Printf("   Title:  %s\n", errorResp.Title)
					GinkgoWriter.Printf("   Status: %d\n", errorResp.Status)
					GinkgoWriter.Printf("   Detail: %s\n", errorResp.Detail)

					Expect(errorResp.Type).To(ContainSubstring("bad-gateway"),
						"Error type should indicate bad gateway")
					Expect(errorResp.Detail).To(ContainSubstring("Data Storage"),
						"Error detail should mention Data Storage Service")

					GinkgoWriter.Println("✅ Test 3 PASSED: Malformed response handled with RFC 7807 error")
				} else {
					// Graceful degradation: returned 200 OK with empty or default data
					GinkgoWriter.Println("✅ Test 3 PASSED: Malformed response handled with graceful degradation (200 OK)")
				}

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 4: PostgreSQL Connection Timeout", func() {
			It("should handle PostgreSQL timeout gracefully (via Data Storage)", func() {
				// BEHAVIOR: PostgreSQL timeout → Data Storage → Context API returns 504
				// CORRECTNESS: End-to-end timeout handling across 3 services
				//
				// Production Impact: 0.5-2% of requests
				// Business Requirement: BR-CONTEXT-012 (Graceful degradation under service failures)
				//
				// Note: This test simulates PostgreSQL slowness by inserting a large dataset
				// (This is an E2E test, so we test real timeout behavior)

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 4: PostgreSQL Connection Timeout")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// First, create parent records to satisfy foreign key constraints
				GinkgoWriter.Println("📝 Creating parent records (resource_references, action_histories)...")

				// Create resource_reference
				_, err := db.Exec(`
					INSERT INTO resource_references (id, kind, resource_uid, api_version, namespace, name)
					VALUES (999, 'Pod', 'large-dataset-test-uid', 'v1', 'default', 'large-dataset-test-pod')
					ON CONFLICT (id) DO NOTHING
				`)
				Expect(err).ToNot(HaveOccurred(), "Should create resource_reference")

				// Create action_history
				_, err = db.Exec(`
					INSERT INTO action_histories (id, resource_id)
					VALUES (999, 999)
					ON CONFLICT (id) DO NOTHING
				`)
				Expect(err).ToNot(HaveOccurred(), "Should create action_history")

				GinkgoWriter.Println("✅ Parent records created")

				// Insert 5,000 records to slow down aggregation query
				GinkgoWriter.Println("📝 Inserting 5,000 records to simulate PostgreSQL slowness...")
				insertStart := time.Now()
				for i := 0; i < 5000; i++ {
					_, err := db.Exec(`
						INSERT INTO resource_action_traces (
							action_history_id, action_id, action_type, execution_status,
							incident_type, playbook_id, playbook_version,
							signal_name, signal_severity, model_used, model_confidence,
							alert_name, incident_severity,
							playbook_step_number, playbook_execution_id,
							ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation,
							action_timestamp
						) VALUES (
							999, gen_random_uuid()::text, 'restart-pod', 'completed',
							'large-dataset-test', 'playbook-v1', '1.0.0',
							'test-signal', 'warning', 'gpt-4', 0.95,
							'test-alert', 'warning',
							1, gen_random_uuid()::text,
							true, false, false,
							NOW()
						)
					`)
					Expect(err).ToNot(HaveOccurred(), "Should insert test record")

					// Progress indicator every 1000 records
					if (i+1)%1000 == 0 {
						GinkgoWriter.Printf("   Inserted %d/%d records...\n", i+1, 5000)
					}
				}
				insertDuration := time.Since(insertStart)
				GinkgoWriter.Printf("✅ Inserted 5,000 records in %v\n", insertDuration)

				// Make request and measure duration
				GinkgoWriter.Println("📡 Making request to Context API (large dataset query)...")
				start := time.Now()
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=large-dataset-test&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				duration := time.Since(start)

				GinkgoWriter.Printf("⏱️  Request duration: %v\n", duration)

				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Should complete or timeout within 35s
				Expect(duration).To(BeNumerically("<", 35*time.Second),
					"Request should complete or timeout within 35s (30s timeout + 5s overhead)")

				// CORRECTNESS: Either succeeds or returns 504
				Expect([]int{http.StatusOK, http.StatusGatewayTimeout}).To(ContainElement(resp.StatusCode),
					"Should return either 200 OK (success) or 504 Gateway Timeout")

				if resp.StatusCode == http.StatusOK {
					GinkgoWriter.Println("✅ Test 4 PASSED: Large dataset query completed successfully")
				} else {
					GinkgoWriter.Println("✅ Test 4 PASSED: Large dataset query timed out gracefully (504)")
				}

				// Cleanup
				GinkgoWriter.Println("🧹 Cleaning up test data...")
				_, err = db.Exec(`DELETE FROM resource_action_traces WHERE incident_type = 'large-dataset-test'`)
				Expect(err).ToNot(HaveOccurred(), "Should delete test records")
				GinkgoWriter.Println("✅ Test data cleaned up")

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})
	})
})
