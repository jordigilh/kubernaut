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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

const (
	// CacheTTL is the time-to-live for cached embeddings (5 minutes)
	CacheTTL = 5 * time.Minute
)

// Pipeline orchestrates embedding generation with caching.
// Business Requirement: BR-STORAGE-012 (Vector embeddings for semantic search)
type Pipeline struct {
	apiClient Client
	cache     Cache
	logger    logr.Logger
}

// NewPipeline creates a new embedding pipeline.
func NewPipeline(apiClient Client, cache Cache, logger logr.Logger) *Pipeline {
	return &Pipeline{
		apiClient: apiClient,
		cache:     cache,
		logger:    logger,
	}
}

// Generate generates an embedding for the given audit.
// Business Requirement: BR-STORAGE-012
func (p *Pipeline) Generate(ctx context.Context, audit *models.RemediationAudit) (*EmbeddingResult, error) {
	// Validate input
	if audit == nil {
		return nil, fmt.Errorf("audit is nil")
	}

	// Convert audit to text representation
	text := auditToText(audit)

	// Generate cache key
	cacheKey := generateCacheKey(text)

	// Check cache first
	if cached, err := p.cache.Get(ctx, cacheKey); err == nil {
		p.logger.V(1).Info("cache hit for embedding",
			"cache_key", cacheKey,
			"dimension", len(cached))

		return &EmbeddingResult{
			Embedding: cached,
			Dimension: len(cached),
			CacheHit:  true,
		}, nil
	}

	// Cache miss - generate embedding via API
	p.logger.V(1).Info("cache miss for embedding, calling API",
		"cache_key", cacheKey)

	embedding, err := p.apiClient.Embed(ctx, text)
	if err != nil {
		p.logger.Error(err, "failed to generate embedding",
			"cache_key", cacheKey)
		return nil, fmt.Errorf("embedding API error: %w", err)
	}

	// Store in cache
	if err := p.cache.Set(ctx, cacheKey, embedding, CacheTTL); err != nil {
		// Log cache failure but don't fail the request
		p.logger.Info("failed to cache embedding",
			"error", err,
			"cache_key", cacheKey)
	}

	return &EmbeddingResult{
		Embedding: embedding,
		Dimension: len(embedding),
		CacheHit:  false,
	}, nil
}

// auditToText converts an audit to a text representation for embedding.
func auditToText(audit *models.RemediationAudit) string {
	var parts []string

	if audit.Name != "" {
		parts = append(parts, fmt.Sprintf("name:%s", audit.Name))
	}
	if audit.Namespace != "" {
		parts = append(parts, fmt.Sprintf("namespace:%s", audit.Namespace))
	}
	if audit.Phase != "" {
		parts = append(parts, fmt.Sprintf("phase:%s", audit.Phase))
	}
	if audit.ActionType != "" {
		parts = append(parts, fmt.Sprintf("action:%s", audit.ActionType))
	}
	if audit.Status != "" {
		parts = append(parts, fmt.Sprintf("status:%s", audit.Status))
	}
	if audit.RemediationRequestID != "" {
		parts = append(parts, fmt.Sprintf("request:%s", audit.RemediationRequestID))
	}
	if audit.SignalFingerprint != "" {
		parts = append(parts, fmt.Sprintf("signal:%s", audit.SignalFingerprint))
	}
	if audit.Severity != "" {
		parts = append(parts, fmt.Sprintf("severity:%s", audit.Severity))
	}
	if audit.Environment != "" {
		parts = append(parts, fmt.Sprintf("env:%s", audit.Environment))
	}
	if audit.ClusterName != "" {
		parts = append(parts, fmt.Sprintf("cluster:%s", audit.ClusterName))
	}
	if audit.TargetResource != "" {
		parts = append(parts, fmt.Sprintf("target:%s", audit.TargetResource))
	}

	return strings.Join(parts, " ")
}

// generateCacheKey generates a deterministic cache key from text.
func generateCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return "emb:" + hex.EncodeToString(hash[:])
}
