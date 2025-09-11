package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// WeaviateVectorDatabase implements ExternalVectorDatabase for Weaviate
// Satisfies BR-VDB-004: Weaviate knowledge graph vector database
type WeaviateVectorDatabase struct {
	config     *WeaviateConfig
	httpClient *http.Client
	log        *logrus.Logger
	embedding  ExternalEmbeddingGenerator
}

// WeaviateConfig holds configuration for Weaviate vector database
type WeaviateConfig struct {
	BaseURL    string        `yaml:"base_url" default:"http://localhost:8080"`
	ClassName  string        `yaml:"class_name" default:"KubernautVector"`
	APIKey     string        `yaml:"api_key"`
	MaxRetries int           `yaml:"max_retries" default:"3"`
	Timeout    time.Duration `yaml:"timeout" default:"30s"`
	BatchSize  int           `yaml:"batch_size" default:"100"`
}

// WeaviateObject represents an object in Weaviate format
type WeaviateObject struct {
	ID         string                 `json:"id,omitempty"`
	Class      string                 `json:"class"`
	Properties map[string]interface{} `json:"properties"`
	Vector     []float64              `json:"vector,omitempty"`
}

// WeaviateBatchRequest represents a batch request for objects
type WeaviateBatchRequest struct {
	Objects []WeaviateObject `json:"objects"`
}

// WeaviateQueryRequest represents a GraphQL query request
type WeaviateQueryRequest struct {
	Query string `json:"query"`
}

// WeaviateQueryResponse represents a GraphQL query response
type WeaviateQueryResponse struct {
	Data   WeaviateQueryData `json:"data"`
	Errors []WeaviateError   `json:"errors,omitempty"`
}

// WeaviateQueryData represents the data section of a query response
type WeaviateQueryData struct {
	Get map[string][]WeaviateResult `json:"Get"`
}

// WeaviateResult represents a single result object
type WeaviateResult struct {
	ID         string                 `json:"_id"`
	Additional WeaviateAdditional     `json:"_additional,omitempty"`
	Properties map[string]interface{} `json:"-"` // Dynamic properties
}

// WeaviateAdditional represents additional metadata
type WeaviateAdditional struct {
	ID       string    `json:"id"`
	Distance float64   `json:"distance,omitempty"`
	Score    float64   `json:"score,omitempty"`
	Vector   []float64 `json:"vector,omitempty"`
}

// WeaviateError represents an error from Weaviate
type WeaviateError struct {
	Message string `json:"message"`
}

// NewWeaviateVectorDatabase creates a new Weaviate vector database instance (BR-VDB-004)
func NewWeaviateVectorDatabase(config *WeaviateConfig, embedding ExternalEmbeddingGenerator, log *logrus.Logger) *WeaviateVectorDatabase {
	return &WeaviateVectorDatabase{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:       log,
		embedding: embedding,
	}
}

// Store stores vectors in Weaviate (BR-VDB-001: Must store high-dimensional vector embeddings)
func (wdb *WeaviateVectorDatabase) Store(ctx context.Context, vectors []VectorData) error {
	if len(vectors) == 0 {
		return nil
	}

	wdb.log.WithField("vector_count", len(vectors)).Info("Storing vectors in Weaviate")

	// Convert to Weaviate format
	weaviateObjects := make([]WeaviateObject, 0, len(vectors))
	for _, v := range vectors {
		// Generate embedding if not provided
		embedding := v.Embedding
		if embedding == nil && v.Text != "" {
			var err error
			embedding, err = wdb.embedding.GenerateEmbedding(ctx, v.Text)
			if err != nil {
				return fmt.Errorf("failed to generate embedding for vector %s: %w", v.ID, err)
			}
		}

		if embedding == nil {
			return fmt.Errorf("no embedding available for vector %s", v.ID)
		}

		// Prepare properties (BR-VDB-011: Must extract meaningful patterns from structured data)
		properties := make(map[string]interface{})
		if v.Metadata != nil {
			for k, val := range v.Metadata {
				properties[k] = val
			}
		}
		if v.Text != "" {
			properties["text"] = v.Text
		}
		if v.Source != "" {
			properties["source"] = v.Source
		}
		if !v.Timestamp.IsZero() {
			properties["timestamp"] = v.Timestamp.Unix()
		}

		weaviateObjects = append(weaviateObjects, WeaviateObject{
			ID:         v.ID,
			Class:      wdb.config.ClassName,
			Properties: properties,
			Vector:     embedding,
		})
	}

	// Process in batches
	batchSize := wdb.config.BatchSize
	for i := 0; i < len(weaviateObjects); i += batchSize {
		end := i + batchSize
		if end > len(weaviateObjects) {
			end = len(weaviateObjects)
		}

		batch := weaviateObjects[i:end]
		if err := wdb.batchObjects(ctx, batch); err != nil {
			return fmt.Errorf("failed to batch objects %d-%d: %w", i, end-1, err)
		}
	}

	wdb.log.WithField("vector_count", len(vectors)).Info("Successfully stored vectors in Weaviate")
	return nil
}

