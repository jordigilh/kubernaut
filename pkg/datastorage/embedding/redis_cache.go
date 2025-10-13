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

package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisCache implements the Cache interface using Redis.
// Business Requirement: BR-STORAGE-013 (Caching for performance)
type RedisCache struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisCache creates a new Redis cache client.
func NewRedisCache(client *redis.Client, logger *zap.Logger) *RedisCache {
	return &RedisCache{
		client: client,
		logger: logger,
	}
}

// Get retrieves a cached embedding by key.
func (r *RedisCache) Get(ctx context.Context, key string) ([]float32, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("cache miss")
		}
		r.logger.Error("failed to get from cache",
			zap.Error(err),
			zap.String("key", key))
		return nil, fmt.Errorf("cache get error: %w", err)
	}

	var embedding []float32
	if err := json.Unmarshal(data, &embedding); err != nil {
		r.logger.Error("failed to unmarshal cached embedding",
			zap.Error(err),
			zap.String("key", key))
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	r.logger.Debug("cache hit",
		zap.String("key", key),
		zap.Int("dimension", len(embedding)))

	return embedding, nil
}

// Set stores an embedding in the cache with TTL.
func (r *RedisCache) Set(ctx context.Context, key string, embedding []float32, ttl time.Duration) error {
	data, err := json.Marshal(embedding)
	if err != nil {
		r.logger.Error("failed to marshal embedding",
			zap.Error(err),
			zap.String("key", key))
		return fmt.Errorf("marshal error: %w", err)
	}

	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		r.logger.Error("failed to set cache",
			zap.Error(err),
			zap.String("key", key),
			zap.Duration("ttl", ttl))
		return fmt.Errorf("cache set error: %w", err)
	}

	r.logger.Debug("cached embedding",
		zap.String("key", key),
		zap.Int("dimension", len(embedding)),
		zap.Duration("ttl", ttl))

	return nil
}

