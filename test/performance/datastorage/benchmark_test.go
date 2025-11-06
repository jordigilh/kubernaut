package datastorage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"
)

// BR-STORAGE-027: Performance Requirements
// - p95 latency: <250ms
// - p99 latency: <500ms
// - Large result sets (1000 records): <1s

const (
	baseURL         = "http://localhost:8080"
	warmupRequests  = 10
	measureRequests = 100
)

// BenchmarkListIncidentsLatency measures p95 and p99 latency
func BenchmarkListIncidentsLatency(b *testing.B) {
	// Warmup
	for i := 0; i < warmupRequests; i++ {
		_, _ = http.Get(baseURL + "/api/v1/incidents?limit=100")
	}

	latencies := make([]time.Duration, 0, measureRequests)

	for i := 0; i < measureRequests; i++ {
		start := time.Now()
		resp, err := http.Get(baseURL + "/api/v1/incidents?limit=100")
		latency := time.Since(start)

		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Unexpected status code: %d", resp.StatusCode)
		}

		latencies = append(latencies, latency)
	}

	// Calculate percentiles
	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	avg := average(latencies)

	b.ReportMetric(float64(p50.Milliseconds()), "p50_ms")
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(p99.Milliseconds()), "p99_ms")
	b.ReportMetric(float64(avg.Milliseconds()), "avg_ms")

	// Verify requirements
	if p95 > 250*time.Millisecond {
		b.Errorf("❌ BR-STORAGE-027: p95 latency %v exceeds target 250ms", p95)
	} else {
		b.Logf("✅ BR-STORAGE-027: p95 latency %v meets target <250ms", p95)
	}

	if p99 > 500*time.Millisecond {
		b.Errorf("❌ BR-STORAGE-027: p99 latency %v exceeds target 500ms", p99)
	} else {
		b.Logf("✅ BR-STORAGE-027: p99 latency %v meets target <500ms", p99)
	}

	b.Logf("Performance Summary:")
	b.Logf("  Average: %v", avg)
	b.Logf("  p50: %v", p50)
	b.Logf("  p95: %v", p95)
	b.Logf("  p99: %v", p99)
}

// BenchmarkLargeResultSet measures performance with 1000 record queries
func BenchmarkLargeResultSet(b *testing.B) {
	latencies := make([]time.Duration, 0, measureRequests)

	for i := 0; i < measureRequests; i++ {
		start := time.Now()
		resp, err := http.Get(baseURL + "/api/v1/incidents?limit=1000")
		latency := time.Since(start)

		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			b.Fatalf("Unexpected status code: %d", resp.StatusCode)
		}

		// Verify we actually got data
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		resp.Body.Close()

		if err != nil {
			b.Fatalf("Failed to decode response: %v", err)
		}

		data, ok := response["data"].([]interface{})
		if !ok {
			b.Fatalf("Invalid response format")
		}

		if len(data) == 0 {
			b.Fatalf("No data returned")
		}

		latencies = append(latencies, latency)
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	avg := average(latencies)

	b.ReportMetric(float64(p50.Milliseconds()), "p50_ms")
	b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")
	b.ReportMetric(float64(p99.Milliseconds()), "p99_ms")
	b.ReportMetric(float64(avg.Milliseconds()), "avg_ms")

	// BR-STORAGE-027: Large result sets should complete in <1s
	if p99 > 1*time.Second {
		b.Errorf("❌ BR-STORAGE-027: Large result set p99 %v exceeds 1s target", p99)
	} else {
		b.Logf("✅ BR-STORAGE-027: Large result set p99 %v meets <1s target", p99)
	}

	b.Logf("Large Result Set (1000 records) Performance:")
	b.Logf("  Average: %v", avg)
	b.Logf("  p50: %v", p50)
	b.Logf("  p95: %v", p95)
	b.Logf("  p99: %v", p99)
}

// BenchmarkConcurrentRequests measures throughput under concurrent load
func BenchmarkConcurrentRequests(b *testing.B) {
	concurrency := 10
	requestsPerWorker := 20

	start := time.Now()
	done := make(chan time.Duration, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			workerStart := time.Now()
			for j := 0; j < requestsPerWorker; j++ {
				resp, err := http.Get(baseURL + "/api/v1/incidents?limit=100")
				if err != nil {
					continue
				}
				resp.Body.Close()
			}
			done <- time.Since(workerStart)
		}()
	}

	// Wait for all workers
	for i := 0; i < concurrency; i++ {
		<-done
	}

	totalDuration := time.Since(start)
	totalRequests := concurrency * requestsPerWorker
	qps := float64(totalRequests) / totalDuration.Seconds()

	b.ReportMetric(qps, "qps")
	b.ReportMetric(float64(totalDuration.Milliseconds()), "total_ms")

	b.Logf("Concurrent Performance (%d workers, %d requests each):", concurrency, requestsPerWorker)
	b.Logf("  Total Duration: %v", totalDuration)
	b.Logf("  Total Requests: %d", totalRequests)
	b.Logf("  QPS: %.2f requests/second", qps)

	// Expect reasonable throughput (>10 QPS minimum)
	if qps < 10 {
		b.Errorf("❌ Throughput %.2f QPS is below minimum 10 QPS", qps)
	} else {
		b.Logf("✅ Throughput %.2f QPS meets minimum requirement", qps)
	}
}

