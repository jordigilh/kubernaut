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
	"time"
)

// EmbeddingCache defines the interface for caching embedding results
type EmbeddingCache interface {
	// Get retrieves a cached embedding by key
	Get(ctx context.Context, key string) ([]float64, bool, error)

	// Set stores an embedding in cache with TTL
	Set(ctx context.Context, key string, embedding []float64, ttl time.Duration) error

	// Delete removes a cached embedding
	Delete(ctx context.Context, key string) error

	// Clear removes all cached embeddings
	Clear(ctx context.Context) error

	// GetStats returns cache statistics
	GetStats(ctx context.Context) CacheStats

	// Close closes the cache connection
	Close() error
}

// CacheStats provides cache performance metrics
type CacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	TotalKeys  int64   `json:"total_keys"`
	CacheType  string  `json:"cache_type"`
	MemoryUsed int64   `json:"memory_used,omitempty"` // For Redis cache
}

// CalculateHitRate calculates the cache hit rate
func (s *CacheStats) CalculateHitRate() {
	total := s.Hits + s.Misses
	if total > 0 {
		s.HitRate = float64(s.Hits) / float64(total)
	}
}

// CacheKey generates a consistent cache key from input
func CacheKey(prefix string, input string) string {
	// Use simple concatenation with separator for deterministic keys
	return prefix + ":" + input
}
