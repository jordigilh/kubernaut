package testutil

import (
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// VectorTestDataFactory provides standardized test data creation for vector storage tests
type VectorTestDataFactory struct{}

// NewVectorTestDataFactory creates a new test data factory for vector storage tests
func NewVectorTestDataFactory() *VectorTestDataFactory {
	return &VectorTestDataFactory{}
}

// CreateTestActionPattern creates a basic test action pattern
func (f *VectorTestDataFactory) CreateTestActionPattern(id, actionType, alertName string) *vector.ActionPattern {
	return &vector.ActionPattern{
		ID:               id,
		ActionType:       actionType,
		AlertName:        alertName,
		AlertSeverity:    "warning",
		Namespace:        "default",
		ResourceType:     "Deployment",
		ResourceName:     "test-app",
		ActionParameters: map[string]interface{}{"replicas": 3},
		ContextLabels:    map[string]string{"app": "test"},
		Embedding:        []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		CreatedAt:        time.Now().Add(-time.Hour),
		EffectivenessData: &vector.EffectivenessData{
			Score:                0.85,
			SuccessCount:         8,
			FailureCount:         2,
			AverageExecutionTime: 5 * time.Minute,
			SideEffectsCount:     0,
			RecurrenceRate:       0.1,
			ContextualFactors:    map[string]float64{"load": 0.7},
			LastAssessed:         time.Now().Add(-30 * time.Minute),
		},
	}
}

// CreateTestPatternWithEmbedding creates a test pattern with specific embedding
func (f *VectorTestDataFactory) CreateTestPatternWithEmbedding(id, actionType, alertName string, embedding []float64, score float64) *vector.ActionPattern {
	pattern := f.CreateTestActionPattern(id, actionType, alertName)
	pattern.Embedding = embedding
	pattern.EffectivenessData.Score = score
	return pattern
}

// CreateTestPatterns creates multiple test patterns for bulk operations
func (f *VectorTestDataFactory) CreateTestPatterns(count int) []*vector.ActionPattern {
	patterns := make([]*vector.ActionPattern, count)
	for i := 0; i < count; i++ {
		patterns[i] = f.CreateTestActionPattern(
			fmt.Sprintf("pattern-%d", i),
			"scale_deployment",
			"HighMemoryUsage",
		)
		// Vary the embeddings
		patterns[i].Embedding = []float64{
			float64(i%10) / 10.0,
			float64((i+1)%10) / 10.0,
			float64((i+2)%10) / 10.0,
			float64((i+3)%10) / 10.0,
			float64((i+4)%10) / 10.0,
		}
	}
	return patterns
}

// CreateSimilarityTestPatterns creates patterns designed for similarity testing
func (f *VectorTestDataFactory) CreateSimilarityTestPatterns() []*vector.ActionPattern {
	return []*vector.ActionPattern{
		f.CreateTestPatternWithEmbedding("similar-1", "scale_deployment", "HighMemoryUsage", []float64{1.0, 0.5, 0.0, 0.0, 0.0}, 0.9),
		f.CreateTestPatternWithEmbedding("similar-2", "scale_deployment", "HighMemoryUsage", []float64{0.9, 0.4, 0.1, 0.1, 0.0}, 0.85),
		f.CreateTestPatternWithEmbedding("different-1", "restart_pod", "PodCrashing", []float64{0.1, 0.9, 0.5, 0.2, 0.8}, 0.7),
		f.CreateTestPatternWithEmbedding("different-2", "scale_deployment", "HighCpuUsage", []float64{0.8, 0.6, 0.2, 0.3, 0.1}, 0.8),
	}
}

// CreateQueryEmbedding creates a test embedding for query operations
func (f *VectorTestDataFactory) CreateQueryEmbedding() []float64 {
	return []float64{0.95, 0.45, 0.05, 0.05, 0.05}
}

// CreateTestTexts creates sample texts for embedding testing
func (f *VectorTestDataFactory) CreateTestTexts() []string {
	return []string{
		"pod memory usage high alert",
		"cpu throttling detected",
		"disk space running low",
		"network connectivity issue",
		"application deployment failed",
	}
}

// CreateEmbeddingServiceConfig creates test configuration for embedding services
func (f *VectorTestDataFactory) CreateEmbeddingServiceConfig() map[string]interface{} {
	return map[string]interface{}{
		"dimension":      384,
		"model_name":     "test-model",
		"max_tokens":     512,
		"temperature":    0.1,
		"timeout":        30 * time.Second,
		"retry_attempts": 3,
	}
}
