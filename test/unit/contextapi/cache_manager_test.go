package contextapi

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
)

var _ = Describe("Cache Manager", func() {
	var (
		ctx    context.Context
		logger *zap.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger, _ = zap.NewDevelopment()
	})

	Context("NewCacheManager", func() {
		It("should create a cache manager with Redis and LRU", func() {
			cfg := &cache.Config{
				RedisAddr:  "localhost:6379",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())
			defer manager.Close()
		})

		It("should work even if Redis is unavailable (graceful degradation)", func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999", // Invalid Redis address
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(manager).ToNot(BeNil())
			defer manager.Close()
		})

		It("should return error if logger is nil", func() {
			cfg := &cache.Config{
				RedisAddr:  "localhost:6379",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
			Expect(manager).To(BeNil())
		})

		It("should return error if LRU size is invalid", func() {
			cfg := &cache.Config{
				RedisAddr:  "localhost:6379",
				LRUSize:    0, // Invalid size
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lru size"))
			Expect(manager).To(BeNil())
		})
	})

	Context("Cache Operations - L1 (Redis) Priority", func() {
		var manager cache.CacheManager

		BeforeEach(func() {
			cfg := &cache.Config{
				RedisAddr:  "localhost:6379",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if manager != nil {
				manager.Close()
			}
		})

		It("should store and retrieve from cache (L1 hit)", func() {
			key := "test-key-l1"
			value := map[string]string{"data": "test-value"}

			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			var retrieved map[string]string
			err = json.Unmarshal(result, &retrieved)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved).To(Equal(value))
		})

		It("should return nil for cache miss", func() {
			result, err := manager.Get(ctx, "non-existent-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should delete cache entries", func() {
			key := "test-key-delete"
			value := map[string]string{"data": "delete-me"}

			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			err = manager.Delete(ctx, key)
			Expect(err).ToNot(HaveOccurred())

			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Context("Graceful Degradation - Redis Down → LRU", func() {
		var manager cache.CacheManager

		BeforeEach(func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999", // Redis unavailable
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if manager != nil {
				manager.Close()
			}
		})

		It("should fallback to LRU when Redis is unavailable", func() {
			key := "test-key-lru-fallback"
			value := map[string]string{"data": "lru-only"}

			// Set should succeed (writes to LRU)
			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			// Get should retrieve from LRU
			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			var retrieved map[string]string
			err = json.Unmarshal(result, &retrieved)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved).To(Equal(value))
		})

		It("should handle delete gracefully when Redis is down", func() {
			key := "test-key-delete-lru"
			value := map[string]string{"data": "delete-lru"}

			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			err = manager.Delete(ctx, key)
			Expect(err).ToNot(HaveOccurred())

			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Context("Multi-Tier Behavior - L1 → L2 Fallback", func() {
		var (
			manager    cache.CacheManager
			redisAddr  string
			testClient *redis.Client
		)

		BeforeEach(func() {
			redisAddr = "localhost:6379"
			cfg := &cache.Config{
				RedisAddr:  redisAddr,
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())

			// Create test Redis client for manual manipulation
			testClient = redis.NewClient(&redis.Options{Addr: redisAddr})
		})

		AfterEach(func() {
			if testClient != nil {
				testClient.Close()
			}
			if manager != nil {
				manager.Close()
			}
		})

		It("should populate L2 on L1 hit", func() {
			Skip("Integration test - requires Redis")

			key := "test-key-l1-to-l2"
			value := map[string]string{"data": "populate-l2"}

			// Set value (writes to L1 and L2)
			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			// First Get retrieves from L1
			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			// Delete from L1 (Redis only)
			err = testClient.Del(ctx, key).Err()
			Expect(err).ToNot(HaveOccurred())

			// Second Get should retrieve from L2
			result, err = manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})
	})

	Context("LRU Eviction", func() {
		var manager cache.CacheManager

		BeforeEach(func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999", // Use LRU only
				LRUSize:    2,              // Small size to trigger eviction
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if manager != nil {
				manager.Close()
			}
		})

		It("should evict least recently used items when cache is full", func() {
			// Add 3 items to a cache with size 2
			err := manager.Set(ctx, "key1", map[string]string{"data": "value1"})
			Expect(err).ToNot(HaveOccurred())

			err = manager.Set(ctx, "key2", map[string]string{"data": "value2"})
			Expect(err).ToNot(HaveOccurred())

			err = manager.Set(ctx, "key3", map[string]string{"data": "value3"})
			Expect(err).ToNot(HaveOccurred())

			// key1 should be evicted (least recently used)
			result, err := manager.Get(ctx, "key1")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeNil())

			// key2 and key3 should still be present
			result, err = manager.Get(ctx, "key2")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			result, err = manager.Get(ctx, "key3")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})
	})

	Context("Serialization", func() {
		var manager cache.CacheManager

		BeforeEach(func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if manager != nil {
				manager.Close()
			}
		})

		It("should handle complex data structures", func() {
			key := "test-complex"
			value := map[string]interface{}{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"nested": map[string]string{"key": "value"},
				"array":  []int{1, 2, 3},
			}

			err := manager.Set(ctx, key, value)
			Expect(err).ToNot(HaveOccurred())

			result, err := manager.Get(ctx, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			var retrieved map[string]interface{}
			err = json.Unmarshal(result, &retrieved)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved["string"]).To(Equal("value"))
			Expect(retrieved["int"]).To(BeNumerically("==", 42))
		})
	})

	Context("HealthCheck", func() {
		It("should report healthy when Redis is available", func() {
			Skip("Integration test - requires Redis")

			cfg := &cache.Config{
				RedisAddr:  "localhost:6379",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			defer manager.Close()

			status, err := manager.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Degraded).To(BeFalse())
		})

		It("should report degraded when Redis is unavailable but LRU works", func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999",
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}

			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			defer manager.Close()

			status, err := manager.HealthCheck(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Degraded).To(BeTrue())
			Expect(status.Message).To(ContainSubstring("Redis unavailable"))
		})
	})

	Context("Statistics Tracking", func() {
		var manager cache.CacheManager

		BeforeEach(func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999", // Use LRU only for predictable stats
				LRUSize:    100,
				DefaultTTL: 5 * time.Minute,
			}
			var err error
			manager, err = cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if manager != nil {
				manager.Close()
			}
		})

		It("should track cache hits (L2)", func() {
			// Set a value
			err := manager.Set(ctx, "hit-key", map[string]string{"data": "value"})
			Expect(err).ToNot(HaveOccurred())

			// Get it (L2 hit)
			_, err = manager.Get(ctx, "hit-key")
			Expect(err).ToNot(HaveOccurred())

			stats := manager.Stats()
			Expect(stats.HitsL2).To(BeNumerically(">", 0))
			Expect(stats.Sets).To(BeNumerically(">", 0))
		})

		It("should track cache misses", func() {
			// Try to get non-existent key
			_, err := manager.Get(ctx, "non-existent")
			Expect(err).ToNot(HaveOccurred())

			stats := manager.Stats()
			Expect(stats.Misses).To(BeNumerically(">", 0))
		})

		It("should track evictions", func() {
			cfg := &cache.Config{
				RedisAddr:  "invalid:9999",
				LRUSize:    2, // Small size to trigger evictions
				DefaultTTL: 5 * time.Minute,
			}
			manager, err := cache.NewCacheManager(cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			defer manager.Close()

			// Add 3 items to trigger eviction
			_ = manager.Set(ctx, "key1", "value1")
			_ = manager.Set(ctx, "key2", "value2")
			_ = manager.Set(ctx, "key3", "value3")

			stats := manager.Stats()
			Expect(stats.Evictions).To(BeNumerically(">", 0))
		})

		It("should calculate hit rate", func() {
			// Set values
			_ = manager.Set(ctx, "key1", "value1")
			_ = manager.Set(ctx, "key2", "value2")

			// Hits
			_, _ = manager.Get(ctx, "key1")
			_, _ = manager.Get(ctx, "key2")

			// Miss
			_, _ = manager.Get(ctx, "non-existent")

			stats := manager.Stats()
			// 2 hits, 1 miss = 66.6% hit rate
			Expect(stats.HitRate()).To(BeNumerically(">", 50))
			Expect(stats.HitRate()).To(BeNumerically("<", 100))
		})

		It("should report cache size", func() {
			// Add some items
			_ = manager.Set(ctx, "key1", "value1")
			_ = manager.Set(ctx, "key2", "value2")

			stats := manager.Stats()
			Expect(stats.TotalSize).To(Equal(2))
			Expect(stats.MaxSize).To(Equal(100))
		})

		It("should report Redis status", func() {
			stats := manager.Stats()
			Expect(stats.RedisStatus).To(Equal("unavailable"))
		})
	})
})