// BenchmarkFilteredQueries measures performance with various filters
func BenchmarkFilteredQueries(b *testing.B) {
	queries := []string{
		"/api/v1/incidents?severity=critical",
		"/api/v1/incidents?action_type=scale",
		"/api/v1/incidents?severity=critical&action_type=scale",
		"/api/v1/incidents?limit=10",
		"/api/v1/incidents?limit=100&offset=100",
	}

	for _, query := range queries {
		b.Run(query, func(b *testing.B) {
			latencies := make([]time.Duration, 0, 50)

			for i := 0; i < 50; i++ {
				start := time.Now()
				resp, err := http.Get(baseURL + query)
				latency := time.Since(start)

				if err != nil {
					b.Fatalf("Request failed: %v", err)
				}
				resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					b.Fatalf("Unexpected status code: %d", resp.StatusCode)
				}

				latencies = append(latencies, latency)
			}

			sort.Slice(latencies, func(i, j int) bool {
				return latencies[i] < latencies[j]
			})

			p95 := latencies[len(latencies)*95/100]
			avg := average(latencies)

			b.ReportMetric(float64(avg.Milliseconds()), "avg_ms")
			b.ReportMetric(float64(p95.Milliseconds()), "p95_ms")

			b.Logf("Query: %s", query)
			b.Logf("  Average: %v, p95: %v", avg, p95)
		})
	}
}

// TestPerformanceReport generates a comprehensive performance report
func TestPerformanceReport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance report in short mode")
	}

	t.Log("====================================")
	t.Log("Data Storage Service Performance Report")
	t.Log("BR-STORAGE-027: Performance Requirements Validation")
	t.Log("====================================")
	t.Log("")

	// Test 1: Basic latency
	t.Log("Test 1: Basic Query Latency (100 requests)")
	latencies := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		start := time.Now()
		resp, err := http.Get(baseURL + "/api/v1/incidents?limit=100")
		latencies[i] = time.Since(start)

		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p50 := latencies[50]
	p95 := latencies[95]
	p99 := latencies[99]
	avg := average(latencies)

	t.Logf("  Average: %v", avg)
	t.Logf("  p50: %v", p50)
	t.Logf("  p95: %v (target: <250ms) %s", p95, passFailEmoji(p95 < 250*time.Millisecond))
	t.Logf("  p99: %v (target: <500ms) %s", p99, passFailEmoji(p99 < 500*time.Millisecond))
	t.Log("")

	// Test 2: Large result sets
	t.Log("Test 2: Large Result Sets (1000 records, 50 requests)")
	largeLatencies := make([]time.Duration, 50)
	for i := 0; i < 50; i++ {
		start := time.Now()
		resp, err := http.Get(baseURL + "/api/v1/incidents?limit=1000")
		largeLatencies[i] = time.Since(start)

		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
	}

	sort.Slice(largeLatencies, func(i, j int) bool {
		return largeLatencies[i] < largeLatencies[j]
	})

	largeP99 := largeLatencies[len(largeLatencies)*99/100]
	largeAvg := average(largeLatencies)

	t.Logf("  Average: %v", largeAvg)
	t.Logf("  p99: %v (target: <1s) %s", largeP99, passFailEmoji(largeP99 < 1*time.Second))
	t.Log("")

	// Final assessment
	t.Log("====================================")
	t.Log("Performance Assessment Summary")
	t.Log("====================================")

	allPass := p95 < 250*time.Millisecond &&
		p99 < 500*time.Millisecond &&
		largeP99 < 1*time.Second

	if allPass {
		t.Log("✅ ALL BR-STORAGE-027 requirements MET")
		t.Log("✅ Service is production-ready from performance perspective")
	} else {
		t.Log("⚠️  Some performance targets not met")
		if p95 >= 250*time.Millisecond {
			t.Logf("   - p95 latency: %v (target: <250ms)", p95)
		}
		if p99 >= 500*time.Millisecond {
			t.Logf("   - p99 latency: %v (target: <500ms)", p99)
		}
		if largeP99 >= 1*time.Second {
			t.Logf("   - Large dataset p99: %v (target: <1s)", largeP99)
		}
	}
	t.Log("")
}

// Helper functions

func average(durations []time.Duration) time.Duration {
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

func passFailEmoji(pass bool) string {
	if pass {
		return "✅"
	}
	return "❌"
}

func init() {
	// Verify service is running before benchmarks
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		fmt.Printf("⚠️  Data Storage Service not running at %s\n", baseURL)
		fmt.Println("   Start service: cd cmd/datastorage && go run main.go")
		return
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✅ Data Storage Service detected at %s\n", baseURL)
	}
}
