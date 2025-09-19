package holmesgpt_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contextpkg "github.com/jordigilh/kubernaut/pkg/ai/context"
	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContextCacheLimits(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context Cache Limits and Monitoring Suite")
}

var _ = Describe("Context Cache Limits and Monitoring", func() {

	// Business Requirement: BR-HOLMES-008 - Context cache management with size limits
	Context("BR-HOLMES-008: Context Cache Size Management", func() {
		var cache *holmesgpt.CachedContextManager
		var logger *logrus.Logger

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests

			// Create cache with size limits for testing
			cache = holmesgpt.NewCachedContextManager(&holmesgpt.ContextCacheConfig{
				MaxSize:         100,  // Limit to 100 entries
				MaxMemory:       1024, // Limit to 1KB for testing
				TTL:             5 * time.Minute,
				CleanupInterval: 10 * time.Second,
			}, logger)
		})

		AfterEach(func() {
			if cache != nil {
				cache.Stop()
			}
		})

		It("should enforce maximum cache size limits", func() {
			// Act: Add entries beyond the limit
			const entriesCount = 150 // Exceed the 100 entry limit

			for i := 0; i < entriesCount; i++ {
				key := fmt.Sprintf("test-key-%03d", i)
				contextData := &contextpkg.ContextData{
					Kubernetes: &contextpkg.KubernetesContext{
						Namespace:    "test-ns",
						ResourceType: "pod",
						CollectedAt:  time.Now(),
					},
				}

				success := cache.Set(key, contextData, 5*time.Minute)
				if i < 100 {
					Expect(success).To(BeTrue(), "Should accept entries within limit")
				}
			}

			// Business Validation: Cache size should not exceed limit
			stats := cache.GetStats()
			Expect(stats.Size).To(BeNumerically("<=", 100),
				"Cache size should not exceed configured limit")
			Expect(stats.EvictionCount).To(BeNumerically(">", 0),
				"Some entries should have been evicted to maintain size limit")
		})

		It("should enforce maximum memory usage limits", func() {
			// Arrange: Create entries larger than the cache limit to force rejection
			largeData := make([]byte, 1200) // 1200 bytes per entry > 1024 cache limit
			for i := range largeData {
				largeData[i] = byte(i % 256)
			}

			// Act: Try to add entries larger than cache limit - some should be rejected
			entriesAdded := 0
			rejectedCount := 0
			for i := 0; i < 5; i++ { // Try to add large entries
				key := fmt.Sprintf("large-key-%03d", i)
				contextData := &contextpkg.ContextData{
					Logs: &contextpkg.LogsContext{
						Source: "test",
						LogEntries: []contextpkg.LogEntry{
							{
								Message:   string(largeData),
								Timestamp: time.Now(),
							},
						},
						CollectedAt: time.Now(),
					},
				}

				success := cache.Set(key, contextData, 5*time.Minute)
				if success {
					entriesAdded++
				} else {
					rejectedCount++
				}
			}

			// Business Validation: Memory usage should not exceed limit
			stats := cache.GetStats()
			Expect(stats.MemoryUsage).To(BeNumerically("<=", 1024),
				"Memory usage should not exceed configured limit")
			Expect(rejectedCount).To(BeNumerically(">", 0),
				"Some entries should be rejected when they exceed cache capacity")

			// Also test with smaller entries that should fit with eviction
			smallData := make([]byte, 200) // 200 bytes per entry
			smallEntriesAdded := 0
			for i := 0; i < 10; i++ {
				key := fmt.Sprintf("small-key-%03d", i)
				contextData := &contextpkg.ContextData{
					Logs: &contextpkg.LogsContext{
						Source: "test",
						LogEntries: []contextpkg.LogEntry{
							{
								Message:   string(smallData),
								Timestamp: time.Now(),
							},
						},
						CollectedAt: time.Now(),
					},
				}

				if cache.Set(key, contextData, 5*time.Minute) {
					smallEntriesAdded++
				}
			}

			finalStats := cache.GetStats()
			Expect(smallEntriesAdded).To(Equal(10), "Small entries should all fit with LRU eviction")
			Expect(finalStats.MemoryUsage).To(BeNumerically("<=", 1024),
				"Memory usage should remain within limit after eviction")
		})

		It("should provide accurate cache statistics and monitoring", func() {
			// Arrange: Add some test data
			testKeys := []string{"key1", "key2", "key3"}
			for _, key := range testKeys {
				contextData := &contextpkg.ContextData{
					Metrics: &contextpkg.MetricsContext{
						Source: "test",
						MetricsData: map[string]float64{
							"cpu_usage": 0.75,
						},
						CollectedAt: time.Now(),
					},
				}
				cache.Set(key, contextData, 5*time.Minute)
			}

			// Access some entries to generate hits
			cache.Get("key1")
			cache.Get("key2")
			cache.Get("key1")        // Hit same key again
			cache.Get("nonexistent") // Miss

			// Business Validation: Statistics should be accurate
			stats := cache.GetStats()
			Expect(stats.Size).To(Equal(3), "Should track correct cache size")
			Expect(stats.HitCount).To(Equal(int64(3)), "Should track cache hits")
			Expect(stats.MissCount).To(Equal(int64(1)), "Should track cache misses")
			Expect(stats.HitRate).To(BeNumerically("~", 0.75, 0.01), "Should calculate correct hit rate")
			Expect(stats.MemoryUsage).To(BeNumerically(">", 0), "Should track memory usage")
		})

		It("should handle concurrent cache operations under size pressure", func() {
			// Arrange: Concurrent operations setup
			const numGoroutines = 20
			const operationsPerGoroutine = 50

			var wg sync.WaitGroup
			var successfulSets int64
			var successfulGets int64
			var evictionsCaused int64

			// Act: Concurrent cache operations
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						key := fmt.Sprintf("worker-%d-key-%d", workerID, j)

						// Set operation
						contextData := &contextpkg.ContextData{
							Events: &contextpkg.EventsContext{
								Source: fmt.Sprintf("worker-%d", workerID),
								Events: []contextpkg.Event{
									{
										Message:   fmt.Sprintf("Event from worker %d operation %d", workerID, j),
										Timestamp: time.Now(),
									},
								},
								CollectedAt: time.Now(),
							},
						}

						initialStats := cache.GetStats()
						success := cache.Set(key, contextData, 5*time.Minute)
						if success {
							atomic.AddInt64(&successfulSets, 1)
						}

						// Check if eviction occurred
						afterStats := cache.GetStats()
						if afterStats.EvictionCount > initialStats.EvictionCount {
							atomic.AddInt64(&evictionsCaused, 1)
						}

						// Get operation
						retrieved := cache.Get(key)
						if retrieved != nil {
							atomic.AddInt64(&successfulGets, 1)
						}

						time.Sleep(time.Microsecond * 5)
					}
				}(i)
			}

			// Wait for completion with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(30 * time.Second):
				Fail("Test timed out - possible deadlock in cache operations")
			}

			// Business Validation: Cache should remain functional under pressure
			finalStats := cache.GetStats()
			finalSets := atomic.LoadInt64(&successfulSets)
			finalGets := atomic.LoadInt64(&successfulGets)
			finalEvictions := atomic.LoadInt64(&evictionsCaused)

			Expect(finalSets).To(BeNumerically(">", 0), "Some set operations should succeed")
			Expect(finalGets).To(BeNumerically(">", 0), "Some get operations should succeed")
			Expect(finalStats.Size).To(BeNumerically("<=", 100), "Cache size should remain within limits")
			Expect(finalEvictions).To(BeNumerically(">", 0), "Evictions should occur due to size pressure")
		})

		It("should implement LRU eviction policy correctly", func() {
			// Arrange: Create cache with sufficient memory but small size limit for testing
			cache.Stop() // Stop the current cache

			cache = holmesgpt.NewCachedContextManager(&holmesgpt.ContextCacheConfig{
				MaxSize:         3,    // Small size limit
				MaxMemory:       2048, // Sufficient memory for testing
				TTL:             5 * time.Minute,
				CleanupInterval: 10 * time.Second,
			}, logger)

			// Fill cache to size capacity (3 entries)
			for i := 1; i <= 3; i++ {
				key := fmt.Sprintf("key%d", i)
				contextData := &contextpkg.ContextData{
					Kubernetes: &contextpkg.KubernetesContext{
						Namespace:   fmt.Sprintf("ns-%d", i),
						CollectedAt: time.Now(),
					},
				}
				success := cache.Set(key, contextData, 5*time.Minute)
				Expect(success).To(BeTrue(), "Should accept entries within size capacity")
			}

			// Verify cache is at capacity
			stats := cache.GetStats()
			Expect(stats.Size).To(Equal(3), "Cache should be at size capacity")

			// Access key2 to make it recently used (move to front of LRU)
			data2 := cache.Get("key2")
			Expect(data2).ToNot(BeNil(), "key2 should exist before access")

			time.Sleep(time.Millisecond * 10) // Ensure different access times

			// Act: Add new entry that should trigger LRU eviction
			newContextData := &contextpkg.ContextData{
				Kubernetes: &contextpkg.KubernetesContext{
					Namespace:   "ns-new",
					CollectedAt: time.Now(),
				},
			}
			success := cache.Set("key4", newContextData, 5*time.Minute)

			// Business Validation: New entry should be accepted and LRU should be evicted
			Expect(success).To(BeTrue(), "New entry should be accepted")

			// Cache size should remain at limit
			finalStats := cache.GetStats()
			Expect(finalStats.Size).To(Equal(3), "Cache size should remain at limit after eviction")
			Expect(finalStats.EvictionCount).To(BeNumerically(">", 0), "At least one eviction should have occurred")

			// Recently accessed key2 should still exist
			Expect(cache.Get("key2")).ToNot(BeNil(), "Recently accessed key2 should remain")

			// Newly added key4 should exist
			Expect(cache.Get("key4")).ToNot(BeNil(), "Newly added key4 should exist")

			// At least one of the older, unaccessed entries should be evicted
			evictedCount := 0
			for _, key := range []string{"key1", "key3"} {
				if cache.Get(key) == nil {
					evictedCount++
				}
			}
			Expect(evictedCount).To(BeNumerically(">=", 1), "At least one LRU entry should be evicted")
		})
	})
})
