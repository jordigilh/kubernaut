//go:build integration
// +build integration

package vector_ai

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// ControlledEmbeddingGenerator provides deterministic embeddings for integration testing
// Following decisions: Controlled test scenarios that guarantee business thresholds
//
// Business Requirements Supported:
// - BR-VDB-AI-001: Consistent embeddings for reproducible search quality validation
// - BR-VDB-AI-002: Realistic embeddings that enable meaningful similarity calculations
//
// This generator creates embeddings that:
// 1. Are deterministic (same input -> same output)
// 2. Have realistic similarity properties for test validation
// 3. Support controlled scenarios for reliable business requirement testing
type ControlledEmbeddingGenerator struct {
	logger *logrus.Logger
}

// NewControlledEmbeddingGenerator creates a new controlled embedding generator
func NewControlledEmbeddingGenerator(logger *logrus.Logger) vector.EmbeddingGenerator {
	if logger == nil {
		logger = logrus.New()
	}

	return &ControlledEmbeddingGenerator{
		logger: logger,
	}
}

// GenerateTextEmbedding creates a deterministic embedding based on input text
// Following project guidelines: Strong business assertions - embeddings must be consistent
func (g *ControlledEmbeddingGenerator) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, fmt.Errorf("input text cannot be empty")
	}

	// Use 1536 dimensions to match OpenAI embedding format for consistency
	const embeddingDim = 1536
	embedding := make([]float64, embeddingDim)

	// Generate deterministic hash of input text
	hasher := sha256.New()
	hasher.Write([]byte(strings.ToLower(strings.TrimSpace(text))))
	hash := hasher.Sum(nil)

	// Create base embedding from hash
	for i := 0; i < embeddingDim; i++ {
		// Use hash bytes cyclically to generate base values
		hashByte := hash[i%len(hash)]
		// Convert to float in range [-1, 1]
		embedding[i] = float64(int(hashByte)-128) / 128.0
	}

	// Add semantic components based on text content for realistic similarity
	g.addSemanticComponents(embedding, text)

	// Normalize the embedding vector
	g.normalizeEmbedding(embedding)

	g.logger.WithFields(logrus.Fields{
		"text_length":    len(text),
		"embedding_dim":  len(embedding),
		"embedding_norm": g.calculateNorm(embedding),
	}).Debug("Generated controlled embedding")

	return embedding, nil
}

// addSemanticComponents adds semantic similarity based on text content
// This enables controlled scenarios with predictable similarity scores
func (g *ControlledEmbeddingGenerator) addSemanticComponents(embedding []float64, text string) {
	text = strings.ToLower(text)

	// Define semantic clusters with their embedding adjustments
	semanticClusters := map[string][]float64{
		"memory":  {0.8, 0.6, 0.4, 0.7, 0.5}, // Memory-related terms
		"cpu":     {0.7, 0.8, 0.5, 0.6, 0.4}, // CPU-related terms
		"disk":    {0.6, 0.5, 0.8, 0.4, 0.7}, // Disk-related terms
		"network": {0.5, 0.7, 0.6, 0.8, 0.4}, // Network-related terms
		"pod":     {0.4, 0.6, 0.7, 0.5, 0.8}, // Kubernetes pod terms
		"alert":   {0.9, 0.7, 0.6, 0.8, 0.5}, // Alert-related terms
		"scale":   {0.6, 0.8, 0.4, 0.7, 0.9}, // Scaling-related terms
	}

	// Apply semantic adjustments based on text content
	for cluster, adjustments := range semanticClusters {
		if strings.Contains(text, cluster) {
			// Apply cluster-specific adjustments to first dimensions
			for i, adj := range adjustments {
				if i < len(embedding) {
					embedding[i] = embedding[i]*0.7 + adj*0.3 // Blend with semantic component
				}
			}
		}
	}

	// Add severity-based components for alert classification
	severityMap := map[string]float64{
		"critical": 0.9,
		"high":     0.7,
		"medium":   0.5,
		"low":      0.3,
	}

	for severity, weight := range severityMap {
		if strings.Contains(text, severity) {
			// Apply severity weight to specific dimensions
			for i := 10; i < 15 && i < len(embedding); i++ {
				embedding[i] = embedding[i]*0.8 + weight*0.2
			}
		}
	}
}

// normalizeEmbedding normalizes the embedding vector to unit length
func (g *ControlledEmbeddingGenerator) normalizeEmbedding(embedding []float64) {
	norm := g.calculateNorm(embedding)
	if norm == 0 {
		return // Avoid division by zero
	}

	for i := range embedding {
		embedding[i] /= norm
	}
}

// calculateNorm computes the L2 norm of the embedding
func (g *ControlledEmbeddingGenerator) calculateNorm(embedding []float64) float64 {
	var sum float64
	for _, val := range embedding {
		sum += val * val
	}
	return math.Sqrt(sum)
}

// GenerateActionEmbedding creates embedding from action data
func (g *ControlledEmbeddingGenerator) GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error) {
	// Combine action type and parameters into text representation
	text := actionType
	for key, value := range parameters {
		text += fmt.Sprintf(" %s:%v", key, value)
	}
	return g.GenerateTextEmbedding(ctx, text)
}

// GenerateContextEmbedding creates embedding from context data
func (g *ControlledEmbeddingGenerator) GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error) {
	text := ""
	for key, value := range labels {
		text += fmt.Sprintf("%s:%s ", key, value)
	}
	for key, value := range metadata {
		text += fmt.Sprintf("%s:%v ", key, value)
	}
	return g.GenerateTextEmbedding(ctx, text)
}

// CombineEmbeddings combines multiple embeddings into one
func (g *ControlledEmbeddingGenerator) CombineEmbeddings(embeddings ...[]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}
	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// Average all embeddings
	dim := len(embeddings[0])
	combined := make([]float64, dim)

	for _, embedding := range embeddings {
		for i := 0; i < dim && i < len(embedding); i++ {
			combined[i] += embedding[i]
		}
	}

	// Normalize by number of embeddings
	for i := range combined {
		combined[i] /= float64(len(embeddings))
	}

	g.normalizeEmbedding(combined)
	return combined
}

// GetEmbeddingDimension returns the dimension of generated embeddings
func (g *ControlledEmbeddingGenerator) GetEmbeddingDimension() int {
	return 1536
}

// IsHealthy checks if the embedding generator is functioning properly
func (g *ControlledEmbeddingGenerator) IsHealthy(ctx context.Context) error {
	// Test embedding generation with a simple input
	_, err := g.GenerateTextEmbedding(ctx, "health check test")
	if err != nil {
		return fmt.Errorf("embedding generator health check failed: %w", err)
	}
	return nil
}
