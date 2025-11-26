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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"go.uber.org/zap"
)

const (
	// EmbeddingDimensions is the expected number of dimensions for all-mpnet-base-v2
	EmbeddingDimensions = 768

	// DefaultTimeout is the default HTTP timeout for embedding requests
	DefaultTimeout = 30 * time.Second

	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 3

	// DefaultRetryDelay is the base delay between retries (exponential backoff)
	DefaultRetryDelay = 1 * time.Second

	// DefaultCacheTTL is the default time-to-live for cached embeddings (24 hours)
	// Note: This differs from pipeline.CacheTTL (5 minutes) which is for a different use case
	DefaultCacheTTL = 24 * time.Hour
)

// service provides text-to-vector embedding generation with caching and retry logic.
// It implements the embedding.Client interface.
//
// Features:
// - HTTP client for Python embedding service (localhost:8086)
// - Redis caching with 24-hour TTL (graceful degradation)
// - Exponential backoff retry logic (3 attempts)
// - Comprehensive error handling and logging
//
// Design Decision: DD-CACHE-001 (Shared Redis Library)
// Business Requirement: BR-STORAGE-014 (Data Storage embedding cache)
//
// Architecture:
// - Sidecar pattern: Python service on localhost:8086
// - L1 cache: Redis (24h TTL, optional)
// - L2 source: Python embedding service
//
// Performance:
// - Cache hit: ~1-3ms (Redis GET)
// - Cache miss: ~50-100ms (Python service + Redis SET)
// - Retry overhead: ~1-3s (exponential backoff)
type service struct {
	baseURL    string
	httpClient *http.Client
	cache      *rediscache.Cache[[]float32]
	logger     *zap.Logger
	maxRetries int
	retryDelay time.Duration
}

// EmbedRequest represents the request payload for the embedding service.
type EmbedRequest struct {
	Text string `json:"text"`
}

// EmbedResponse represents the response from the embedding service.
type EmbedResponse struct {
	Embedding  []float32 `json:"embedding"`
	Dimensions int       `json:"dimensions"`
	Model      string    `json:"model"`
}

