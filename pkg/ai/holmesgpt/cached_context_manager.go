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

package holmesgpt

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	contextpkg "github.com/jordigilh/kubernaut/pkg/ai/context"
	"github.com/sirupsen/logrus"
)

// ContextCacheConfig defines configuration for context cache management
// Business Requirement: BR-HOLMES-008 - Context cache management
type ContextCacheConfig struct {
	MaxSize         int           `yaml:"max_size" json:"max_size"`                 // Maximum number of entries
	MaxMemory       int64         `yaml:"max_memory" json:"max_memory"`             // Maximum memory usage in bytes
	TTL             time.Duration `yaml:"ttl" json:"ttl"`                           // Time to live for entries
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"` // How often to run cleanup
}

// CachedContextManager manages context cache with size and memory limits
// Business Requirement: BR-HOLMES-008 - Context cache management with intelligent invalidation
// RACE CONDITION FIX: Comprehensive thread-safe cache with proper size limits
type CachedContextManager struct {
	config  *ContextCacheConfig
	cache   map[string]*cacheEntry
	lruList *list.List
	mutex   sync.RWMutex
	logger  *logrus.Logger

	// Atomic counters for statistics
	hitCount      int64
	missCount     int64
	evictionCount int64
	memoryUsage   int64

	// Control channels
	stopChan      chan struct{}
	cleanupTicker *time.Ticker
}

// cacheEntry represents a single cache entry with LRU tracking
type cacheEntry struct {
	key        string
	data       *contextpkg.ContextData
	size       int64
	createdAt  time.Time
	expiresAt  time.Time
	lastAccess time.Time
	lruElement *list.Element
}

// CacheStats provides cache statistics
// Business Requirement: BR-HOLMES-008 - Cache monitoring and metrics
type CacheStats struct {
	Size          int     `json:"size"`
	MaxSize       int     `json:"max_size"`
	MemoryUsage   int64   `json:"memory_usage"`
	MaxMemory     int64   `json:"max_memory"`
	HitCount      int64   `json:"hit_count"`
	MissCount     int64   `json:"miss_count"`
	EvictionCount int64   `json:"eviction_count"`
	HitRate       float64 `json:"hit_rate"`
}

// NewCachedContextManager creates a new context cache manager
// Business Requirement: BR-HOLMES-008 - Context cache with size limits
func NewCachedContextManager(config *ContextCacheConfig, logger *logrus.Logger) *CachedContextManager {
	manager := &CachedContextManager{
		config:        config,
		cache:         make(map[string]*cacheEntry),
		lruList:       list.New(),
		logger:        logger,
		stopChan:      make(chan struct{}),
		cleanupTicker: time.NewTicker(config.CleanupInterval),
	}

	// Start cleanup goroutine
	go manager.cleanupLoop()

	logger.WithFields(logrus.Fields{
		"max_size":   config.MaxSize,
		"max_memory": config.MaxMemory,
		"ttl":        config.TTL,
	}).Info("Context cache manager initialized with size limits")

	return manager
}

// Set stores context data in the cache with size and memory limit enforcement
// Business Requirement: BR-HOLMES-008 - Cache size management
func (ccm *CachedContextManager) Set(key string, data *contextpkg.ContextData, ttl time.Duration) bool {
	if data == nil {
		return false
	}

	now := time.Now()
	expiresAt := now.Add(ttl)
	entrySize := ccm.calculateContextSize(data)

	ccm.mutex.Lock()
	defer ccm.mutex.Unlock()

	// Check if adding this entry would exceed memory limit
	currentMemory := atomic.LoadInt64(&ccm.memoryUsage)
	if existingEntry, exists := ccm.cache[key]; exists {
		// Update existing entry
		ccm.removeFromLRU(existingEntry)
		atomic.AddInt64(&ccm.memoryUsage, -existingEntry.size)
		currentMemory -= existingEntry.size
	}

	if currentMemory+entrySize > ccm.config.MaxMemory {
		// Try to make space by evicting LRU entries
		if !ccm.makeSpaceForSize(entrySize) {
			ccm.logger.WithFields(logrus.Fields{
				"key":            key,
				"entry_size":     entrySize,
				"current_memory": currentMemory,
				"max_memory":     ccm.config.MaxMemory,
			}).Warn("Cannot add entry - would exceed memory limit even after eviction")
			return false
		}
	}

	// Check size limit
	if len(ccm.cache) >= ccm.config.MaxSize {
		// Evict LRU entry to make space
		if !ccm.evictLRU() {
			ccm.logger.Warn("Cannot add entry - cache is at size limit and eviction failed")
			return false
		}
	}

	// Create and add new entry
	entry := &cacheEntry{
		key:        key,
		data:       data,
		size:       entrySize,
		createdAt:  now,
		expiresAt:  expiresAt,
		lastAccess: now,
	}

	// Add to LRU list (most recent at front)
	entry.lruElement = ccm.lruList.PushFront(entry)

	// Add to cache map
	ccm.cache[key] = entry

	// Update memory usage
	atomic.AddInt64(&ccm.memoryUsage, entrySize)

	ccm.logger.WithFields(logrus.Fields{
		"key":        key,
		"size":       entrySize,
		"expires_at": expiresAt,
	}).Debug("Added entry to context cache")

	return true
}

