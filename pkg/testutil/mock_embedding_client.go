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

package testutil

import (
	"context"
	"fmt"

	"github.com/pgvector/pgvector-go"
)

// MockEmbeddingClient is a mock implementation of embedding.EmbeddingAPIClient for unit tests.
// It allows tests to control embedding generation behavior without requiring a real embedding service.
type MockEmbeddingClient struct {
	// EmbedFunc allows tests to customize the Embed behavior
	EmbedFunc func(ctx context.Context, text string) ([]float32, error)

	// CallCount tracks how many times Embed was called
	CallCount int

	// LastText stores the last text passed to Embed
	LastText string
}

// NewMockEmbeddingClient creates a new mock embedding client with default behavior.
// By default, it returns a 768-dimensional zero vector.
func NewMockEmbeddingClient() *MockEmbeddingClient {
	return &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float32, error) {
			// Return a 768-dimensional zero vector by default
			return make([]float32, 768), nil
		},
	}
}

// Embed implements embedding.EmbeddingAPIClient interface.
func (m *MockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	m.CallCount++
	m.LastText = text

	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, text)
	}

	// Fallback: return 768-dimensional zero vector
	return make([]float32, 768), nil
}

// WithEmbedding configures the mock to return a specific embedding vector.
func (m *MockEmbeddingClient) WithEmbedding(embedding []float32) *MockEmbeddingClient {
	m.EmbedFunc = func(ctx context.Context, text string) ([]float32, error) {
		return embedding, nil
	}
	return m
}

// WithError configures the mock to return an error.
func (m *MockEmbeddingClient) WithError(err error) *MockEmbeddingClient {
	m.EmbedFunc = func(ctx context.Context, text string) ([]float32, error) {
		return nil, err
	}
	return m
}

// WithDeterministicEmbedding configures the mock to return embeddings based on text length.
// This is useful for tests that need predictable but unique embeddings.
func (m *MockEmbeddingClient) WithDeterministicEmbedding() *MockEmbeddingClient {
	m.EmbedFunc = func(ctx context.Context, text string) ([]float32, error) {
		embedding := make([]float32, 768)
		// Generate deterministic values based on text length
		textLen := float32(len(text))
		for i := range embedding {
			embedding[i] = textLen / float32(i+1) / 1000.0
		}
		return embedding, nil
	}
	return m
}

// Reset resets the mock's state (call count and last text).
func (m *MockEmbeddingClient) Reset() {
	m.CallCount = 0
	m.LastText = ""
}

// AssertCalled returns an error if Embed was not called the expected number of times.
func (m *MockEmbeddingClient) AssertCalled(expectedCount int) error {
	if m.CallCount != expectedCount {
		return fmt.Errorf("expected Embed to be called %d times, but was called %d times", expectedCount, m.CallCount)
	}
	return nil
}

// AssertNotCalled returns an error if Embed was called.
func (m *MockEmbeddingClient) AssertNotCalled() error {
	if m.CallCount > 0 {
		return fmt.Errorf("expected Embed to not be called, but was called %d times", m.CallCount)
	}
	return nil
}

// Health implements embedding.Client interface.
// Always returns nil (healthy) by default for testing.
func (m *MockEmbeddingClient) Health(ctx context.Context) error {
	return nil
}

// ========================================
// embedding.Service INTERFACE IMPLEMENTATION
// ========================================
// These methods implement the embedding.Service interface
// required by server.WithEmbeddingService()

// GenerateEmbedding implements embedding.Service interface.
// Returns a pgvector.Vector for use in semantic search.
func (m *MockEmbeddingClient) GenerateEmbedding(ctx context.Context, text string) (*pgvector.Vector, error) {
	embedding, err := m.Embed(ctx, text)
	if err != nil {
		return nil, err
	}
	vec := pgvector.NewVector(embedding)
	return &vec, nil
}

// GenerateBatchEmbeddings implements embedding.Service interface.
// Generates embeddings for multiple texts.
func (m *MockEmbeddingClient) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([]*pgvector.Vector, error) {
	results := make([]*pgvector.Vector, len(texts))
	for i, text := range texts {
		vec, err := m.GenerateEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}
