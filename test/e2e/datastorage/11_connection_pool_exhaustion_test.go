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

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
//
// Parallel Execution: ✅ ENABLED
// - Each E2E process has isolated DataStorage service in unique namespace
// - Connection pool (max_open_conns=25) is per-service, not global
// - No shared resources that would require Serial execution

var _ = Describe("BR-DS-006: Connection Pool Efficiency - Handle Traffic Bursts Without Degradation", Label("e2e", "gap-3.1", "p0"), Ordered, func() {

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

				testID := fmt.Sprintf("test-pool-%s", uuid.New().String()[:8])
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

						// Create type-safe workflow execution payload
					workflowPayload := ogenclient.WorkflowExecutionAuditPayload{
						EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
						ExecutionName:   fmt.Sprintf("wf-pool-test-%s-%d", testID, index),
						WorkflowID:      "pool-exhaustion-test-workflow",
						WorkflowVersion: "v1.0.0",
						ContainerImage:  "registry.io/test/pool-workflow@sha256:abc123def",
						TargetResource:  "deployment/test-app",
						Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
					}

					// Marshal event_data using ogen's jx.Encoder
					var e jx.Encoder
					workflowPayload.Encode(&e)
					eventDataJSON := e.Bytes()

					// Create audit event payload as map for proper JSON serialization
					auditEvent := map[string]interface{}{
						"version":         "1.0",
						"event_type":      "workflowexecution.workflow.completed",
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"event_category":  "workflowexecution",
						"event_action":    "completed",
						"event_outcome":   "success",
						"actor_type":      "ServiceAccount",
						"actor_id":        "system:serviceaccount:workflowexecution:workflowexecution-sa",
						"resource_type":   "WorkflowExecution",
						"resource_id":     fmt.Sprintf("wf-pool-test-%s-%d", testID, index),
						"correlation_id":  fmt.Sprintf("remediation-pool-test-%s-%d", testID, index),
						"event_data":      json.RawMessage(eventDataJSON), // ✅ Required field added
					}

						// Marshal to JSON
						payloadBytes, err := json.Marshal(auditEvent)
						if err != nil {
							results[index].err = err
							return
						}

						// POST to audit events endpoint
						resp, err := http.Post(
							dataStorageURL+"/api/v1/audit/events",
							"application/json",
							bytes.NewReader(payloadBytes),
						)

						results[index].duration = time.Since(requestStart)
						results[index].err = err

						if err == nil {
							results[index].statusCode = resp.StatusCode
							_ = resp.Body.Close()
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

				// NOTE: Connection pool metrics deferred to V1.1 (data-driven decision)
				// See: docs/handoff/DS_V1.0_V1.1_ROADMAP.md for implementation plan
				// When implemented, verify: datastorage_db_connection_wait_time_seconds histogram
			})
		})
	})

	Describe("Connection Pool Recovery", func() {
		It("should recover gracefully after burst subsides", func() {
			// BUSINESS SCENARIO: Burst traffic → pool exhausted → burst ends → pool recovers

			// ARRANGE: Create burst (50 requests)
			GinkgoWriter.Println("🚀 Creating burst traffic...")
			var wg sync.WaitGroup
			testID := fmt.Sprintf("test-pool-%s", uuid.New().String()[:8])

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					auditEvent := map[string]interface{}{
						"version":         "1.0",
						"event_type":      "workflow.completed",
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"event_category":  "workflow",
						"event_action":    "completed",
						"event_outcome":   "success",
						"actor_type":      "service",
						"actor_id":        "workflow-service",
						"resource_type":   "Workflow",
						"resource_id":     fmt.Sprintf("wf-recovery-%s-%d", testID, index),
						"correlation_id":  fmt.Sprintf("remediation-recovery-%s-%d", testID, index),
						"event_data": map[string]interface{}{
							"recovery_test": true,
						},
					}

					payloadBytes, _ := json.Marshal(auditEvent)
					resp, err := http.Post(
						dataStorageURL+"/api/v1/audit/events",
						"application/json",
						bytes.NewReader(payloadBytes),
					)
					if err == nil {
						_ = resp.Body.Close()
					}
				}(i)
			}

			wg.Wait()
			GinkgoWriter.Println("✅ Burst completed")

			// ACT: Wait for connections to be released and service to recover
			// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep() for async operations
			GinkgoWriter.Println("🔍 Waiting for connection pool to recover...")
			var normalDuration time.Duration
			var normalResp *http.Response
			Eventually(func() bool {
				// ACT: Send normal request after burst
				normalEvent := map[string]interface{}{
					"version":         "1.0",
					"event_type":      "workflow.completed",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "workflow",
					"event_action":    "completed",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "workflow-service",
					"resource_type":   "Workflow",
					"resource_id":     fmt.Sprintf("wf-normal-%s", testID),
					"correlation_id":  fmt.Sprintf("remediation-normal-%s", testID),
					"event_data": map[string]interface{}{
						"normal_after_burst": true,
					},
				}

				payloadBytes, err := json.Marshal(normalEvent)
				if err != nil {
					return false
				}

				normalStart := time.Now()
				resp, err := http.Post(
					dataStorageURL+"/api/v1/audit/events",
					"application/json",
					bytes.NewReader(payloadBytes),
				)
				normalDuration = time.Since(normalStart)

				if err != nil || resp == nil {
					return false
				}
				defer func() { _ = resp.Body.Close() }()

				// Connection pool recovered when: 201/202 response AND fast (<1s)
				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return false
				}
				if normalDuration >= 1*time.Second {
					return false
				}

				normalResp = resp
				return true
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Connection pool MUST recover after burst - normal request should succeed quickly (<1s)")

			GinkgoWriter.Printf("✅ Connection pool recovered, normal request took %v\n", normalDuration)

			// ASSERT: Normal request succeeded quickly after burst
			Expect(normalResp).ToNot(BeNil(), "Normal request should have succeeded")
			Expect(normalResp.StatusCode).To(SatisfyAny(
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
