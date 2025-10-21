package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisClient provides L1 cache functionality
// BR-CONTEXT-002: Performance Optimization - Redis-based L1 cache
type RedisClient struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisClient creates a new Redis client
// Following Data Storage Service caching patterns
//
// BR-CONTEXT-002: Performance Optimization - connection pooling for cache
func NewRedisClient(addr string, logger *zap.Logger) (*RedisClient, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "", // No password for local dev
		DB:           0,  // Default DB
		PoolSize:     10, // Connection pool size
		MinIdleConns: 5,  // Minimum idle connections
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("Redis connection failed",
			zap.Error(err),
			zap.String("addr", addr),
		)
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	logger.Info("Redis client created successfully",
		zap.String("addr", addr),
		zap.Int("pool_size", 10),
		zap.Int("min_idle_conns", 5),
	)

	return &RedisClient{
		client: client,
		logger: logger,
	}, nil
}

// Ping verifies Redis connectivity
// BR-CONTEXT-008: REST API - health check support for cache layer
func (r *RedisClient) Ping(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	if err := r.client.Ping(ctx).Err(); err != nil {
		r.logger.Error("Redis ping failed", zap.Error(err))
		return fmt.Errorf("redis ping failed: %w", err)
	}

	return nil
}

// Close closes the Redis client connection
func (r *RedisClient) Close() error {
	if r.client == nil {
		return nil
	}

	if err := r.client.Close(); err != nil {
		r.logger.Error("Failed to close Redis connection", zap.Error(err))
		return fmt.Errorf("failed to close redis: %w", err)
	}

	r.logger.Info("Redis client closed successfully")
	return nil
}

// GetClient returns the underlying redis.Client instance
func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}
