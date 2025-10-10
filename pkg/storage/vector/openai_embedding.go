<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// OpenAIEmbeddingService implements EmbeddingGenerator for OpenAI's embedding API
type OpenAIEmbeddingService struct {
	apiKey         string
	httpClient     *http.Client
	log            *logrus.Logger
	config         *OpenAIConfig
	cache          EmbeddingCache
	rateLimiter    *rate.Limiter
	mu             sync.RWMutex // Protects rate limiter and configuration updates
	modelValidated bool         // Track if model has been validated
}

// OpenAIConfig holds configuration for OpenAI embedding service
type OpenAIConfig struct {
	Model         string            `yaml:"model" default:"text-embedding-3-small"`
	MaxRetries    int               `yaml:"max_retries" default:"3"`
	Timeout       time.Duration     `yaml:"timeout" default:"30s"`
	BaseURL       string            `yaml:"base_url" default:"https://api.openai.com/v1"`
	BatchSize     int               `yaml:"batch_size" default:"100"`
	RateLimit     int               `yaml:"rate_limit" default:"60"`
	Dimensions    int               `yaml:"dimensions" default:"1536"`
<<<<<<< HEAD
	ModelOptions  map[string]string `yaml:"model_options,omitempty"`  // Additional model-specific options
	FallbackModel string            `yaml:"fallback_model,omitempty"` // Fallback model if primary fails
=======
	ModelOptions  map[string]string `yaml:"model_options,omitempty"`       // Additional model-specific options
	FallbackModel string            `yaml:"fallback_model,omitempty"`      // Fallback model if primary fails
>>>>>>> crd_implementation
	ValidateModel bool              `yaml:"validate_model" default:"true"` // Validate model availability on startup
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
		Model:         "text-embedding-3-small",
		MaxRetries:    3,
		Timeout:       30 * time.Second,
		BaseURL:       "https://api.openai.com/v1",
		BatchSize:     100,
		RateLimit:     60,
		Dimensions:    1536,
		ValidateModel: true,
	}

	return &OpenAIEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:            log,
		config:         config,
		cache:          cache,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.RateLimit), config.RateLimit),
		modelValidated: false,
	}
}

// NewOpenAIEmbeddingServiceWithConfig creates a new OpenAI embedding service with custom configuration
// BR-VDB-001: Support configurable base URL for testing and custom endpoints
func NewOpenAIEmbeddingServiceWithConfig(apiKey string, cache EmbeddingCache, log *logrus.Logger, config *OpenAIConfig) *OpenAIEmbeddingService {
	if config == nil {
		// Use default configuration
		return NewOpenAIEmbeddingService(apiKey, cache, log)
	}

	// Set defaults for missing configuration
	if config.Model == "" {
		config.Model = "text-embedding-3-small"
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.RateLimit == 0 {
		config.RateLimit = 60
	}
	if config.Dimensions == 0 {
		config.Dimensions = 1536
	}

	return &OpenAIEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:            log,
		config:         config,
		cache:          cache,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.RateLimit), config.RateLimit),
		modelValidated: false,
	}
}

