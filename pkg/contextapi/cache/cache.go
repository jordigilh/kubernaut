// Package cache provides multi-tier caching for Context API
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
)

// Cache provides multi-tier caching (Redis L1 + LRU L2)
//
// BR-CONTEXT-003: Multi-tier caching (Redis + LRU)
type Cache interface {
	// GetIncidents retrieves cached incidents list
	GetIncidents(ctx context.Context, key string) ([]*models.IncidentEvent, int, error)

	// SetIncidents caches incidents list
	SetIncidents(ctx context.Context, key string, incidents []*models.IncidentEvent, total int, ttl time.Duration) error

	// GetIncident retrieves a single cached incident
	GetIncident(ctx context.Context, key string) (*models.IncidentEvent, error)

	// SetIncident caches a single incident
	SetIncident(ctx context.Context, key string, incident *models.IncidentEvent, ttl time.Duration) error

	// Delete removes a cache entry
	Delete(ctx context.Context, key string) error

	// Close closes cache connections
	Close() error
}

// cacheEntry represents a cached value with expiration
type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// MultiTierCache implements Cache with Redis L1 and in-memory L2
type MultiTierCache struct {
	redis      *redis.Client
	memory     map[string]*cacheEntry
	mu         sync.RWMutex
	maxSize    int
	defaultTTL time.Duration
}

// Config holds cache configuration
type Config struct {
	RedisAddr    string
	RedisDB      int
	LRUSize      int
	DefaultTTL   time.Duration
	MaxValueSize int64 // Maximum cached object size in bytes; 0=default(5MB), -1=unlimited
}

const (
	// DefaultMaxValueSize is the default maximum size for cached objects (5MB)
	// Day 11: Large object OOM prevention
	DefaultMaxValueSize = 5 * 1024 * 1024 // 5MB
)

// CachedIncidentsList wraps incidents list with total count
type CachedIncidentsList struct {
	Incidents []*models.IncidentEvent `json:"incidents"`
	Total     int                     `json:"total"`
}

// NewCache creates a new multi-tier cache
func NewCache(cfg *Config) (*MultiTierCache, error) {
	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
		DB:   cfg.RedisDB,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cache := &MultiTierCache{
		redis:      rdb,
		memory:     make(map[string]*cacheEntry),
		maxSize:    cfg.LRUSize,
		defaultTTL: cfg.DefaultTTL,
	}

	if err := rdb.Ping(ctx).Err(); err != nil {
		// Redis unavailable, use memory only
		cache.redis = nil
	}

	return cache, nil
}

// GenerateCacheKey generates a cache key from parameters
func GenerateCacheKey(prefix string, params interface{}) (string, error) {
	// Serialize params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal params: %w", err)
	}

	// Hash params
	hash := sha256.Sum256(paramsJSON)

	// Generate key
	return fmt.Sprintf("%s:%x", prefix, hash), nil
}

// GetIncidents retrieves cached incidents list
func (c *MultiTierCache) GetIncidents(ctx context.Context, key string) ([]*models.IncidentEvent, int, error) {
	// Try L2 cache (memory) first
	c.mu.RLock()
	if entry, ok := c.memory[key]; ok {
		if time.Now().Before(entry.expiresAt) {
			c.mu.RUnlock()
			var cached CachedIncidentsList
			if err := json.Unmarshal(entry.data, &cached); err == nil {
				return cached.Incidents, cached.Total, nil
			}
		} else {
			// Expired, remove it
			c.mu.RUnlock()
			c.mu.Lock()
			delete(c.memory, key)
			c.mu.Unlock()
		}
	} else {
		c.mu.RUnlock()
	}

	// Try L1 cache (Redis)
	if c.redis != nil {
		data, err := c.redis.Get(ctx, key).Bytes()
		if err == nil {
			var cached CachedIncidentsList
			if err := json.Unmarshal(data, &cached); err == nil {
				// Populate L2 cache
				c.mu.Lock()
				c.memory[key] = &cacheEntry{
					data:      data,
					expiresAt: time.Now().Add(c.defaultTTL),
				}
				c.mu.Unlock()
				return cached.Incidents, cached.Total, nil
			}
		}
		if err != redis.Nil {
			return nil, 0, fmt.Errorf("redis get error: %w", err)
		}
	}

	return nil, 0, ErrCacheMiss
}

// SetIncidents caches incidents list
func (c *MultiTierCache) SetIncidents(ctx context.Context, key string, incidents []*models.IncidentEvent, total int, ttl time.Duration) error {
	// Prepare cached data
	cached := CachedIncidentsList{
		Incidents: incidents,
		Total:     total,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("failed to marshal incidents: %w", err)
	}

	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Set in L2 cache (memory)
	c.mu.Lock()
	c.memory[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	// Set in L1 cache (Redis)
	if c.redis != nil {
		if err := c.redis.Set(ctx, key, data, ttl).Err(); err != nil {
			return fmt.Errorf("redis set error: %w", err)
		}
	}

	return nil
}

// GetIncident retrieves a single cached incident
func (c *MultiTierCache) GetIncident(ctx context.Context, key string) (*models.IncidentEvent, error) {
	// Try L2 cache (memory) first
	c.mu.RLock()
	if entry, ok := c.memory[key]; ok {
		if time.Now().Before(entry.expiresAt) {
			c.mu.RUnlock()
			var incident models.IncidentEvent
			if err := json.Unmarshal(entry.data, &incident); err == nil {
				return &incident, nil
			}
		} else {
			// Expired, remove it
			c.mu.RUnlock()
			c.mu.Lock()
			delete(c.memory, key)
			c.mu.Unlock()
		}
	} else {
		c.mu.RUnlock()
	}

	// Try L1 cache (Redis)
	if c.redis != nil {
		data, err := c.redis.Get(ctx, key).Bytes()
		if err == nil {
			var incident models.IncidentEvent
			if err := json.Unmarshal(data, &incident); err == nil {
				// Populate L2 cache
				c.mu.Lock()
				c.memory[key] = &cacheEntry{
					data:      data,
					expiresAt: time.Now().Add(c.defaultTTL),
				}
				c.mu.Unlock()
				return &incident, nil
			}
		}
		if err != redis.Nil {
			return nil, fmt.Errorf("redis get error: %w", err)
		}
	}

	return nil, ErrCacheMiss
}

// SetIncident caches a single incident
func (c *MultiTierCache) SetIncident(ctx context.Context, key string, incident *models.IncidentEvent, ttl time.Duration) error {
	data, err := json.Marshal(incident)
	if err != nil {
		return fmt.Errorf("failed to marshal incident: %w", err)
	}

	if ttl == 0 {
		ttl = c.defaultTTL
	}

	// Set in L2 cache (memory)
	c.mu.Lock()
	c.memory[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	// Set in L1 cache (Redis)
	if c.redis != nil {
		if err := c.redis.Set(ctx, key, data, ttl).Err(); err != nil {
			return fmt.Errorf("redis set error: %w", err)
		}
	}

	return nil
}

// Delete removes a cache entry
func (c *MultiTierCache) Delete(ctx context.Context, key string) error {
	// Remove from L2 cache (memory)
	c.mu.Lock()
	delete(c.memory, key)
	c.mu.Unlock()

	// Remove from L1 cache (Redis)
	if c.redis != nil {
		if err := c.redis.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("redis del error: %w", err)
		}
	}

	return nil
}

// Close closes cache connections
func (c *MultiTierCache) Close() error {
	if c.redis != nil {
		return c.redis.Close()
	}
	return nil
}
