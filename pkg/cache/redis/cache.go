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

package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Cache provides a generic, type-safe Redis cache with JSON serialization.
//
// This cache implementation provides:
// - Type safety: Generic type parameter T ensures compile-time type checking
// - JSON serialization: Automatic marshaling/unmarshaling of Go types
// - Key hashing: SHA256 hashing for deterministic, collision-resistant keys
// - TTL support: Automatic expiration via Redis EXPIRE command
// - Graceful degradation: Returns errors instead of panicking when Redis unavailable
//
// Design Decision: DD-CACHE-001 (Shared Redis Library)
// Extracted from Gateway patterns to provide reusable caching across services.
//
// Business Requirements:
// - BR-STORAGE-014: Data Storage must cache embeddings with graceful degradation
// - BR-GATEWAY-013: Gateway must cache deduplication data with graceful degradation
//
// Type Parameter:
//   - T: Any Go type that can be marshaled to/from JSON
//
// Example:
//
//	// Cache embeddings ([]float32)
//	embeddingCache := redis.NewCache[[]float32](client, "embeddings", 24*time.Hour)
//	embedding, err := embeddingCache.Get(ctx, "text content")
//	if err == redis.ErrCacheMiss {
//	    // Generate embedding
//	    embedding = generateEmbedding("text content")
//	    _ = embeddingCache.Set(ctx, "text content", &embedding)
//	}
type Cache[T any] struct {
	client *Client
	prefix string
	ttl    time.Duration
}

// ErrCacheMiss is returned when a key is not found in the cache.
var ErrCacheMiss = fmt.Errorf("cache miss")

// NewCache creates a new type-safe Redis cache.
//
// Parameters:
//   - client: Redis client with connection management
//   - prefix: Cache key prefix for namespacing (e.g., "embeddings", "deduplication")
//   - ttl: Time-to-live for cache entries (0 = no expiration)
//
// Returns:
//   - *Cache[T]: Type-safe cache ready for use
//
// Example:
//
//	// Cache embeddings with 24-hour TTL
//	embeddingCache := redis.NewCache[[]float32](client, "embeddings", 24*time.Hour)
//
//	// Cache deduplication data with 5-minute TTL
//	dedupCache := redis.NewCache[bool](client, "dedup", 5*time.Minute)
func NewCache[T any](client *Client, prefix string, ttl time.Duration) *Cache[T] {
	return &Cache[T]{
		client: client,
		prefix: prefix,
		ttl:    ttl,
	}
}

// Get retrieves a value from the cache.
//
// This method:
// 1. Hashes the key with SHA256 for deterministic, collision-resistant storage
// 2. Checks Redis connection (graceful degradation if unavailable)
// 3. Retrieves JSON from Redis
// 4. Unmarshals JSON to type T
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - key: Cache key (will be hashed with SHA256)
//
// Returns:
//   - *T: Pointer to cached value (nil if cache miss or error)
//   - error: ErrCacheMiss if key not found, other errors for Redis/JSON failures
//
// Example:
//
//	embedding, err := cache.Get(ctx, "text content")
//	if err == redis.ErrCacheMiss {
//	    // Cache miss: generate embedding
//	    embedding = generateEmbedding("text content")
//	    _ = cache.Set(ctx, "text content", &embedding)
//	} else if err != nil {
//	    // Redis error: proceed without cache (graceful degradation)
//	    logger.Warn("Redis unavailable", zap.Error(err))
//	    embedding = generateEmbedding("text content")
//	}
func (c *Cache[T]) Get(ctx context.Context, key string) (*T, error) {
	// Ensure Redis connection (graceful degradation if unavailable)
	if err := c.client.EnsureConnection(ctx); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	// Hash key for deterministic, collision-resistant storage
	hashedKey := c.hashKey(key)

	// Retrieve JSON from Redis
	redisClient := c.client.GetClient()
	jsonData, err := redisClient.Get(ctx, hashedKey).Result()
	if err == goredis.Nil {
		return nil, ErrCacheMiss
	} else if err != nil {
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	// Unmarshal JSON to type T
	var value T
	if err := json.Unmarshal([]byte(jsonData), &value); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	return &value, nil
}

// Set stores a value in the cache with TTL.
//
// This method:
// 1. Hashes the key with SHA256 for deterministic, collision-resistant storage
// 2. Checks Redis connection (graceful degradation if unavailable)
// 3. Marshals value to JSON
// 4. Stores JSON in Redis with TTL
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - key: Cache key (will be hashed with SHA256)
//   - value: Pointer to value to cache (must be JSON-serializable)
//
// Returns:
//   - error: nil if successful, error if Redis/JSON failures
//
// Example:
//
//	embedding := []float32{0.1, 0.2, 0.3}
//	err := cache.Set(ctx, "text content", &embedding)
//	if err != nil {
//	    // Redis error: log but don't fail (graceful degradation)
//	    logger.Warn("Failed to cache embedding", zap.Error(err))
//	}
func (c *Cache[T]) Set(ctx context.Context, key string, value *T) error {
	// Ensure Redis connection (graceful degradation if unavailable)
	if err := c.client.EnsureConnection(ctx); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}

	// Hash key for deterministic, collision-resistant storage
	hashedKey := c.hashKey(key)

	// Marshal value to JSON
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	// Store JSON in Redis with TTL
	redisClient := c.client.GetClient()
	if err := redisClient.Set(ctx, hashedKey, jsonData, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// hashKey generates a deterministic SHA256 hash of the cache key.
//
// This method provides:
// - Deterministic hashing: Same input always produces same hash
// - Collision resistance: SHA256 provides ~2^256 keyspace
// - Key normalization: Long keys become fixed-length 64-character hex strings
// - Namespace isolation: Prefix prevents key collisions between caches
//
// Key format: sha256(prefix + ":" + key)
//
// Example:
//   - Input: prefix="embeddings", key="text content"
//   - Output: "embeddings:a3c4f5..." (64-character hex string)
//
// Parameters:
//   - key: Original cache key
//
// Returns:
//   - string: Hashed key with prefix (format: "prefix:hexhash")
func (c *Cache[T]) hashKey(key string) string {
	// Combine prefix and key for namespace isolation
	fullKey := c.prefix + ":" + key

	// SHA256 hash for deterministic, collision-resistant storage
	hash := sha256.Sum256([]byte(fullKey))
	hexHash := hex.EncodeToString(hash[:])

	// Return prefixed hash for Redis storage
	return c.prefix + ":" + hexHash
}

