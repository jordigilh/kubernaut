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

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheManager orchestrates multi-tier caching (Redis L1 + LRU L2)
// BR-CONTEXT-005: Multi-tier caching with graceful degradation
//
// Cache hierarchy:
//  1. L1 (Redis): Distributed cache for multi-instance deployments
//  2. L2 (LRU): In-memory cache for fast access
//  3. L3 (Database): Fallback when cache misses
type CacheManager interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value interface{}) error
	Delete(ctx context.Context, key string) error
	HealthCheck(ctx context.Context) (*HealthStatus, error)
	Stats() Stats
	Close() error
}

// HealthStatus reports cache health
type HealthStatus struct {
	Degraded bool   // True if Redis unavailable but LRU works
	Message  string // Health status message
}

// multiTierCache implements CacheManager
type multiTierCache struct {
	redis        *redis.Client
	memory       map[string]*cacheEntry
	mu           sync.RWMutex
	maxSize      int
	maxValueSize int64 // Day 11: Maximum cached object size (bytes)
	logger       *zap.Logger
	ttl          time.Duration
	redisAvail   bool // Track Redis availability for health checks
	stats        cacheStats
}

// NewCacheManager creates a new multi-tier cache manager
// BR-CONTEXT-005: Cache manager initialization with graceful degradation
//
// Graceful degradation:
//   - If Redis is unavailable, cache manager still works with LRU only
//   - Set operations write to both L1 and L2 (best effort for L1)
//   - Get operations try L1 → L2 → miss
//
// Returns error only if:
//   - Logger is nil
//   - LRU size is invalid (<= 0)
func NewCacheManager(cfg *Config, logger *zap.Logger) (CacheManager, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	if cfg.LRUSize <= 0 {
		return nil, fmt.Errorf("lru size must be > 0, got %d", cfg.LRUSize)
	}

	// Create Redis client (L1) - best effort
	// REFACTOR Phase: Use configurable DB for parallel test isolation
	// Each test file can use a different Redis database (0-15)
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     "",          // No password for local dev
		DB:           cfg.RedisDB, // Configurable DB for test isolation
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test Redis connectivity (graceful degradation)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisAvail := false
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis unavailable, using LRU only (graceful degradation)",
			zap.Error(err),
			zap.String("address", cfg.RedisAddr))
		redisClient = nil // Disable Redis
	} else {
		redisAvail = true
		logger.Info("Cache manager created with Redis L1 + LRU L2",
			zap.String("redis_addr", cfg.RedisAddr),
			zap.Int("lru_size", cfg.LRUSize),
			zap.Duration("ttl", cfg.DefaultTTL))
	}

	// Apply default max value size if not configured
	// Day 11: Large object OOM prevention
	maxValueSize := cfg.MaxValueSize
	if maxValueSize == 0 {
		maxValueSize = DefaultMaxValueSize // 5MB default
	}
	// -1 means unlimited (disable size checks)

	return &multiTierCache{
		redis:        redisClient,
		memory:       make(map[string]*cacheEntry),
		maxSize:      cfg.LRUSize,
		maxValueSize: maxValueSize,
		logger:       logger,
		ttl:          cfg.DefaultTTL,
		redisAvail:   redisAvail,
	}, nil
}

