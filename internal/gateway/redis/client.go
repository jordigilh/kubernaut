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
	"time"

	"github.com/go-redis/redis/v8"
)

// Config holds Redis connection configuration
type Config struct {
	// Addr is the Redis server address (host:port)
	Addr string

	// Password is the Redis authentication password (optional)
	Password string

	// DB is the Redis database number (0-15)
	DB int

	// PoolSize is the maximum number of socket connections
	// High concurrency support: 100 connections for 100+ alerts/second throughput
	PoolSize int

	// MinIdleConns is the minimum number of idle connections
	MinIdleConns int

	// DialTimeout is the timeout for establishing new connections
	DialTimeout time.Duration
}

// NewClient creates a Redis client with connection pooling optimized for high throughput
//
// Configuration:
// - PoolSize: 100 connections (supports 100+ alerts/second)
// - MinIdleConns: 10 connections (reduce latency for burst traffic)
// - DialTimeout: 10ms (fast connection establishment)
//
// This configuration is based on Gateway Service performance requirements:
// - p95 < 50ms overall latency
// - Redis operations: p95 < 5ms, p99 < 10ms
// - Throughput: >100 alerts/second
func NewClient(config *Config) (*redis.Client, error) {
	// Default configuration for production use
	if config.PoolSize == 0 {
		config.PoolSize = 100
	}
	if config.MinIdleConns == 0 {
		config.MinIdleConns = 10
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 10 * time.Millisecond
	}

	client := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  config.DialTimeout,

		// Additional optimizations
		ReadTimeout:  3 * time.Second, // Matches p99 < 10ms with buffer
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second, // Wait for connection from pool

		// Connection health checks
		MaxConnAge:         0,               // No max age (keep connections alive)
		IdleTimeout:        5 * time.Minute, // Close idle connections after 5 minutes
		IdleCheckFrequency: 1 * time.Minute, // Check for idle connections every minute
	})

	// Verify connection on startup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return client, nil
}

// HealthCheck verifies Redis connectivity
// Used by /ready endpoint to check Redis dependency health
func HealthCheck(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
