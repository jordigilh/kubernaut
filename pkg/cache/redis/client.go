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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Client provides a Redis client with graceful degradation and lazy connection.
//
// This client implements the following patterns:
// - Lazy connection: Only connects when first operation is attempted
// - Graceful degradation: Returns errors instead of panicking when Redis unavailable
// - Automatic recovery: Reconnects automatically when Redis becomes available
// - Thread-safe: Safe for concurrent use across multiple goroutines
//
// Design Decision: DD-CACHE-001 (Shared Redis Library)
// Extracted from Gateway's DeduplicationService to provide reusable Redis patterns
// across multiple services (Gateway, Data Storage).
//
// Key features:
// - Double-checked locking for connection establishment (prevents thundering herd)
// - Atomic boolean for fast-path connection checks (~0.1μs)
// - Structured logging for connection events
//
// Business Requirements:
// - BR-GATEWAY-013: Gateway must start even when Redis temporarily unavailable
// - BR-STORAGE-014: Data Storage must cache embeddings with graceful degradation
type Client struct {
	client      *redis.Client
	logger      *zap.Logger
	connected   atomic.Bool  // Track Redis connection state (for lazy connection)
	connCheckMu sync.Mutex   // Protects connection check (prevent thundering herd)
}

// NewClient creates a new Redis client with lazy connection.
//
// The client will not connect to Redis until the first operation (EnsureConnection).
// This allows services to start even when Redis is temporarily unavailable.
//
// Parameters:
//   - opts: Redis connection options (addr, password, db, timeouts, pool size)
//   - logger: Structured logger for connection events and errors
//
// Returns:
//   - *Client: Redis client ready for use (connection established on first operation)
//
// Example:
//
//	opts := &redis.Options{
//	    Addr:     "localhost:6379",
//	    Password: "",
//	    DB:       0,
//	}
//	client := NewClient(opts, logger)
//	defer client.Close()
func NewClient(opts *redis.Options, logger *zap.Logger) *Client {
	return &Client{
		client: redis.NewClient(opts),
		logger: logger,
	}
}

// EnsureConnection verifies Redis is available using lazy connection pattern.
//
// This method implements graceful degradation for Redis failures:
// 1. Fast path: If already connected, return immediately (no Redis call)
// 2. Slow path: If not connected, try to ping Redis
// 3. On success: Mark as connected (subsequent calls use fast path)
// 4. On failure: Return error (caller implements graceful degradation)
//
// Concurrency: Uses double-checked locking to prevent thundering herd
// Performance: Fast path is ~0.1μs (atomic load), slow path is ~1-3ms (Redis ping)
//
// This pattern allows services to:
// - Start even when Redis is temporarily unavailable
// - Recover automatically when Redis becomes available
// - Handle Redis failures gracefully without crashing
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//
// Returns:
//   - error: nil if connected, error if Redis unavailable
//
// Example:
//
//	if err := client.EnsureConnection(ctx); err != nil {
//	    // Graceful degradation: proceed without cache
//	    logger.Warn("Redis unavailable, proceeding without cache", zap.Error(err))
//	    return nil
//	}
func (c *Client) EnsureConnection(ctx context.Context) error {
	// Fast path: already connected
	if c.connected.Load() {
		return nil
	}

	// Slow path: need to check connection
	c.connCheckMu.Lock()
	defer c.connCheckMu.Unlock()

	// Double-check after acquiring lock (another goroutine might have connected)
	if c.connected.Load() {
		return nil
	}

	// Try to connect
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis unavailable: %w", err)
	}

	// Mark as connected (enables fast path for future calls)
	c.connected.Store(true)
	c.logger.Info("Redis connection established")
	return nil
}

// GetClient returns the underlying go-redis client for advanced operations.
//
// Use this method when you need direct access to Redis commands not provided
// by the Cache[T] abstraction (e.g., HSET, ZADD, pub/sub).
//
// WARNING: Caller is responsible for calling EnsureConnection before using the client.
//
// Returns:
//   - *redis.Client: Underlying go-redis client
//
// Example:
//
//	if err := client.EnsureConnection(ctx); err != nil {
//	    return err
//	}
//	redisClient := client.GetClient()
//	result, err := redisClient.HIncrBy(ctx, "key", "field", 1).Result()
func (c *Client) GetClient() *redis.Client {
	return c.client
}

// Close closes the Redis connection and releases resources.
//
// This method should be called when the client is no longer needed (e.g., service shutdown).
// After calling Close, the client should not be used.
//
// Returns:
//   - error: nil if closed successfully, error if close failed
//
// Example:
//
//	client := NewClient(opts, logger)
//	defer client.Close()
func (c *Client) Close() error {
	if err := c.client.Close(); err != nil {
		c.logger.Error("Failed to close Redis client", zap.Error(err))
		return fmt.Errorf("failed to close redis client: %w", err)
	}
	c.connected.Store(false)
	c.logger.Info("Redis client closed")
	return nil
}

