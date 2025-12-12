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

package datastorage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	auditpkg "github.com/jordigilh/kubernaut/pkg/audit"
)

// ========================================
// GAP 3.1: CONNECTION POOL EXHAUSTION TEST
// ========================================
//
// Business Requirement: BR-STORAGE-027 (Performance under load)
// Gap Analysis: TRIAGE_DS_TEST_COVERAGE_GAP_ANALYSIS_V3.md - Gap 3.1
// Priority: P0
// Estimated Effort: 1.5 hours
// Confidence: 93%
//
// BUSINESS OUTCOME:
// DS handles connection pool exhaustion gracefully (no HTTP 503 rejections)
//
// MISSING SCENARIO:
// - Config: max_open_conns=25
// - Burst: 50 concurrent writes
// - Expected: First 25 acquire immediately, remaining 25 queue (not rejected)
// - All 50 complete within timeout (30s)
// - Metric: datastorage_db_connection_wait_time_seconds tracks queueing
//
// TDD RED PHASE: Tests define contract, implementation will follow
// ========================================

var _ = Describe("GAP 3.1: Connection Pool Exhaustion", Label("gap-3.1", "p0"), Serial, func() {

	Describe("Burst Traffic Handling", func() {
		Context("when 50 concurrent writes exceed max_open_conns (25)", func() {
			It("should queue requests gracefully without rejecting (HTTP 503)", func() {
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				GinkgoWriter.Println("GAP 3.1: Testing connection pool exhaustion under burst load")
				GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

				// ARRANGE: Config max_open_conns=25 (from config file)
				// ARRANGE: Create 50 concurrent audit write requests
				concurrentRequests := 50
				maxOpenConns := 25

				var wg sync.WaitGroup
				results := make([]struct {
					statusCode int
					duration   time.Duration
					err        error
				}, concurrentRequests)

				testID := generateTestID()
				startTime := time.Now()

				GinkgoWriter.Printf("🚀 Starting %d concurrent audit writes (pool size: %d)...\n",
					concurrentRequests, maxOpenConns)

				// ACT: Fire 50 concurrent POST requests
				for i := 0; i < concurrentRequests; i++ {
					wg.Add(1)
					go func(index int) {
						defer wg.Done()
						defer GinkgoRecover()

						requestStart := time.Now()

						// Create unique audit event
						auditEvent := &auditpkg.AuditEvent{
							EventID:        generateTestUUID(),
							EventVersion:   "1.0",
							EventTimestamp: time.Now().UTC(),
							EventType:      "workflow.completed",
							EventCategory:  "workflow",
							EventAction:    "completed",
							EventOutcome:   "success",
							ActorType:      "service",
							ActorID:        "workflow-service",
							ResourceType:   "Workflow",
							ResourceID:     fmt.Sprintf("wf-pool-test-%s-%d", testID, index),
							CorrelationID:  fmt.Sprintf("remediation-pool-test-%s-%d", testID, index),
							EventData:      []byte(fmt.Sprintf(`{"pool_test":true,"index":%d}`, index)),
						}

						// Marshal to JSON
						payloadBytes, err := json.Marshal(auditEvent)
						if err != nil {
							results[index].err = err
							return
						}

						// POST to audit events endpoint
						resp, err := http.Post(
							datastorageURL+"/api/v1/audit-events",
							"application/json",
							bytes.NewReader(payloadBytes),
						)

						results[index].duration = time.Since(requestStart)
						results[index].err = err

						if err == nil {
							results[index].statusCode = resp.StatusCode
							resp.Body.Close()
						}
					}(i)
				}

				// Wait for all requests to complete
				wg.Wait()
				totalDuration := time.Since(startTime)

				GinkgoWriter.Printf("✅ All %d requests completed in %v\n", concurrentRequests, totalDuration)

				// ASSERT: NO HTTP 503 Service Unavailable errors
				successCount := 0
				failureCount := 0
				rejectedCount := 0 // HTTP 503

				for i, result := range results {
					Expect(result.err).ToNot(HaveOccurred(),
						fmt.Sprintf("Request %d should not have HTTP error", i))

					// BUSINESS VALUE: NO rejections (503) - all requests queued successfully
					Expect(result.statusCode).To(SatisfyAny(
						Equal(http.StatusCreated),  // 201 - Direct write succeeded
						Equal(http.StatusAccepted), // 202 - DLQ fallback
					), fmt.Sprintf("Request %d should not be rejected with 503", i))

					if result.statusCode == http.StatusCreated {
						successCount++
					} else if result.statusCode == http.StatusAccepted {
						// Acceptable - DLQ fallback if DB temporarily slow
						successCount++
					} else if result.statusCode == http.StatusServiceUnavailable {
						rejectedCount++
					} else {
						failureCount++
					}
				}

				GinkgoWriter.Printf("📊 Results: Success=%d, Rejected(503)=%d, Other Failures=%d\n",
					successCount, rejectedCount, failureCount)

				// ASSERT: All requests accepted (success or queued)
				Expect(successCount).To(Equal(concurrentRequests),
					"All requests should be accepted (either 201 Created or 202 Accepted)")

				Expect(rejectedCount).To(Equal(0),
					"NO requests should be rejected with HTTP 503 - connection pool should queue, not reject")

				// ASSERT: Reasonable throughput (all complete within 30s)
				Expect(totalDuration).To(BeNumerically("<", 30*time.Second),
					"All 50 requests should complete within 30s timeout")

				// BUSINESS OUTCOME: Graceful degradation
				// - First 25 connections: Acquire immediately from pool
				// - Next 25 connections: Queue and wait for available connection
				// - Result: ALL requests succeed, NONE rejected
				// - Better to queue (slower) than reject (data loss)

				// Calculate average request duration
				var totalRequestDuration time.Duration
				for _, result := range results {
					totalRequestDuration += result.duration
				}
				avgDuration := totalRequestDuration / time.Duration(concurrentRequests)

				GinkgoWriter.Printf("⏱️  Average request duration: %v\n", avgDuration)
				GinkgoWriter.Printf("⏱️  Total burst duration: %v\n", totalDuration)

				// TODO: When metrics implemented, verify:
				// datastorage_db_connection_wait_time_seconds histogram
				// Shows queueing for requests 26-50
			})
		})

		Context("when connection pool at capacity", func() {
			It("should expose metrics showing connection pool usage", func() {
				// ARRANGE: Get current metrics baseline
				// ACT: Generate load to fill connection pool
				// ASSERT: Metrics show pool utilization

				// TODO: Implement when Prometheus metrics endpoint available
				// GET /metrics
				// Verify metrics:
				// - datastorage_db_connections_open (current)
				// - datastorage_db_connections_in_use (active)
				// - datastorage_db_connections_idle (available)
				// - datastorage_db_connection_wait_duration_seconds (histogram)
				// - datastorage_db_max_open_connections (configured max)

				GinkgoWriter.Println("⏳ PENDING: Metrics implementation for connection pool monitoring")
				Skip("Metrics endpoint not yet implemented - will implement in TDD GREEN phase")
			})
		})
	})

	Describe("Connection Pool Recovery", func() {
		It("should recover gracefully after burst subsides", func() {
			// BUSINESS SCENARIO: Burst traffic → pool exhausted → burst ends → pool recovers

			// ARRANGE: Create burst (50 requests)
			GinkgoWriter.Println("🚀 Creating burst traffic...")
			var wg sync.WaitGroup
			testID := generateTestID()

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					auditEvent := &auditpkg.AuditEvent{
						EventID:        generateTestUUID(),
						EventVersion:   "1.0",
						EventTimestamp: time.Now().UTC(),
						EventType:      "workflow.completed",
						EventCategory:  "workflow",
						EventAction:    "completed",
						EventOutcome:   "success",
						ActorType:      "service",
						ActorID:        "workflow-service",
						ResourceType:   "Workflow",
						ResourceID:     fmt.Sprintf("wf-recovery-%s-%d", testID, index),
						CorrelationID:  fmt.Sprintf("remediation-recovery-%s-%d", testID, index),
						EventData:      []byte(`{"recovery_test":true}`),
					}

					payloadBytes, _ := json.Marshal(auditEvent)
					resp, err := http.Post(
						datastorageURL+"/api/v1/audit-events",
						"application/json",
						bytes.NewReader(payloadBytes),
					)
					if err == nil {
						resp.Body.Close()
					}
				}(i)
			}

			wg.Wait()
			GinkgoWriter.Println("✅ Burst completed")

			// ACT: Wait for connections to be released
			time.Sleep(2 * time.Second)

			// ACT: Send normal request after burst
			GinkgoWriter.Println("🔍 Sending normal request after burst...")
			normalEvent := &auditpkg.AuditEvent{
				EventID:        generateTestUUID(),
				EventVersion:   "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "workflow.completed",
				EventCategory:  "workflow",
				EventAction:    "completed",
				EventOutcome:   "success",
				ActorType:      "service",
				ActorID:        "workflow-service",
				ResourceType:   "Workflow",
				ResourceID:     fmt.Sprintf("wf-normal-%s", testID),
				CorrelationID:  fmt.Sprintf("remediation-normal-%s", testID),
				EventData:      []byte(`{"normal_after_burst":true}`),
			}

			payloadBytes, err := json.Marshal(normalEvent)
			Expect(err).ToNot(HaveOccurred())

			normalStart := time.Now()
			resp, err := http.Post(
				datastorageURL+"/api/v1/audit-events",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			normalDuration := time.Since(normalStart)

			// ASSERT: Normal request succeeds quickly after burst
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusCreated),
				Equal(http.StatusAccepted),
			))

			// ASSERT: Response time back to normal (<1s, not queued)
			Expect(normalDuration).To(BeNumerically("<", 1*time.Second),
				"Connection pool should recover - normal request should be fast")

			GinkgoWriter.Printf("✅ Pool recovered - normal request: %v\n", normalDuration)

			// BUSINESS VALUE: Connection pool is resilient
			// - Handles burst traffic gracefully (queues requests)
			// - Recovers quickly after burst subsides
			// - Normal operations resume without service restart
		})
	})
})
