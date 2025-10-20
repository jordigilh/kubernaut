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

package cache

import "sync/atomic"

// Stats tracks cache performance metrics
// BR-CONTEXT-005: Cache performance monitoring
type Stats struct {
	HitsL1      uint64 // Redis hits
	HitsL2      uint64 // Memory hits
	Misses      uint64 // Cache misses
	Sets        uint64 // Cache sets
	Evictions   uint64 // LRU evictions
	Errors      uint64 // Redis errors
	TotalSize   int    // Current memory cache size
	MaxSize     int    // Maximum memory cache size
	RedisStatus string // "available" or "unavailable"
}

// cacheStats holds atomic counters for cache statistics
type cacheStats struct {
	hitsL1    atomic.Uint64
	hitsL2    atomic.Uint64
	misses    atomic.Uint64
	sets      atomic.Uint64
	evictions atomic.Uint64
	errors    atomic.Uint64
}

// RecordHitL1 increments L1 (Redis) hit counter
func (s *cacheStats) RecordHitL1() {
	s.hitsL1.Add(1)
}

// RecordHitL2 increments L2 (memory) hit counter
func (s *cacheStats) RecordHitL2() {
	s.hitsL2.Add(1)
}

// RecordMiss increments miss counter
func (s *cacheStats) RecordMiss() {
	s.misses.Add(1)
}

// RecordSet increments set counter
func (s *cacheStats) RecordSet() {
	s.sets.Add(1)
}

// RecordEviction increments eviction counter
func (s *cacheStats) RecordEviction() {
	s.evictions.Add(1)
}

// RecordError increments error counter
func (s *cacheStats) RecordError() {
	s.errors.Add(1)
}

// Snapshot returns current statistics snapshot
func (s *cacheStats) Snapshot() Stats {
	return Stats{
		HitsL1:    s.hitsL1.Load(),
		HitsL2:    s.hitsL2.Load(),
		Misses:    s.misses.Load(),
		Sets:      s.sets.Load(),
		Evictions: s.evictions.Load(),
		Errors:    s.errors.Load(),
	}
}

// HitRate returns cache hit rate percentage (0-100)
func (s *Stats) HitRate() float64 {
	total := s.HitsL1 + s.HitsL2 + s.Misses
	if total == 0 {
		return 0.0
	}
	hits := s.HitsL1 + s.HitsL2
	return float64(hits) / float64(total) * 100.0
}
