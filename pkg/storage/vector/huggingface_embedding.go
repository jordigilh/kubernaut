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

// HuggingFaceEmbeddingService implements ExternalEmbeddingGenerator for HuggingFace's embedding API
// Satisfies BR-VDB-002: HuggingFace embedding service integration
type HuggingFaceEmbeddingService struct {
	apiKey         string
	httpClient     *http.Client
	log            *logrus.Logger
	config         *HuggingFaceConfig
	cache          EmbeddingCache
	modelValidated bool // Track if model has been validated
	rateLimiter    *rate.Limiter
	mu             sync.RWMutex // Protects rate limiter and model validation state
}

// HuggingFaceConfig holds configuration for HuggingFace embedding service
type HuggingFaceConfig struct {
	Model         string            `yaml:"model" default:"sentence-transformers/all-MiniLM-L6-v2"`
	MaxRetries    int               `yaml:"max_retries" default:"3"`
	Timeout       time.Duration     `yaml:"timeout" default:"30s"`
	BaseURL       string            `yaml:"base_url" default:"https://api-inference.huggingface.co/pipeline/feature-extraction"`
	BatchSize     int               `yaml:"batch_size" default:"50"`
	RateLimit     int               `yaml:"rate_limit" default:"100"`
	Dimensions    int               `yaml:"dimensions" default:"384"`
<<<<<<< HEAD
	ModelOptions  map[string]string `yaml:"model_options,omitempty"`  // Additional model-specific options
	FallbackModel string            `yaml:"fallback_model,omitempty"` // Fallback model if primary fails
=======
	ModelOptions  map[string]string `yaml:"model_options,omitempty"`       // Additional model-specific options
	FallbackModel string            `yaml:"fallback_model,omitempty"`      // Fallback model if primary fails
>>>>>>> crd_implementation
	ValidateModel bool              `yaml:"validate_model" default:"true"` // Validate model availability on startup
}

