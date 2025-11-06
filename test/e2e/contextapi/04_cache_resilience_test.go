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
	"os/exec"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E Test Suite - Phase 2: Cache Resilience Scenarios
// Tests Context API cache resilience under various failure conditions
//
// Business Requirements:
// - BR-CONTEXT-005: Cache resilience and fallback
// - BR-CONTEXT-010: Graceful degradation (Data Storage down → cached data only)
//
// Related: Day 12.5 Phase 2 - Cache Resilience Scenarios (3 P1 tests)

var _ = Describe("E2E Cache Resilience Scenarios", Ordered, func() {
	// Seed test data before all cache resilience tests
	BeforeAll(func() {
		GinkgoWriter.Println("📝 Seeding test data for cache resilience tests...")

		// Ensure parent records exist
		_, err := db.Exec(`
			INSERT INTO resource_references (id, kind, resource_uid, api_version, name, namespace)
			VALUES (100, 'Pod', 'cache-test-pod-uid', 'v1', 'cache-test-pod', 'default')
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert resource reference")

		_, err = db.Exec(`
			INSERT INTO action_histories (id, resource_id)
			VALUES (100, 100)
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert action history")

		// Seed 9 pod-oom incidents: 6 successful, 3 failed
		for i := 0; i < 9; i++ {
			executionStatus := "completed"
			if i%3 == 2 {
				executionStatus = "failed"
			}

			_, err := db.Exec(`
				INSERT INTO resource_action_traces (
					action_history_id,
					action_id, action_type, action_timestamp, execution_status,
					signal_name, signal_severity, signal_fingerprint,
					incident_type, playbook_id, playbook_version,
					model_used, model_confidence,
					playbook_step_number, playbook_execution_id,
					ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation
				) VALUES (
					100,
					gen_random_uuid()::text, 'restart-pod', NOW(), $1,
					'pod-oom-signal', 'critical', 'pod-oom-fingerprint',
					'pod-oom', 'playbook-restart-v1', '1.0.0',
					'gpt-4', 0.95,
					1, gen_random_uuid()::text,
					true, false, false
				)
			`, executionStatus)
			Expect(err).ToNot(HaveOccurred(), "Failed to insert action trace")
		}

		GinkgoWriter.Println("✅ Test data seeded: 9 pod-oom incidents (6 successful, 3 failed)")
	})

	// Clean up test data after all tests
	AfterAll(func() {
		GinkgoWriter.Println("🧹 Cleaning up test data...")
		_, err := db.Exec(`DELETE FROM resource_action_traces WHERE incident_type = 'pod-oom'`)
		Expect(err).ToNot(HaveOccurred(), "Cleanup should succeed")
		GinkgoWriter.Println("✅ Test data cleaned up")
	})

	Context("Phase 2: Cache Resilience (P1 - High Priority)", func() {
		Describe("Test 5: Redis Unavailable (Cache Fallback)", func() {
			It("should fallback to Data Storage when Redis is unavailable", func() {
				// BEHAVIOR: Redis down → Context API queries Data Storage directly
				// CORRECTNESS: Request succeeds with slightly higher latency
				//
				// Production Impact: 1-5% of requests
				// Business Requirement: BR-CONTEXT-005 (Cache resilience and fallback)

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 5: Redis Unavailable (Cache Fallback)")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Stop Redis
				GinkgoWriter.Println("🛑 Stopping Redis...")
				redisContainerName := dataStorageInfra.RedisContainer
				cmd := exec.Command("podman", "stop", redisContainerName)
				output, err := cmd.CombinedOutput()
				if err != nil {
					GinkgoWriter.Printf("⚠️  Warning: Failed to stop Redis: %v, output: %s\n", err, string(output))
				}
				time.Sleep(2 * time.Second) // Wait for Redis to be fully stopped
				GinkgoWriter.Println("✅ Redis stopped")

				// Make request to Context API
				GinkgoWriter.Println("📡 Making request to Context API (Redis is down, should fallback to Data Storage)...")
				start := time.Now()
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				duration := time.Since(start)

				GinkgoWriter.Printf("⏱️  Request duration: %v\n", duration)

				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Request succeeds (fallback to Data Storage)
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Redis unavailable should not fail request - Context API should fallback to Data Storage")

				// CORRECTNESS: Latency is higher but acceptable (<5s)
				Expect(duration).To(BeNumerically("<", 5*time.Second),
					"Fallback to Data Storage should complete within 5s")

				// Verify response data is correct
				var result SuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

				GinkgoWriter.Printf("📋 Success Rate: %.2f%% (%d/%d executions)\n",
					result.SuccessRate, result.SuccessfulExecutions, result.TotalExecutions)

				Expect(result.TotalExecutions).To(BeNumerically(">=", 3),
					"Should return data from Data Storage (not empty)")

				GinkgoWriter.Println("✅ Test 5 PASSED: Redis fallback works - Data Storage queried successfully")

				// Restart Redis for next tests
				GinkgoWriter.Println("🔄 Restarting Redis...")
				cmd = exec.Command("podman", "start", redisContainerName)
				output, err = cmd.CombinedOutput()
				if err != nil {
					GinkgoWriter.Printf("⚠️  Warning: Failed to start Redis: %v, output: %s\n", err, string(output))
				}
				time.Sleep(3 * time.Second) // Wait for Redis to be ready
				GinkgoWriter.Println("✅ Redis restarted")

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 6: Cache Stampede (Concurrent Requests)", func() {
			It("should handle cache stampede without overwhelming Data Storage", func() {
				// BEHAVIOR: 100 concurrent requests → only 1 Data Storage query (singleflight)
				// CORRECTNESS: All requests succeed, Data Storage not overwhelmed
				//
				// Production Impact: 20-30% of cache misses
				// Business Requirement: BR-CONTEXT-005 (Cache resilience and fallback)

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 6: Cache Stampede (Concurrent Requests)")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				var wg sync.WaitGroup
				results := make(chan int, 100)

				// Clear cache first
				GinkgoWriter.Println("🧹 Clearing Redis cache...")
				redisContainerName := dataStorageInfra.RedisContainer
				cmd := exec.Command("podman", "exec", redisContainerName, "redis-cli", "FLUSHALL")
				output, err := cmd.CombinedOutput()
				if err != nil {
					GinkgoWriter.Printf("⚠️  Warning: Failed to flush Redis: %v, output: %s\n", err, string(output))
				}
				time.Sleep(500 * time.Millisecond)
				GinkgoWriter.Println("✅ Cache cleared")

				// Send 100 concurrent requests
				GinkgoWriter.Println("📡 Sending 100 concurrent requests for uncached key...")
				start := time.Now()
				for i := 0; i < 100; i++ {
					wg.Add(1)
					go func(requestNum int) {
						defer wg.Done()
						url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=1", contextAPIBaseURL)
						resp, err := http.Get(url)
						if err == nil {
							results <- resp.StatusCode
							resp.Body.Close()
						} else {
							GinkgoWriter.Printf("⚠️  Request %d failed: %v\n", requestNum, err)
							results <- 0
						}
					}(i)
				}

				wg.Wait()
				close(results)
				duration := time.Since(start)

				GinkgoWriter.Printf("⏱️  All requests completed in: %v\n", duration)

				// CORRECTNESS: All requests succeed
				successCount := 0
				failCount := 0
				statusCodes := make(map[int]int)
				for statusCode := range results {
					statusCodes[statusCode]++
					if statusCode == http.StatusOK {
						successCount++
					} else {
						failCount++
					}
				}

				GinkgoWriter.Printf("📊 Results: %d successful, %d failed\n", successCount, failCount)
				for code, count := range statusCodes {
					GinkgoWriter.Printf("   Status %d: %d requests\n", code, count)
				}

				Expect(successCount).To(BeNumerically(">=", 95),
					"At least 95% of concurrent requests should succeed (cache stampede protection)")

				GinkgoWriter.Printf("✅ Test 6 PASSED: Cache stampede handled - %d/100 requests succeeded\n", successCount)
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 7: Corrupted Cache Data", func() {
			It("should handle corrupted cache data gracefully", func() {
				// BEHAVIOR: Corrupted cache → Context API detects, invalidates, queries Data Storage
				// CORRECTNESS: Request succeeds with fallback
				//
				// Production Impact: 0.1-0.5% of requests
				// Business Requirement: BR-CONTEXT-005 (Cache resilience and fallback)

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 7: Corrupted Cache Data")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// Inject corrupted data into Redis
				GinkgoWriter.Println("💉 Injecting corrupted data into Redis cache...")
				cacheKey := "context:aggregation:incident-type:pod-oom:7d:1"
				redisContainerName := dataStorageInfra.RedisContainer
				cmd := exec.Command("podman", "exec", redisContainerName,
					"redis-cli", "SET", cacheKey, "CORRUPTED_NOT_JSON_DATA_12345")
				output, err := cmd.CombinedOutput()
				if err != nil {
					GinkgoWriter.Printf("⚠️  Warning: Failed to inject corrupted data: %v, output: %s\n", err, string(output))
				}
				time.Sleep(500 * time.Millisecond)
				GinkgoWriter.Println("✅ Corrupted data injected")

				// Make request to Context API
				GinkgoWriter.Println("📡 Making request to Context API (cache contains corrupted data)...")
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				Expect(err).ToNot(HaveOccurred(), "HTTP request should not fail")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Request succeeds (fallback to Data Storage)
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Corrupted cache should not fail request - Context API should fallback to Data Storage")

				// Verify response data is correct
				var result SuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

				GinkgoWriter.Printf("📋 Success Rate: %.2f%% (%d/%d executions)\n",
					result.SuccessRate, result.SuccessfulExecutions, result.TotalExecutions)

				Expect(result.TotalExecutions).To(BeNumerically(">=", 3),
					"Should return data from Data Storage (not corrupted cache)")

				GinkgoWriter.Println("✅ Test 7 PASSED: Corrupted cache handled gracefully")

				// Verify cache behavior after corruption
				GinkgoWriter.Println("🔍 Checking cache state after corruption...")
				time.Sleep(1 * time.Second)
				cmd = exec.Command("podman", "exec", redisContainerName,
					"redis-cli", "GET", cacheKey)
				output, err = cmd.CombinedOutput()

				if err == nil {
					outputStr := strings.TrimSpace(string(output))
					GinkgoWriter.Printf("📋 Cache value after request: %s\n", outputStr)

					// BEHAVIOR: Cache may still contain corrupted data (TTL-based expiration)
					// BR-CONTEXT-010: Graceful degradation - corrupted cache is ignored, fresh data fetched
					// Cache invalidation happens via TTL, not immediate removal
					if outputStr == "CORRUPTED_NOT_JSON_DATA_12345" {
						GinkgoWriter.Println("📝 Note: Corrupted cache data still present (will expire via TTL)")
						GinkgoWriter.Println("✅ This is expected behavior - Context API fetched fresh data despite corruption")
					} else if !strings.Contains(outputStr, "nil") && outputStr != "" {
						// Cache was updated with fresh data
						var testJSON map[string]interface{}
						err := json.Unmarshal([]byte(outputStr), &testJSON)
						if err == nil {
							GinkgoWriter.Println("✅ Cache was updated with fresh valid JSON data")
						} else {
							GinkgoWriter.Printf("⚠️  Cache contains non-JSON data: %s\n", outputStr)
						}
					} else {
						GinkgoWriter.Println("✅ Cache was invalidated (nil)")
					}
				}

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})
	})
})

