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

package embedding

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pgvector/pgvector-go"
)

// ========================================
// EMBEDDING SERVICE
// ========================================
// BR-STORAGE-013: Generate vector embeddings for semantic search
// Model: sentence-transformers/all-mpnet-base-v2 (768 dimensions per migration 016)
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
	logger logr.Logger
}

// NewPlaceholderService creates a new placeholder embedding service
func NewPlaceholderService(logger logr.Logger) Service {
	return &PlaceholderService{
		logger: logger,
	}
}

// GenerateEmbedding generates a placeholder embedding
// TODO: Replace with real embedding generation
func (s *PlaceholderService) GenerateEmbedding(ctx context.Context, text string) (*pgvector.Vector, error) {
	s.logger.Info("Using placeholder embedding service - replace with real implementation",
		"text_preview", truncateText(text, 50),
	)

	// Generate a simple placeholder embedding (768 dimensions per migration 016)
	// In production, this would call a real embedding model
	embedding := make([]float32, 768)

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
	s.logger.Info("Using placeholder batch embedding service - replace with real implementation",
		"batch_size", len(texts),
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
