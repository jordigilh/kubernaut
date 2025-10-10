<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package k8s

import (
	"sync"
	"time"
)

// ServiceCache provides caching for discovered services
// Business Requirement: BR-HOLMES-021 - Service discovery result caching
type ServiceCache struct {
	services    map[string]*DetectedService
	ttl         time.Duration
	maxTTL      time.Duration
	mu          sync.RWMutex
	hitCount    int64
	missCount   int64
	lastCleanup time.Time
}

// NewServiceCache creates a new service cache
func NewServiceCache(ttl, maxTTL time.Duration) *ServiceCache {
	cache := &ServiceCache{
		services:    make(map[string]*DetectedService),
		ttl:         ttl,
		maxTTL:      maxTTL,
		lastCleanup: time.Now(),
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// SetService stores a service in the cache
func (sc *ServiceCache) SetService(service *DetectedService) {
	if service == nil {
		return
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	key := sc.getServiceKey(service.Namespace, service.Name)
	service.LastChecked = time.Now()
	sc.services[key] = service
}

// GetService retrieves a service from the cache
func (sc *ServiceCache) GetService(key string) *DetectedService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	service, exists := sc.services[key]
	if !exists {
		sc.missCount++
		return nil
	}

	// Check if service has expired
	if sc.isExpired(service) {
		sc.missCount++
		return nil
	}

	sc.hitCount++
	return service
}

// GetServiceByNamespace retrieves a service by namespace and name
func (sc *ServiceCache) GetServiceByNamespace(namespace, name string) *DetectedService {
	key := sc.getServiceKey(namespace, name)
	return sc.GetService(key)
}

// GetAllServices returns all non-expired services from the cache
func (sc *ServiceCache) GetAllServices() map[string]*DetectedService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	result := make(map[string]*DetectedService)
	for key, service := range sc.services {
		if !sc.isExpired(service) {
			result[key] = service
		}
	}

	return result
}

// GetServicesByType returns all services of a specific type
func (sc *ServiceCache) GetServicesByType(serviceType string) []*DetectedService {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	var services []*DetectedService
	for _, service := range sc.services {
		if !sc.isExpired(service) && service.ServiceType == serviceType {
			services = append(services, service)
		}
	}

	return services
}

// RemoveService removes a service from the cache
func (sc *ServiceCache) RemoveService(key string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	delete(sc.services, key)
}

// GetHitRate returns the cache hit rate
// Business Requirement: BR-PERF-014 - Service discovery cache hit rate monitoring
func (sc *ServiceCache) GetHitRate() float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	total := sc.hitCount + sc.missCount
	if total == 0 {
		return 0.0
	}

	return float64(sc.hitCount) / float64(total)
}

// GetStats returns cache statistics
func (sc *ServiceCache) GetStats() CacheStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return CacheStats{
		HitCount:     sc.hitCount,
		MissCount:    sc.missCount,
		HitRate:      sc.GetHitRate(),
		ServiceCount: len(sc.services),
		LastCleanup:  sc.lastCleanup,
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	HitCount     int64     `json:"hit_count"`
	MissCount    int64     `json:"miss_count"`
	HitRate      float64   `json:"hit_rate"`
	ServiceCount int       `json:"service_count"`
	LastCleanup  time.Time `json:"last_cleanup"`
}

// Clear removes all services from the cache
func (sc *ServiceCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.services = make(map[string]*DetectedService)
	sc.hitCount = 0
	sc.missCount = 0
}

// getServiceKey creates a cache key for a service
func (sc *ServiceCache) getServiceKey(namespace, name string) string {
	return namespace + "/" + name
}

// isExpired checks if a service has expired
func (sc *ServiceCache) isExpired(service *DetectedService) bool {
	if service == nil {
		return true
	}

	// Use different TTL based on service availability
	ttl := sc.ttl
	if !service.Available {
		// Expire unhealthy services faster
		ttl = sc.ttl / 2
	}

	return time.Since(service.LastChecked) > ttl
}

// cleanupExpired removes expired services from the cache
func (sc *ServiceCache) cleanupExpired() {
	// Ensure minimum ticker interval to prevent panic
	cleanupInterval := sc.ttl / 2
	if cleanupInterval <= 0 {
		cleanupInterval = 30 * time.Second // Default minimum cleanup interval
	}

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		sc.performCleanup()
	}
}

// performCleanup performs the actual cleanup of expired services
func (sc *ServiceCache) performCleanup() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	now := time.Now()
	keysToDelete := make([]string, 0)

	for key, service := range sc.services {
		if sc.isExpiredUnsafe(service, now) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(sc.services, key)
	}

	sc.lastCleanup = now

	if len(keysToDelete) > 0 {
		// Reset hit/miss counters periodically to prevent overflow
		if sc.hitCount > 10000 || sc.missCount > 10000 {
			sc.hitCount = sc.hitCount / 2
			sc.missCount = sc.missCount / 2
		}
	}
}

// isExpiredUnsafe checks if a service has expired without locking (internal use only)
func (sc *ServiceCache) isExpiredUnsafe(service *DetectedService, now time.Time) bool {
	if service == nil {
		return true
	}

	ttl := sc.ttl
	if !service.Available {
		ttl = sc.ttl / 2
	}

	return now.Sub(service.LastChecked) > ttl
}
