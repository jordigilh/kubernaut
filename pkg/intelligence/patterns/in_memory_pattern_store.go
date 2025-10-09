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

package patterns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/sirupsen/logrus"
)

// InMemoryPatternStore provides an in-memory implementation of PatternStore
// This serves as both a reference implementation and a fallback option for testing
// Business Requirements: BR-PATTERN-001 through BR-PATTERN-005 - Pattern storage and retrieval
type InMemoryPatternStore struct {
	patterns map[string]*shared.DiscoveredPattern
	mutex    sync.RWMutex
	log      *logrus.Logger
}

// NewInMemoryPatternStore creates a new in-memory pattern store
func NewInMemoryPatternStore(log *logrus.Logger) PatternStore {
	if log == nil {
		log = logrus.New()
		log.SetLevel(logrus.WarnLevel)
	}

	return &InMemoryPatternStore{
		patterns: make(map[string]*shared.DiscoveredPattern),
		log:      log,
	}
}

// StorePattern stores a discovered pattern in memory
// Business Requirement: BR-PATTERN-001 - Store discovered patterns for future reference
func (store *InMemoryPatternStore) StorePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	if pattern.ID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Create a copy to avoid external modifications
	patternCopy := *pattern
	if patternCopy.CreatedAt.IsZero() {
		patternCopy.CreatedAt = time.Now()
	}
	patternCopy.UpdatedAt = time.Now()
	patternCopy.LastSeen = time.Now()

	store.patterns[pattern.ID] = &patternCopy

	store.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_type": pattern.Type,
		"confidence":   pattern.Confidence,
		"frequency":    pattern.Frequency,
	}).Debug("Stored discovered pattern in memory")

	return nil
}

// GetPatterns retrieves patterns from memory based on filters
// Business Requirement: BR-PATTERN-002 - Retrieve patterns based on search criteria
func (store *InMemoryPatternStore) GetPatterns(ctx context.Context, filters map[string]interface{}) ([]*shared.DiscoveredPattern, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	var result []*shared.DiscoveredPattern

	// Extract limit from filters if present (not used for pattern matching)
	limit := -1
	if limitValue, exists := filters["limit"]; exists {
		if limitInt, ok := limitValue.(int); ok {
			limit = limitInt
		}
	}

	for _, pattern := range store.patterns {
		if store.matchesFilters(pattern, filters) {
			// Return copy to prevent external modifications
			patternCopy := *pattern
			result = append(result, &patternCopy)

			// Apply limit if specified
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	store.log.WithFields(logrus.Fields{
		"filters":        filters,
		"found_patterns": len(result),
		"total_patterns": len(store.patterns),
		"limit_applied":  limit,
	}).Debug("Retrieved patterns from memory")

	return result, nil
}

// UpdatePattern updates an existing pattern in memory
// Business Requirement: BR-PATTERN-003 - Update pattern information and metadata
func (store *InMemoryPatternStore) UpdatePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	if pattern.ID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.patterns[pattern.ID]; !exists {
		return fmt.Errorf("pattern with ID %s not found for update", pattern.ID)
	}

	// Create a copy and preserve original CreatedAt timestamp
	patternCopy := *pattern
	if existing := store.patterns[pattern.ID]; existing != nil {
		patternCopy.CreatedAt = existing.CreatedAt
	}
	patternCopy.UpdatedAt = time.Now()
	patternCopy.LastSeen = time.Now()

	store.patterns[pattern.ID] = &patternCopy

	store.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_type": pattern.Type,
		"confidence":   pattern.Confidence,
	}).Debug("Updated pattern in memory")

	return nil
}

// DeletePattern removes a pattern from memory
// Business Requirement: BR-PATTERN-004 - Remove outdated or invalid patterns
func (store *InMemoryPatternStore) DeletePattern(ctx context.Context, patternID string) error {
	if patternID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	if _, exists := store.patterns[patternID]; !exists {
		return fmt.Errorf("pattern with ID %s not found for deletion", patternID)
	}

	delete(store.patterns, patternID)

	store.log.WithField("pattern_id", patternID).Debug("Deleted pattern from memory")

	return nil
}

// GetPattern retrieves a single pattern by ID
// Business Requirement: BR-PATTERN-005 - Direct pattern access by identifier
func (store *InMemoryPatternStore) GetPattern(ctx context.Context, patternID string) (*shared.DiscoveredPattern, error) {
	if patternID == "" {
		return nil, fmt.Errorf("pattern ID cannot be empty")
	}

	store.mutex.RLock()
	defer store.mutex.RUnlock()

	pattern, exists := store.patterns[patternID]
	if !exists {
		return nil, fmt.Errorf("pattern with ID %s not found", patternID)
	}

	// Return copy to prevent external modifications
	patternCopy := *pattern
	return &patternCopy, nil
}

// ListPatterns retrieves all patterns of a specific type
// Business Requirement: BR-PATTERN-002 - Pattern categorization and filtering
func (store *InMemoryPatternStore) ListPatterns(ctx context.Context, patternType string) ([]*shared.DiscoveredPattern, error) {
	filters := make(map[string]interface{})
	if patternType != "" {
		filters["type"] = patternType
	}

	return store.GetPatterns(ctx, filters)
}

// GetPatternCount returns the total number of patterns stored
func (store *InMemoryPatternStore) GetPatternCount() int {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	return len(store.patterns)
}

// Clear removes all patterns from memory (useful for testing)
func (store *InMemoryPatternStore) Clear() {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.patterns = make(map[string]*shared.DiscoveredPattern)
	store.log.Debug("Cleared all patterns from memory")
}

// matchesFilters checks if a pattern matches the given filter criteria
func (store *InMemoryPatternStore) matchesFilters(pattern *shared.DiscoveredPattern, filters map[string]interface{}) bool {
	if len(filters) == 0 {
		return true
	}

	for key, value := range filters {
		// Skip query parameters that are not pattern attributes
		if key == "limit" {
			continue
		}

		switch key {
		case "type":
			if stringValue, ok := value.(string); ok && pattern.Type != stringValue {
				return false
			}
		case "confidence_min":
			if floatValue, ok := value.(float64); ok && pattern.Confidence < floatValue {
				return false
			}
		case "confidence_max":
			if floatValue, ok := value.(float64); ok && pattern.Confidence > floatValue {
				return false
			}
		case "frequency_min":
			if intValue, ok := value.(int); ok && pattern.Frequency < intValue {
				return false
			}
		case "frequency_max":
			if intValue, ok := value.(int); ok && pattern.Frequency > intValue {
				return false
			}
		case "start_time":
			if timeValue, ok := value.(time.Time); ok && pattern.LastSeen.Before(timeValue) {
				return false
			}
		case "end_time":
			if timeValue, ok := value.(time.Time); ok && pattern.LastSeen.After(timeValue) {
				return false
			}
		default:
			// For unknown filters, try to match in metadata
			if pattern.Metadata != nil {
				if metadataValue, exists := pattern.Metadata[key]; exists {
					if metadataValue != value {
						return false
					}
				} else {
					return false // Filter key not found in metadata
				}
			} else {
				return false // No metadata to match against
			}
		}
	}

	return true
}

// IsHealthy performs a health check on the in-memory pattern store
func (store *InMemoryPatternStore) IsHealthy(ctx context.Context) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	// Simple health check - ensure we can access the patterns map
	if store.patterns == nil {
		return fmt.Errorf("pattern store is not properly initialized")
	}

	return nil
}
