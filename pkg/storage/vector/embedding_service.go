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
	"context"
	"crypto/md5"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// LocalEmbeddingService provides local embedding generation without external dependencies
// This is a reference implementation that can be replaced with more sophisticated models
type LocalEmbeddingService struct {
	dimension   int
	vocabulary  map[string]int
	mutex       sync.RWMutex
	log         *logrus.Logger
	initialized bool
}

// NewLocalEmbeddingService creates a new local embedding service
func NewLocalEmbeddingService(dimension int, log *logrus.Logger) *LocalEmbeddingService {
	if dimension <= 0 {
		dimension = 384 // Default dimension matching sentence-transformers/all-MiniLM-L6-v2
	}

	service := &LocalEmbeddingService{
		dimension:  dimension,
		vocabulary: make(map[string]int),
		log:        log,
	}

	// Initialize with common Kubernetes and alerting vocabulary
	service.initializeVocabulary()

	return service
}

// GenerateTextEmbedding creates embedding from text using local algorithms
func (s *LocalEmbeddingService) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return s.createZeroEmbedding(), nil
	}

	// Normalize and tokenize text
	tokens := s.tokenize(strings.ToLower(text))

	// Create embedding using multiple techniques
	embedding := make([]float64, s.dimension)

	// 1. Term Frequency-based representation (first 1/3 of dimensions)
	tfEmbedding := s.createTFEmbedding(tokens)
	copy(embedding[:len(tfEmbedding)], tfEmbedding)

	// 2. Hash-based features (middle 1/3 of dimensions)
	hashEmbedding := s.createHashEmbedding(text)
	start := len(tfEmbedding)
	copy(embedding[start:start+len(hashEmbedding)], hashEmbedding)

	// 3. Positional and semantic features (last 1/3 of dimensions)
	semanticEmbedding := s.createSemanticEmbedding(tokens)
	start = start + len(hashEmbedding)
	copy(embedding[start:start+len(semanticEmbedding)], semanticEmbedding)

	// Normalize the embedding vector
	s.normalizeVector(embedding)

	s.log.WithFields(logrus.Fields{
		"text_length":   len(text),
		"tokens_count":  len(tokens),
		"embedding_dim": len(embedding),
		"text_preview":  s.truncateString(text, 50),
	}).Debug("Generated text embedding")

	return embedding, nil
}

// GenerateActionEmbedding creates embedding from action data
func (s *LocalEmbeddingService) GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error) {
	// Combine action type and parameters into text
	var textParts []string
	textParts = append(textParts, actionType)

	// Extract meaningful parameter values
	for key, value := range parameters {
		switch v := value.(type) {
		case string:
			textParts = append(textParts, fmt.Sprintf("%s:%s", key, v))
		case int, int32, int64:
			textParts = append(textParts, fmt.Sprintf("%s:%v", key, v))
		case float32, float64:
			textParts = append(textParts, fmt.Sprintf("%s:%.2f", key, v))
		case bool:
			textParts = append(textParts, fmt.Sprintf("%s:%v", key, v))
		}
	}

	text := strings.Join(textParts, " ")
	return s.GenerateTextEmbedding(ctx, text)
}

// GenerateContextEmbedding creates embedding from context data
func (s *LocalEmbeddingService) GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error) {
	var textParts []string

	// Process labels
	for key, value := range labels {
		textParts = append(textParts, fmt.Sprintf("%s:%s", key, value))
	}

	// Process metadata
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			textParts = append(textParts, fmt.Sprintf("%s:%s", key, v))
		case int, int32, int64:
			textParts = append(textParts, fmt.Sprintf("%s:%v", key, v))
		case float32, float64:
			textParts = append(textParts, fmt.Sprintf("%s:%.2f", key, v))
		}
	}

	text := strings.Join(textParts, " ")
	return s.GenerateTextEmbedding(ctx, text)
}

