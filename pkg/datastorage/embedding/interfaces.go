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
	"time"
)

// Client defines the interface for embedding generation services.
// Business Requirement: BR-STORAGE-012 (Vector embeddings for semantic search)
// Business Requirement: BR-STORAGE-014 (Workflow embedding generation)
//
// This interface allows for easy mocking in unit tests and follows Go naming conventions
// (no package name repetition - use embedding.Client, not embedding.EmbeddingClient).
type Client interface {
	// Embed generates a 768-dimensional embedding vector from text.
	// Returns the embedding vector or an error if generation fails.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Health checks if the embedding service is healthy and available.
	// Returns an error if the service is unhealthy or unreachable.
	Health(ctx context.Context) error
}

// Cache defines the interface for embedding cache storage.
// Business Requirement: BR-STORAGE-013 (Caching for performance)
type Cache interface {
	// Get retrieves a cached embedding by key.
	Get(ctx context.Context, key string) ([]float32, error)

	// Set stores an embedding in the cache with TTL.
	Set(ctx context.Context, key string, embedding []float32, ttl time.Duration) error
}

// EmbeddingResult represents the result of embedding generation.
type EmbeddingResult struct {
	// Embedding is the generated vector (768 dimensions per migration 016)
	Embedding []float32

	// Dimension is the embedding vector size
	Dimension int

	// CacheHit indicates if the result came from cache
	CacheHit bool
}

// Embedder defines the interface for text-to-vector embedding generation.
// This interface allows for easy mocking in unit tests.
// Business Requirement: BR-STORAGE-014 (Workflow embedding generation)
type Embedder interface {
	// Embed generates a 768-dimensional embedding vector for the given text.
	// Returns the embedding vector or an error if generation fails.
	Embed(ctx context.Context, text string) ([]float32, error)
}
