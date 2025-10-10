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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// PineconeVectorDatabase implements ExternalVectorDatabase for Pinecone
// Satisfies BR-VDB-003: Pinecone vector database backend
type PineconeVectorDatabase struct {
	apiKey     string
	config     *PineconeConfig
	httpClient *http.Client
	log        *logrus.Logger
	embedding  ExternalEmbeddingGenerator
}

// PineconeConfig holds configuration for Pinecone vector database
type PineconeConfig struct {
	Environment string        `yaml:"environment" default:"us-west1-gcp-free"`
	IndexName   string        `yaml:"index_name" default:"kubernaut"`
	Dimensions  int           `yaml:"dimensions" default:"1536"`
	Metric      string        `yaml:"metric" default:"cosine"`
	MaxRetries  int           `yaml:"max_retries" default:"3"`
	Timeout     time.Duration `yaml:"timeout" default:"30s"`
	BatchSize   int           `yaml:"batch_size" default:"100"`
}

// PineconeVector represents a vector in Pinecone format
type PineconeVector struct {
	ID       string                 `json:"id"`
	Values   []float64              `json:"values"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PineconeUpsertRequest represents a request to upsert vectors
type PineconeUpsertRequest struct {
	Vectors   []PineconeVector `json:"vectors"`
	Namespace string           `json:"namespace,omitempty"`
}

// PineconeQueryRequest represents a query request
type PineconeQueryRequest struct {
	TopK            int                    `json:"topK"`
	Filter          map[string]interface{} `json:"filter,omitempty"`
	IncludeValues   bool                   `json:"includeValues,omitempty"`
	IncludeMetadata bool                   `json:"includeMetadata,omitempty"`
	Vector          []float64              `json:"vector,omitempty"`
	ID              string                 `json:"id,omitempty"`
	Namespace       string                 `json:"namespace,omitempty"`
	SparseVector    interface{}            `json:"sparseVector,omitempty"`
}

// PineconeQueryResponse represents a query response
type PineconeQueryResponse struct {
	Matches []PineconeMatch `json:"matches"`
	Results []struct {
		Matches []PineconeMatch `json:"matches"`
	} `json:"results,omitempty"`
}

// PineconeMatch represents a single match result
type PineconeMatch struct {
	ID       string                 `json:"id"`
	Score    float64                `json:"score"`
	Values   []float64              `json:"values,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewPineconeVectorDatabase creates a new Pinecone vector database instance (BR-VDB-003)
func NewPineconeVectorDatabase(apiKey string, config *PineconeConfig, embedding ExternalEmbeddingGenerator, log *logrus.Logger) *PineconeVectorDatabase {
	return &PineconeVectorDatabase{
		apiKey: apiKey,
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:       log,
		embedding: embedding,
	}
}

// Store stores vectors in Pinecone (BR-VDB-001: Must store high-dimensional vector embeddings)
func (pdb *PineconeVectorDatabase) Store(ctx context.Context, vectors []VectorData) error {
	if len(vectors) == 0 {
		return nil
	}

	pdb.log.WithField("vector_count", len(vectors)).Info("Storing vectors in Pinecone")

	// Convert to Pinecone format
	pineconeVectors := make([]PineconeVector, 0, len(vectors))
	for _, v := range vectors {
		// Generate embedding if not provided
		embedding := v.Embedding
		if embedding == nil && v.Text != "" {
			var err error
			embedding, err = pdb.embedding.GenerateEmbedding(ctx, v.Text)
			if err != nil {
				return fmt.Errorf("failed to generate embedding for vector %s: %w", v.ID, err)
			}
		}

		if embedding == nil {
			return fmt.Errorf("no embedding available for vector %s", v.ID)
		}

		// Prepare metadata
		metadata := make(map[string]interface{})
		if v.Metadata != nil {
			metadata = v.Metadata
		}
		if v.Text != "" {
			metadata["text"] = v.Text
		}
		if v.Source != "" {
			metadata["source"] = v.Source
		}
		if !v.Timestamp.IsZero() {
			metadata["timestamp"] = v.Timestamp.Unix()
		}

		pineconeVectors = append(pineconeVectors, PineconeVector{
			ID:       v.ID,
			Values:   embedding,
			Metadata: metadata,
		})
	}

	// Process in batches
	batchSize := pdb.config.BatchSize
	for i := 0; i < len(pineconeVectors); i += batchSize {
		end := i + batchSize
		if end > len(pineconeVectors) {
			end = len(pineconeVectors)
		}

		batch := pineconeVectors[i:end]
		if err := pdb.upsertBatch(ctx, batch); err != nil {
			return fmt.Errorf("failed to upsert batch %d-%d: %w", i, end-1, err)
		}
	}

	pdb.log.WithField("vector_count", len(vectors)).Info("Successfully stored vectors in Pinecone")
	return nil
}

// Query searches for similar vectors (BR-VDB-002: Must support efficient similarity search)
func (pdb *PineconeVectorDatabase) Query(ctx context.Context, embedding []float64, topK int, filters map[string]interface{}) ([]BaseSearchResult, error) {
	req := &PineconeQueryRequest{
		TopK:            topK,
		Vector:          embedding,
		Filter:          filters,
		IncludeValues:   false, // Usually not needed for similarity search
		IncludeMetadata: true,
	}

	response, err := pdb.executeQuery(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Pinecone query: %w", err)
	}

	// Convert to BaseSearchResult format
	results := make([]BaseSearchResult, 0, len(response.Matches))
	for _, match := range response.Matches {
		result := BaseSearchResult{
			ID:       match.ID,
			Score:    float32(match.Score),
			Metadata: match.Metadata,
		}

		results = append(results, result)
	}

	return results, nil
}

// QueryByText searches for similar vectors using text input (BR-VDB-016: Must support k-nearest neighbor search)
func (pdb *PineconeVectorDatabase) QueryByText(ctx context.Context, text string, topK int, filters map[string]interface{}) ([]BaseSearchResult, error) {
	// Generate embedding for the query text
	embedding, err := pdb.embedding.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for query text: %w", err)
	}

	return pdb.Query(ctx, embedding, topK, filters)
}

