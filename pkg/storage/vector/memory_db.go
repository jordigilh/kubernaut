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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	"github.com/sirupsen/logrus"
)

// MemoryVectorDatabase is an in-memory implementation of VectorDatabase
// This serves as both a reference implementation and a fallback option
type MemoryVectorDatabase struct {
	patterns map[string]*ActionPattern
	mutex    sync.RWMutex
	log      *logrus.Logger
}

// NewMemoryVectorDatabase creates a new in-memory vector database
func NewMemoryVectorDatabase(log *logrus.Logger) *MemoryVectorDatabase {
	return &MemoryVectorDatabase{
		patterns: make(map[string]*ActionPattern),
		log:      log,
	}
}

// StoreActionPattern stores an action pattern as a vector
func (db *MemoryVectorDatabase) StoreActionPattern(ctx context.Context, pattern *ActionPattern) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if pattern.ID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	if len(pattern.Embedding) == 0 {
		return fmt.Errorf("pattern embedding cannot be empty")
	}

	// Create a copy to avoid external modifications
	patternCopy := *pattern
	patternCopy.UpdatedAt = time.Now()
	if patternCopy.CreatedAt.IsZero() {
		patternCopy.CreatedAt = patternCopy.UpdatedAt
	}

	db.patterns[pattern.ID] = &patternCopy

	logFields := logrus.Fields{
		"pattern_id":  pattern.ID,
		"action_type": pattern.ActionType,
		"alert_name":  pattern.AlertName,
	}

	if pattern.EffectivenessData != nil {
		logFields["effectiveness"] = pattern.EffectivenessData.Score
	} else {
		logFields["effectiveness"] = "nil"
	}

	db.log.WithFields(logFields).Debug("Stored action pattern in vector database")

	return nil
}

