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

// OpenAIEmbeddingService implements EmbeddingGenerator for OpenAI's embedding API
type OpenAIEmbeddingService struct {
	apiKey     string
	httpClient *http.Client
	log        *logrus.Logger
	config     *OpenAIConfig
	cache      EmbeddingCache
}

// OpenAIConfig holds configuration for OpenAI embedding service
type OpenAIConfig struct {
	Model      string        `yaml:"model" default:"text-embedding-3-small"`
	MaxRetries int           `yaml:"max_retries" default:"3"`
	Timeout    time.Duration `yaml:"timeout" default:"30s"`
	BaseURL    string        `yaml:"base_url" default:"https://api.openai.com/v1"`
	BatchSize  int           `yaml:"batch_size" default:"100"`
	RateLimit  int           `yaml:"rate_limit" default:"60"`
	Dimensions int           `yaml:"dimensions" default:"1536"`
}

// OpenAIEmbeddingRequest represents the request structure for OpenAI embeddings
type OpenAIEmbeddingRequest struct {
	Input      interface{} `json:"input"`
	Model      string      `json:"model"`
	Dimensions *int        `json:"dimensions,omitempty"`
}

// OpenAIEmbeddingResponse represents the response structure from OpenAI
type OpenAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIEmbeddingService creates a new OpenAI embedding service
func NewOpenAIEmbeddingService(apiKey string, cache EmbeddingCache, log *logrus.Logger) *OpenAIEmbeddingService {
	config := &OpenAIConfig{
		Model:      "text-embedding-3-small",
		MaxRetries: 3,
		Timeout:    30 * time.Second,
		BaseURL:    "https://api.openai.com/v1",
		BatchSize:  100,
		RateLimit:  60,
		Dimensions: 1536,
	}

	return &OpenAIEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:    log,
		config: config,
		cache:  cache,
	}
}

// GenerateEmbedding generates a single embedding from text
func (oes *OpenAIEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	// Check cache first
	if oes.cache != nil {
		if cached, found, err := oes.cache.Get(ctx, text); err == nil && found {
			oes.log.Debug("Returning cached OpenAI embedding")
			return cached, nil
		}
	}

	// Create request
	req := &OpenAIEmbeddingRequest{
		Input: text,
		Model: oes.config.Model,
	}
	if oes.config.Dimensions > 0 {
		req.Dimensions = &oes.config.Dimensions
	}

	embedding, err := oes.callOpenAIAPI(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAI embedding: %w", err)
	}

	// Cache the result
	if oes.cache != nil {
		if err := oes.cache.Set(ctx, text, embedding, time.Hour*24); err != nil {
			oes.log.WithError(err).Warn("Failed to cache OpenAI embedding")
		}
	}

	return embedding, nil
}

// GenerateBatchEmbeddings generates embeddings for multiple texts
func (oes *OpenAIEmbeddingService) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	var results [][]float64
	batchSize := oes.config.BatchSize

	oes.log.WithFields(logrus.Fields{
		"total_texts": len(texts),
		"batch_size":  batchSize,
	}).Debug("Generating batch embeddings with OpenAI")

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		batchEmbeddings, err := oes.generateBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to generate batch embeddings (batch %d-%d): %w", i, end-1, err)
		}

		results = append(results, batchEmbeddings...)
	}

	return results, nil
}

// generateBatch processes a single batch of texts
func (oes *OpenAIEmbeddingService) generateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	// Check cache for each text
	var uncachedTexts []string
	var uncachedIndices []int
	results := make([][]float64, len(texts))

	if oes.cache != nil {
		for i, text := range texts {
			if cached, found, err := oes.cache.Get(ctx, text); err == nil && found {
				results[i] = cached
			} else {
				uncachedTexts = append(uncachedTexts, text)
				uncachedIndices = append(uncachedIndices, i)
			}
		}
	} else {
		uncachedTexts = texts
		for i := range texts {
			uncachedIndices = append(uncachedIndices, i)
		}
	}

	// If all texts are cached, return results
	if len(uncachedTexts) == 0 {
		oes.log.Debug("All embeddings found in cache")
		return results, nil
	}

	// Create request for uncached texts
	req := &OpenAIEmbeddingRequest{
		Input: uncachedTexts,
		Model: oes.config.Model,
	}
	if oes.config.Dimensions > 0 {
		req.Dimensions = &oes.config.Dimensions
	}

	// Call OpenAI API
	response, err := oes.callOpenAIAPIBatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate batch embeddings: %w", err)
	}

	// Map results back to original indices and cache them
	for i, embedding := range response {
		if i >= len(uncachedIndices) {
			break
		}
		originalIndex := uncachedIndices[i]
		results[originalIndex] = embedding

		// Cache the result
		if oes.cache != nil {
			if err := oes.cache.Set(ctx, uncachedTexts[i], embedding, time.Hour*24); err != nil {
				oes.log.WithError(err).Warn("Failed to cache embedding")
			}
		}
	}

	return results, nil
}

// callOpenAIAPI makes a single embedding API call to OpenAI
func (oes *OpenAIEmbeddingService) callOpenAIAPI(ctx context.Context, req *OpenAIEmbeddingRequest) ([]float64, error) {
	response, err := oes.callOpenAIAPIBatch(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(response) == 0 {
		return nil, fmt.Errorf("no embeddings returned from OpenAI API")
	}

	return response[0], nil
}

// callOpenAIAPIBatch makes a batch embedding API call to OpenAI
func (oes *OpenAIEmbeddingService) callOpenAIAPIBatch(ctx context.Context, req *OpenAIEmbeddingRequest) ([][]float64, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= oes.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			oes.log.WithField("backoff", backoff).Debug("Backing off before retry")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", oes.config.BaseURL+"/embeddings", bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+oes.apiKey)

		resp, err := oes.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				oes.log.WithError(err).Error("Failed to close HTTP response body")
			}
		}()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(body))
			// Don't retry on authentication or client errors
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}

		var apiResp OpenAIEmbeddingResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			lastErr = fmt.Errorf("failed to unmarshal response: %w", err)
			continue
		}

		// Extract embeddings from response
		embeddings := make([][]float64, len(apiResp.Data))
		for i, data := range apiResp.Data {
			embeddings[i] = data.Embedding
		}

		oes.log.WithFields(logrus.Fields{
			"embeddings_count": len(embeddings),
			"model":            apiResp.Model,
			"tokens_used":      apiResp.Usage.TotalTokens,
		}).Debug("Successfully generated OpenAI embeddings")

		return embeddings, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", oes.config.MaxRetries, lastErr)
}

// GetDimension returns the dimensionality of embeddings
func (oes *OpenAIEmbeddingService) GetDimension() int {
	return oes.config.Dimensions
}

// GetModel returns the model name
func (oes *OpenAIEmbeddingService) GetModel() string {
	return oes.config.Model
}
