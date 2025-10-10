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
	"time"
)

// ExternalVectorDatabase defines the interface for external vector database services
type ExternalVectorDatabase interface {
	// Store stores vectors in the external database
	Store(ctx context.Context, vectors []VectorData) error

	// Query searches for similar vectors
	Query(ctx context.Context, embedding []float64, topK int, filters map[string]interface{}) ([]BaseSearchResult, error)

	// QueryByText searches for similar vectors using text input
	QueryByText(ctx context.Context, text string, topK int, filters map[string]interface{}) ([]BaseSearchResult, error)

	// Delete removes vectors by their IDs
	Delete(ctx context.Context, ids []string) error

	// DeleteByFilter removes vectors matching the filter
	DeleteByFilter(ctx context.Context, filters map[string]interface{}) error

	// Close closes the database connection
	Close() error
}

// VectorData represents data to be stored in a vector database
type VectorData struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text,omitempty"`
	Embedding []float64              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Timestamp time.Time              `json:"timestamp,omitempty"`
}

// ExternalVectorSearchResult represents a search result from external vector databases
// Now uses embedded BaseSearchResult for consistency
// ExternalVectorSearchResult alias removed - use BaseSearchResult directly

// ExternalEmbeddingGenerator creates embeddings for external services
type ExternalEmbeddingGenerator interface {
	// GenerateEmbedding generates a single embedding from text
	GenerateEmbedding(ctx context.Context, text string) ([]float64, error)

	// GenerateBatchEmbeddings generates embeddings for multiple texts
	GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error)

	// GetDimension returns the dimensionality of embeddings
	GetDimension() int

	// GetModel returns the model name
	GetModel() string
}
