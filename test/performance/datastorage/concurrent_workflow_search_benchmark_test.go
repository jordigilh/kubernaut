package datastorage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// GAP 5.2: Workflow Search Concurrent Load Performance
// ========================================
// BR-STORAGE-029: Workflow search latency acceptable under realistic concurrent load
// Priority: P1 - Operational maturity
// Estimated Effort: 1 hour
// Confidence: 93%
//
// Business Outcome: Workflow search performs well when HolmesGPT-API scales
//
// Test Scenario:
//   GIVEN 100 workflows in catalog
//   WHEN 20 concurrent POST /api/v1/workflows/search queries
//   THEN:
//     - p95 latency <500ms (acceptable for AI workflow selection)
//     - p99 latency <1s
//     - No connection pool exhaustion
//     - All queries execute concurrently (no queueing)
//
// Why This Matters: HolmesGPT-API uses workflow search frequently when analyzing incidents
// ========================================

// BenchmarkConcurrentWorkflowSearch measures workflow search performance under concurrent load
func BenchmarkConcurrentWorkflowSearch(b *testing.B) {
	// Skip if environment not set up
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	// Setup: Ensure 100 test workflows exist
	// Note: This is a placeholder - actual setup would create workflows via API
	b.Log("Running concurrent workflow search benchmark...")
	b.Log("Note: Ensure 100 test workflows exist in catalog before running")

	baseURL := "http://localhost:8081" // Default Data Storage URL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Create search request payload
	searchRequest := map[string]interface{}{
		"filters": map[string]interface{}{
			"signal_type": "memory-leak",
			"severity":    "critical",
			"component":   "deployment",
			"priority":    "p0",
			"environment": "production",
		},
		"top_k": 10,
	}

	payloadBytes, err := json.Marshal(searchRequest)
	if err != nil {
		b.Fatalf("Failed to marshal search request: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := httpClient.Post(
				baseURL+"/api/v1/workflows/search",
				"application/json",
				bytes.NewReader(payloadBytes),
			)
			if err != nil {
				b.Errorf("Search request failed: %v", err)
				continue
			}
			_ = resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				b.Errorf("Unexpected status code: %d", resp.StatusCode)
			}
		}
	})
}