// Delete removes vectors by their IDs (BR-VDB-004: Must support vector deletion)
func (pdb *PineconeVectorDatabase) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	pdb.log.WithField("id_count", len(ids)).Info("Deleting vectors from Pinecone")

	req := map[string]interface{}{
		"ids": ids,
	}

	if err := pdb.executeDelete(ctx, req); err != nil {
		return fmt.Errorf("failed to delete vectors: %w", err)
	}

	pdb.log.WithField("id_count", len(ids)).Info("Successfully deleted vectors from Pinecone")
	return nil
}

// DeleteByFilter removes vectors matching the filter (BR-VDB-018: Must support filtered search)
func (pdb *PineconeVectorDatabase) DeleteByFilter(ctx context.Context, filters map[string]interface{}) error {
	if len(filters) == 0 {
		return fmt.Errorf("filter cannot be empty for delete by filter operation")
	}

	pdb.log.WithField("filters", filters).Info("Deleting vectors by filter from Pinecone")

	req := map[string]interface{}{
		"filter": filters,
	}

	if err := pdb.executeDelete(ctx, req); err != nil {
		return fmt.Errorf("failed to delete vectors by filter: %w", err)
	}

	pdb.log.WithField("filters", filters).Info("Successfully deleted vectors by filter from Pinecone")
	return nil
}

// Close closes the database connection (BR-REL-015: Must support emergency read-only mode)
func (pdb *PineconeVectorDatabase) Close() error {
	// Pinecone is HTTP-based, no persistent connections to close
	pdb.log.Info("Closing Pinecone vector database connection")
	return nil
}

// upsertBatch uploads a batch of vectors to Pinecone
func (pdb *PineconeVectorDatabase) upsertBatch(ctx context.Context, vectors []PineconeVector) error {
	req := &PineconeUpsertRequest{
		Vectors: vectors,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal upsert request: %w", err)
	}

	url := fmt.Sprintf("https://%s-%s.svc.%s.pinecone.io/vectors/upsert",
		pdb.config.IndexName, generateProjectID(), pdb.config.Environment)

	return pdb.executeRequest(ctx, "POST", url, reqBody, nil)
}

// executeQuery executes a query request to Pinecone
func (pdb *PineconeVectorDatabase) executeQuery(ctx context.Context, req *PineconeQueryRequest) (*PineconeQueryResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query request: %w", err)
	}

	url := fmt.Sprintf("https://%s-%s.svc.%s.pinecone.io/query",
		pdb.config.IndexName, generateProjectID(), pdb.config.Environment)

	var response PineconeQueryResponse
	if err := pdb.executeRequest(ctx, "POST", url, reqBody, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// executeDelete executes a delete request to Pinecone
func (pdb *PineconeVectorDatabase) executeDelete(ctx context.Context, req map[string]interface{}) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal delete request: %w", err)
	}

	url := fmt.Sprintf("https://%s-%s.svc.%s.pinecone.io/vectors/delete",
		pdb.config.IndexName, generateProjectID(), pdb.config.Environment)

	return pdb.executeRequest(ctx, "POST", url, reqBody, nil)
}

// executeRequest executes an HTTP request to Pinecone with retry logic
func (pdb *PineconeVectorDatabase) executeRequest(ctx context.Context, method, url string, body []byte, response interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= pdb.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			pdb.log.WithField("backoff", backoff).Debug("Backing off before retry")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Api-Key", pdb.apiKey)

		resp, err := pdb.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				pdb.log.WithError(err).Error("Failed to close HTTP response body")
			}
		}()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("inecone API error (status %d): %s", resp.StatusCode, string(respBody))
			// Don't retry on authentication or client errors
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return lastErr
			}
			continue
		}

		// If no response expected, return success
		if response == nil {
			return nil
		}

		// Parse response
		if err := json.Unmarshal(respBody, response); err != nil {
			lastErr = fmt.Errorf("failed to unmarshal response: %w", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d retries: %w", pdb.config.MaxRetries, lastErr)
}

// generateProjectID generates a project ID for Pinecone URLs
// In practice, this would come from Pinecone console or API
func generateProjectID() string {
	return "project" // This should be configurable in a real implementation
}
