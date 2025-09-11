package holmesgpt

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ToolsetConfigCache provides caching for toolset configurations
// Business Requirement: BR-HOLMES-021 - Toolset configuration caching
type ToolsetConfigCache struct {
	toolsets    map[string]*ToolsetConfig
	ttl         time.Duration
	mu          sync.RWMutex
	hitCount    int64
	missCount   int64
	log         *logrus.Logger
	lastCleanup time.Time
}

// NewToolsetConfigCache creates a new toolset configuration cache
func NewToolsetConfigCache(ttl time.Duration, log *logrus.Logger) *ToolsetConfigCache {
	cache := &ToolsetConfigCache{
		toolsets:    make(map[string]*ToolsetConfig),
		ttl:         ttl,
		log:         log,
		lastCleanup: time.Now(),
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// SetToolset stores a toolset configuration in the cache
func (tcc *ToolsetConfigCache) SetToolset(toolset *ToolsetConfig) {
	if toolset == nil {
		return
	}

	tcc.mu.Lock()
	defer tcc.mu.Unlock()

	toolset.LastUpdated = time.Now()
	tcc.toolsets[toolset.Name] = toolset

	tcc.log.WithFields(logrus.Fields{
		"toolset_name": toolset.Name,
		"service_type": toolset.ServiceType,
	}).Debug("Cached toolset configuration")
}

// GetToolset retrieves a toolset configuration from the cache
func (tcc *ToolsetConfigCache) GetToolset(name string) *ToolsetConfig {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	toolset, exists := tcc.toolsets[name]
	if !exists {
		tcc.missCount++
		return nil
	}

	// Check if toolset has expired
	if tcc.isExpired(toolset) {
		tcc.missCount++
		return nil
	}

	tcc.hitCount++
	return toolset
}

// GetAllToolsets returns all non-expired toolset configurations
func (tcc *ToolsetConfigCache) GetAllToolsets() []*ToolsetConfig {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	var toolsets []*ToolsetConfig
	for _, toolset := range tcc.toolsets {
		if !tcc.isExpired(toolset) {
			toolsets = append(toolsets, toolset)
		}
	}

	return toolsets
}

// GetToolsetsByType returns all toolsets of a specific service type
func (tcc *ToolsetConfigCache) GetToolsetsByType(serviceType string) []*ToolsetConfig {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	var toolsets []*ToolsetConfig
	for _, toolset := range tcc.toolsets {
		if !tcc.isExpired(toolset) && toolset.ServiceType == serviceType {
			toolsets = append(toolsets, toolset)
		}
	}

	return toolsets
}

// GetEnabledToolsets returns all enabled toolset configurations
func (tcc *ToolsetConfigCache) GetEnabledToolsets() []*ToolsetConfig {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	var toolsets []*ToolsetConfig
	for _, toolset := range tcc.toolsets {
		if !tcc.isExpired(toolset) && toolset.Enabled {
			toolsets = append(toolsets, toolset)
		}
	}

	return toolsets
}

// RemoveToolset removes a toolset configuration from the cache
func (tcc *ToolsetConfigCache) RemoveToolset(name string) {
	tcc.mu.Lock()
	defer tcc.mu.Unlock()

	if _, exists := tcc.toolsets[name]; exists {
		delete(tcc.toolsets, name)
		tcc.log.WithField("toolset_name", name).Debug("Removed toolset from cache")
	}
}

// GetHitRate returns the cache hit rate
// Business Requirement: BR-PERF-014 - Cache hit rate monitoring
func (tcc *ToolsetConfigCache) GetHitRate() float64 {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	total := tcc.hitCount + tcc.missCount
	if total == 0 {
		return 0.0
	}

	return float64(tcc.hitCount) / float64(total)
}

// GetStats returns cache statistics
func (tcc *ToolsetConfigCache) GetStats() ToolsetCacheStats {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	return ToolsetCacheStats{
		Size:        len(tcc.toolsets),
		HitCount:    tcc.hitCount,
		MissCount:   tcc.missCount,
		HitRate:     tcc.GetHitRateUnsafe(),
		LastCleanup: tcc.lastCleanup,
		TTL:         tcc.ttl,
	}
}

// ToolsetCacheStats represents cache statistics
type ToolsetCacheStats struct {
	Size        int           `json:"size"`
	HitCount    int64         `json:"hit_count"`
	MissCount   int64         `json:"miss_count"`
	HitRate     float64       `json:"hit_rate"`
	LastCleanup time.Time     `json:"last_cleanup"`
	TTL         time.Duration `json:"ttl"`
}

// Clear removes all toolset configurations from the cache
func (tcc *ToolsetConfigCache) Clear() {
	tcc.mu.Lock()
	defer tcc.mu.Unlock()

	tcc.toolsets = make(map[string]*ToolsetConfig)
	tcc.hitCount = 0
	tcc.missCount = 0
	tcc.log.Info("Cleared toolset configuration cache")
}

// UpdateToolsetEnabled updates the enabled status of a toolset
func (tcc *ToolsetConfigCache) UpdateToolsetEnabled(name string, enabled bool) bool {
	tcc.mu.Lock()
	defer tcc.mu.Unlock()

	toolset, exists := tcc.toolsets[name]
	if !exists || tcc.isExpired(toolset) {
		return false
	}

	toolset.Enabled = enabled
	toolset.LastUpdated = time.Now()

	tcc.log.WithFields(logrus.Fields{
		"toolset_name": name,
		"enabled":      enabled,
	}).Debug("Updated toolset enabled status")

	return true
}

// isExpired checks if a toolset configuration has expired (internal use only)
func (tcc *ToolsetConfigCache) isExpired(toolset *ToolsetConfig) bool {
	if toolset == nil {
		return true
	}

	// Baseline toolsets (kubernetes, internet) never expire
	if toolset.ServiceType == "kubernetes" || toolset.ServiceType == "internet" {
		return false
	}

	return time.Since(toolset.LastUpdated) > tcc.ttl
}

// GetHitRateUnsafe returns the cache hit rate without locking (internal use only)
func (tcc *ToolsetConfigCache) GetHitRateUnsafe() float64 {
	total := tcc.hitCount + tcc.missCount
	if total == 0 {
		return 0.0
	}
	return float64(tcc.hitCount) / float64(total)
}

// cleanupExpired removes expired toolset configurations from the cache
func (tcc *ToolsetConfigCache) cleanupExpired() {
	ticker := time.NewTicker(tcc.ttl / 2) // Cleanup more frequently than TTL
	defer ticker.Stop()

	for range ticker.C {
		tcc.performCleanup()
	}
}

// performCleanup performs the actual cleanup of expired toolsets
func (tcc *ToolsetConfigCache) performCleanup() {
	tcc.mu.Lock()
	defer tcc.mu.Unlock()

	now := time.Now()
	keysToDelete := make([]string, 0)

	for name, toolset := range tcc.toolsets {
		if tcc.isExpiredUnsafe(toolset, now) {
			keysToDelete = append(keysToDelete, name)
		}
	}

	for _, key := range keysToDelete {
		delete(tcc.toolsets, key)
		tcc.log.WithField("toolset_name", key).Debug("Removed expired toolset from cache")
	}

	tcc.lastCleanup = now

	if len(keysToDelete) > 0 {
		tcc.log.WithField("removed_count", len(keysToDelete)).Debug("Cleaned up expired toolsets")

		// Reset hit/miss counters periodically to prevent overflow
		if tcc.hitCount > 10000 || tcc.missCount > 10000 {
			tcc.hitCount = tcc.hitCount / 2
			tcc.missCount = tcc.missCount / 2
		}
	}
}

// isExpiredUnsafe checks if a toolset has expired without locking (internal use only)
func (tcc *ToolsetConfigCache) isExpiredUnsafe(toolset *ToolsetConfig, now time.Time) bool {
	if toolset == nil {
		return true
	}

	// Baseline toolsets never expire
	if toolset.ServiceType == "kubernetes" || toolset.ServiceType == "internet" {
		return false
	}

	return now.Sub(toolset.LastUpdated) > tcc.ttl
}

// GetToolsetsByPriority returns toolsets sorted by priority
// Business Requirement: BR-HOLMES-024 - Toolset priority ordering
func (tcc *ToolsetConfigCache) GetToolsetsByPriority() []*ToolsetConfig {
	toolsets := tcc.GetAllToolsets()
	return SortToolsetsByPriority(toolsets)
}

// GetAvailableCapabilities returns all unique capabilities across toolsets
func (tcc *ToolsetConfigCache) GetAvailableCapabilities() []string {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	capabilitySet := make(map[string]bool)
	for _, toolset := range tcc.toolsets {
		if !tcc.isExpired(toolset) && toolset.Enabled {
			for _, capability := range toolset.Capabilities {
				capabilitySet[capability] = true
			}
		}
	}

	capabilities := make([]string, 0, len(capabilitySet))
	for capability := range capabilitySet {
		capabilities = append(capabilities, capability)
	}

	return capabilities
}

// FindToolsetsByCapability returns toolsets that have a specific capability
func (tcc *ToolsetConfigCache) FindToolsetsByCapability(capability string) []*ToolsetConfig {
	tcc.mu.RLock()
	defer tcc.mu.RUnlock()

	var matchingToolsets []*ToolsetConfig
	for _, toolset := range tcc.toolsets {
		if !tcc.isExpired(toolset) && toolset.Enabled {
			for _, toolsetCapability := range toolset.Capabilities {
				if toolsetCapability == capability {
					matchingToolsets = append(matchingToolsets, toolset)
					break
				}
			}
		}
	}

	// Sort by priority
	return SortToolsetsByPriority(matchingToolsets)
}