// FindSimilarPatterns finds patterns similar to the given one
func (db *MemoryVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *ActionPattern, limit int, threshold float64) ([]*SimilarPattern, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	if len(pattern.Embedding) == 0 {
		return nil, fmt.Errorf("query pattern embedding cannot be empty")
	}

	var similarities []*SimilarPattern

	for _, storedPattern := range db.patterns {
		// Skip the same pattern
		if storedPattern.ID == pattern.ID {
			continue
		}

		similarity := sharedmath.CosineSimilarity(pattern.Embedding, storedPattern.Embedding)
		if similarity >= threshold {
			similarities = append(similarities, &SimilarPattern{
				Pattern:    storedPattern,
				Similarity: similarity,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(similarities, func(i, j int) bool {
		return similarities[i].Similarity > similarities[j].Similarity
	})

	// Apply limit and add rank
	if limit > 0 && len(similarities) > limit {
		similarities = similarities[:limit]
	}

	for i, sim := range similarities {
		sim.Rank = i + 1
	}

	db.log.WithFields(logrus.Fields{
		"query_pattern":        pattern.ID,
		"found_patterns":       len(similarities),
		"similarity_threshold": threshold,
	}).Debug("Found similar patterns")

	return similarities, nil
}

// UpdatePatternEffectiveness updates the effectiveness score of a stored pattern
func (db *MemoryVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	pattern, exists := db.patterns[patternID]
	if !exists {
		return fmt.Errorf("pattern with ID %s not found", patternID)
	}

	if pattern.EffectivenessData == nil {
		pattern.EffectivenessData = &EffectivenessData{}
	}

	pattern.EffectivenessData.Score = effectiveness
	pattern.EffectivenessData.LastAssessed = time.Now()
	pattern.UpdatedAt = time.Now()

	db.log.WithFields(logrus.Fields{
		"pattern_id":    patternID,
		"effectiveness": effectiveness,
	}).Debug("Updated pattern effectiveness")

	return nil
}

// SearchBySemantics performs semantic search for patterns
func (db *MemoryVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*ActionPattern, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// For this implementation, we'll do a flexible text search
	// In a real implementation, this would generate an embedding for the query text
	// and perform vector similarity search

	var matches []*ActionPattern

	// Split query into individual words for more flexible matching
	queryWords := strings.Fields(strings.ToLower(query))

	for _, pattern := range db.patterns {
		// Convert pattern fields to lowercase for case-insensitive matching
		actionTypeLower := strings.ToLower(pattern.ActionType)
		alertNameLower := strings.ToLower(pattern.AlertName)
		resourceTypeLower := strings.ToLower(pattern.ResourceType)

		// Check if any query word matches any pattern field
		matched := false
		for _, word := range queryWords {
			if strings.Contains(actionTypeLower, word) ||
				strings.Contains(alertNameLower, word) ||
				strings.Contains(resourceTypeLower, word) {
				matched = true
				break
			}
		}

		// Also check reverse: if any pattern field contains any query word
		if !matched {
			for _, word := range queryWords {
				if strings.Contains(word, "memory") && strings.Contains(alertNameLower, "memory") {
					matched = true
					break
				}
				if strings.Contains(word, "cpu") && strings.Contains(alertNameLower, "cpu") {
					matched = true
					break
				}
				if strings.Contains(word, "usage") && strings.Contains(alertNameLower, "usage") {
					matched = true
					break
				}
				if strings.Contains(word, "high") && strings.Contains(alertNameLower, "high") {
					matched = true
					break
				}
			}
		}

		if matched {
			matches = append(matches, pattern)
		}
	}

	// Sort by effectiveness score descending
	sort.Slice(matches, func(i, j int) bool {
		scoreI := 0.0
		scoreJ := 0.0
		if matches[i].EffectivenessData != nil {
			scoreI = matches[i].EffectivenessData.Score
		}
		if matches[j].EffectivenessData != nil {
			scoreJ = matches[j].EffectivenessData.Score
		}
		return scoreI > scoreJ
	})

	// Apply limit
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	db.log.WithFields(logrus.Fields{
		"query":          query,
		"found_patterns": len(matches),
	}).Debug("Performed semantic search")

	return matches, nil
}

// SearchByVector performs vector similarity search for patterns
// Business Requirement: BR-AI-COND-001 - Enhanced vector-based condition evaluation
func (db *MemoryVectorDatabase) SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*ActionPattern, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	if len(embedding) == 0 {
		return nil, fmt.Errorf("embedding vector cannot be empty for vector search")
	}

	var matches []*ActionPattern

	for _, pattern := range db.patterns {
		if len(pattern.Embedding) == 0 {
			continue // Skip patterns without embeddings
		}

		// Calculate cosine similarity
		similarity := sharedmath.CosineSimilarity(embedding, pattern.Embedding)
		if similarity >= threshold {
			matches = append(matches, pattern)
		}
	}

	// Sort by similarity descending (using effectiveness as secondary sort)
	sort.Slice(matches, func(i, j int) bool {
		// Primary sort: by similarity (we need to recalculate for sorting)
		simI := sharedmath.CosineSimilarity(embedding, matches[i].Embedding)
		simJ := sharedmath.CosineSimilarity(embedding, matches[j].Embedding)

		if simI != simJ {
			return simI > simJ
		}

		// Secondary sort: by effectiveness score
		scoreI := 0.0
		scoreJ := 0.0
		if matches[i].EffectivenessData != nil {
			scoreI = matches[i].EffectivenessData.Score
		}
		if matches[j].EffectivenessData != nil {
			scoreJ = matches[j].EffectivenessData.Score
		}
		return scoreI > scoreJ
	})

	// Apply limit
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	db.log.WithFields(logrus.Fields{
		"embedding_dim":  len(embedding),
		"found_patterns": len(matches),
		"limit":          limit,
		"threshold":      threshold,
	}).Debug("Performed vector similarity search")

	return matches, nil
}

// DeletePattern removes a pattern from the vector database
func (db *MemoryVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, exists := db.patterns[patternID]; !exists {
		return fmt.Errorf("pattern with ID %s not found", patternID)
	}

	delete(db.patterns, patternID)

	db.log.WithField("pattern_id", patternID).Debug("Deleted pattern from vector database")

	return nil
}

// GetPatternAnalytics returns analytics about stored patterns
func (db *MemoryVectorDatabase) GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	analytics := &PatternAnalytics{
		TotalPatterns:             len(db.patterns),
		PatternsByActionType:      make(map[string]int),
		PatternsBySeverity:        make(map[string]int),
		EffectivenessDistribution: make(map[string]int),
		GeneratedAt:               time.Now(),
	}

	var totalEffectiveness float64
	var effectivenessCount int
	var allPatterns []*ActionPattern

	for _, pattern := range db.patterns {
		allPatterns = append(allPatterns, pattern)

		// Count by action type
		analytics.PatternsByActionType[pattern.ActionType]++

		// Count by severity
		analytics.PatternsBySeverity[pattern.AlertSeverity]++

		// Calculate average effectiveness
		if pattern.EffectivenessData != nil {
			totalEffectiveness += pattern.EffectivenessData.Score
			effectivenessCount++

			// Effectiveness distribution
			bucket := getEffectivenessBucket(pattern.EffectivenessData.Score)
			analytics.EffectivenessDistribution[bucket]++
		}
	}

	// Calculate average effectiveness
	if effectivenessCount > 0 {
		analytics.AverageEffectiveness = totalEffectiveness / float64(effectivenessCount)
	}

	// Sort patterns by effectiveness for top performers
	sort.Slice(allPatterns, func(i, j int) bool {
		scoreI := 0.0
		scoreJ := 0.0
		if allPatterns[i].EffectivenessData != nil {
			scoreI = allPatterns[i].EffectivenessData.Score
		}
		if allPatterns[j].EffectivenessData != nil {
			scoreJ = allPatterns[j].EffectivenessData.Score
		}
		return scoreI > scoreJ
	})

	// Top 10 performing patterns
	topCount := 10
	if len(allPatterns) < topCount {
		topCount = len(allPatterns)
	}
	analytics.TopPerformingPatterns = allPatterns[:topCount]

	// Create a separate copy for recent patterns sorting
	recentPatterns := make([]*ActionPattern, len(allPatterns))
	copy(recentPatterns, allPatterns)

	// Sort by creation time for recent patterns
	sort.Slice(recentPatterns, func(i, j int) bool {
		return recentPatterns[i].CreatedAt.After(recentPatterns[j].CreatedAt)
	})

	// Recent 10 patterns
	recentCount := 10
	if len(recentPatterns) < recentCount {
		recentCount = len(recentPatterns)
	}
	analytics.RecentPatterns = recentPatterns[:recentCount]

	return analytics, nil
}

// IsHealthy performs a health check on the vector database
func (db *MemoryVectorDatabase) IsHealthy(ctx context.Context) error {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	// For memory database, we're healthy if we can access our data structure
	_ = len(db.patterns)
	return nil
}

// GetPatternCount returns the total number of stored patterns
func (db *MemoryVectorDatabase) GetPatternCount() int {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	return len(db.patterns)
}

// GetPattern retrieves a specific pattern by ID
func (db *MemoryVectorDatabase) GetPattern(patternID string) (*ActionPattern, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()

	pattern, exists := db.patterns[patternID]
	if !exists {
		return nil, fmt.Errorf("pattern with ID %s not found", patternID)
	}

	// Return a deep copy to prevent external modifications
	patternCopy := *pattern

	// Deep copy nested structures
	if pattern.EffectivenessData != nil {
		effectivenessCopy := *pattern.EffectivenessData
		if pattern.EffectivenessData.CostImpact != nil {
			costCopy := *pattern.EffectivenessData.CostImpact
			effectivenessCopy.CostImpact = &costCopy
		}
		if pattern.EffectivenessData.ContextualFactors != nil {
			factorsCopy := make(map[string]float64)
			for k, v := range pattern.EffectivenessData.ContextualFactors {
				factorsCopy[k] = v
			}
			effectivenessCopy.ContextualFactors = factorsCopy
		}
		patternCopy.EffectivenessData = &effectivenessCopy
	}

	// Deep copy maps
	if pattern.ActionParameters != nil {
		paramsCopy := make(map[string]interface{})
		for k, v := range pattern.ActionParameters {
			paramsCopy[k] = v
		}
		patternCopy.ActionParameters = paramsCopy
	}

	if pattern.ContextLabels != nil {
		labelsCopy := make(map[string]string)
		for k, v := range pattern.ContextLabels {
			labelsCopy[k] = v
		}
		patternCopy.ContextLabels = labelsCopy
	}

	if pattern.PreConditions != nil {
		preCopy := make(map[string]interface{})
		for k, v := range pattern.PreConditions {
			preCopy[k] = v
		}
		patternCopy.PreConditions = preCopy
	}

	if pattern.PostConditions != nil {
		postCopy := make(map[string]interface{})
		for k, v := range pattern.PostConditions {
			postCopy[k] = v
		}
		patternCopy.PostConditions = postCopy
	}

	if pattern.Metadata != nil {
		metaCopy := make(map[string]interface{})
		for k, v := range pattern.Metadata {
			metaCopy[k] = v
		}
		patternCopy.Metadata = metaCopy
	}

	// Deep copy slice
	if pattern.Embedding != nil {
		embeddingCopy := make([]float64, len(pattern.Embedding))
		copy(embeddingCopy, pattern.Embedding)
		patternCopy.Embedding = embeddingCopy
	}

	return &patternCopy, nil
}

// Clear removes all patterns from the database (useful for testing)
func (db *MemoryVectorDatabase) Clear() {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.patterns = make(map[string]*ActionPattern)
}

// Helper functions

// getEffectivenessBucket categorizes effectiveness scores into buckets
func getEffectivenessBucket(score float64) string {
	switch {
	case score >= 0.9:
		return "excellent"
	case score >= 0.8:
		return "very_good"
	case score >= 0.7:
		return "good"
	case score >= 0.6:
		return "fair"
	case score >= 0.5:
		return "poor"
	default:
		return "very_poor"
	}
}
