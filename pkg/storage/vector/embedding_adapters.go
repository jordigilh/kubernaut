package vector

import (
	"context"
	"fmt"
)

// EmbeddingGeneratorAdapter adapts ExternalEmbeddingGenerator to EmbeddingGenerator interface
type EmbeddingGeneratorAdapter struct {
	external ExternalEmbeddingGenerator
}

// NewEmbeddingGeneratorAdapter creates an adapter for external embedding generators
func NewEmbeddingGeneratorAdapter(external ExternalEmbeddingGenerator) EmbeddingGenerator {
	return &EmbeddingGeneratorAdapter{
		external: external,
	}
}

// GenerateTextEmbedding creates embedding from text using the external service
func (ega *EmbeddingGeneratorAdapter) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	return ega.external.GenerateEmbedding(ctx, text)
}

// GenerateActionEmbedding creates embedding from action data
func (ega *EmbeddingGeneratorAdapter) GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error) {
	// Combine action type and parameters into text
	text := fmt.Sprintf("Action: %s, Parameters: %v", actionType, parameters)
	return ega.external.GenerateEmbedding(ctx, text)
}

// GenerateContextEmbedding creates embedding from context data
func (ega *EmbeddingGeneratorAdapter) GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error) {
	// Combine labels and metadata into text
	text := fmt.Sprintf("Labels: %v, Metadata: %v", labels, metadata)
	return ega.external.GenerateEmbedding(ctx, text)
}

// CombineEmbeddings combines multiple embeddings into one
func (ega *EmbeddingGeneratorAdapter) CombineEmbeddings(embeddings ...[]float64) []float64 {
	if len(embeddings) == 0 {
		return []float64{}
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// Simple averaging approach
	dimension := len(embeddings[0])
	result := make([]float64, dimension)

	for _, embedding := range embeddings {
		for i, value := range embedding {
			if i < dimension {
				result[i] += value
			}
		}
	}

	// Average the values
	count := float64(len(embeddings))
	for i := range result {
		result[i] /= count
	}

	return result
}

// GetEmbeddingDimension returns the dimension of generated embeddings
func (ega *EmbeddingGeneratorAdapter) GetEmbeddingDimension() int {
	return ega.external.GetDimension()
}

// ExternalVectorDatabaseAdapter adapts ExternalVectorDatabase to work with VectorDatabase interface
type ExternalVectorDatabaseAdapter struct {
	external         ExternalVectorDatabase
	embeddingService EmbeddingGenerator
}

// NewExternalVectorDatabaseAdapter creates an adapter for external vector databases
func NewExternalVectorDatabaseAdapter(external ExternalVectorDatabase, embeddingService EmbeddingGenerator) VectorDatabase {
	return &ExternalVectorDatabaseAdapter{
		external:         external,
		embeddingService: embeddingService,
	}
}

// StoreActionPattern stores an action pattern as a vector
func (evda *ExternalVectorDatabaseAdapter) StoreActionPattern(ctx context.Context, pattern *ActionPattern) error {
	// BR-VALIDATION-01: Validate pattern data before storage
	if pattern.ID == "" {
		return fmt.Errorf("pattern ID cannot be empty")
	}

	if len(pattern.Embedding) == 0 {
		return fmt.Errorf("pattern embedding cannot be empty")
	}

	vectorData := VectorData{
		ID:        pattern.ID,
		Text:      fmt.Sprintf("%s %s %s", pattern.ActionType, pattern.AlertName, pattern.AlertSeverity),
		Embedding: pattern.Embedding,
		Metadata: map[string]interface{}{
			"action_type":    pattern.ActionType,
			"alert_name":     pattern.AlertName,
			"alert_severity": pattern.AlertSeverity,
			"namespace":      pattern.Namespace,
			"resource_type":  pattern.ResourceType,
			"resource_name":  pattern.ResourceName,
			"effectiveness":  pattern.EffectivenessData,
			"created_at":     pattern.CreatedAt,
			"updated_at":     pattern.UpdatedAt,
		},
		Timestamp: pattern.CreatedAt,
	}

	return evda.external.Store(ctx, []VectorData{vectorData})
}

// FindSimilarPatterns finds patterns similar to the given one
func (evda *ExternalVectorDatabaseAdapter) FindSimilarPatterns(ctx context.Context, pattern *ActionPattern, limit int, threshold float64) ([]*SimilarPattern, error) {
	results, err := evda.external.Query(ctx, pattern.Embedding, limit, nil)
	if err != nil {
		return nil, err
	}

	var similarPatterns []*SimilarPattern
	for i, result := range results {
		if float64(result.Score) >= threshold {
			// Convert back to ActionPattern (simplified)
			actionPattern := &ActionPattern{
				ID: result.ID,
				// Populate other fields from metadata if available
			}

			if metadata := result.Metadata; metadata != nil {
				if actionType, ok := metadata["action_type"].(string); ok {
					actionPattern.ActionType = actionType
				}
				if alertName, ok := metadata["alert_name"].(string); ok {
					actionPattern.AlertName = alertName
				}
				// Add more field mappings as needed
			}

			similarPatterns = append(similarPatterns, &SimilarPattern{
				Pattern:    actionPattern,
				Similarity: float64(result.Score),
				Rank:       i + 1,
			})
		}
	}

	return similarPatterns, nil
}

// UpdatePatternEffectiveness updates the effectiveness score of a stored pattern
func (evda *ExternalVectorDatabaseAdapter) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	// This is a simplified implementation - in practice, you'd need to
	// retrieve, update, and re-store the pattern
	return fmt.Errorf("update pattern effectiveness not yet fully implemented for external databases")
}

