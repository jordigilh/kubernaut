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

package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// RedisEmbeddingCache implements EmbeddingCache using Redis
type RedisEmbeddingCache struct {
	client *redis.Client
	log    *logrus.Logger

	// Statistics (atomic operations for thread-safety)
	hits   int64
	misses int64
}

// NewRedisEmbeddingCache creates a new Redis-based embedding cache
func NewRedisEmbeddingCache(addr, password string, db int, log *logrus.Logger) (*RedisEmbeddingCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// Connection pooling settings
		PoolSize:     10,
		MinIdleConns: 3,

		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		// Retry settings
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	cache := &RedisEmbeddingCache{
		client: client,
		log:    log,
	}

	log.WithFields(logrus.Fields{
		"addr": addr,
		"db":   db,
	}).Info("Connected to Redis embedding cache")

	return cache, nil
}

// Get retrieves a cached embedding by key
func (r *RedisEmbeddingCache) Get(ctx context.Context, key string) ([]float64, bool, error) {
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			atomic.AddInt64(&r.misses, 1)
			return nil, false, nil // Cache miss, not an error
		}
		atomic.AddInt64(&r.misses, 1)
		return nil, false, fmt.Errorf("failed to get from Redis: %w", err)
	}

	var embedding []float64
	err = json.Unmarshal([]byte(result), &embedding)
	if err != nil {
		atomic.AddInt64(&r.misses, 1)
		r.log.WithError(err).WithField("key", key).Warn("Failed to unmarshal cached embedding")
		return nil, false, fmt.Errorf("failed to unmarshal embedding: %w", err)
	}

	atomic.AddInt64(&r.hits, 1)

	r.log.WithFields(logrus.Fields{
		"key":            key,
		"embedding_size": len(embedding),
	}).Debug("Cache hit: retrieved embedding from Redis")

	return embedding, true, nil
}

// Set stores an embedding in cache with TTL
func (r *RedisEmbeddingCache) Set(ctx context.Context, key string, embedding []float64, ttl time.Duration) error {
	data, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	err = r.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set in Redis: %w", err)
	}

	r.log.WithFields(logrus.Fields{
		"key":            key,
		"embedding_size": len(embedding),
		"ttl_seconds":    ttl.Seconds(),
	}).Debug("Stored embedding in Redis cache")

	return nil
}

// Delete removes a cached embedding
func (r *RedisEmbeddingCache) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete from Redis: %w", err)
	}

	r.log.WithField("key", key).Debug("Deleted embedding from Redis cache")
	return nil
}

// Clear removes all cached embeddings
func (r *RedisEmbeddingCache) Clear(ctx context.Context) error {
	err := r.client.FlushDB(ctx).Err()
	if err != nil {
		return fmt.Errorf("failed to clear Redis cache: %w", err)
	}

	// Reset statistics
	atomic.StoreInt64(&r.hits, 0)
	atomic.StoreInt64(&r.misses, 0)

	r.log.Info("Cleared Redis embedding cache")
	return nil
}

// GetStats returns cache statistics
func (r *RedisEmbeddingCache) GetStats(ctx context.Context) CacheStats {
	hits := atomic.LoadInt64(&r.hits)
	misses := atomic.LoadInt64(&r.misses)

	// Get memory usage from Redis INFO command
	memoryInfo, err := r.client.Info(ctx, "memory").Result()
	var memoryUsed int64
	if err == nil {
		// Parse used_memory from Redis INFO output
		// This is a simplified parsing - in production, use proper parsing
		memoryUsed = r.parseMemoryUsage(memoryInfo)
	}

	// Get total keys count
	totalKeys, err := r.client.DBSize(ctx).Result()
	if err != nil {
		r.log.WithError(err).Warn("Failed to get Redis DB size")
		totalKeys = 0
	}

	stats := CacheStats{
		Hits:       hits,
		Misses:     misses,
		TotalKeys:  totalKeys,
		CacheType:  "redis",
		MemoryUsed: memoryUsed,
	}

	stats.CalculateHitRate()
	return stats
}

// parseMemoryUsage extracts used_memory from Redis INFO output
func (r *RedisEmbeddingCache) parseMemoryUsage(info string) int64 {
	// Parse Redis INFO output for used_memory
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory:") {
			// Extract memory value after the colon
			if parts := strings.Split(line, ":"); len(parts) == 2 {
				if value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
					return value
				}
			}
		}
	}
	return 0
}

// Close closes the Redis connection
func (r *RedisEmbeddingCache) Close() error {
	err := r.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close Redis connection: %w", err)
	}

	r.log.Info("Closed Redis embedding cache connection")
	return nil
}

// HealthCheck performs a health check on the Redis connection
func (r *RedisEmbeddingCache) HealthCheck(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
}
