package contextapi

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
)

// Helper Functions for Cache Fallback Tests

// createCacheWithoutRedis creates cache manager with invalid Redis address
// This simulates Redis being unavailable, forcing LRU-only mode
func createCacheWithoutRedis() cache.CacheManager {
	cfg := &cache.Config{
		RedisAddr:  "localhost:9999", // Invalid port (no Redis running)
		RedisDB:    0,
		LRUSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}
	manager, err := cache.NewCacheManager(cfg, logger)
	Expect(err).ToNot(HaveOccurred(), "Cache manager should initialize despite Redis unavailability")
	return manager
}

// createCacheWithRedis creates cache manager with working Redis
// Uses DB 4 for Suite 2 (parallel test isolation)
func createCacheWithRedis() cache.CacheManager {
	cfg := &cache.Config{
		RedisAddr:  "localhost:6379",
		RedisDB:    4, // Use DB 4 for Suite 2 (parallel isolation)
		LRUSize:    1000,
		DefaultTTL: 5 * time.Minute,
	}
	manager, err := cache.NewCacheManager(cfg, logger)
	Expect(err).ToNot(HaveOccurred(), "Cache manager should initialize with Redis")
	return manager
}

var _ = Describe("Cache Fallback Integration Tests", func() {
	var (
		testCtx context.Context
		cancel  context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	Context("Initialization Scenarios", func() {
		It("should initialize with LRU-only when Redis is unavailable", func() {
			// Day 8 Suite 2 - Test #1 (REFACTOR Phase Complete - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: RED → GREEN → REFACTOR (using helper function)
			// Tests that manager initializes successfully despite Redis being down

			// REFACTOR: Use helper function for cache creation
			manager := createCacheWithoutRedis()
			Expect(manager).ToNot(BeNil(),
				"Manager instance should be created")

			// Verify degraded state through health check
			health, err := manager.HealthCheck(testCtx)
			Expect(err).ToNot(HaveOccurred(),
				"Health check should not fail")
			Expect(health).ToNot(BeNil(),
				"Health status should be returned")

			// ✅ Business Value Assertion: System reports degraded state
			Expect(health.Degraded).To(BeTrue(),
				"Should be in degraded state when Redis is unavailable")
			Expect(health.Message).To(ContainSubstring("Redis unavailable"),
				"Health message should indicate Redis is unavailable")
			Expect(health.Message).To(ContainSubstring("LRU"),
				"Health message should indicate LRU-only mode")
		})
	})

	Context("Runtime Fallback Scenarios", func() {
		It("should handle Set and Get with LRU-only (no Redis)", func() {
			// Day 8 Suite 2 - Test #2 (REFACTOR Phase Complete - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: RED → GREEN → REFACTOR complete
			// Tests cache operations work without Redis (LRU only)

			// Create cache manager without Redis
			manager := createCacheWithoutRedis()

			// Test data
			testKey := "test-key-lru"
			testValue := map[string]string{"value": "test-data", "type": "lru-only"}

			// Set value (should succeed with LRU only)
			err := manager.Set(testCtx, testKey, testValue)
			Expect(err).ToNot(HaveOccurred(),
				"Set should succeed with LRU only")

			// Get value (should retrieve from LRU)
			data, err := manager.Get(testCtx, testKey)
			Expect(err).ToNot(HaveOccurred(),
				"Get should succeed from LRU")
			Expect(data).ToNot(BeNil(),
				"Should retrieve data from L2")

			// ✅ Business Value Assertion: Verify L2 hit recorded
			stats := manager.Stats()
			Expect(stats.HitsL2).To(BeNumerically(">", 0),
				"L2 hit should be recorded when Redis is unavailable")
			Expect(stats.HitsL1).To(Equal(uint64(0)),
				"L1 hits should be zero when Redis is unavailable")
		})

		It("should fallback to L2 when Redis times out during Get", func() {
			// Day 8 Suite 2 - Test #3 (REFACTOR Phase Complete - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: RED → GREEN → REFACTOR complete
			// Tests L2 fallback when Redis times out

			// Create cache with Redis available
			manager := createCacheWithRedis()

			// Set value in both L1 and L2
			testKey := "timeout-test-key"
			testValue := map[string]string{"data": "timeout-test"}
			err := manager.Set(testCtx, testKey, testValue)
			Expect(err).ToNot(HaveOccurred())

			// Simulate Redis timeout with extremely short context
			// This will cause Redis Get to timeout, but L2 should still work
			shortCtx, cancel := context.WithTimeout(testCtx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond) // Ensure timeout expires

			// Should fallback to L2 despite Redis timeout
			data, err := manager.Get(shortCtx, testKey)

			// ✅ Business Value Assertion: Operation succeeds despite timeout
			Expect(err).ToNot(HaveOccurred(),
				"Get should succeed despite Redis timeout")
			Expect(data).ToNot(BeNil(),
				"Should retrieve from L2 cache")

			// Verify L2 fallback happened
			stats := manager.Stats()
			Expect(stats.HitsL2).To(BeNumerically(">", 0),
				"L2 hit should be recorded when Redis times out")
		})

		It("should complete Set to L2 when Redis times out", func() {
			// Day 8 Suite 2 - Test #4 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: Test Set completes despite Redis timeout
			// Expected: Set succeeds by setting L2 only

			manager := createCacheWithRedis()

			// Simulate Redis timeout during Set
			shortCtx, cancel := context.WithTimeout(testCtx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond)

			testKey := "set-timeout-key"
			testValue := map[string]string{"data": "set-test"}

			// Set with timeout context (Redis will timeout, L2 will succeed)
			err := manager.Set(shortCtx, testKey, testValue)
			Expect(err).ToNot(HaveOccurred(),
				"Set should succeed (L2 only)")

			// Verify value is in L2 (use normal context for Get)
			data, err := manager.Get(testCtx, testKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).ToNot(BeNil(),
				"Value should be in L2")
		})
	})

	Context("LRU Behavior", func() {
		It("should evict oldest entries when LRU is full", func() {
			// Day 8 Suite 2 - Test #5 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: Test LRU eviction when cache is full
			// Expected: Oldest entries evicted when limit reached

			// Create cache with small LRU for testing eviction
			cfg := &cache.Config{
				RedisAddr:  "localhost:9999", // No Redis (LRU only)
				RedisDB:    0,
				LRUSize:    3, // Small size to trigger eviction
				DefaultTTL: 5 * time.Minute,
			}
			smallCache, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())

			// Fill LRU beyond capacity
			_ = smallCache.Set(testCtx, "key1", "value1")
			_ = smallCache.Set(testCtx, "key2", "value2")
			_ = smallCache.Set(testCtx, "key3", "value3")
			_ = smallCache.Set(testCtx, "key4", "value4") // Should evict key1
			_ = smallCache.Set(testCtx, "key5", "value5") // Should evict key2

			// Verify oldest entries evicted
			data1, _ := smallCache.Get(testCtx, "key1")
			data2, _ := smallCache.Get(testCtx, "key2")
			data3, _ := smallCache.Get(testCtx, "key3")
			data4, _ := smallCache.Get(testCtx, "key4")
			data5, _ := smallCache.Get(testCtx, "key5")

			Expect(data1).To(BeNil(), "key1 should be evicted")
			Expect(data2).To(BeNil(), "key2 should be evicted")
			Expect(data3).ToNot(BeNil(), "key3 should still exist")
			Expect(data4).ToNot(BeNil(), "key4 should still exist")
			Expect(data5).ToNot(BeNil(), "key5 should still exist")
		})
	})

	Context("Health Check Scenarios", func() {
		It("should report degraded state when Redis is down", func() {
			// Day 8 Suite 2 - Test #6 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: Test health check reports degraded state
			// Expected: Degraded=true when Redis unavailable

			lruOnlyCache := createCacheWithoutRedis()

			health, err := lruOnlyCache.HealthCheck(testCtx)

			Expect(err).ToNot(HaveOccurred())
			Expect(health.Degraded).To(BeTrue(),
				"Should be degraded without Redis")
			Expect(health.Message).To(ContainSubstring("Redis unavailable"))
		})

		It("should report healthy state when Redis is up", func() {
			// Day 8 Suite 2 - Test #7 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: Test health check reports healthy state
			// Expected: Degraded=false when Redis available

			normalCache := createCacheWithRedis()

			health, err := normalCache.HealthCheck(testCtx)

			Expect(err).ToNot(HaveOccurred())
			Expect(health.Degraded).To(BeFalse(),
				"Should not be degraded with Redis")
			Expect(health.Message).To(ContainSubstring("healthy"))
		})
	})

	Context("Statistics Tracking", func() {
		It("should track error statistics when Redis operations fail", func() {
			// Day 8 Suite 2 - Test #8 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Multi-tier caching with graceful degradation
			//
			// ✅ Pure TDD: Test error stats tracked during Redis failures
			// Expected: Error count increases when Redis times out

			cacheWithRedis := createCacheWithRedis()
			initialStats := cacheWithRedis.Stats()

			// Simulate multiple Redis errors with timeout context
			shortCtx, cancel := context.WithTimeout(testCtx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond)

			// Trigger Redis timeouts
			for i := 0; i < 5; i++ {
				_, _ = cacheWithRedis.Get(shortCtx, fmt.Sprintf("error-key-%d", i))
			}

			// Verify error stats increased
			finalStats := cacheWithRedis.Stats()
			errorIncrease := finalStats.Errors - initialStats.Errors

			Expect(errorIncrease).To(BeNumerically(">", 0),
				"Error count should increase when Redis times out")
		})
	})
})
