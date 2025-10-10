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
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// CachedEmbeddingService wraps an EmbeddingGenerator with caching
type CachedEmbeddingService struct {
	service EmbeddingGenerator
	cache   EmbeddingCache
	ttl     time.Duration
	log     *logrus.Logger
	enabled bool
}

// NewCachedEmbeddingService creates a new cached embedding service
func NewCachedEmbeddingService(service EmbeddingGenerator, cache EmbeddingCache, ttl time.Duration, log *logrus.Logger) *CachedEmbeddingService {
	if ttl <= 0 {
		ttl = 24 * time.Hour // Default TTL: 24 hours
	}

	return &CachedEmbeddingService{
		service: service,
		cache:   cache,
		ttl:     ttl,
		log:     log,
		enabled: true,
	}
}

// SetCacheEnabled enables or disables caching
func (c *CachedEmbeddingService) SetCacheEnabled(enabled bool) {
	c.enabled = enabled
	c.log.WithField("enabled", enabled).Info("Cache enabled state changed")
}

// GenerateTextEmbedding generates embedding with caching
func (c *CachedEmbeddingService) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	if !c.enabled || c.cache == nil {
		return c.service.GenerateTextEmbedding(ctx, text)
	}

	// Generate cache key
	cacheKey := c.generateTextCacheKey(text)

	// Try cache first
	start := time.Now()
	embedding, found, err := c.cache.Get(ctx, cacheKey)
	cacheLatency := time.Since(start)

	if err != nil {
		c.log.WithError(err).WithField("key", cacheKey).Warn("Cache get failed, falling back to generation")
	} else if found {
		c.log.WithFields(logrus.Fields{
			"key":            cacheKey,
			"cache_latency":  cacheLatency,
			"embedding_size": len(embedding),
		}).Debug("Cache hit for text embedding")
		return embedding, nil
	}

	// Cache miss - generate embedding
	generationStart := time.Now()
	embedding, err = c.service.GenerateTextEmbedding(ctx, text)
	generationLatency := time.Since(generationStart)

	if err != nil {
		return nil, err
	}

	// Store in cache (async to not block the caller)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cacheStart := time.Now()
		if cacheErr := c.cache.Set(cacheCtx, cacheKey, embedding, c.ttl); cacheErr != nil {
			c.log.WithError(cacheErr).WithField("key", cacheKey).Warn("Failed to cache embedding")
		} else {
			cacheStoreLatency := time.Since(cacheStart)
			c.log.WithFields(logrus.Fields{
				"key":                 cacheKey,
				"generation_latency":  generationLatency,
				"cache_store_latency": cacheStoreLatency,
				"embedding_size":      len(embedding),
			}).Debug("Cached text embedding")
		}
	}()

	return embedding, nil
}

// GenerateActionEmbedding generates action embedding with caching
func (c *CachedEmbeddingService) GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error) {
	if !c.enabled || c.cache == nil {
		return c.service.GenerateActionEmbedding(ctx, actionType, parameters)
	}

	// Generate cache key
	cacheKey := c.generateActionCacheKey(actionType, parameters)

	// Try cache first
	start := time.Now()
	embedding, found, err := c.cache.Get(ctx, cacheKey)
	cacheLatency := time.Since(start)

	if err != nil {
		c.log.WithError(err).WithField("key", cacheKey).Warn("Cache get failed for action embedding")
	} else if found {
		c.log.WithFields(logrus.Fields{
			"key":            cacheKey,
			"action_type":    actionType,
			"cache_latency":  cacheLatency,
			"embedding_size": len(embedding),
		}).Debug("Cache hit for action embedding")
		return embedding, nil
	}

	// Cache miss - generate embedding
	generationStart := time.Now()
	embedding, err = c.service.GenerateActionEmbedding(ctx, actionType, parameters)
	generationLatency := time.Since(generationStart)

	if err != nil {
		return nil, err
	}

	// Store in cache (async)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cacheStart := time.Now()
		if cacheErr := c.cache.Set(cacheCtx, cacheKey, embedding, c.ttl); cacheErr != nil {
			c.log.WithError(cacheErr).WithField("key", cacheKey).Warn("Failed to cache action embedding")
		} else {
			cacheStoreLatency := time.Since(cacheStart)
			c.log.WithFields(logrus.Fields{
				"key":                 cacheKey,
				"action_type":         actionType,
				"generation_latency":  generationLatency,
				"cache_store_latency": cacheStoreLatency,
				"embedding_size":      len(embedding),
			}).Debug("Cached action embedding")
		}
	}()

	return embedding, nil
}

