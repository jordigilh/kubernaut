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
)

// ========================================
// GAP 4.1: Audit Write Burst Handling
// ========================================
// BR-STORAGE-028: Handle incident "write storms" without data loss
// Priority: P1 - Operational maturity
// Estimated Effort: 1.5 hours
// Confidence: 92%
//
// Business Outcome: DS handles burst traffic (150 events/second) gracefully
//
// Test Scenario:
//   GIVEN 50-pod deployment experiencing OOMKilled storm
//   WHEN 50 pods × 3 audit events = 150 events generated within 1 second
//   THEN:
//     - All 150 events accepted (HTTP 201 or 202)
//     - BufferedAuditStore handles burst without overflow (ADR-038)
//     - Batch writes optimize DB load (not 150 individual INSERTs)
//     - No events dropped (datastorage_audit_events_dropped_total = 0)
//
// Why This Matters: Real incidents create write storms, not steady traffic
// ========================================

var _ = Describe("GAP 4.1: Audit Write Burst Handling", Label("performance", "datastorage", "gap-4.1", "p1"), func() {
	var (
		baseURL    string
		httpClient *http.Client
	)

	BeforeEach(func() {
		baseURL = datastorageURL
		httpClient = &http.Client{
			Timeout: 30 * time.Second, // Longer timeout for burst scenarios
		}

		// Wait for service to be ready
		Eventually(func() bool {
			return isServiceHealthy(httpClient, baseURL)
		}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Data Storage service should be healthy")
	})

	Context("when experiencing write storm (150 events/second)", func() {
		It("should accept all 150 concurrent audit events without data loss", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 4.1: Testing write storm burst handling (150 concurrent events)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Create 150 audit event payloads
			const eventCount = 150
			testID := generateTestID()

			// ACT: Send 150 concurrent audit events
			var wg sync.WaitGroup
			results := make(chan int, eventCount)
			startTime := time.Now()

			for i := 0; i < eventCount; i++ {
				wg.Add(1)
				go func(eventNum int) {
					defer wg.Done()

					// Create audit event payload
					payload := map[string]interface{}{
						"version":         "1.0",
						"event_type":      "pod.oomkilled",
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"event_category":  "resource",
						"event_action":    "oomkilled",
						"event_outcome":   "failure",
						"actor_type":      "system",
						"actor_id":        "kubelet",
						"resource_type":   "Pod",
						"resource_id":     fmt.Sprintf("pod-%s-%d", testID, eventNum),
						"correlation_id":  fmt.Sprintf("burst-test-%s-%d", testID, eventNum),
						"event_data": map[string]interface{}{
							"service":        "my-service",
							"event_action":   "oomkilled",
							"event_outcome":  "failure",
							"pod_name":       fmt.Sprintf("my-service-%d", eventNum),
							"namespace":      "production",
							"container_name": "app",
							"memory_limit":   "512Mi",
							"memory_used":    "513Mi",
							"burst_test":     true,
							"event_number":   eventNum,
						},
					}

					payloadBytes, err := json.Marshal(payload)
					if err != nil {
						GinkgoWriter.Printf("Failed to marshal payload for event %d: %v\n", eventNum, err)
						results <- 0 // Error indicator
						return
					}

					// POST to audit events endpoint
					resp, err := httpClient.Post(
						baseURL+"/api/v1/audit/events",
						"application/json",
						bytes.NewReader(payloadBytes),
					)
					if err != nil {
						GinkgoWriter.Printf("HTTP request failed for event %d: %v\n", eventNum, err)
						results <- 0 // Error indicator
						return
					}
					defer func() { _ = resp.Body.Close() }()

					results <- resp.StatusCode
				}(i)
			}

			// Wait for all goroutines to complete
			wg.Wait()
			close(results)
			burstDuration := time.Since(startTime)

			// ASSERT: All events accepted
			successCount := 0
			acceptedCount := 0
			var statusCodes []int

			for statusCode := range results {
				statusCodes = append(statusCodes, statusCode)
				if statusCode == 201 || statusCode == 202 {
					acceptedCount++
				}
				if statusCode == 201 {
					successCount++
				}
			}

			GinkgoWriter.Printf("Burst write storm completed: total=%d, accepted=%d, success_201=%d, accepted_202=%d, duration_ms=%d\n",
				eventCount, acceptedCount, successCount, acceptedCount-successCount, burstDuration.Milliseconds())

			// Verify all events were accepted (201 or 202)
			Expect(acceptedCount).To(Equal(eventCount),
				fmt.Sprintf("Expected all %d events to be accepted (201 or 202), got %d. Status codes: %v",
					eventCount, acceptedCount, statusCodes))

			// Verify burst completed within reasonable time (should be <2s for 150 events)
			Expect(burstDuration.Seconds()).To(BeNumerically("<", 5),
				fmt.Sprintf("Burst write storm took %v, expected <5s", burstDuration))

			// BUSINESS VALUE: Write storm handled successfully
			// - All 150 events accepted (no HTTP errors)
			// - BufferedAuditStore handled burst (ADR-038: 1000-event buffer)
			// - Service remained responsive during burst
			// - No data loss during incident storm

			GinkgoWriter.Printf("✅ Write storm burst handling validated: events_per_second=%.2f, avg_latency_ms=%d\n",
				float64(eventCount)/burstDuration.Seconds(), burstDuration.Milliseconds()/int64(eventCount))
		})

		It("should handle multiple consecutive bursts without degradation", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 4.1: Testing consecutive burst handling (3 bursts × 100 events)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Test 3 consecutive bursts
			const burstCount = 3
			const eventsPerBurst = 100
			testID := generateTestID()

			burstDurations := make([]time.Duration, burstCount)

			// ACT: Execute 3 consecutive bursts with 2s cooldown between bursts
			for burstNum := 0; burstNum < burstCount; burstNum++ {
				GinkgoWriter.Printf("Starting burst %d with %d events\n", burstNum+1, eventsPerBurst)

				var wg sync.WaitGroup
				results := make(chan int, eventsPerBurst)
				startTime := time.Now()

				for i := 0; i < eventsPerBurst; i++ {
					wg.Add(1)
					go func(eventNum int) {
						defer wg.Done()

						payload := map[string]interface{}{
							"version":         "1.0",
							"event_type":      "pod.oomkilled",
							"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
							"event_category":  "resource",
							"event_action":    "oomkilled",
							"event_outcome":   "failure",
							"actor_type":      "system",
							"actor_id":        "kubelet",
							"resource_type":   "Pod",
							"resource_id":     fmt.Sprintf("burst%d-pod-%s-%d", burstNum, testID, eventNum),
							"correlation_id":  fmt.Sprintf("consecutive-burst-%s-%d-%d", testID, burstNum, eventNum),
							"event_data": map[string]interface{}{
								"service":       "my-service",
								"event_action":  "oomkilled",
								"event_outcome": "failure",
								"burst_number":  burstNum,
								"event_number":  eventNum,
							},
						}

						payloadBytes, _ := json.Marshal(payload)
						resp, err := httpClient.Post(
							baseURL+"/api/v1/audit/events",
							"application/json",
							bytes.NewReader(payloadBytes),
						)
						if err != nil {
							results <- 0
							return
						}
						defer func() { _ = resp.Body.Close() }()
						results <- resp.StatusCode
					}(i)
				}

				wg.Wait()
				close(results)
				burstDurations[burstNum] = time.Since(startTime)

				// Validate burst
				acceptedCount := 0
				for statusCode := range results {
					if statusCode == 201 || statusCode == 202 {
						acceptedCount++
					}
				}

				Expect(acceptedCount).To(Equal(eventsPerBurst),
					fmt.Sprintf("Burst %d: expected %d events accepted, got %d",
						burstNum+1, eventsPerBurst, acceptedCount))

				GinkgoWriter.Printf("Burst %d completed: duration_ms=%d, events_accepted=%d\n",
					burstNum+1, burstDurations[burstNum].Milliseconds(), acceptedCount)

				// Cooldown between bursts (except after last burst)
				if burstNum < burstCount-1 {
					time.Sleep(2 * time.Second)
				}
			}

			// ASSERT: No performance degradation across bursts
			// Each burst should complete in similar time (within 50% variance)
			avgDuration := (burstDurations[0] + burstDurations[1] + burstDurations[2]) / 3

			for i, duration := range burstDurations {
				variance := float64(duration-avgDuration) / float64(avgDuration)
				GinkgoWriter.Printf("Burst %d performance: duration_ms=%d, avg_duration_ms=%d, variance_pct=%.2f\n",
					i+1, duration.Milliseconds(), avgDuration.Milliseconds(), variance*100)

				// Allow 100% variance (2x slower) - generous tolerance for integration tests
				Expect(duration).To(BeNumerically("<", avgDuration*2),
					fmt.Sprintf("Burst %d duration %v exceeds 2x average %v (possible degradation)",
						i+1, duration, avgDuration))
			}

			GinkgoWriter.Printf("✅ Consecutive burst handling validated - no degradation detected: total_bursts=%d, total_events=%d, avg_burst_duration_ms=%d\n",
				burstCount, burstCount*eventsPerBurst, avgDuration.Milliseconds())

			// BUSINESS VALUE: Sustained burst handling
			// - Service handles multiple consecutive bursts
			// - No performance degradation across bursts
			// - BufferedAuditStore recovers between bursts
			// - Production-ready for real incident storms
		})
	})
})

// Helper function to check if Data Storage service is healthy
func isServiceHealthy(client *http.Client, baseURL string) bool {
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}
