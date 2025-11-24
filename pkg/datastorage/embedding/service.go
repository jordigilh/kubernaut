package embedding

import (
	"context"
	"fmt"

	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"
)

// ========================================
// EMBEDDING SERVICE
// ========================================
// BR-STORAGE-013: Generate vector embeddings for semantic search
// Model: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
//
// This service generates vector embeddings from text for semantic search.
// The embeddings are used by pgvector for similarity search in PostgreSQL.

// Service defines the interface for generating embeddings
type Service interface {
	// GenerateEmbedding generates a vector embedding from text
	GenerateEmbedding(ctx context.Context, text string) (*pgvector.Vector, error)

	// GenerateBatchEmbeddings generates embeddings for multiple texts
	GenerateBatchEmbeddings(ctx context.Context, texts []string) ([]*pgvector.Vector, error)
}

// ========================================
// PLACEHOLDER IMPLEMENTATION
// ========================================
// TODO: Replace with real embedding service (e.g., HuggingFace, OpenAI, local model)
// For now, this returns a placeholder embedding for development/testing

// PlaceholderService is a temporary implementation that returns placeholder embeddings
type PlaceholderService struct {
	logger *zap.Logger
}

// NewPlaceholderService creates a new placeholder embedding service
func NewPlaceholderService(logger *zap.Logger) Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PlaceholderService{
		logger: logger,
	}
}

// GenerateEmbedding generates a placeholder embedding
// TODO: Replace with real embedding generation
func (s *PlaceholderService) GenerateEmbedding(ctx context.Context, text string) (*pgvector.Vector, error) {
	s.logger.Warn("Using placeholder embedding service - replace with real implementation",
		zap.String("text_preview", truncateText(text, 50)),
	)

	// Generate a simple placeholder embedding (384 dimensions, all zeros)
	// In production, this would call a real embedding model
	embedding := make([]float32, 384)

	// Add some variation based on text length (just for testing)
	textLen := float32(len(text))
	for i := range embedding {
		embedding[i] = textLen / 1000.0
	}

	vec := pgvector.NewVector(embedding)
	return &vec, nil
}

// GenerateBatchEmbeddings generates placeholder embeddings for multiple texts
// TODO: Replace with real batch embedding generation
func (s *PlaceholderService) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([]*pgvector.Vector, error) {
	s.logger.Warn("Using placeholder batch embedding service - replace with real implementation",
		zap.Int("batch_size", len(texts)),
	)

	embeddings := make([]*pgvector.Vector, len(texts))
	for i, text := range texts {
		embedding, err := s.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// truncateText truncates text to maxLen characters
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