// CombineEmbeddings combines multiple embeddings into one
func (s *LocalEmbeddingService) CombineEmbeddings(embeddings ...[]float64) []float64 {
	if len(embeddings) == 0 {
		return s.createZeroEmbedding()
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// Use weighted average combination
	combined := make([]float64, s.dimension)
	weight := 1.0 / float64(len(embeddings))

	for _, embedding := range embeddings {
		if len(embedding) != s.dimension {
			s.log.WithFields(logrus.Fields{
				"expected_dim": s.dimension,
				"actual_dim":   len(embedding),
			}).Warn("Embedding dimension mismatch, skipping")
			continue
		}

		for i, val := range embedding {
			combined[i] += val * weight
		}
	}

	s.normalizeVector(combined)
	return combined
}

// GetEmbeddingDimension returns the dimension of generated embeddings
func (s *LocalEmbeddingService) GetEmbeddingDimension() int {
	return s.dimension
}

// Private helper methods

func (s *LocalEmbeddingService) initializeVocabulary() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Common Kubernetes and alerting terms
	commonTerms := []string{
		// Kubernetes resources
		"pod", "deployment", "service", "configmap", "secret", "namespace",
		"ingress", "persistentvolume", "persistentvolumeclaim", "node",
		"replicaset", "daemonset", "statefulset", "job", "cronjob",

		// Alert severities
		"critical", "warning", "info", "error", "debug",

		// Action types
		"scale", "restart", "delete", "create", "update", "patch",
		"drain", "cordon", "uncordon", "rollout", "rollback",

		// Common alert names
		"memory", "cpu", "disk", "network", "unavailable", "down",
		"high", "low", "usage", "pressure", "limit", "quota",
		"timeout", "failed", "error", "outage", "degraded",

		// Resource attributes
		"container", "image", "volume", "port", "env", "resource",
		"limit", "request", "quota", "label", "annotation",

		// Operational terms
		"healthy", "unhealthy", "ready", "notready", "running",
		"pending", "failed", "succeeded", "terminating",
	}

	for i, term := range commonTerms {
		s.vocabulary[term] = i + 1 // Start from 1, reserve 0 for unknown
	}

	s.initialized = true
	if s.log != nil {
		s.log.WithField("vocabulary_size", len(s.vocabulary)).Debug("Initialized embedding vocabulary")
	}
}

func (s *LocalEmbeddingService) tokenize(text string) []string {
	// Simple tokenization: split on whitespace and common delimiters
	replacer := strings.NewReplacer(
		".", " ", ",", " ", ":", " ", ";", " ", "!", " ", "?", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ",
		"/", " ", "\\", " ", "-", " ", "_", " ", "=", " ",
	)

	normalized := replacer.Replace(text)
	tokens := strings.Fields(normalized)

	// Filter out very short tokens
	var filtered []string
	for _, token := range tokens {
		if len(token) >= 2 {
			filtered = append(filtered, token)
		}
	}

	return filtered
}

