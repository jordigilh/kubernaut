package vector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// cacheEntry represents a cached embedding with expiration
type cacheEntry struct {
	embedding []float64
	expiresAt time.Time
}

// MemoryEmbeddingCache implements EmbeddingCache using in-memory storage
type MemoryEmbeddingCache struct {
	cache   map[string]*cacheEntry
	mutex   sync.RWMutex
	log     *logrus.Logger
	maxSize int

	// Statistics (atomic operations for thread-safety)
	hits   int64
	misses int64

	// Cleanup goroutine control
	stopCh chan struct{}
	done   chan struct{}
	closed bool
}

// NewMemoryEmbeddingCache creates a new in-memory embedding cache
func NewMemoryEmbeddingCache(maxSize int, log *logrus.Logger) *MemoryEmbeddingCache {
	if maxSize <= 0 {
		maxSize = 1000 // Default max size
	}

	cache := &MemoryEmbeddingCache{
		cache:   make(map[string]*cacheEntry),
		log:     log,
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupExpiredEntries()

	log.WithField("max_size", maxSize).Info("Created memory embedding cache")

	return cache
}

// Get retrieves a cached embedding by key
func (m *MemoryEmbeddingCache) Get(ctx context.Context, key string) ([]float64, bool, error) {
	m.mutex.RLock()
	entry, exists := m.cache[key]
	m.mutex.RUnlock()

	if !exists {
		atomic.AddInt64(&m.misses, 1)
		return nil, false, nil
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Remove expired entry
		m.mutex.Lock()
		delete(m.cache, key)
		m.mutex.Unlock()

		atomic.AddInt64(&m.misses, 1)
		return nil, false, nil
	}

	atomic.AddInt64(&m.hits, 1)

	// Return a copy to avoid external modifications
	embeddingCopy := make([]float64, len(entry.embedding))
	copy(embeddingCopy, entry.embedding)

	m.log.WithFields(logrus.Fields{
		"key":            key,
		"embedding_size": len(embeddingCopy),
	}).Debug("Cache hit: retrieved embedding from memory")

	return embeddingCopy, true, nil
}

// Set stores an embedding in cache with TTL
func (m *MemoryEmbeddingCache) Set(ctx context.Context, key string, embedding []float64, ttl time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if we need to evict entries (simple LRU-style eviction)
	if len(m.cache) >= m.maxSize {
		// Evict at least 1 entry, but prefer 25% of cache size
		evictCount := m.maxSize / 4
		if evictCount < 1 {
			evictCount = 1
		}
		m.evictOldestEntries(evictCount)
	}

	// Calculate expiration time
	var expiresAt time.Time
	if ttl < 0 {
		// Negative TTL means immediate expiration (set to past time)
		expiresAt = time.Now().Add(-time.Hour)
	} else if ttl == 0 {
		// Zero TTL means no expiration (set to far future time)
		expiresAt = time.Now().Add(365 * 24 * time.Hour)
	} else {
		// Positive TTL - normal expiration
		expiresAt = time.Now().Add(ttl)
	}

	// Store a copy to avoid external modifications
	embeddingCopy := make([]float64, len(embedding))
	copy(embeddingCopy, embedding)

	m.cache[key] = &cacheEntry{
		embedding: embeddingCopy,
		expiresAt: expiresAt,
	}

	m.log.WithFields(logrus.Fields{
		"key":            key,
		"embedding_size": len(embedding),
		"ttl_seconds":    ttl.Seconds(),
		"cache_size":     len(m.cache),
	}).Debug("Stored embedding in memory cache")

	return nil
}

// Delete removes a cached embedding
func (m *MemoryEmbeddingCache) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.cache, key)

	m.log.WithField("key", key).Debug("Deleted embedding from memory cache")
	return nil
}

// Clear removes all cached embeddings
func (m *MemoryEmbeddingCache) Clear(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.cache = make(map[string]*cacheEntry)

	// Reset statistics
	atomic.StoreInt64(&m.hits, 0)
	atomic.StoreInt64(&m.misses, 0)

	m.log.Info("Cleared memory embedding cache")
	return nil
}

// GetStats returns cache statistics
func (m *MemoryEmbeddingCache) GetStats(ctx context.Context) CacheStats {
	m.mutex.RLock()
	totalKeys := int64(len(m.cache))

	// Calculate approximate memory usage
	var memoryUsed int64
	for _, entry := range m.cache {
		memoryUsed += int64(len(entry.embedding)) * 8 // 8 bytes per float64
		memoryUsed += 50                              // Approximate overhead per entry
	}
	m.mutex.RUnlock()

	hits := atomic.LoadInt64(&m.hits)
	misses := atomic.LoadInt64(&m.misses)

	stats := CacheStats{
		Hits:       hits,
		Misses:     misses,
		TotalKeys:  totalKeys,
		CacheType:  "memory",
		MemoryUsed: memoryUsed,
	}

	stats.CalculateHitRate()
	return stats
}

// Close closes the memory cache (stops cleanup goroutine)
func (m *MemoryEmbeddingCache) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil // Already closed
	}

	m.closed = true
	close(m.stopCh)
	<-m.done

	m.log.Info("Closed memory embedding cache")
	return nil
}

// evictOldestEntries removes the specified number of oldest entries
// This is a simplified LRU implementation - in production, use proper LRU with access tracking
func (m *MemoryEmbeddingCache) evictOldestEntries(count int) {
	if count <= 0 || len(m.cache) == 0 {
		return
	}

	// Simple eviction strategy: remove entries that expire soonest
	type keyExpiry struct {
		key       string
		expiresAt time.Time
	}

	var entries []keyExpiry
	for key, entry := range m.cache {
		entries = append(entries, keyExpiry{
			key:       key,
			expiresAt: entry.expiresAt,
		})
	}

	// Sort by expiration time (earliest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].expiresAt.After(entries[j].expiresAt) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove the oldest entries
	evicted := 0
	for _, entry := range entries {
		if evicted >= count {
			break
		}
		delete(m.cache, entry.key)
		evicted++
	}

	if evicted > 0 {
		m.log.WithFields(logrus.Fields{
			"evicted_count": evicted,
			"cache_size":    len(m.cache),
		}).Debug("Evicted entries from memory cache")
	}
}

// cleanupExpiredEntries runs periodically to remove expired entries
func (m *MemoryEmbeddingCache) cleanupExpiredEntries() {
	defer close(m.done)

	ticker := time.NewTicker(1 * time.Minute) // Cleanup every minute
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performCleanup()
		}
	}
}

// performCleanup removes expired entries from the cache
func (m *MemoryEmbeddingCache) performCleanup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, entry := range m.cache {
		if now.After(entry.expiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(m.cache, key)
	}

	if len(expiredKeys) > 0 {
		m.log.WithFields(logrus.Fields{
			"expired_count": len(expiredKeys),
			"cache_size":    len(m.cache),
		}).Debug("Cleaned up expired entries from memory cache")
	}
}
