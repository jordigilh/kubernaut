package vector

import (
	"encoding/json"
	"time"
)

// UnifiedSearchResult represents the standard result structure for all vector search operations
// This replaces the inconsistent VectorSearchResult/BaseSearchResult/ExternalVectorSearchResult types
type UnifiedSearchResult struct {
	// Core identification and scoring
	ID    string  `json:"id"`
	Score float32 `json:"score"`

	// Content and context
	Text      string                 `json:"text,omitempty"`
	Embedding []float64              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`

	// Source tracking
	Source    string    `json:"source,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`

	// Search-specific context
	Distance float64 `json:"distance,omitempty"` // Raw distance for debugging
	Rank     int     `json:"rank,omitempty"`     // Position in result set
}

// UnifiedSearchResultSet represents a complete set of search results with metadata
type UnifiedSearchResultSet struct {
	// Core results
	Results    []UnifiedSearchResult `json:"results"`
	TotalCount int                   `json:"total_count"`

	// Query context
	QueryVector         []float64 `json:"query_vector,omitempty"`
	QueryText           string    `json:"query_text,omitempty"`
	SimilarityThreshold float64   `json:"similarity_threshold"`

	// Performance metrics
	SearchTime    time.Duration `json:"search_time"`
	IndexedCount  int           `json:"indexed_count,omitempty"`
	FilteredCount int           `json:"filtered_count,omitempty"`

	// Pagination
	Offset  int  `json:"offset,omitempty"`
	Limit   int  `json:"limit"`
	HasMore bool `json:"has_more"`
}

// SearchResultConverter provides methods to convert between different result formats
type SearchResultConverter struct{}

// NewSearchResultConverter creates a new converter instance
func NewSearchResultConverter() *SearchResultConverter {
	return &SearchResultConverter{}
}

// FromBaseSearchResult converts BaseSearchResult to UnifiedSearchResult
func (src *SearchResultConverter) FromBaseSearchResult(base BaseSearchResult) UnifiedSearchResult {
	return UnifiedSearchResult{
		ID:       base.ID,
		Score:    base.Score,
		Metadata: base.Metadata,
	}
}

// FromBaseSearchResults converts slice of BaseSearchResult to UnifiedSearchResultSet
func (src *SearchResultConverter) FromBaseSearchResults(results []BaseSearchResult, searchTime time.Duration) UnifiedSearchResultSet {
	unified := make([]UnifiedSearchResult, len(results))
	for i, result := range results {
		unified[i] = src.FromBaseSearchResult(result)
		unified[i].Rank = i + 1
	}

	return UnifiedSearchResultSet{
		Results:    unified,
		TotalCount: len(unified),
		SearchTime: searchTime,
		Limit:      len(unified),
		HasMore:    false,
	}
}

// ToBaseSearchResults converts UnifiedSearchResult slice to BaseSearchResult slice
func (src *SearchResultConverter) ToBaseSearchResults(unified []UnifiedSearchResult) []BaseSearchResult {
	base := make([]BaseSearchResult, len(unified))
	for i, result := range unified {
		base[i] = BaseSearchResult{
			ID:       result.ID,
			Score:    result.Score,
			Metadata: result.Metadata,
		}
	}
	return base
}

// FromSimilarPattern converts SimilarPattern to UnifiedSearchResult
func (src *SearchResultConverter) FromSimilarPattern(similar *SimilarPattern) UnifiedSearchResult {
	metadata := make(map[string]interface{})

	// Extract pattern metadata
	if similar.Pattern != nil {
		metadata["action_type"] = similar.Pattern.ActionType
		metadata["alert_name"] = similar.Pattern.AlertName
		metadata["alert_severity"] = similar.Pattern.AlertSeverity
		metadata["namespace"] = similar.Pattern.Namespace
		metadata["resource_type"] = similar.Pattern.ResourceType
		metadata["resource_name"] = similar.Pattern.ResourceName
		metadata["created_at"] = similar.Pattern.CreatedAt
		metadata["updated_at"] = similar.Pattern.UpdatedAt

		// Add effectiveness data if available
		if similar.Pattern.EffectivenessData != nil {
			effData, _ := json.Marshal(similar.Pattern.EffectivenessData)
			metadata["effectiveness_data"] = string(effData)
		}
	}

	return UnifiedSearchResult{
		ID:        similar.Pattern.ID,
		Score:     float32(similar.Similarity),
		Embedding: similar.Pattern.Embedding,
		Metadata:  metadata,
		Source:    "pattern_database",
		Timestamp: similar.Pattern.CreatedAt,
		Rank:      similar.Rank,
	}
}

// FromSimilarPatterns converts slice of SimilarPattern to UnifiedSearchResultSet
func (src *SearchResultConverter) FromSimilarPatterns(patterns []*SimilarPattern, searchTime time.Duration) UnifiedSearchResultSet {
	unified := make([]UnifiedSearchResult, len(patterns))
	for i, pattern := range patterns {
		unified[i] = src.FromSimilarPattern(pattern)
	}

	return UnifiedSearchResultSet{
		Results:    unified,
		TotalCount: len(unified),
		SearchTime: searchTime,
		Limit:      len(unified),
		HasMore:    false,
	}
}

// SearchResultFactory provides factory methods for creating search results
type SearchResultFactory struct {
	converter *SearchResultConverter
}

// NewSearchResultFactory creates a new factory instance
func NewSearchResultFactory() *SearchResultFactory {
	return &SearchResultFactory{
		converter: NewSearchResultConverter(),
	}
}

// CreateEmpty creates an empty result set
func (srf *SearchResultFactory) CreateEmpty() UnifiedSearchResultSet {
	return UnifiedSearchResultSet{
		Results:    []UnifiedSearchResult{},
		TotalCount: 0,
		SearchTime: 0,
		Limit:      0,
		HasMore:    false,
	}
}

// CreateFromScore creates a single result with just ID and score
func (srf *SearchResultFactory) CreateFromScore(id string, score float32) UnifiedSearchResult {
	return UnifiedSearchResult{
		ID:    id,
		Score: score,
		Rank:  1,
	}
}

// Helper functions for backward compatibility

// VectorSearchResultSet conversion functions removed - use UnifiedSearchResultSet directly
