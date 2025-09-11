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

// HuggingFaceEmbeddingService implements ExternalEmbeddingGenerator for HuggingFace's embedding API
// Satisfies BR-VDB-002: HuggingFace embedding service integration
type HuggingFaceEmbeddingService struct {
	apiKey     string
	httpClient *http.Client
	log        *logrus.Logger
	config     *HuggingFaceConfig
	cache      EmbeddingCache
}

// HuggingFaceConfig holds configuration for HuggingFace embedding service
type HuggingFaceConfig struct {
	Model      string        `yaml:"model" default:"sentence-transformers/all-MiniLM-L6-v2"`
	MaxRetries int           `yaml:"max_retries" default:"3"`
	Timeout    time.Duration `yaml:"timeout" default:"30s"`
	BaseURL    string        `yaml:"base_url" default:"https://api-inference.huggingface.co/pipeline/feature-extraction"`
	BatchSize  int           `yaml:"batch_size" default:"50"`
	RateLimit  int           `yaml:"rate_limit" default:"100"`
	Dimensions int           `yaml:"dimensions" default:"384"`
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
		Model:      "sentence-transformers/all-MiniLM-L6-v2",
		MaxRetries: 3,
		Timeout:    30 * time.Second,
		BaseURL:    "https://api-inference.huggingface.co/pipeline/feature-extraction",
		BatchSize:  50,
		RateLimit:  100,
		Dimensions: 384,
	}

	return &HuggingFaceEmbeddingService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		log:    log,
		config: config,
		cache:  cache,
	}
}

// GenerateEmbedding generates a single embedding from text (BR-VDB-006: Must generate embeddings from text)
func (hfs *HuggingFaceEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
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
