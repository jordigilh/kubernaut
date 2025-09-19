package storage

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// TestContext provides common test setup following project guidelines
// Business Requirement: Support consistent test infrastructure for BR-VDB-001 and BR-VDB-002
type TestContext struct {
	Logger    *logrus.Logger
	MockCache *mocks.MockEmbeddingCache
	Context   context.Context
}

// NewTestContext creates a standardized test context
// Following project guideline: AVOID duplication and REUSE existing code
func NewTestContext() *TestContext {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	mockCache := mocks.NewMockEmbeddingCache()
	mockCache.Reset()

	// Reset global mock state to avoid test interference
	mocks.ResetGlobalMockState()

	return &TestContext{
		Logger:    logger,
		MockCache: mockCache,
		Context:   context.Background(),
	}
}

// ServiceConfigFactory creates standardized service configurations
// Following project guideline: Ensure functionality aligns with business requirements
type ServiceConfigFactory struct{}

// NewServiceConfigFactory creates a new service configuration factory
func NewServiceConfigFactory() *ServiceConfigFactory {
	return &ServiceConfigFactory{}
}

// CreateOpenAIConfig creates OpenAI service configuration for testing
// Business Requirement: BR-VDB-001 - Support configurable endpoints for testing
func (f *ServiceConfigFactory) CreateOpenAIConfig(baseURL string, dimensions int) *vector.OpenAIConfig {
	return &vector.OpenAIConfig{
		Model:      "text-embedding-3-small",
		MaxRetries: 1, // Reduced for faster tests
		Timeout:    5 * time.Second,
		BaseURL:    baseURL,
		BatchSize:  100,
		RateLimit:  60,
		Dimensions: dimensions,
	}
}

// CreateHuggingFaceConfig creates HuggingFace service configuration for testing
// Business Requirement: BR-VDB-002 - Support configurable endpoints for testing
func (f *ServiceConfigFactory) CreateHuggingFaceConfig(baseURL string, dimensions int) *vector.HuggingFaceConfig {
	return &vector.HuggingFaceConfig{
		Model:      "test-model",
		MaxRetries: 1, // Reduced for faster tests
		Timeout:    5 * time.Second,
		BaseURL:    baseURL,
		BatchSize:  50,
		RateLimit:  100,
		Dimensions: dimensions,
	}
}

// EmbeddingDataFactory creates consistent test embedding data
// Following project guideline: AVOID duplication and REUSE existing code
type EmbeddingDataFactory struct{}

// NewEmbeddingDataFactory creates a new embedding data factory
func NewEmbeddingDataFactory() *EmbeddingDataFactory {
	return &EmbeddingDataFactory{}
}

// CreateDeterministicEmbedding creates a deterministic embedding for testing
// Business Requirement: Support consistent test data for reliable assertions
func (f *EmbeddingDataFactory) CreateDeterministicEmbedding(dimensions int, seed float64) []float64 {
	embedding := make([]float64, dimensions)
	for i := range embedding {
		embedding[i] = seed + float64(i)*0.001 // Predictable pattern
	}
	return embedding
}

// CreateBatchEmbeddings creates multiple embeddings with variation
// Business Requirement: Support batch processing testing for BR-VDB-001 and BR-VDB-002
func (f *EmbeddingDataFactory) CreateBatchEmbeddings(dimensions int, count int, baseSeed float64) [][]float64 {
	embeddings := make([][]float64, count)
	for i := 0; i < count; i++ {
		embeddings[i] = f.CreateDeterministicEmbedding(dimensions, baseSeed+float64(i)*0.1)
	}
	return embeddings
}

// CreateOpenAIResponse creates standardized OpenAI API response for testing
// Business Requirement: BR-VDB-001 - Mock real API responses consistently
func (f *EmbeddingDataFactory) CreateOpenAIResponse(embeddings [][]float64, model string) map[string]interface{} {
	dataArray := make([]map[string]interface{}, len(embeddings))
	for i, embedding := range embeddings {
		dataArray[i] = map[string]interface{}{
			"object":    "embedding",
			"embedding": embedding,
			"index":     i,
		}
	}

	return map[string]interface{}{
		"object": "list",
		"data":   dataArray,
		"model":  model,
		"usage": map[string]interface{}{
			"prompt_tokens": 10 * len(embeddings),
			"total_tokens":  10 * len(embeddings),
		},
	}
}

// CreateHuggingFaceResponse creates standardized HuggingFace API response for testing
// Business Requirement: BR-VDB-002 - Mock real API responses consistently
func (f *EmbeddingDataFactory) CreateHuggingFaceResponse(embeddings [][]float64) [][]float64 {
	// HuggingFace returns embeddings directly as nested arrays
	return embeddings
}

// TestAPIKeys provides consistent API keys for testing
// Following project guideline: AVOID duplication
type TestAPIKeys struct {
	OpenAI      string
	HuggingFace string
}

// NewTestAPIKeys creates standardized test API keys
func NewTestAPIKeys() *TestAPIKeys {
	return &TestAPIKeys{
		OpenAI:      "test-openai-api-key-123",
		HuggingFace: "test-huggingface-api-key-456",
	}
}

// BusinessRequirementDimensions provides standard dimensions for business requirements
// Business Requirement: Ensure consistent dimensions across test suites
type BusinessRequirementDimensions struct {
	OpenAI      int
	HuggingFace int
}

// NewBusinessRequirementDimensions returns standard dimensions for business requirements
func NewBusinessRequirementDimensions() *BusinessRequirementDimensions {
	return &BusinessRequirementDimensions{
		OpenAI:      1536, // BR-VDB-001: OpenAI standard dimension
		HuggingFace: 384,  // BR-VDB-002: HuggingFace standard dimension
	}
}