// GenerateEmbedding generates a single embedding from text
func (oes *OpenAIEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	// Apply rate limiting before processing
	if err := oes.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiting error: %w", err)
	}

	// Validate model before first use if enabled
	oes.mu.RLock()
	validated := oes.modelValidated
	oes.mu.RUnlock()

	if !validated && oes.config.ValidateModel {
		if err := oes.ValidateModel(ctx); err != nil {
			return nil, fmt.Errorf("model validation failed: %w", err)
		}
	}

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

	// Apply rate limiting for batch operations (potentially multiple API calls)
	if err := oes.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiting error: %w", err)
	}

	// Validate model before first use if enabled
	oes.mu.RLock()
	validated := oes.modelValidated
	oes.mu.RUnlock()

	if !validated && oes.config.ValidateModel {
		if err := oes.ValidateModel(ctx); err != nil {
			return nil, fmt.Errorf("model validation failed: %w", err)
		}
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

		// Apply rate limiting for each batch to respect API limits
		if i > 0 { // Skip first batch as we already waited above
			if err := oes.rateLimiter.Wait(ctx); err != nil {
				return nil, fmt.Errorf("rate limiting error for batch %d-%d: %w", i, end-1, err)
			}
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

// ValidateModel validates that the specified model is available and working
// BR-VDB-002: Model validation for production deployment
func (oes *OpenAIEmbeddingService) ValidateModel(ctx context.Context) error {
	if !oes.config.ValidateModel {
		oes.log.Debug("Model validation disabled in configuration")
		oes.mu.Lock()
		oes.modelValidated = true
		oes.mu.Unlock()
		return nil
	}

	oes.mu.RLock()
	if oes.modelValidated {
		oes.mu.RUnlock()
		return nil // Already validated
	}
	oes.mu.RUnlock()

	oes.log.WithField("model", oes.config.Model).Info("Validating OpenAI model availability")

	// Test with a simple input to validate model availability
	testText := "test"
	req := &OpenAIEmbeddingRequest{
		Input: testText,
		Model: oes.config.Model,
	}
	if oes.config.Dimensions > 0 {
		req.Dimensions = &oes.config.Dimensions
	}

	// Use shorter timeout for validation
	originalTimeout := oes.httpClient.Timeout
	oes.httpClient.Timeout = 15 * time.Second
	defer func() {
		oes.httpClient.Timeout = originalTimeout
	}()

	_, err := oes.callOpenAIAPI(ctx, req)
	if err != nil {
		oes.log.WithError(err).WithField("model", oes.config.Model).Error("Model validation failed")

		// Try fallback model if configured
		if oes.config.FallbackModel != "" {
			oes.log.WithField("fallback_model", oes.config.FallbackModel).Info("Attempting to use fallback model")
			originalModel := oes.config.Model
			oes.config.Model = oes.config.FallbackModel

			req.Model = oes.config.FallbackModel
			_, fallbackErr := oes.callOpenAIAPI(ctx, req)
			if fallbackErr != nil {
				// Restore original model
				oes.config.Model = originalModel
				return fmt.Errorf("both primary model (%s) and fallback model (%s) validation failed: primary=%w, fallback=%v",
					originalModel, oes.config.FallbackModel, err, fallbackErr)
			}

			oes.log.WithField("fallback_model", oes.config.FallbackModel).Info("Successfully switched to fallback model")
		} else {
			return fmt.Errorf("model validation failed for %s: %w", oes.config.Model, err)
		}
	}

	oes.mu.Lock()
	oes.modelValidated = true
	oes.mu.Unlock()
	oes.log.WithField("model", oes.config.Model).Info("Model validation successful")
	return nil
}

// SetModel changes the model being used and revalidates if needed
// BR-VDB-002: Support configurable model selection for different use cases
func (oes *OpenAIEmbeddingService) SetModel(model string) error {
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	oes.mu.Lock()
	defer oes.mu.Unlock()

	if model == oes.config.Model {
		return nil // No change needed
	}

	oes.log.WithFields(logrus.Fields{
		"old_model": oes.config.Model,
		"new_model": model,
	}).Info("Changing OpenAI model")

	oes.config.Model = model
	oes.modelValidated = false // Mark as needing revalidation

	return nil
}

// GetSupportedModels returns a list of commonly supported OpenAI models
// BR-VDB-002: Provide guidance for model selection
func (oes *OpenAIEmbeddingService) GetSupportedModels() []ModelInfo {
	return []ModelInfo{
		{
			Name:        "text-embedding-3-small",
			Description: "Smallest embedding model with 1536 dimensions - cost efficient",
			Dimensions:  1536,
			UseCase:     "general",
		},
		{
			Name:        "text-embedding-3-large",
			Description: "Large embedding model with 3072 dimensions - highest quality",
			Dimensions:  3072,
			UseCase:     "high_quality",
		},
		{
			Name:        "text-embedding-ada-002",
			Description: "Legacy embedding model with 1536 dimensions - stable and reliable",
			Dimensions:  1536,
			UseCase:     "legacy",
		},
	}
}

// IsModelValidated returns whether the current model has been validated
func (oes *OpenAIEmbeddingService) IsModelValidated() bool {
	oes.mu.RLock()
	defer oes.mu.RUnlock()
	return oes.modelValidated
}

// UpdateRateLimit updates the rate limiter with new rate limit settings
// BR-VDB-002: Support dynamic configuration updates
func (oes *OpenAIEmbeddingService) UpdateRateLimit(requestsPerSecond int) {
	oes.mu.Lock()
	defer oes.mu.Unlock()

	oes.config.RateLimit = requestsPerSecond
	oes.rateLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)

	oes.log.WithField("new_rate_limit", requestsPerSecond).Info("Updated OpenAI rate limit")
}

// GetCurrentRateLimit returns the current rate limit setting
func (oes *OpenAIEmbeddingService) GetCurrentRateLimit() int {
	oes.mu.RLock()
	defer oes.mu.RUnlock()
	return oes.config.RateLimit
}

// GetTokenUsage returns token usage statistics for the last API call
// BR-VDB-002: Provide usage analytics for cost optimization
func (oes *OpenAIEmbeddingService) GetTokenUsage() map[string]interface{} {
	// This would be populated during API calls in a real implementation
	// For now, return basic usage information
	return map[string]interface{}{
<<<<<<< HEAD
		"model":       oes.config.Model,
		"rate_limit":  oes.config.RateLimit,
		"batch_size":  oes.config.BatchSize,
		"dimensions":  oes.config.Dimensions,
		"validated":   oes.IsModelValidated(),
=======
		"model":      oes.config.Model,
		"rate_limit": oes.config.RateLimit,
		"batch_size": oes.config.BatchSize,
		"dimensions": oes.config.Dimensions,
		"validated":  oes.IsModelValidated(),
>>>>>>> crd_implementation
	}
}