// Get retrieves value from cache (L1 → L2 → miss)
// BR-CONTEXT-005: Multi-tier cache retrieval
//
// Flow:
//  1. Try L1 (Redis) - if hit, populate L2 and return
//  2. Try L2 (LRU) - if hit, return
//  3. Return nil (cache miss)
//
// Returns:
//   - []byte: cached value (JSON-encoded)
//   - error: nil on success or cache miss, error only on unexpected failures
func (c *multiTierCache) Get(ctx context.Context, key string) ([]byte, error) {
	// Try L1 (Redis) if available
	if c.redis != nil {
		val, err := c.redis.Get(ctx, key).Bytes()
		if err == nil {
			c.stats.RecordHitL1()
			c.logger.Debug("cache hit L1 (Redis)", zap.String("key", key))
			// Populate L2 for faster next access
			c.mu.Lock()
			c.memory[key] = &cacheEntry{
				data:      val,
				expiresAt: time.Now().Add(c.ttl),
			}
			c.mu.Unlock()
			return val, nil
		}
		// Redis error (not found or connection issue) - continue to L2
		if err != redis.Nil {
			c.stats.RecordError()
		}
	}

	// Try L2 (memory)
	c.mu.RLock()
	entry, ok := c.memory[key]
	c.mu.RUnlock()

	if ok {
		// Check if expired
		if time.Now().After(entry.expiresAt) {
			// Expired - delete and return miss
			c.mu.Lock()
			delete(c.memory, key)
			c.mu.Unlock()
			c.stats.RecordMiss()
			c.logger.Debug("cache miss (expired)", zap.String("key", key))
			return nil, nil
		}
		c.stats.RecordHitL2()
		c.logger.Debug("cache hit L2 (memory)", zap.String("key", key))
		return entry.data, nil
	}

	// Cache miss
	c.stats.RecordMiss()
	c.logger.Debug("cache miss", zap.String("key", key))
	return nil, nil
}