// HealthResponse represents the response from the health check endpoint.
type HealthResponse struct {
	Status     string `json:"status"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

// NewClient creates a new embedding client with default configuration.
//
// Parameters:
//   - baseURL: Embedding service URL (e.g., "http://localhost:8086")
//   - cache: Redis cache for embeddings (nil to disable caching)
//   - logger: Structured logger
//
// Returns:
//   - Client: Embedding client ready for use (implements embedding.Client interface)
//
// Example:
//
//	cache := rediscache.NewCache[[]float32](redisClient, "embeddings", 24*time.Hour)
//	client := embedding.NewClient("http://localhost:8086", cache, logger)
//	embedding, err := client.Embed(ctx, "OOMKilled pod in production")
func NewClient(baseURL string, cache *rediscache.Cache[[]float32], logger *zap.Logger) Client {
	return &service{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		cache:      cache,
		logger:     logger,
		maxRetries: DefaultMaxRetries,
		retryDelay: DefaultRetryDelay,
	}
}

// Embed generates a 768-dimensional embedding vector for the given text.
//
// This method implements a two-tier caching strategy:
// 1. Check Redis cache (L1) - ~1-3ms
// 2. Call Python embedding service (L2) - ~50-100ms
// 3. Store result in Redis cache for future requests
//
// Retry Logic:
// - Exponential backoff: 1s, 2s, 4s
// - Retries on transient errors (network, 5xx)
// - No retry on client errors (4xx)
//
// Graceful Degradation:
// - If Redis unavailable: Skip cache, call service directly
// - If service unavailable: Return error after retries
//
// Parameters:
//   - ctx: Context for timeout and cancellation
//   - text: Input text (1-10000 characters)
//
// Returns:
//   - []float32: 768-dimensional embedding vector
//   - error: nil on success, error on failure
//
// Example:
//
//	embedding, err := client.Embed(ctx, "OOMKilled pod in production namespace")
//	if err != nil {
//	    return fmt.Errorf("failed to generate embedding: %w", err)
//	}
//	// Use embedding for semantic search
//	results, err := repository.SearchByEmbedding(ctx, embedding, filters)
func (c *service) Embed(ctx context.Context, text string) ([]float32, error) {
	// Validate input
	if text == "" {
		return nil, fmt.Errorf("text must be non-empty")
	}

	// Check cache (L1)
	if c.cache != nil {
		cached, err := c.cache.Get(ctx, text)
		if err == nil && cached != nil {
			c.logger.Debug("Cache hit for embedding",
				zap.String("text", text[:min(50, len(text))]),
				zap.Int("dimensions", len(*cached)))
			return *cached, nil
		}

		// Log cache miss (not an error)
		if err != rediscache.ErrCacheMiss {
			c.logger.Warn("Cache lookup failed, proceeding without cache",
				zap.Error(err),
				zap.String("text", text[:min(50, len(text))]))
		}
	}

	// Generate embedding with retry logic (L2)
	embedding, err := c.embedWithRetry(ctx, text)
	if err != nil {
		return nil, err
	}

	// Store in cache for future requests (fire-and-forget)
	if c.cache != nil {
		go func() {
			// Use background context to avoid cancellation
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := c.cache.Set(cacheCtx, text, &embedding); err != nil {
				c.logger.Warn("Failed to cache embedding",
					zap.Error(err),
					zap.String("text", text[:min(50, len(text))]))
			} else {
				c.logger.Debug("Cached embedding",
					zap.String("text", text[:min(50, len(text))]),
					zap.Duration("ttl", DefaultCacheTTL))
			}
		}()
	}

	return embedding, nil
}

// embedWithRetry calls the embedding service with exponential backoff retry logic.
//
// Retry Strategy:
// - Attempt 1: Immediate
// - Attempt 2: 1s delay
// - Attempt 3: 2s delay
// - Attempt 4: 4s delay
//
// Retryable Errors:
// - Network errors (connection refused, timeout)
// - HTTP 5xx errors (server errors)
//
// Non-Retryable Errors:
// - HTTP 4xx errors (client errors - invalid input)
// - Context cancellation
func (c *service) embedWithRetry(ctx context.Context, text string) ([]float32, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Attempt embedding generation
		embedding, err := c.callEmbeddingService(ctx, text)
		if err == nil {
			if attempt > 0 {
				c.logger.Info("Embedding generation succeeded after retry",
					zap.Int("attempt", attempt+1),
					zap.String("text", text[:min(50, len(text))]))
			}
			return embedding, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			c.logger.Warn("Non-retryable error, aborting",
				zap.Error(err),
				zap.Int("attempt", attempt+1))
			return nil, err
		}

		// Last attempt - don't retry
		if attempt == c.maxRetries {
			break
		}

		// Calculate exponential backoff delay
		delay := c.retryDelay * time.Duration(1<<attempt) // 1s, 2s, 4s
		c.logger.Warn("Embedding generation failed, retrying",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Duration("retry_delay", delay))

		// Wait before retry (with context cancellation check)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return nil, fmt.Errorf("embedding generation failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// callEmbeddingService makes a single HTTP request to the embedding service.
func (c *service) callEmbeddingService(ctx context.Context, text string) ([]float32, error) {
	// Prepare request payload
	reqBody := EmbedRequest{Text: text}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + "/api/v1/embed"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var embedResp EmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Validate dimensions
	if len(embedResp.Embedding) != EmbeddingDimensions {
		return nil, fmt.Errorf("unexpected embedding dimensions: got %d, expected %d",
			len(embedResp.Embedding), EmbeddingDimensions)
	}

	return embedResp.Embedding, nil
}

// Health checks the health of the embedding service.
//
// Returns:
//   - error: nil if healthy, error if unhealthy or unreachable
//
// Example:
//
//	if err := client.Health(ctx); err != nil {
//	    logger.Error("Embedding service unhealthy", zap.Error(err))
//	}
func (c *service) Health(ctx context.Context) error {
	url := c.baseURL + "/health"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse health response
	var healthResp HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		return fmt.Errorf("failed to parse health response: %w", err)
	}

	// Validate model configuration
	if healthResp.Dimensions != EmbeddingDimensions {
		return fmt.Errorf("embedding dimensions mismatch: got %d, expected %d",
			healthResp.Dimensions, EmbeddingDimensions)
	}

	c.logger.Debug("Embedding service healthy",
		zap.String("model", healthResp.Model),
		zap.Int("dimensions", healthResp.Dimensions))

	return nil
}

// isRetryable determines if an error should trigger a retry.
func isRetryable(err error) bool {
	// Network errors are retryable
	if err == nil {
		return false
	}

	errStr := err.Error()

	// HTTP 5xx errors are retryable
	if contains(errStr, "status 5") {
		return true
	}

	// HTTP 4xx errors are NOT retryable (client errors)
	if contains(errStr, "status 4") {
		return false
	}

	// Network errors are retryable
	if contains(errStr, "connection refused") ||
		contains(errStr, "timeout") ||
		contains(errStr, "no such host") ||
		contains(errStr, "network is unreachable") {
		return true
	}

	// Context errors are NOT retryable
	if contains(errStr, "context canceled") ||
		contains(errStr, "context deadline exceeded") {
		return false
	}

	// Default: retry
	return true
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
