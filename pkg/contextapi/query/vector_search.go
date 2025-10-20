// Package query provides vector search functionality for Context API
package query

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/client"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
)

// VectorSearch performs semantic search using pgvector
//
// BR-CONTEXT-002: Semantic search on embeddings
type VectorSearch struct {
	client client.Client
	logger *zap.Logger
}

// NewVectorSearch creates a new vector search instance
func NewVectorSearch(dbClient client.Client, logger *zap.Logger) *VectorSearch {
	return &VectorSearch{
		client: dbClient,
		logger: logger,
	}
}

// FindSimilar performs vector similarity search
// BR-CONTEXT-002: Semantic search using pgvector cosine similarity
func (vs *VectorSearch) FindSimilar(ctx context.Context, query *models.PatternMatchQuery) ([]*models.SimilarIncident, error) {
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pattern match query: %w", err)
	}

	// Use client's semantic search
	params := &models.SemanticSearchParams{
		Embedding: query.Embedding,
		Threshold: query.Threshold,
		Limit:     query.Limit,
		Namespace: query.Namespace,
		Severity:  query.Severity,
	}

	incidents, similarities, err := vs.client.SemanticSearch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}

	// Convert to SimilarIncident format
	results := make([]*models.SimilarIncident, len(incidents))
	for i, incident := range incidents {
		var similarity float32
		if i < len(similarities) {
			similarity = similarities[i]
		}

		results[i] = &models.SimilarIncident{
			IncidentEvent: *incident,
			Similarity:    similarity,
		}
	}

	return results, nil
}