func (s *LocalEmbeddingService) createTFEmbedding(tokens []string) []float64 {
	// Create term frequency embedding (first third of dimensions)
	dimSize := s.dimension / 3
	embedding := make([]float64, dimSize)

	if len(tokens) == 0 {
		return embedding
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Count token frequencies
	tokenCounts := make(map[string]int)
	for _, token := range tokens {
		tokenCounts[token]++
	}

	// Map tokens to embedding dimensions
	for token, count := range tokenCounts {
		if vocabId, exists := s.vocabulary[token]; exists {
			// Map vocabulary ID to embedding dimension
			dim := vocabId % dimSize
			embedding[dim] += float64(count) / float64(len(tokens))
		} else {
			// Use hash for unknown tokens
			h := fnv.New32a()
			h.Write([]byte(token))
			dim := int(h.Sum32()) % dimSize
			embedding[dim] += float64(count) / float64(len(tokens))
		}
	}

	return embedding
}

func (s *LocalEmbeddingService) createHashEmbedding(text string) []float64 {
	// Create hash-based embedding (middle third of dimensions)
	dimSize := s.dimension / 3
	embedding := make([]float64, dimSize)

	if text == "" {
		return embedding
	}

	// Use multiple hash functions for better distribution
	hashFunctions := []func([]byte) uint32{
		func(data []byte) uint32 {
			h := fnv.New32a()
			h.Write(data)
			return h.Sum32()
		},
		func(data []byte) uint32 {
			sum := md5.Sum(data)
			return uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
		},
	}

	for i, hashFunc := range hashFunctions {
		hash := hashFunc([]byte(text))

		// Distribute hash across multiple dimensions
		for j := 0; j < 3; j++ {
			dim := (int(hash) + i*3 + j) % dimSize
			embedding[dim] = math.Sin(float64(hash + uint32(j))) // Sine for bounded values
		}
	}

	return embedding
}

func (s *LocalEmbeddingService) createSemanticEmbedding(tokens []string) []float64 {
	// Create semantic embedding (last third of dimensions)
	dimSize := s.dimension / 3
	embedding := make([]float64, dimSize)

	if len(tokens) == 0 {
		return embedding
	}

	// Simple semantic groupings
	semanticGroups := map[string][]string{
		"resource": {"pod", "deployment", "service", "node", "container"},
		"severity": {"critical", "warning", "error", "info"},
		"action":   {"scale", "restart", "delete", "create", "update"},
		"metric":   {"memory", "cpu", "disk", "network", "usage"},
		"status":   {"healthy", "unhealthy", "ready", "failed", "running"},
	}

	// Calculate semantic scores
	groupScores := make(map[string]float64)
	for _, token := range tokens {
		for group, groupTokens := range semanticGroups {
			for _, groupToken := range groupTokens {
				if strings.Contains(token, groupToken) || strings.Contains(groupToken, token) {
					groupScores[group] += 1.0 / float64(len(tokens))
				}
			}
		}
	}

	// Map semantic scores to embedding dimensions
	groupIndex := 0
	for group, score := range groupScores {
		if groupIndex >= dimSize {
			break
		}

		// Use hash to determine base dimension for this group
		h := fnv.New32a()
		h.Write([]byte(group))
		baseDim := int(h.Sum32()) % (dimSize - 2)

		// Spread group influence across multiple dimensions
		embedding[baseDim] += score
		if baseDim+1 < dimSize {
			embedding[baseDim+1] += score * 0.5
		}
		if baseDim+2 < dimSize {
			embedding[baseDim+2] += score * 0.3
		}

		groupIndex++
	}

	return embedding
}

func (s *LocalEmbeddingService) createZeroEmbedding() []float64 {
	return make([]float64, s.dimension)
}

func (s *LocalEmbeddingService) normalizeVector(vector []float64) {
	// L2 normalization
	var sumSquares float64
	for _, val := range vector {
		sumSquares += val * val
	}

	if sumSquares > 0 {
		norm := math.Sqrt(sumSquares)
		for i := range vector {
			vector[i] /= norm
		}
	}
}

func (s *LocalEmbeddingService) truncateString(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// HybridEmbeddingService combines local and external embedding services
type HybridEmbeddingService struct {
	local    *LocalEmbeddingService
	external EmbeddingGenerator // Could be OpenAI, HuggingFace, etc.
	useLocal bool
	log      *logrus.Logger
}

// NewHybridEmbeddingService creates a hybrid service with fallback
func NewHybridEmbeddingService(localService *LocalEmbeddingService, externalService EmbeddingGenerator, log *logrus.Logger) *HybridEmbeddingService {
	return &HybridEmbeddingService{
		local:    localService,
		external: externalService,
		useLocal: true, // Default to local, can be configured
		log:      log,
	}
}

// SetUseLocal controls whether to use local or external service
func (h *HybridEmbeddingService) SetUseLocal(useLocal bool) {
	h.useLocal = useLocal
}

// GenerateTextEmbedding generates embeddings with fallback
func (h *HybridEmbeddingService) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	if h.useLocal || h.external == nil {
		return h.local.GenerateTextEmbedding(ctx, text)
	}

	// Try external service first, fallback to local
	embedding, err := h.external.GenerateTextEmbedding(ctx, text)
	if err != nil {
		h.log.WithError(err).Warn("External embedding service failed, falling back to local")
		return h.local.GenerateTextEmbedding(ctx, text)
	}

	return embedding, nil
}

// GenerateActionEmbedding generates action embeddings with fallback
func (h *HybridEmbeddingService) GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error) {
	if h.useLocal || h.external == nil {
		return h.local.GenerateActionEmbedding(ctx, actionType, parameters)
	}

	embedding, err := h.external.GenerateActionEmbedding(ctx, actionType, parameters)
	if err != nil {
		h.log.WithError(err).Warn("External embedding service failed for action, falling back to local")
		return h.local.GenerateActionEmbedding(ctx, actionType, parameters)
	}

	return embedding, nil
}

// GenerateContextEmbedding generates context embeddings with fallback
func (h *HybridEmbeddingService) GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error) {
	if h.useLocal || h.external == nil {
		return h.local.GenerateContextEmbedding(ctx, labels, metadata)
	}

	embedding, err := h.external.GenerateContextEmbedding(ctx, labels, metadata)
	if err != nil {
		h.log.WithError(err).Warn("External embedding service failed for context, falling back to local")
		return h.local.GenerateContextEmbedding(ctx, labels, metadata)
	}

	return embedding, nil
}

// CombineEmbeddings combines multiple embeddings
func (h *HybridEmbeddingService) CombineEmbeddings(embeddings ...[]float64) []float64 {
	return h.local.CombineEmbeddings(embeddings...)
}

// GetEmbeddingDimension returns the embedding dimension
func (h *HybridEmbeddingService) GetEmbeddingDimension() int {
	if h.useLocal || h.external == nil {
		return h.local.GetEmbeddingDimension()
	}
	return h.external.GetEmbeddingDimension()
}
