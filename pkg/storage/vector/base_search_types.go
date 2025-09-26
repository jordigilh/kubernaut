package vector

// BaseSearchResult provides common fields for individual search results
// This eliminates the type mismatch between ExternalVectorSearchResult and expected interfaces
type BaseSearchResult struct {
	ID       string                 `json:"id"`
	Score    float32                `json:"score"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// VectorSearchResultSet represents a complete set of search results
// Maintained for backward compatibility, but unified with UnifiedSearchResultSet
// VectorSearchResultSet alias removed - use UnifiedSearchResultSet directly

// PatternSearchResultSet represents pattern-specific search results
type PatternSearchResultSet struct {
	UnifiedSearchResultSet                        // Embedded unified search results
	Patterns               []*UnifiedSearchResult `json:"patterns"` // Pattern-specific results
}