// Query searches for similar vectors (BR-VDB-002: Must support efficient similarity search)
func (wdb *WeaviateVectorDatabase) Query(ctx context.Context, embedding []float64, topK int, filters map[string]interface{}) ([]BaseSearchResult, error) {
	// Build GraphQL query for vector similarity search (BR-VDB-016: Must support k-nearest neighbor search)
	whereClause := ""
	if len(filters) > 0 {
		whereClause = wdb.buildWhereClause(filters)
	}

	query := fmt.Sprintf(`{
		Get {
			%s(
				nearVector: {
					vector: %s
				}
				limit: %d
				%s
			) {
				text
				source
				timestamp
				_additional {
					id
					distance
					vector
				}
			}
		}
	}`, wdb.config.ClassName, wdb.formatVector(embedding), topK, whereClause)

	req := &WeaviateQueryRequest{
		Query: query,
	}

	response, err := wdb.executeGraphQLQuery(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Weaviate query: %w", err)
	}

	// Extract results from GraphQL response
	results := make([]BaseSearchResult, 0)
	if classResults, ok := response.Data.Get[wdb.config.ClassName]; ok {
		for _, result := range classResults {
			searchResult := BaseSearchResult{
				ID:       result.Additional.ID,
				Score:    float32(1.0 - result.Additional.Distance), // Convert distance to similarity score
				Metadata: result.Properties,
			}

			results = append(results, searchResult)
		}
	}

	return results, nil
}

// QueryByText searches for similar vectors using text input (BR-VDB-017: Must provide approximate nearest neighbor search)
func (wdb *WeaviateVectorDatabase) QueryByText(ctx context.Context, text string, topK int, filters map[string]interface{}) ([]BaseSearchResult, error) {
	// Generate embedding for the query text
	embedding, err := wdb.embedding.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for query text: %w", err)
	}

	return wdb.Query(ctx, embedding, topK, filters)
}

// Delete removes vectors by their IDs (BR-VDB-004: Must support vector deletion)
func (wdb *WeaviateVectorDatabase) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	wdb.log.WithField("id_count", len(ids)).Info("Deleting vectors from Weaviate")

	for _, id := range ids {
		if err := wdb.deleteObject(ctx, id); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", id, err)
		}
	}

	wdb.log.WithField("id_count", len(ids)).Info("Successfully deleted vectors from Weaviate")
	return nil
}

// DeleteByFilter removes vectors matching the filter (BR-VDB-018: Must support filtered search)
func (wdb *WeaviateVectorDatabase) DeleteByFilter(ctx context.Context, filters map[string]interface{}) error {
	if len(filters) == 0 {
		return fmt.Errorf("filter cannot be empty for delete by filter operation")
	}

	wdb.log.WithField("filters", filters).Info("Deleting vectors by filter from Weaviate")

	// First, find objects matching the filter
	whereClause := wdb.buildWhereClause(filters)
	query := fmt.Sprintf(`{
		Get {
			%s(
				%s
			) {
				_additional {
					id
				}
			}
		}
	}`, wdb.config.ClassName, whereClause)

	req := &WeaviateQueryRequest{
		Query: query,
	}

	response, err := wdb.executeGraphQLQuery(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to query objects for deletion: %w", err)
	}

	// Extract IDs and delete them
	var ids []string
	if classResults, ok := response.Data.Get[wdb.config.ClassName]; ok {
		for _, result := range classResults {
			ids = append(ids, result.Additional.ID)
		}
	}

	if len(ids) > 0 {
		if err := wdb.Delete(ctx, ids); err != nil {
			return fmt.Errorf("failed to delete objects by filter: %w", err)
		}
	}

	wdb.log.WithFields(logrus.Fields{
		"filters":       filters,
		"deleted_count": len(ids),
	}).Info("Successfully deleted vectors by filter from Weaviate")
	return nil
}