// SearchBySemantics performs semantic search for patterns
func (evda *ExternalVectorDatabaseAdapter) SearchBySemantics(ctx context.Context, query string, limit int) ([]*ActionPattern, error) {
	results, err := evda.external.QueryByText(ctx, query, limit, nil)
	if err != nil {
		return nil, err
	}

	var patterns []*ActionPattern
	for _, result := range results {
		// Convert back to ActionPattern (simplified)
		pattern := &ActionPattern{
			ID: result.ID,
		}

		if metadata := result.Metadata; metadata != nil {
			if actionType, ok := metadata["action_type"].(string); ok {
				pattern.ActionType = actionType
			}
			if alertName, ok := metadata["alert_name"].(string); ok {
				pattern.AlertName = alertName
			}
			// Add more field mappings as needed
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// DeletePattern removes a pattern from the vector database
func (evda *ExternalVectorDatabaseAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return evda.external.Delete(ctx, []string{patternID})
}

// GetPatternAnalytics returns analytics about stored patterns
func (evda *ExternalVectorDatabaseAdapter) GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error) {
	// This would require querying all patterns and analyzing them
	// For now, return a placeholder
	return &PatternAnalytics{
		TotalPatterns:             0,
		PatternsByActionType:      make(map[string]int),
		PatternsBySeverity:        make(map[string]int),
		AverageEffectiveness:      0.0,
		TopPerformingPatterns:     []*ActionPattern{},
		RecentPatterns:            []*ActionPattern{},
		EffectivenessDistribution: make(map[string]int),
	}, nil
}

// IsHealthy checks the health of the external database
func (evda *ExternalVectorDatabaseAdapter) IsHealthy(ctx context.Context) error {
	// Simple health check - try to query with empty embedding
	_, err := evda.external.Query(ctx, []float64{}, 1, nil)
	if err != nil {
		return fmt.Errorf("external vector database health check failed: %w", err)
	}
	return nil
}

// ExternalEmbeddingAdapter adapts standard EmbeddingGenerator to ExternalEmbeddingGenerator
type ExternalEmbeddingAdapter struct {
	standard EmbeddingGenerator
}

// GenerateEmbedding generates a single embedding from text
func (eea *ExternalEmbeddingAdapter) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	return eea.standard.GenerateTextEmbedding(ctx, text)
}

// GenerateBatchEmbeddings generates embeddings for multiple texts
func (eea *ExternalEmbeddingAdapter) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := eea.standard.GenerateTextEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		results[i] = embedding
	}
	return results, nil
}

// GetDimension returns the dimensionality of embeddings
func (eea *ExternalEmbeddingAdapter) GetDimension() int {
	return eea.standard.GetEmbeddingDimension()
}

// GetModel returns the model name (placeholder)
func (eea *ExternalEmbeddingAdapter) GetModel() string {
	return "adapted-embedding-service"
}
