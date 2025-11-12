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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E Performance & Boundary Conditions Tests
// Day 12.5 - Phase 3: Performance & Boundary Conditions (P1-P2)
//
// These tests validate Context API behavior under performance stress and boundary conditions:
// - Large dataset aggregation (10,000+ records)
// - Concurrent request handling (50 simultaneous requests)
// - Multi-dimensional aggregation E2E flow
//
// Related: BR-CONTEXT-010 (Graceful degradation), BR-STORAGE-031-05 (Multi-dimensional aggregation)

var _ = Describe("E2E Performance & Boundary Conditions", Ordered, Label("e2e", "performance"), func() {
	Context("Phase 3: Performance & Boundary Conditions (P1-P2)", func() {
		Describe("Test 8: Large Dataset Aggregation", func() {
			It("should handle large dataset aggregation within 10s", func() {
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 8: Large Dataset Aggregation")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// BEHAVIOR: Large dataset → Context API returns within 10s
				// CORRECTNESS: Aggregation is accurate despite large dataset

				// Insert 10,000 action traces
				GinkgoWriter.Println("📝 Inserting 10,000 test records...")
				insertStart := time.Now()

				// First, ensure parent records exist
				_, err := db.Exec(`
					INSERT INTO resource_references (id, kind, resource_uid, api_version, name, namespace)
					VALUES (9999, 'Pod', 'large-dataset-pod-uid', 'v1', 'large-dataset-pod', 'default')
					ON CONFLICT (id) DO NOTHING
				`)
				Expect(err).ToNot(HaveOccurred(), "Failed to insert resource reference")

				_, err = db.Exec(`
					INSERT INTO action_histories (id, resource_id)
					VALUES (9999, 9999)
					ON CONFLICT (id) DO NOTHING
				`)
				Expect(err).ToNot(HaveOccurred(), "Failed to insert action history")

				// Insert 10,000 traces
				for i := 0; i < 10000; i++ {
					executionStatus := "completed"
					if i%2 == 1 {
						executionStatus = "failed"
					}

					_, err := db.Exec(`
						INSERT INTO resource_action_traces (
							action_history_id, action_id, action_type, execution_status,
							incident_type, playbook_id, playbook_version,
							signal_name, signal_severity, signal_fingerprint,
							model_used, model_confidence,
							playbook_step_number, playbook_execution_id,
							ai_selected_playbook, ai_chained_playbooks, ai_manual_escalation,
							action_timestamp
						) VALUES (
							9999, gen_random_uuid()::text, 'restart-pod', $1,
							'large-dataset', 'playbook-v1', '1.0.0',
							'test-signal', 'warning', 'large-dataset-fingerprint',
							'gpt-4', 0.95,
							1, gen_random_uuid()::text,
							true, false, false,
							NOW()
						)
					`, executionStatus)

					Expect(err).ToNot(HaveOccurred(), "Failed to insert action trace")

					if (i+1)%1000 == 0 {
						GinkgoWriter.Printf("   Inserted %d/%d records...\n", i+1, 10000)
					}
				}

				insertDuration := time.Since(insertStart)
				GinkgoWriter.Printf("✅ Test data inserted in %v\n", insertDuration)

				// Query aggregation
				GinkgoWriter.Println("📡 Making aggregation request...")
				start := time.Now()
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=large-dataset&time_range=24h&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				duration := time.Since(start)

				Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
				defer resp.Body.Close()

				GinkgoWriter.Printf("⏱️  Request duration: %v\n", duration)
				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Completes within 10s
				Expect(duration).To(BeNumerically("<", 10*time.Second),
					"Large dataset aggregation should complete within 10s")

				// CORRECTNESS: Response is valid
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Large dataset aggregation should return 200 OK")

				var result SuccessRateResponse
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

				GinkgoWriter.Printf("📋 Success Rate: %.2f%% (%d/%d executions)\n",
					result.SuccessRate, result.SuccessfulExecutions, result.TotalExecutions)

				Expect(result.TotalExecutions).To(Equal(10000),
					"Should aggregate all 10,000 records")
				Expect(result.SuccessRate).To(BeNumerically("~", 50.0, 1.0),
					"Success rate should be ~50% (5000 completed / 10000 total)")

				// Cleanup
				GinkgoWriter.Println("🧹 Cleaning up test data...")
				_, err = db.Exec(`DELETE FROM resource_action_traces WHERE incident_type = 'large-dataset'`)
				Expect(err).ToNot(HaveOccurred(), "Cleanup should succeed")

				GinkgoWriter.Println("✅ Test 8 PASSED: Large dataset handled efficiently")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 9: Concurrent Requests (Load Test)", func() {
			It("should handle 50 concurrent requests without errors", func() {
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 9: Concurrent Requests (Load Test)")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// BEHAVIOR: 50 concurrent requests → all succeed
				// CORRECTNESS: No race conditions, all responses valid

				var wg sync.WaitGroup
				results := make(chan error, 50)
				durations := make(chan time.Duration, 50)

				GinkgoWriter.Println("📡 Sending 50 concurrent requests...")
				start := time.Now()

				for i := 0; i < 50; i++ {
					wg.Add(1)
					go func(requestID int) {
						defer wg.Done()

						reqStart := time.Now()
						url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/incident-type?incident_type=pod-oom&time_range=7d&min_samples=1", contextAPIBaseURL)
						resp, err := http.Get(url)
						reqDuration := time.Since(reqStart)
						durations <- reqDuration

						if err != nil {
							results <- fmt.Errorf("request %d failed: %w", requestID, err)
							return
						}
						defer resp.Body.Close()

						if resp.StatusCode != http.StatusOK {
							results <- fmt.Errorf("request %d returned status %d", requestID, resp.StatusCode)
							return
						}

						var result SuccessRateResponse
						err = json.NewDecoder(resp.Body).Decode(&result)
						if err != nil {
							results <- fmt.Errorf("request %d failed to decode: %w", requestID, err)
							return
						}

						results <- nil
					}(i)
				}

				wg.Wait()
				totalDuration := time.Since(start)
				close(results)
				close(durations)

				GinkgoWriter.Printf("⏱️  Total duration: %v\n", totalDuration)

				// Calculate average request duration
				var totalReqDuration time.Duration
				var maxDuration time.Duration
				var minDuration time.Duration = time.Hour
				count := 0

				for d := range durations {
					totalReqDuration += d
					count++
					if d > maxDuration {
						maxDuration = d
					}
					if d < minDuration {
						minDuration = d
					}
				}

				avgDuration := totalReqDuration / time.Duration(count)
				GinkgoWriter.Printf("📊 Request durations:\n")
				GinkgoWriter.Printf("   Average: %v\n", avgDuration)
				GinkgoWriter.Printf("   Min:     %v\n", minDuration)
				GinkgoWriter.Printf("   Max:     %v\n", maxDuration)

				// CORRECTNESS: All requests succeed
				errorCount := 0
				for err := range results {
					if err != nil {
						errorCount++
						GinkgoWriter.Printf("❌ Error: %v\n", err)
					}
				}

				Expect(errorCount).To(Equal(0),
					"All 50 concurrent requests should succeed")

				GinkgoWriter.Printf("✅ Concurrent load test: 50/50 requests succeeded (0 errors)\n")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})

		Describe("Test 10: Multi-Dimensional Aggregation E2E", func() {
			It("should complete multi-dimensional aggregation flow", func() {
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("🧪 Test 10: Multi-Dimensional Aggregation E2E")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// BEHAVIOR: Query with incident_type + playbook_id
				// CORRECTNESS: Returns accurate multi-dimensional aggregation

				GinkgoWriter.Println("📡 Making multi-dimensional aggregation request...")
				url := fmt.Sprintf("%s/api/v1/aggregation/success-rate/multi-dimensional?incident_type=pod-oom&playbook_id=playbook-restart-v1&time_range=7d&min_samples=1", contextAPIBaseURL)
				resp, err := http.Get(url)
				Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
				defer resp.Body.Close()

				GinkgoWriter.Printf("📊 Response Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

				// BEHAVIOR: Returns 200 OK
				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Multi-dimensional aggregation should succeed")

				// CORRECTNESS: Response structure is valid
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), "Response should be valid JSON")

				GinkgoWriter.Printf("📋 Response keys: %v\n", getMapKeys(result))

				// Validate multi-dimensional response structure
				Expect(result["dimensions"]).ToNot(BeNil(),
					"Response should include dimensions")
				Expect(result["success_rate"]).ToNot(BeNil(),
					"Response should include success_rate")
				Expect(result["total_executions"]).ToNot(BeNil(),
					"Response should include total_executions")

				// Validate dimensions structure
				queryDims, ok := result["dimensions"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "dimensions should be an object")

				GinkgoWriter.Printf("📋 Query dimensions: %v\n", queryDims)

				Expect(queryDims["incident_type"]).To(Equal("pod-oom"),
					"Query dimensions should include incident_type")
				Expect(queryDims["playbook_id"]).To(Equal("playbook-restart-v1"),
					"Query dimensions should include playbook_id")

				successRate, ok := result["success_rate"].(float64)
				Expect(ok).To(BeTrue(), "success_rate should be a number")

				totalExecs, ok := result["total_executions"].(float64)
				Expect(ok).To(BeTrue(), "total_executions should be a number")

				GinkgoWriter.Printf("✅ Multi-dimensional aggregation: %.2f%% success rate (%d executions)\n",
					successRate, int(totalExecs))

				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			})
		})
	})
})

// Helper function to get map keys
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