// Get retrieves context data from the cache
// Business Requirement: BR-HOLMES-008 - Context retrieval with LRU tracking
func (ccm *CachedContextManager) Get(key string) *contextpkg.ContextData {
	ccm.mutex.Lock()
	defer ccm.mutex.Unlock()

	entry, exists := ccm.cache[key]
	if !exists {
		atomic.AddInt64(&ccm.missCount, 1)
		return nil
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		ccm.removeEntryUnsafe(key, entry)
		atomic.AddInt64(&ccm.missCount, 1)
		return nil
	}

	// Update LRU (move to front)
	entry.lastAccess = time.Now()
	ccm.lruList.MoveToFront(entry.lruElement)

	atomic.AddInt64(&ccm.hitCount, 1)
	return entry.data
}

// GetStats returns cache statistics
// Business Requirement: BR-HOLMES-008 - Cache monitoring
func (ccm *CachedContextManager) GetStats() CacheStats {
	ccm.mutex.RLock()
	defer ccm.mutex.RUnlock()

	hitCount := atomic.LoadInt64(&ccm.hitCount)
	missCount := atomic.LoadInt64(&ccm.missCount)
	total := hitCount + missCount

	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hitCount) / float64(total)
	}

	return CacheStats{
		Size:          len(ccm.cache),
		MaxSize:       ccm.config.MaxSize,
		MemoryUsage:   atomic.LoadInt64(&ccm.memoryUsage),
		MaxMemory:     ccm.config.MaxMemory,
		HitCount:      hitCount,
		MissCount:     missCount,
		EvictionCount: atomic.LoadInt64(&ccm.evictionCount),
		HitRate:       hitRate,
	}
}

// Stop gracefully shuts down the cache manager
func (ccm *CachedContextManager) Stop() {
	close(ccm.stopChan)
	ccm.cleanupTicker.Stop()

	ccm.mutex.Lock()
	defer ccm.mutex.Unlock()

	// Clear cache
	ccm.cache = make(map[string]*cacheEntry)
	ccm.lruList = list.New()
	atomic.StoreInt64(&ccm.memoryUsage, 0)

	ccm.logger.Info("Context cache manager stopped")
}

// makeSpaceForSize attempts to free up enough memory for the given size
func (ccm *CachedContextManager) makeSpaceForSize(requiredSize int64) bool {
	targetMemory := ccm.config.MaxMemory - requiredSize
	currentMemory := atomic.LoadInt64(&ccm.memoryUsage)

	evictedCount := 0
	maxEvictions := len(ccm.cache) // Prevent infinite loop

	for currentMemory > targetMemory && evictedCount < maxEvictions {
		if !ccm.evictLRU() {
			break
		}
		evictedCount++
		currentMemory = atomic.LoadInt64(&ccm.memoryUsage)
	}

	return currentMemory <= targetMemory
}

// evictLRU evicts the least recently used entry
func (ccm *CachedContextManager) evictLRU() bool {
	if ccm.lruList.Len() == 0 {
		return false
	}

	// Get least recently used entry (back of list)
	lastElement := ccm.lruList.Back()
	if lastElement == nil {
		return false
	}

	entry := lastElement.Value.(*cacheEntry)
	ccm.removeEntryUnsafe(entry.key, entry)
	atomic.AddInt64(&ccm.evictionCount, 1)

	ccm.logger.WithFields(logrus.Fields{
		"evicted_key":  entry.key,
		"evicted_size": entry.size,
		"reason":       "LRU_eviction",
	}).Debug("Evicted entry from context cache")

	return true
}