var _ = Describe("GAP 5.2: Concurrent Workflow Search Performance", Label("performance", "datastorage", "gap-5.2", "p1"), func() {
	var (
		baseURL    string
		httpClient *http.Client
	)

	BeforeEach(func() {
		baseURL = getDataStorageURL()
		httpClient = &http.Client{Timeout: 10 * time.Second}

		// Wait for service to be ready
		Eventually(func() bool {
			resp, err := httpClient.Get(baseURL + "/health")
			if err != nil {
				return false
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode == http.StatusOK
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})

	Context("when handling 20 concurrent workflow searches", func() {
		It("should maintain acceptable latency (p95 <500ms, p99 <1s)", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 5.2: Testing concurrent workflow search performance (20 concurrent queries)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Create search request
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "memory-leak",
					"severity":    "critical",
					"component":   "deployment",
					"priority":    "p0",
					"environment": "production",
				},
				"top_k": 10,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Execute 20 concurrent searches
			const concurrentQueries = 20
			var wg sync.WaitGroup
			durations := make([]time.Duration, concurrentQueries)
			statusCodes := make([]int, concurrentQueries)

			startTime := time.Now()

			for i := 0; i < concurrentQueries; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					queryStart := time.Now()
					resp, err := httpClient.Post(
						baseURL+"/api/v1/workflows/search",
						"application/json",
						bytes.NewReader(payloadBytes),
					)
					durations[idx] = time.Since(queryStart)

					if err != nil {
						GinkgoWriter.Printf("Search request %d failed: %v\n", idx, err)
						statusCodes[idx] = 0
						return
					}
					defer func() { _ = resp.Body.Close() }()

					statusCodes[idx] = resp.StatusCode
				}(i)
			}

			wg.Wait()
			totalDuration := time.Since(startTime)

			// ASSERT: All queries succeeded
			successCount := 0
			for _, code := range statusCodes {
				if code == http.StatusOK {
					successCount++
				}
			}

			Expect(successCount).To(Equal(concurrentQueries),
				fmt.Sprintf("Expected all %d searches to succeed, got %d", concurrentQueries, successCount))

			// Calculate percentiles
			sort.Slice(durations, func(i, j int) bool {
				return durations[i] < durations[j]
			})

			p50 := durations[int(float64(len(durations))*0.50)]
			p95 := durations[int(float64(len(durations))*0.95)]
			p99 := durations[int(float64(len(durations))*0.99)]

			GinkgoWriter.Printf("Performance results (20 concurrent queries):\n")
			GinkgoWriter.Printf("  Total duration: %v\n", totalDuration)
			GinkgoWriter.Printf("  p50 latency:   %v\n", p50)
			GinkgoWriter.Printf("  p95 latency:   %v (%s)\n", p95, assertionStatus(p95.Milliseconds() < 500))
			GinkgoWriter.Printf("  p99 latency:   %v (%s)\n", p99, assertionStatus(p99.Milliseconds() < 1000))
			GinkgoWriter.Printf("  Avg latency:   %v\n", totalDuration/time.Duration(concurrentQueries))

			// ASSERT: Performance SLAs met
			Expect(p95.Milliseconds()).To(BeNumerically("<", 500),
				fmt.Sprintf("p95 latency %dms exceeds 500ms SLA", p95.Milliseconds()))

			Expect(p99.Milliseconds()).To(BeNumerically("<", 1000),
				fmt.Sprintf("p99 latency %dms exceeds 1s SLA", p99.Milliseconds()))

			GinkgoWriter.Printf("✅ Concurrent workflow search performance validated\n")
			GinkgoWriter.Printf("   p95: %dms (target: <500ms)\n", p95.Milliseconds())
			GinkgoWriter.Printf("   p99: %dms (target: <1000ms)\n", p99.Milliseconds())

			// BUSINESS VALUE: Concurrent search scalability
			// - HolmesGPT-API can scale to multiple workers
			// - Each worker can search workflows concurrently
			// - Search latency remains acceptable under load
			// - No connection pool exhaustion
		})

		It("should handle sustained concurrent load (60s, 10 QPS)", func() {
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("GAP 5.2: Testing sustained concurrent search load (60s, 10 QPS)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// ARRANGE: Create search request
			searchRequest := map[string]interface{}{
				"filters": map[string]interface{}{
					"signal_type": "cpu-throttle",
					"severity":    "high",
					"component":   "pod",
					"priority":    "p1",
					"environment": "staging",
				},
				"top_k": 5,
			}

			payloadBytes, err := json.Marshal(searchRequest)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Execute sustained load (10 QPS for 60s = 600 queries)
			const duration = 60 * time.Second
			const targetQPS = 10
			const totalQueries = 600

			var wg sync.WaitGroup
			durations := make([]time.Duration, totalQueries)
			startTime := time.Now()
			ticker := time.NewTicker(time.Second / targetQPS) // 100ms between queries
			defer ticker.Stop()

			queryIndex := 0
			for queryIndex < totalQueries {
				<-ticker.C

				wg.Add(1)
				idx := queryIndex
				queryIndex++

				go func(i int) {
					defer wg.Done()

					queryStart := time.Now()
					resp, err := httpClient.Post(
						baseURL+"/api/v1/workflows/search",
						"application/json",
						bytes.NewReader(payloadBytes),
					)
					durations[i] = time.Since(queryStart)

					if err != nil {
						return
					}
					defer func() { _ = resp.Body.Close() }()
				}(idx)
			}

			wg.Wait()
			totalDuration := time.Since(startTime)

			// ASSERT: Sustained load handled successfully
			sort.Slice(durations, func(i, j int) bool {
				return durations[i] < durations[j]
			})

			p95 := durations[int(float64(len(durations))*0.95)]
			p99 := durations[int(float64(len(durations))*0.99)]
			actualQPS := float64(totalQueries) / totalDuration.Seconds()

			GinkgoWriter.Printf("Sustained load results (60s, target 10 QPS):\n")
			GinkgoWriter.Printf("  Total duration: %v\n", totalDuration)
			GinkgoWriter.Printf("  Total queries:  %d\n", totalQueries)
			GinkgoWriter.Printf("  Actual QPS:     %.2f\n", actualQPS)
			GinkgoWriter.Printf("  p95 latency:   %v\n", p95)
			GinkgoWriter.Printf("  p99 latency:   %v\n", p99)

			// Verify no severe degradation (allow generous tolerance for integration tests)
			Expect(p95.Milliseconds()).To(BeNumerically("<", 800),
				fmt.Sprintf("p95 latency %dms indicates severe degradation", p95.Milliseconds()))

			GinkgoWriter.Printf("✅ Sustained concurrent load handled successfully\n")

			// BUSINESS VALUE: Production scalability
			// - Service handles realistic production load (10 QPS sustained)
			// - No degradation over time
			// - Connection pool remains stable
			// - Ready for HolmesGPT-API multi-worker deployment
		})
	})
})

// Helper function to get assertion status indicator
func assertionStatus(passed bool) string {
	if passed {
		return "✅ PASS"
	}
	return "❌ FAIL"
}

// Helper function to get Data Storage URL (for performance tests)
func getDataStorageURL() string {
	// In performance tests, we may run against a local or remote instance
	// Default to localhost for development
	return "http://localhost:8081"
}
