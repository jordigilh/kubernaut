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
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// AUDIT CLIENT TIMING INTEGRATION TESTS
// 📋 Purpose: Reproduce audit buffer flush timing bug (RO team issue)
// Authority: DS_AUDIT_CLIENT_TEST_GAP_ANALYSIS_DEC_27_2025.md
// ========================================
//
// This file tests the FULL STACK audit path:
//   Service → audit.BufferedStore → HTTP Client → DataStorage API → PostgreSQL
//
// **Critical Difference from Other Tests**:
// - Other tests: Direct HTTP POST (bypass audit client)
// - These tests: Use audit.BufferedStore (production path)
//
// **Goal**: Reproduce the 50-90s delay bug reported by RO team
//
// Bug Report: DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md
// Gap Analysis: DS_AUDIT_CLIENT_TEST_GAP_ANALYSIS_DEC_27_2025.md
//
// ========================================

var _ = Describe("Audit Client Timing Integration Tests",  Label("audit-client", "timing"), func() {
	var (
		auditStore    audit.AuditStore
		dsClient      *dsgen.ClientWithResponses
		testCtx       context.Context
		testCancel    context.CancelFunc
		correlationID string
	)

	BeforeEach(func() {

		// Create test context
		testCtx, testCancel = context.WithTimeout(context.Background(), 2*time.Minute)

		// Ensure service is ready (simple HTTP health check)
		Eventually(func() bool {
			resp, err := http.Get(datastorageURL + "/health")
			if err != nil || resp == nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == 200
		}, "10s", "500ms").Should(BeTrue(), "Data Storage Service should be ready")

		// Create DataStorage client using audit.NewOpenAPIClientAdapter
		var err error
		httpClient, err := audit.NewOpenAPIClientAdapter(datastorageURL, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())

		// Create OpenAPI client for queries
		dsClient, err = dsgen.NewClientWithResponses(datastorageURL)
		Expect(err).ToNot(HaveOccurred())

		// Create audit client with PRODUCTION configuration
		// DD-AUDIT-004: Use small buffer for basic timing tests (not stress tests)
		auditConfig := audit.Config{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 1 * time.Second, // CRITICAL: This is what RO team uses
			MaxRetries:    3,
		}

		// Create buffered audit store with REAL HTTP client
		auditStore, err = audit.NewBufferedStore(httpClient, auditConfig, "test-service", logger)
		Expect(err).ToNot(HaveOccurred())

		// Generate unique correlation ID for test isolation
		correlationID = uuid.New().String()

		// Clean up test data
		_, _ = db.Exec("DELETE FROM audit_events WHERE correlation_id = $1", correlationID)
	})

	AfterEach(func() {
		if auditStore != nil {
			_ = auditStore.Close()
		}
		if testCancel != nil {
			testCancel()
		}

		// Clean up test data
		if correlationID != "" {
			_, _ = db.Exec("DELETE FROM audit_events WHERE correlation_id = $1", correlationID)
		}
	})

	Context("Flush Timing (RO Team Bug Reproduction)", func() {
		It("should flush event within configured interval (1 second)", func() {
			By("Creating audit event using REAL audit client")
			event := &dsgen.AuditEventRequest{
				Version:        "1.0",
				EventType:      "test.timing",
				EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
				EventAction:    "test_action",
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				EventTimestamp: time.Now(),
				CorrelationId:  correlationID,
				EventData:      map[string]interface{}{"test": "timing"},
			}

			By("Emitting event through audit.BufferedStore")
			start := time.Now()
			err := auditStore.StoreAudit(testCtx, event)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for event to become queryable in DataStorage")
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(testCtx, &dsgen.QueryAuditEventsParams{
					CorrelationId: &correlationID,
				})
				if err != nil {
					GinkgoWriter.Printf("Query error: %v\n", err)
					return 0
				}
				if resp.JSON200 == nil {
					GinkgoWriter.Printf("No JSON200 response (status: %d)\n", resp.StatusCode())
					return 0
				}
				if resp.JSON200.Data == nil {
					return 0
				}
				return len(*resp.JSON200.Data)
			}, "10s", "100ms").Should(Equal(1), "Event should become queryable")

			elapsed := time.Since(start)

			By("Verifying flush timing")
			GinkgoWriter.Printf("✅ Event became queryable in %v\n", elapsed)
			GinkgoWriter.Printf("   - Expected: < 3s (1s flush + margin)\n")
			GinkgoWriter.Printf("   - Actual: %v\n", elapsed)

			// CRITICAL TEST: RO team reports 50-90s delays
			Expect(elapsed).To(BeNumerically("<", 3*time.Second),
				"Event should be queryable within 3s (RO reports 50-90s bug)")
		})

		It("should flush buffered events on Close()", func() {
			By("Buffering 5 events")
			for i := 0; i < 5; i++ {
				event := &dsgen.AuditEventRequest{
					Version:        "1.0",
					EventType:      "test.shutdown",
					EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
					EventAction:    "test_shutdown",
					EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
					EventTimestamp: time.Now(),
					CorrelationId:  correlationID,
					EventData:      map[string]interface{}{"test": "shutdown", "index": i},
				}

				err := auditStore.StoreAudit(testCtx, event)
				Expect(err).ToNot(HaveOccurred())
			}

			By("Closing audit store (simulate graceful shutdown)")
			start := time.Now()
			err := auditStore.Close()
			closeTime := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("✅ Close() took %v\n", closeTime)

			// Close() should wait for flush
			Expect(closeTime).To(BeNumerically(">", 100*time.Millisecond),
				"Close() should wait for flush")

			By("Verifying all 5 events were flushed")
			var eventCount int
			err = db.Get(&eventCount, "SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(eventCount).To(Equal(5),
				"All buffered events should be flushed on Close()")
		})

		// ═══════════════════════════════════════════════════════════════
		// STRESS TEST: High Concurrency Load (Attempt to Reproduce RO Bug)
		// ═══════════════════════════════════════════════════════════════

		It("should maintain flush timing under high concurrent load (RO team scenario)", Label("stress"), func() {
			By("Creating 50 concurrent goroutines emitting events")
			numGoroutines := 50
			eventsPerGoroutine := 10
			totalEvents := numGoroutines * eventsPerGoroutine

			done := make(chan bool)
			startTime := time.Now()

			// Launch concurrent goroutines
			for i := 0; i < numGoroutines; i++ {
				go func(routineID int) {
					defer GinkgoRecover()
					for j := 0; j < eventsPerGoroutine; j++ {
						emitTime := time.Now()
						event := &dsgen.AuditEventRequest{
							Version:        "1.0",
							EventType:      "test.stress",
							EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
							EventAction:    "stress_test",
							EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
							EventTimestamp: emitTime,
							CorrelationId:  correlationID,
							EventData:      map[string]interface{}{"routine": routineID, "event": j},
						}

						err := auditStore.StoreAudit(testCtx, event)
						if err != nil {
							GinkgoWriter.Printf("⚠️  Event emission failed (routine %d, event %d): %v\n", routineID, j, err)
						}

						// Small delay to simulate realistic traffic (10ms = 100 events/sec per goroutine)
						time.Sleep(10 * time.Millisecond)
					}
					done <- true
				}(i)
			}

			// Wait for all emissions
			for i := 0; i < numGoroutines; i++ {
				<-done
			}
			emissionComplete := time.Since(startTime)
			GinkgoWriter.Printf("✅ Emitted %d events from %d goroutines in %v\n", totalEvents, numGoroutines, emissionComplete)

			By("Waiting for events to become queryable (allowing for buffer drops)")
			// Give time for async writes to complete
			time.Sleep(5 * time.Second)

		// Get final count
		resp, err := dsClient.QueryAuditEventsWithResponse(testCtx, &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.JSON200).ToNot(BeNil())

		// Use Total field from response (not paginated data length)
		finalEventCount := 0
		if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
			finalEventCount = *resp.JSON200.Pagination.Total
		}

		totalTime := time.Since(startTime)

		GinkgoWriter.Printf("✅ Final event count: %d/%d (%.1f%% success rate)\n",
			finalEventCount, totalEvents, float64(finalEventCount)/float64(totalEvents)*100)

			By("Analyzing timing statistics")

			// Calculate statistics
			var (
				maxDelay     time.Duration
				totalDelay   time.Duration
				delaysOver5s int
			)

			// Calculate delays from queryable events
			if resp.JSON200.Data != nil {
				for _, event := range *resp.JSON200.Data {
					emitTime := event.EventTimestamp
					// Use event_timestamp from DB as proxy for "queryable time"
					// (in reality, there's a small delay, but this approximates it)
					delay := time.Since(emitTime)
					if delay > maxDelay {
						maxDelay = delay
					}
					totalDelay += delay
					if delay > 5*time.Second {
						delaysOver5s++
					}
				}
			}

			var avgDelay time.Duration
			if finalEventCount > 0 {
				avgDelay = totalDelay / time.Duration(finalEventCount)
			}

			By("Reporting timing statistics")
			GinkgoWriter.Printf("\n📊 STRESS TEST TIMING STATISTICS:\n")
			GinkgoWriter.Printf("   - Total events: %d\n", totalEvents)
			GinkgoWriter.Printf("   - Concurrent goroutines: %d\n", numGoroutines)
			GinkgoWriter.Printf("   - Total time: %v\n", totalTime)
			GinkgoWriter.Printf("   - Emission time: %v\n", emissionComplete)
			GinkgoWriter.Printf("   - Average delay: %v\n", avgDelay)
			GinkgoWriter.Printf("   - Maximum delay: %v\n", maxDelay)
			GinkgoWriter.Printf("   - Events with >5s delay: %d (%.1f%%)\n", delaysOver5s, float64(delaysOver5s)/float64(totalEvents)*100)
			GinkgoWriter.Printf("\n")

			// CRITICAL: Check if we reproduced the RO team bug
			if maxDelay > 10*time.Second {
				GinkgoWriter.Printf("🚨 BUG REPRODUCED: Max delay %v exceeds 10s threshold!\n", maxDelay)
				GinkgoWriter.Printf("   This matches the RO team's report of 50-90s delays.\n")
			} else {
				GinkgoWriter.Printf("✅ No timing bug detected under this load pattern.\n")
				GinkgoWriter.Printf("   RO team bug may require different conditions.\n")
			}

			// Test expectations
			Expect(finalEventCount).To(BeNumerically(">=", totalEvents*8/10),
				"At least 80% of events should be delivered (buffer saturation expected under stress)")

			if finalEventCount > 0 {
				Expect(maxDelay).To(BeNumerically("<", 10*time.Second),
					"Maximum delay should be <10s (RO team reports 50-90s bug)")
				Expect(avgDelay).To(BeNumerically("<", 3*time.Second),
					"Average delay should be reasonable")
				Expect(delaysOver5s).To(BeNumerically("<", finalEventCount/10),
					"<10% of delivered events should have >5s delay")
			}
		})

		// ═══════════════════════════════════════════════════════════════
		// DD-AUDIT-004: Buffer Sizing Validation (Burst Traffic Handling)
		// ═══════════════════════════════════════════════════════════════

		It("should prevent event loss under burst traffic with DD-AUDIT-004 buffer sizing", Label("stress"), func() {
			By("Creating audit store with DD-AUDIT-004 MEDIUM tier buffer (30K)")
			gatewayConfig := audit.RecommendedConfig("gateway") // 30,000 buffer

			httpClient, err := audit.NewOpenAPIClientAdapter(datastorageURL, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			gatewayAuditStore, err := audit.NewBufferedStore(httpClient, gatewayConfig, "gateway", logger)
			Expect(err).ToNot(HaveOccurred())
			defer gatewayAuditStore.Close()

			By("Emitting 25,000 burst events (stress test scenario from DD-AUDIT-004)")
			numGoroutines := 50
			eventsPerGoroutine := 500 // 50 * 500 = 25,000 total
			totalEvents := numGoroutines * eventsPerGoroutine

			done := make(chan bool, numGoroutines)
			startTime := time.Now()
			failedEmissions := 0

			// Launch concurrent goroutines (burst pattern, no delays)
			for i := 0; i < numGoroutines; i++ {
				go func(routineID int) {
					defer GinkgoRecover()
					for j := 0; j < eventsPerGoroutine; j++ {
						event := &dsgen.AuditEventRequest{
							Version:        "1.0",
							EventType:      "test.dd_audit_004",
							EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
							EventAction:    "buffer_saturation_test",
							EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
							EventTimestamp: time.Now(),
							CorrelationId:  correlationID,
							EventData:      map[string]interface{}{"routine": routineID, "event": j},
						}

						err := gatewayAuditStore.StoreAudit(testCtx, event)
						if err != nil {
							failedEmissions++
							GinkgoWriter.Printf("⚠️  Event emission failed (routine %d, event %d): %v\n", routineID, j, err)
						}
					}
					done <- true
				}(i)
			}

			// Wait for all emissions
			for i := 0; i < numGoroutines; i++ {
				<-done
			}
			emissionTime := time.Since(startTime)

			GinkgoWriter.Printf("\n📊 DD-AUDIT-004 BUFFER SATURATION TEST:\n")
			GinkgoWriter.Printf("   - Total events: %d\n", totalEvents)
			GinkgoWriter.Printf("   - Buffer size: %d (DD-AUDIT-004 MEDIUM tier)\n", gatewayConfig.BufferSize)
			GinkgoWriter.Printf("   - Concurrent goroutines: %d\n", numGoroutines)
			GinkgoWriter.Printf("   - Emission time: %v\n", emissionTime)
			GinkgoWriter.Printf("   - Failed emissions: %d\n", failedEmissions)

			By("Waiting for all events to be written (allow flush cycles)")
			time.Sleep(5 * time.Second)

		By("Verifying event delivery rate")
		resp, err := dsClient.QueryAuditEventsWithResponse(testCtx, &dsgen.QueryAuditEventsParams{
			CorrelationId: &correlationID,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.JSON200).ToNot(BeNil())

		// Use Total field from response (not paginated data length)
		deliveredEvents := 0
		if resp.JSON200.Pagination != nil && resp.JSON200.Pagination.Total != nil {
			deliveredEvents = *resp.JSON200.Pagination.Total
		}

		successRate := float64(deliveredEvents) / float64(totalEvents) * 100
		droppedEvents := totalEvents - deliveredEvents
		dropRate := float64(droppedEvents) / float64(totalEvents) * 100

		paginatedCount := 0
		if resp.JSON200.Data != nil {
			paginatedCount = len(*resp.JSON200.Data)
		}

		GinkgoWriter.Printf("\n📊 DD-AUDIT-004 VALIDATION RESULTS:\n")
		GinkgoWriter.Printf("   ✅ Delivered: %d/%d (%.1f%%)\n", deliveredEvents, totalEvents, successRate)
		GinkgoWriter.Printf("   ❌ Dropped: %d/%d (%.1f%%)\n", droppedEvents, totalEvents, dropRate)
		GinkgoWriter.Printf("   📄 Paginated results: %d (API default limit: 50)\n", paginatedCount)
		GinkgoWriter.Printf("\n")

			// DD-AUDIT-004 SUCCESS CRITERIA:
			// - Previous failure: 90% loss with 20K buffer on 25K burst
			// - Expected: <1% loss with 30K buffer on 25K burst (ADR-032 target)
			Expect(successRate).To(BeNumerically(">=", 99.0),
				"DD-AUDIT-004: Should achieve ≥99% delivery (was 10% with old sizing)")

			Expect(dropRate).To(BeNumerically("<", 1.0),
				"DD-AUDIT-004: Should maintain <1% drop rate per ADR-032")

			if successRate >= 99.0 {
				GinkgoWriter.Printf("✅ DD-AUDIT-004 VALIDATION PASSED: Buffer sizing prevents event loss under burst traffic\n")
			}
		})

		It("should maintain flush timing with sustained burst traffic (extreme stress)", Label("stress", "slow"), func() {
			Skip("Enable manually for extreme stress testing")

			By("Creating sustained high-volume traffic for 30 seconds")
			duration := 30 * time.Second
			targetRatePerSec := 100 // 100 events/second
			numGoroutines := 10
			eventsPerGoroutine := (int(duration.Seconds()) * targetRatePerSec) / numGoroutines

			startTime := time.Now()
			done := make(chan bool)
			errorCount := 0
			successCount := 0

			for i := 0; i < numGoroutines; i++ {
				go func(routineID int) {
					defer GinkgoRecover()
					for j := 0; j < eventsPerGoroutine; j++ {
						event := &dsgen.AuditEventRequest{
							Version:        "1.0",
							EventType:      "test.burst",
							EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
							EventAction:    "burst_test",
							EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
							EventTimestamp: time.Now(),
							CorrelationId:  correlationID,
							EventData:      map[string]interface{}{"routine": routineID, "event": j},
						}

						err := auditStore.StoreAudit(testCtx, event)
						if err != nil {
							errorCount++
						} else {
							successCount++
						}

						// Throttle to achieve target rate
						time.Sleep(time.Duration(1000/targetRatePerSec) * time.Millisecond)
					}
					done <- true
				}(i)
			}

			// Wait for completion
			for i := 0; i < numGoroutines; i++ {
				<-done
			}

			totalTime := time.Since(startTime)
			expectedEvents := numGoroutines * eventsPerGoroutine

			GinkgoWriter.Printf("\n📊 EXTREME STRESS TEST RESULTS:\n")
			GinkgoWriter.Printf("   - Duration: %v\n", totalTime)
			GinkgoWriter.Printf("   - Expected events: %d\n", expectedEvents)
			GinkgoWriter.Printf("   - Success: %d\n", successCount)
			GinkgoWriter.Printf("   - Errors: %d\n", errorCount)
			GinkgoWriter.Printf("   - Success rate: %.1f%%\n", float64(successCount)/float64(expectedEvents)*100)

			// Verify events eventually become queryable
			Eventually(func() int {
				resp, err := dsClient.QueryAuditEventsWithResponse(testCtx, &dsgen.QueryAuditEventsParams{
					CorrelationId: &correlationID,
				})
				if err != nil || resp.JSON200 == nil || resp.JSON200.Data == nil {
					return 0
				}
				return len(*resp.JSON200.Data)
			}, "120s", "1s").Should(BeNumerically(">=", expectedEvents*9/10),
				"At least 90% of events should be queryable")
		})
	})
})