// Set stores value in cache (writes to L1 + L2)
// BR-CONTEXT-005: Multi-tier cache storage
//
// Flow:
//  1. Marshal value to JSON
//  2. Validate size against MaxValueSize (Day 11: OOM prevention)
//  3. Write to L1 (Redis) - best effort, log warning if fails
//  4. Write to L2 (LRU) - always succeeds
//
// Returns error if:
//   - JSON marshaling fails
//   - Object size exceeds MaxValueSize (unless MaxValueSize=-1 for unlimited)
func (c *multiTierCache) Set(ctx context.Context, key string, value interface{}) error {
	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		c.logger.Error("failed to marshal value",
			zap.Error(err),
			zap.String("key", key))
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Day 11: Validate object size (OOM prevention)
	// MaxValueSize = -1 means unlimited (skip size check)
	if c.maxValueSize > 0 {
		dataSize := int64(len(data))
		if dataSize > c.maxValueSize {
			c.stats.RecordError()
			c.logger.Warn("object exceeds maximum size, rejecting",
				zap.String("key", key),
				zap.Int64("size", dataSize),
				zap.Int64("max_size", c.maxValueSize))
			return fmt.Errorf("object size (%d bytes) exceeds maximum size (%d bytes)", dataSize, c.maxValueSize)
		}
	}

	// Write to L1 (Redis) - best effort
	if c.redis != nil {
		if err := c.redis.Set(ctx, key, data, c.ttl).Err(); err != nil {
			c.stats.RecordError()
			c.logger.Warn("Redis set failed, continuing with L2 only",
				zap.Error(err),
				zap.String("key", key))
		}
	}

	// Always write to L2 (memory) with LRU eviction
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict (simple LRU - evict soonest to expire)
	if len(c.memory) >= c.maxSize {
		// Find entry expiring soonest
		var evictKey string
		earliest := time.Now().Add(c.ttl * 2) // Start with future time
		for k, entry := range c.memory {
			if entry.expiresAt.Before(earliest) {
				evictKey = k
				earliest = entry.expiresAt
			}
		}
		// Evict entry
		if evictKey != "" {
			delete(c.memory, evictKey)
			c.stats.RecordEviction()
		}
	}

	c.memory[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.stats.RecordSet()
	c.logger.Debug("cache set", zap.String("key", key))

	return nil
}

// Delete removes value from cache (L1 + L2)
// BR-CONTEXT-005: Cache invalidation
//
// Flow:
//  1. Delete from L1 (Redis) - best effort
//  2. Delete from L2 (LRU) - always succeeds
//
// Returns nil (never fails)
func (c *multiTierCache) Delete(ctx context.Context, key string) error {
	// Delete from L1 (Redis) - best effort
	if c.redis != nil {
		if err := c.redis.Del(ctx, key).Err(); err != nil {
			c.logger.Warn("Redis delete failed, continuing with L2",
				zap.Error(err),
				zap.String("key", key))
		}
	}

	// Delete from L2 (memory)
	c.mu.Lock()
	delete(c.memory, key)
	c.mu.Unlock()
	c.logger.Debug("cache deleted", zap.String("key", key))

	return nil
}

// HealthCheck reports cache health status
// BR-CONTEXT-008: REST API health check support
//
// Returns:
//   - HealthStatus: healthy if Redis available, degraded if LRU-only or thrashing
//   - error: nil (health check always succeeds)
//
// Day 11 Scenario 1.2: Detects cache thrashing (high eviction rate)
// - Eviction rate > 80% indicates cache size < working set
// - Alerts operators to resize cache
func (c *multiTierCache) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	// Get current stats for thrashing detection
	stats := c.stats.Snapshot()

	// Day 11: Check for cache thrashing (high eviction rate)
	// Eviction rate = evictions / total sets
	// >80% indicates cache is undersized for working set
	degraded := false
	message := ""

	if stats.Sets > 0 {
		evictionRate := float64(stats.Evictions) / float64(stats.Sets)
		if evictionRate > 0.8 {
			// High eviction rate detected - cache thrashing
			degraded = true
			message = fmt.Sprintf("Cache thrashing detected (eviction rate: %.1f%%) - consider increasing cache size", evictionRate*100)
		}
	}

	// Check Redis connectivity
	if c.redis != nil {
		// Test Redis connectivity
		if err := c.redis.Ping(ctx).Err(); err != nil {
			// Redis failed - degraded mode
			if message != "" {
				// Both Redis unavailable AND thrashing
				message = fmt.Sprintf("%s; Redis unavailable: %v (using LRU only)", message, err)
			} else {
				message = fmt.Sprintf("Redis unavailable: %v (using LRU only)", err)
			}
			return &HealthStatus{
				Degraded: true,
				Message:  message,
			}, nil
		}

		// Redis healthy - check if thrashing detected
		if degraded {
			return &HealthStatus{
				Degraded: true,
				Message:  message,
			}, nil
		}

		// Redis healthy, no thrashing
		return &HealthStatus{
			Degraded: false,
			Message:  "Cache healthy (Redis L1 + LRU L2)",
		}, nil
	}

	// Redis disabled - degraded mode
	if message != "" {
		// Both Redis unavailable AND thrashing
		message = fmt.Sprintf("%s; Redis unavailable (using LRU L2 only)", message)
	} else {
		message = "Redis unavailable (using LRU L2 only)"
	}

	return &HealthStatus{
		Degraded: true,
		Message:  message,
	}, nil
}

// Close closes cache connections
func (c *multiTierCache) Close() error {
	if c.redis != nil {
		if err := c.redis.Close(); err != nil {
			c.logger.Error("failed to close Redis client", zap.Error(err))
			return fmt.Errorf("failed to close Redis client: %w", err)
		}
	}
	c.logger.Info("Cache manager closed",
		zap.Uint64("total_hits_l1", c.stats.hitsL1.Load()),
		zap.Uint64("total_hits_l2", c.stats.hitsL2.Load()),
		zap.Uint64("total_misses", c.stats.misses.Load()),
		zap.Uint64("total_evictions", c.stats.evictions.Load()))
	return nil
}

// Stats returns current cache statistics
// BR-CONTEXT-005: Cache performance monitoring
func (c *multiTierCache) Stats() Stats {
	c.mu.RLock()
	memSize := len(c.memory)
	c.mu.RUnlock()

	stats := c.stats.Snapshot()
	stats.TotalSize = memSize
	stats.MaxSize = c.maxSize

	if c.redisAvail {
		stats.RedisStatus = "available"
	} else {
		stats.RedisStatus = "unavailable"
	}

	return stats
}