// HuggingFaceRequest represents the request structure for HuggingFace embeddings
type HuggingFaceRequest struct {
	Inputs     interface{}            `json:"inputs"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// NewHuggingFaceEmbeddingService creates a new HuggingFace embedding service (BR-VDB-002)
func NewHuggingFaceEmbeddingService(apiKey string, cache EmbeddingCache, log *logrus.Logger) *HuggingFaceEmbeddingService {
	config := &HuggingFaceConfig{
		Model:         "sentence-transformers/all-MiniLM-L6-v2",
		MaxRetries:    3,
		Timeout:       30 * time.Second,
		BaseURL:       "https://api-inference.huggingface.co/pipeline/feature-extraction",
		BatchSize:     50,
		RateLimit:     100,
		Dimensions:    384,
		ValidateModel: true,
	}

	service := &HuggingFaceEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:            log,
		config:         config,
		cache:          cache,
		modelValidated: false,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.RateLimit), config.RateLimit),
	}

	return service
}

// NewHuggingFaceEmbeddingServiceWithConfig creates a new HuggingFace embedding service with custom configuration
// BR-VDB-002: Support configurable base URL for testing and custom endpoints
func NewHuggingFaceEmbeddingServiceWithConfig(apiKey string, cache EmbeddingCache, log *logrus.Logger, config *HuggingFaceConfig) *HuggingFaceEmbeddingService {
	if config == nil {
		// Use default configuration
		return NewHuggingFaceEmbeddingService(apiKey, cache, log)
	}

	// Set defaults for missing configuration
	if config.Model == "" {
		config.Model = "sentence-transformers/all-MiniLM-L6-v2"
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api-inference.huggingface.co/pipeline/feature-extraction"
	}
	if config.BatchSize == 0 {
		config.BatchSize = 50
	}
	if config.RateLimit == 0 {
		config.RateLimit = 100
	}
	if config.Dimensions == 0 {
		config.Dimensions = 384
	}

	service := &HuggingFaceEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:            log,
		config:         config,
		cache:          cache,
		modelValidated: false,
		rateLimiter:    rate.NewLimiter(rate.Limit(config.RateLimit), config.RateLimit),
	}

	return service
}

// GenerateEmbedding generates a single embedding from text (BR-VDB-006: Must generate embeddings from text)
func (hfs *HuggingFaceEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	// Apply rate limiting before processing
	if err := hfs.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiting error: %w", err)
	}

	// Validate model before first use (BR-VDB-002: Model validation for production)
	hfs.mu.RLock()
	validated := hfs.modelValidated
	hfs.mu.RUnlock()

	if !validated && hfs.config.ValidateModel {
		if err := hfs.ValidateModel(ctx); err != nil {
			return nil, fmt.Errorf("model validation failed: %w", err)
		}
	}

	// Check cache first (BR-VDB-007: Must support caching of embeddings)
	if hfs.cache != nil {
		if cached, found, err := hfs.cache.Get(ctx, text); err == nil && found {
			hfs.log.Debug("Returning cached HuggingFace embedding")
			return cached, nil
		}
	}

	// Create request
	req := &HuggingFaceRequest{
		Inputs: text,
		Options: map[string]interface{}{
			"wait_for_model": true,
		},
	}

	embedding, err := hfs.callHuggingFaceAPI(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HuggingFace embedding: %w", err)
	}

	// Cache the result (BR-VDB-007: Must support caching to reduce computation costs)
	if hfs.cache != nil {
		if err := hfs.cache.Set(ctx, text, embedding, time.Hour*24); err != nil {
			hfs.log.WithError(err).Warn("Failed to cache HuggingFace embedding")
		}
	}

	return embedding, nil
}

// GenerateBatchEmbeddings generates embeddings for multiple texts (BR-VDB-009: Must support batch embedding generation)
func (hfs *HuggingFaceEmbeddingService) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	// Apply rate limiting for batch operations (potentially multiple API calls)
	if err := hfs.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiting error: %w", err)
	}

	// Validate model before first use (BR-VDB-002: Model validation for production)
	hfs.mu.RLock()
	validated := hfs.modelValidated
	hfs.mu.RUnlock()

	if !validated && hfs.config.ValidateModel {
		if err := hfs.ValidateModel(ctx); err != nil {
			return nil, fmt.Errorf("model validation failed: %w", err)
		}
	}

	var results [][]float64
	batchSize := hfs.config.BatchSize

	hfs.log.WithFields(logrus.Fields{
		"total_texts": len(texts),
		"batch_size":  batchSize,
	}).Debug("Generating batch embeddings with HuggingFace")

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		// Apply rate limiting for each batch to respect API limits
		if i > 0 { // Skip first batch as we already waited above
			if err := hfs.rateLimiter.Wait(ctx); err != nil {
				return nil, fmt.Errorf("rate limiting error for batch %d-%d: %w", i, end-1, err)
			}
		}

		batch := texts[i:end]
		batchEmbeddings, err := hfs.generateBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to generate batch embeddings (batch %d-%d): %w", i, end-1, err)
		}

		results = append(results, batchEmbeddings...)
	}

	return results, nil
}

// generateBatch processes a single batch of texts
func (hfs *HuggingFaceEmbeddingService) generateBatch(ctx context.Context, texts []string) ([][]float64, error) {
	// Check cache for each text
	var uncachedTexts []string
	var uncachedIndices []int
	results := make([][]float64, len(texts))

	if hfs.cache != nil {
		for i, text := range texts {
			if cached, found, err := hfs.cache.Get(ctx, text); err == nil && found {
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
		hfs.log.Debug("All embeddings found in cache")
		return results, nil
	}

	// Create request for uncached texts
	req := &HuggingFaceRequest{
		Inputs: uncachedTexts,
		Options: map[string]interface{}{
			"wait_for_model": true,
		},
	}

	// Call HuggingFace API
	response, err := hfs.callHuggingFaceAPIBatch(ctx, req)
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
		if hfs.cache != nil {
			if err := hfs.cache.Set(ctx, uncachedTexts[i], embedding, time.Hour*24); err != nil {
				hfs.log.WithError(err).Warn("Failed to cache embedding")
			}
		}
	}

	return results, nil
}

// callHuggingFaceAPI makes a single embedding API call to HuggingFace
func (hfs *HuggingFaceEmbeddingService) callHuggingFaceAPI(ctx context.Context, req *HuggingFaceRequest) ([]float64, error) {
	response, err := hfs.callHuggingFaceAPIBatch(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(response) == 0 {
		return nil, fmt.Errorf("no embeddings returned from HuggingFace API")
	}

	return response[0], nil
}

// callHuggingFaceAPIBatch makes a batch embedding API call to HuggingFace
func (hfs *HuggingFaceEmbeddingService) callHuggingFaceAPIBatch(ctx context.Context, req *HuggingFaceRequest) ([][]float64, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= hfs.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			hfs.log.WithField("backoff", backoff).Debug("Backing off before retry")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Use model-specific URL if specified
		apiURL := hfs.config.BaseURL
		if hfs.config.Model != "" {
			apiURL = fmt.Sprintf("https://api-inference.huggingface.co/models/%s", hfs.config.Model)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if hfs.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+hfs.apiKey)
		}

		resp, err := hfs.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				hfs.log.WithError(err).Error("Failed to close HTTP response body")
			}
		}()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HuggingFace API error (status %d): %s", resp.StatusCode, string(body))
			// Don't retry on authentication or client errors
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}

		// HuggingFace returns embeddings as nested arrays
		var apiResp [][]float64
		if err := json.Unmarshal(body, &apiResp); err != nil {
			// Try single embedding format
			var singleEmbedding []float64
			if err := json.Unmarshal(body, &singleEmbedding); err != nil {
				lastErr = fmt.Errorf("failed to unmarshal response: %w", err)
				continue
			}
			apiResp = [][]float64{singleEmbedding}
		}

		hfs.log.WithFields(logrus.Fields{
			"embeddings_count": len(apiResp),
			"model":            hfs.config.Model,
			"dimension":        len(apiResp[0]),
		}).Debug("Successfully generated HuggingFace embeddings")

		return apiResp, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", hfs.config.MaxRetries, lastErr)
}

// GetDimension returns the dimensionality of embeddings (BR-VDB-010: Must implement embedding versioning)
func (hfs *HuggingFaceEmbeddingService) GetDimension() int {
	return hfs.config.Dimensions
}

// GetModel returns the model name
func (hfs *HuggingFaceEmbeddingService) GetModel() string {
	return hfs.config.Model
}

// ValidateModel validates that the specified model is available and working
// BR-VDB-002: Model validation for production deployment
func (hfs *HuggingFaceEmbeddingService) ValidateModel(ctx context.Context) error {
	if !hfs.config.ValidateModel {
		hfs.log.Debug("Model validation disabled in configuration")
		hfs.mu.Lock()
		hfs.modelValidated = true
		hfs.mu.Unlock()
		return nil
	}

	hfs.mu.RLock()
	if hfs.modelValidated {
		hfs.mu.RUnlock()
		return nil // Already validated
	}
	hfs.mu.RUnlock()

	hfs.log.WithField("model", hfs.config.Model).Info("Validating HuggingFace model availability")

	// Test with a simple input to validate model availability
	testText := "test"
	req := &HuggingFaceRequest{
		Inputs: testText,
		Options: map[string]interface{}{
			"wait_for_model": true,
		},
	}

	// Use shorter timeout for validation
	originalTimeout := hfs.httpClient.Timeout
	hfs.httpClient.Timeout = 15 * time.Second
	defer func() {
		hfs.httpClient.Timeout = originalTimeout
	}()

	_, err := hfs.callHuggingFaceAPI(ctx, req)
	if err != nil {
		hfs.log.WithError(err).WithField("model", hfs.config.Model).Error("Model validation failed")

		// Try fallback model if configured
		if hfs.config.FallbackModel != "" {
			hfs.log.WithField("fallback_model", hfs.config.FallbackModel).Info("Attempting to use fallback model")
			originalModel := hfs.config.Model
			hfs.config.Model = hfs.config.FallbackModel

			_, fallbackErr := hfs.callHuggingFaceAPI(ctx, req)
			if fallbackErr != nil {
				// Restore original model
				hfs.config.Model = originalModel
				return fmt.Errorf("both primary model (%s) and fallback model (%s) validation failed: primary=%w, fallback=%v",
					originalModel, hfs.config.FallbackModel, err, fallbackErr)
			}

			hfs.log.WithField("fallback_model", hfs.config.FallbackModel).Info("Successfully switched to fallback model")
		} else {
			return fmt.Errorf("model validation failed for %s: %w", hfs.config.Model, err)
		}
	}

	hfs.mu.Lock()
	hfs.modelValidated = true
	hfs.mu.Unlock()
	hfs.log.WithField("model", hfs.config.Model).Info("Model validation successful")
	return nil
}

// SetModel changes the model being used and revalidates if needed
// BR-VDB-002: Support configurable model selection for different use cases
func (hfs *HuggingFaceEmbeddingService) SetModel(model string) error {
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	hfs.mu.Lock()
	defer hfs.mu.Unlock()

	if model == hfs.config.Model {
		return nil // No change needed
	}

	hfs.log.WithFields(logrus.Fields{
		"old_model": hfs.config.Model,
		"new_model": model,
	}).Info("Changing HuggingFace model")

	hfs.config.Model = model
	hfs.modelValidated = false // Mark as needing revalidation

	return nil
}

// GetSupportedModels returns a list of commonly supported HuggingFace models
// BR-VDB-002: Provide guidance for model selection
func (hfs *HuggingFaceEmbeddingService) GetSupportedModels() []ModelInfo {
	return []ModelInfo{
		{
			Name:        "sentence-transformers/all-MiniLM-L6-v2",
			Description: "Lightweight, fast model with 384 dimensions - good for general use",
			Dimensions:  384,
			UseCase:     "general",
		},
		{
			Name:        "sentence-transformers/all-mpnet-base-v2",
			Description: "High-quality model with 768 dimensions - best performance",
			Dimensions:  768,
			UseCase:     "high_quality",
		},
		{
			Name:        "sentence-transformers/all-distilroberta-v1",
			Description: "Balanced performance and speed with 768 dimensions",
			Dimensions:  768,
			UseCase:     "balanced",
		},
		{
			Name:        "sentence-transformers/paraphrase-albert-small-v2",
			Description: "Small, efficient model with 768 dimensions",
			Dimensions:  768,
			UseCase:     "efficient",
		},
		{
			Name:        "sentence-transformers/multi-qa-MiniLM-L6-cos-v1",
			Description: "Optimized for question-answering tasks with 384 dimensions",
			Dimensions:  384,
			UseCase:     "qa",
		},
	}
}

// ModelInfo provides information about available models
type ModelInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Dimensions  int    `json:"dimensions"`
	UseCase     string `json:"use_case"`
}

// IsModelValidated returns whether the current model has been validated
func (hfs *HuggingFaceEmbeddingService) IsModelValidated() bool {
	hfs.mu.RLock()
	defer hfs.mu.RUnlock()
	return hfs.modelValidated
}

// UpdateRateLimit updates the rate limiter with new rate limit settings
// BR-VDB-002: Support dynamic configuration updates
func (hfs *HuggingFaceEmbeddingService) UpdateRateLimit(requestsPerSecond int) {
	hfs.mu.Lock()
	defer hfs.mu.Unlock()

	hfs.config.RateLimit = requestsPerSecond
	hfs.rateLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond)

	hfs.log.WithField("new_rate_limit", requestsPerSecond).Info("Updated HuggingFace rate limit")
}

// GetCurrentRateLimit returns the current rate limit setting
func (hfs *HuggingFaceEmbeddingService) GetCurrentRateLimit() int {
	hfs.mu.RLock()
	defer hfs.mu.RUnlock()
	return hfs.config.RateLimit
}