// Close closes the database connection
func (wdb *WeaviateVectorDatabase) Close() error {
	// Weaviate is HTTP-based, no persistent connections to close
	wdb.log.Info("Closing Weaviate vector database connection")
	return nil
}

// batchObjects uploads a batch of objects to Weaviate
func (wdb *WeaviateVectorDatabase) batchObjects(ctx context.Context, objects []WeaviateObject) error {
	req := &WeaviateBatchRequest{
		Objects: objects,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}

	url := wdb.config.BaseURL + "/v1/batch/objects"
	return wdb.executeRequest(ctx, "POST", url, reqBody, nil)
}

// executeGraphQLQuery executes a GraphQL query
func (wdb *WeaviateVectorDatabase) executeGraphQLQuery(ctx context.Context, req *WeaviateQueryRequest) (*WeaviateQueryResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	url := wdb.config.BaseURL + "/v1/graphql"
	var response WeaviateQueryResponse
	if err := wdb.executeRequest(ctx, "POST", url, reqBody, &response); err != nil {
		return nil, err
	}

	// Check for GraphQL errors
	if len(response.Errors) > 0 {
		errorMsgs := make([]string, len(response.Errors))
		for i, err := range response.Errors {
			errorMsgs[i] = err.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(errorMsgs, "; "))
	}

	return &response, nil
}

// deleteObject deletes a single object by ID
func (wdb *WeaviateVectorDatabase) deleteObject(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/v1/objects/%s/%s", wdb.config.BaseURL, wdb.config.ClassName, id)
	return wdb.executeRequest(ctx, "DELETE", url, nil, nil)
}

// executeRequest executes an HTTP request to Weaviate with retry logic
func (wdb *WeaviateVectorDatabase) executeRequest(ctx context.Context, method, url string, body []byte, response interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= wdb.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			wdb.log.WithField("backoff", backoff).Debug("Backing off before retry")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		var httpReq *http.Request
		var err error
		if body != nil {
			httpReq, err = http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
		} else {
			httpReq, err = http.NewRequestWithContext(ctx, method, url, nil)
		}
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if wdb.config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+wdb.config.APIKey)
		}

		resp, err := wdb.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("weaviate API error (status %d): %s", resp.StatusCode, string(respBody))
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

	return fmt.Errorf("failed after %d retries: %w", wdb.config.MaxRetries, lastErr)
}

// buildWhereClause builds a GraphQL where clause from filters
func (wdb *WeaviateVectorDatabase) buildWhereClause(filters map[string]interface{}) string {
	if len(filters) == 0 {
		return ""
	}

	conditions := make([]string, 0, len(filters))
	for key, value := range filters {
		switch v := value.(type) {
		case string:
			conditions = append(conditions, fmt.Sprintf(`{path: ["%s"], operator: Equal, valueString: "%s"}`, key, v))
		case int, int64:
			conditions = append(conditions, fmt.Sprintf(`{path: ["%s"], operator: Equal, valueNumber: %v}`, key, v))
		case float64:
			conditions = append(conditions, fmt.Sprintf(`{path: ["%s"], operator: Equal, valueNumber: %f}`, key, v))
		case bool:
			conditions = append(conditions, fmt.Sprintf(`{path: ["%s"], operator: Equal, valueBoolean: %t}`, key, v))
		}
	}

	if len(conditions) == 1 {
		return fmt.Sprintf("where: %s", conditions[0])
	} else if len(conditions) > 1 {
		return fmt.Sprintf("where: {operator: And, operands: [%s]}", strings.Join(conditions, ", "))
	}

	return ""
}

// formatVector formats a vector for GraphQL query
func (wdb *WeaviateVectorDatabase) formatVector(vector []float64) string {
	vectorStrs := make([]string, len(vector))
	for i, v := range vector {
		vectorStrs[i] = fmt.Sprintf("%f", v)
	}
	return fmt.Sprintf("[%s]", strings.Join(vectorStrs, ", "))
}