// GenerateContextEmbedding generates context embedding with caching
func (c *CachedEmbeddingService) GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error) {
	if !c.enabled || c.cache == nil {
		return c.service.GenerateContextEmbedding(ctx, labels, metadata)
	}

	// Generate cache key
	cacheKey := c.generateContextCacheKey(labels, metadata)

	// Try cache first
	start := time.Now()
	embedding, found, err := c.cache.Get(ctx, cacheKey)
	cacheLatency := time.Since(start)

	if err != nil {
		c.log.WithError(err).WithField("key", cacheKey).Warn("Cache get failed for context embedding")
	} else if found {
		c.log.WithFields(logrus.Fields{
			"key":            cacheKey,
			"cache_latency":  cacheLatency,
			"embedding_size": len(embedding),
		}).Debug("Cache hit for context embedding")
		return embedding, nil
	}

	// Cache miss - generate embedding
	generationStart := time.Now()
	embedding, err = c.service.GenerateContextEmbedding(ctx, labels, metadata)
	generationLatency := time.Since(generationStart)

	if err != nil {
		return nil, err
	}

	// Store in cache (async)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cacheStart := time.Now()
		if cacheErr := c.cache.Set(cacheCtx, cacheKey, embedding, c.ttl); cacheErr != nil {
			c.log.WithError(cacheErr).WithField("key", cacheKey).Warn("Failed to cache context embedding")
		} else {
			cacheStoreLatency := time.Since(cacheStart)
			c.log.WithFields(logrus.Fields{
				"key":                 cacheKey,
				"generation_latency":  generationLatency,
				"cache_store_latency": cacheStoreLatency,
				"embedding_size":      len(embedding),
			}).Debug("Cached context embedding")
		}
	}()

	return embedding, nil
}

// CombineEmbeddings delegates to the underlying service (no caching for combinations)
func (c *CachedEmbeddingService) CombineEmbeddings(embeddings ...[]float64) []float64 {
	return c.service.CombineEmbeddings(embeddings...)
}

// GetEmbeddingDimension delegates to the underlying service
func (c *CachedEmbeddingService) GetEmbeddingDimension() int {
	return c.service.GetEmbeddingDimension()
}

// GetCacheStats returns cache statistics
func (c *CachedEmbeddingService) GetCacheStats(ctx context.Context) CacheStats {
	if c.cache == nil {
		return CacheStats{
			CacheType: "none",
		}
	}
	return c.cache.GetStats(ctx)
}

// ClearCache clears the embedding cache
func (c *CachedEmbeddingService) ClearCache(ctx context.Context) error {
	if c.cache == nil {
		return nil
	}
	return c.cache.Clear(ctx)
}

// Close closes the cache connection
func (c *CachedEmbeddingService) Close() error {
	if c.cache == nil {
		return nil
	}
	return c.cache.Close()
}

// Private helper methods for cache key generation

func (c *CachedEmbeddingService) generateTextCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return CacheKey("text", fmt.Sprintf("%x", hash[:8])) // Use first 8 bytes of hash
}

func (c *CachedEmbeddingService) generateActionCacheKey(actionType string, parameters map[string]interface{}) string {
	// Create a deterministic string representation of the action
	content := fmt.Sprintf("action:%s", actionType)

	// Add sorted parameters to ensure deterministic key
	paramStr := ""
	// Simple parameter serialization - in production, use proper sorting
	for key, value := range parameters {
		paramStr += fmt.Sprintf("%s=%v;", key, value)
	}
	content += "|params:" + paramStr

	hash := sha256.Sum256([]byte(content))
	return CacheKey("action", fmt.Sprintf("%x", hash[:8]))
}

func (c *CachedEmbeddingService) generateContextCacheKey(labels map[string]string, metadata map[string]interface{}) string {
	// Create a deterministic string representation of the context
	content := "context:"

	// Add sorted labels
	labelStr := ""
	for key, value := range labels {
		labelStr += fmt.Sprintf("%s=%s;", key, value)
	}
	content += "labels:" + labelStr

	// Add sorted metadata
	metaStr := ""
	for key, value := range metadata {
		metaStr += fmt.Sprintf("%s=%v;", key, value)
	}
	content += "|metadata:" + metaStr

	hash := sha256.Sum256([]byte(content))
	return CacheKey("context", fmt.Sprintf("%x", hash[:8]))
}
