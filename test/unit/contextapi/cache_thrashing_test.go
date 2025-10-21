package contextapi

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
)

// ===================================================================
// EDGE CASE TESTING: Cache Thrashing Detection (Scenario 1.2)
// ===================================================================

var _ = Describe("Cache Thrashing Detection (Scenario 1.2)", func() {
	var (
		ctx    context.Context
		logger *zap.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger, _ = zap.NewDevelopment()
	})

	Context("Edge Case 1.2: Rapid Cache Thrashing (P2)", func() {
		It("should detect cache thrashing and report in health check", func() {
			// Day 11 Scenario 1.2 (DO-RED Phase - Pure TDD)
			// BR-CONTEXT-005: Cache performance monitoring
			//
			// Production Reality: ✅ Common During Traffic Spikes
			// - Happens when cache size < working set
			// - Causes poor hit rates despite caching
			// - Observed in undersized caches
			//
			// ✅ Pure TDD: Test written FIRST (RED), then implement (GREEN)
			//
			// Expected Behavior:
			// - High eviction rate (>80%) indicates thrashing
			// - Health check should report degraded performance
			// - Alerts operators to resize cache

			// Create small cache (100 items) to simulate undersized cache
			cfg := &cache.Config{
				RedisAddr:  "localhost:9999", // Invalid (no Redis for unit test)
				RedisDB:    0,
				LRUSize:    100, // SMALL cache to force thrashing
				DefaultTTL: 5 * 60,
			}
			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred(), "Cache manager should initialize")

			// Insert 1000 unique keys (10x cache size)
			// This will cause 900 evictions → 90% eviction rate
			for i := 0; i < 1000; i++ {
				key := fmt.Sprintf("key-%d", i)
				value := fmt.Sprintf("value-%d", i)
				err := manager.Set(ctx, key, value)
				Expect(err).ToNot(HaveOccurred(), "Set should succeed")
			}

			// Verify high eviction rate
			stats := manager.Stats()
			evictionRate := float64(stats.Evictions) / 1000.0

			// ✅ Business Value Assertion: High eviction rate detected
			Expect(evictionRate).To(BeNumerically(">", 0.8),
				"Eviction rate should be >80% for thrashing detection")

			// ✅ Business Value Assertion: Health check reports degraded performance
			health, err := manager.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred(), "Health check should succeed")
			Expect(health).ToNot(BeNil(), "Health status should be returned")

			// Expected: Health check detects thrashing and reports it
			Expect(health.Degraded).To(BeTrue(),
				"Cache should be in degraded state due to thrashing")
			Expect(health.Message).To(ContainSubstring("eviction rate"),
				"Health message should mention eviction rate")
			Expect(health.Message).To(Or(
				ContainSubstring("thrashing"),
				ContainSubstring("high eviction"),
			), "Health message should indicate thrashing condition")
		})

		It("should report healthy state with low eviction rate", func() {
			// Day 11 Scenario 1.2 (DO-RED Phase - Pure TDD)
			// Validates that healthy caches don't trigger false positives

			// Create appropriately sized cache
			cfg := &cache.Config{
				RedisAddr:  "localhost:9999",
				RedisDB:    0,
				LRUSize:    1000, // Large enough for working set
				DefaultTTL: 5 * 60,
			}
			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())

			// Insert 100 keys (10% of cache size)
			// This should NOT cause thrashing
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("key-%d", i)
				value := fmt.Sprintf("value-%d", i)
				err := manager.Set(ctx, key, value)
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify low eviction rate
			stats := manager.Stats()
			Expect(stats.Evictions).To(Equal(uint64(0)),
				"Should have no evictions with sufficient cache size")

			// ✅ Business Value Assertion: Health check reports healthy state
			health, err := manager.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Expected: No thrashing detected, healthy state
			// Note: health.Degraded may be true if Redis is unavailable (graceful degradation)
			// but message should NOT mention thrashing/eviction rate
			if !health.Degraded {
				// Healthy state: Redis available, no thrashing
				Expect(health.Message).ToNot(ContainSubstring("thrashing"),
					"Healthy cache should not report thrashing")
				Expect(health.Message).ToNot(ContainSubstring("high eviction"),
					"Healthy cache should not report high eviction rate")
			} else {
				// Degraded due to Redis unavailability only (not thrashing)
				Expect(health.Message).To(ContainSubstring("Redis unavailable"),
					"Degraded state should be due to Redis, not thrashing")
				Expect(health.Message).ToNot(ContainSubstring("eviction rate"),
					"Thrashing should not be reported when evictions are low")
			}
		})

		It("should calculate eviction rate correctly with edge cases", func() {
			// Day 11 Scenario 1.2 (DO-RED Phase - Pure TDD)
			// Edge case: What happens with very few operations?

			cfg := &cache.Config{
				RedisAddr:  "localhost:9999",
				RedisDB:    0,
				LRUSize:    10, // Very small cache
				DefaultTTL: 5 * 60,
			}
			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())

			// Insert only 5 keys (no evictions expected)
			for i := 0; i < 5; i++ {
				key := fmt.Sprintf("key-%d", i)
				err := manager.Set(ctx, key, "value")
				Expect(err).ToNot(HaveOccurred())
			}

			stats := manager.Stats()

			// ✅ Business Value Assertion: No false positive for small workloads
			Expect(stats.Evictions).To(Equal(uint64(0)),
				"No evictions when cache is not full")

			health, err := manager.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should not report thrashing with zero evictions
			Expect(health.Message).ToNot(ContainSubstring("thrashing"),
				"Should not report thrashing with zero evictions")
		})
	})
})