// removeEntryUnsafe removes an entry from cache (must hold mutex)
func (ccm *CachedContextManager) removeEntryUnsafe(key string, entry *cacheEntry) {
	delete(ccm.cache, key)
	ccm.removeFromLRU(entry)
	atomic.AddInt64(&ccm.memoryUsage, -entry.size)
}

// removeFromLRU removes entry from LRU list
func (ccm *CachedContextManager) removeFromLRU(entry *cacheEntry) {
	if entry.lruElement != nil {
		ccm.lruList.Remove(entry.lruElement)
		entry.lruElement = nil
	}
}

// cleanupLoop runs periodic cleanup of expired entries
func (ccm *CachedContextManager) cleanupLoop() {
	for {
		select {
		case <-ccm.stopChan:
			return
		case <-ccm.cleanupTicker.C:
			ccm.cleanupExpired()
		}
	}
}

// cleanupExpired removes expired entries
func (ccm *CachedContextManager) cleanupExpired() {
	ccm.mutex.Lock()
	defer ccm.mutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, entry := range ccm.cache {
		if now.After(entry.expiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if entry, exists := ccm.cache[key]; exists {
			ccm.removeEntryUnsafe(key, entry)
		}
	}

	if len(expiredKeys) > 0 {
		ccm.logger.WithFields(logrus.Fields{
			"expired_entries": len(expiredKeys),
		}).Debug("Cleaned up expired context cache entries")
	}
}

// calculateContextSize estimates the memory usage of context data
func (ccm *CachedContextManager) calculateContextSize(data *contextpkg.ContextData) int64 {
	size := int64(0)

	// Base struct size
	size += 64 // Approximate struct overhead

	// Kubernetes context
	if data.Kubernetes != nil {
		size += int64(len(data.Kubernetes.Namespace))
		size += int64(len(data.Kubernetes.ResourceType))
		size += int64(len(data.Kubernetes.ResourceName))
		for k, v := range data.Kubernetes.Labels {
			size += int64(len(k) + len(v))
		}
		for k, v := range data.Kubernetes.Annotations {
			size += int64(len(k) + len(v))
		}
		size += 128 // ClusterInfo overhead
	}

	// Metrics context
	if data.Metrics != nil {
		size += int64(len(data.Metrics.Source))
		size += int64(len(data.Metrics.MetricsData) * 16) // Approximate map overhead
		size += int64(len(data.Metrics.Aggregations) * 32)
		size += 64 // TimeRange overhead
	}

	// Logs context
	if data.Logs != nil {
		size += int64(len(data.Logs.Source))
		size += int64(len(data.Logs.LogLevel))
		for _, entry := range data.Logs.LogEntries {
			size += int64(len(entry.Message))
			size += int64(len(entry.Level))
			size += int64(len(entry.Source))
			for k, v := range entry.Labels {
				size += int64(len(k) + len(v))
			}
		}
		size += 64 // TimeRange overhead
	}

	// Events context
	if data.Events != nil {
		size += int64(len(data.Events.Source))
		for _, event := range data.Events.Events {
			size += int64(len(event.Type))
			size += int64(len(event.Reason))
			size += int64(len(event.Message))
			size += int64(len(event.Source))
		}
		size += int64(len(data.Events.EventTypes) * 16)
		size += 64 // TimeRange overhead
	}

	// Action history context
	if data.ActionHistory != nil {
		for _, action := range data.ActionHistory.Actions {
			size += int64(len(action.ActionID))
			size += int64(len(action.ActionType))
			size += 128 // Parameters map overhead
		}
		size += 64 // TimeRange overhead
	}

	// Other contexts (Traces, NetworkFlows, AuditLogs)
	if data.Traces != nil {
		size += int64(len(data.Traces.Source))
		size += int64(data.Traces.SpanCount * 8)
		size += 64
	}

	if data.NetworkFlows != nil {
		size += int64(len(data.NetworkFlows.Source))
		size += int64(len(data.NetworkFlows.Connections) * 64)
		size += 64
	}

	if data.AuditLogs != nil {
		size += int64(len(data.AuditLogs.Source))
		for _, event := range data.AuditLogs.AuditEvents {
			size += int64(len(event.User))
			size += int64(len(event.Action))
			size += int64(len(event.Resource))
			size += int64(len(event.Result))
		}
		size += 64
	}

	return size
}
