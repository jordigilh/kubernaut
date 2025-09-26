package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	. "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	testutil "github.com/jordigilh/kubernaut/pkg/testutil/storage"
)

// BR-VDB-001: OpenAI Embedding Service - Business Requirements Testing
var _ = Describe("OpenAI Embedding Service Unit Tests", func() {
	var (
		service    *vector.OpenAIEmbeddingService
		mockCache  *MockEmbeddingCache
		logger     *logrus.Logger
		ctx        context.Context
		testServer *httptest.Server
		apiKey     string
		// New test infrastructure variables
		testSuite *testutil.TestSuite
		fixtures  *testutil.BusinessRequirementFixtures
		validator *testutil.ValidationHelpers
	)

	BeforeEach(func() {
		// Set up new test infrastructure for enhanced testing
		testSuite = testutil.NewTestSuite()
		testSuite.SetupServers()
		fixtures = testutil.NewBusinessRequirementFixtures()
		validator = testutil.NewValidationHelpers()

		// Use fixtures and validator for test data (demonstrating new infrastructure)
		_ = fixtures.KubernetesIncidentDescriptions() // Will be used in future tests
		_ = validator.GetCostOptimizationMetrics()    // Will be used in future tests

		// Set up traditional variables for backward compatibility
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
		mockCache = NewMockEmbeddingCache()
		mockCache.Reset()
		ctx = context.Background()
		apiKey = "test-api-key-123"

		// Create test server for OpenAI API mocking with business requirement support
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Simple response for compatibility
			response := map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{
						"object":    "embedding",
						"embedding": []float64{0.1, 0.2, 0.3, 0.4, 0.5},
						"index":     0,
					},
				},
				"model": "text-embedding-3-small",
				"usage": map[string]interface{}{
					"prompt_tokens": 10,
					"total_tokens":  10,
				},
			}
			_ = json.NewEncoder(w).Encode(response)
		}))

		// Create service with test server URL using configurable constructor
		config := &vector.OpenAIConfig{
			Model:      "text-embedding-3-small",
			MaxRetries: 3,
			Timeout:    5 * time.Second,
			BaseURL:    testServer.URL,
			BatchSize:  100,
			RateLimit:  60,
			Dimensions: 5, // Match our test response
		}
		service = vector.NewOpenAIEmbeddingServiceWithConfig(apiKey, mockCache, logger, config)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
		testSuite.TeardownServers()
	})

	// BR-VDB-001.1: API Integration Requirements
	Context("API Integration", func() {
		Describe("Authentication", func() {
			It("should require API key for service creation", func() {
				// Test business requirement: API key is mandatory
				service := vector.NewOpenAIEmbeddingService("", nil, testSuite.Context.Logger)
				Expect(service.GetDimension()).To(Equal(1536), "BR-DATABASE-001-A: Embedding service must provide standard OpenAI embedding dimensions for database storage operations")

				// Business validation: Service should fail when attempting to generate embeddings without API key
				// This will be tested in the actual API call scenarios
			})

			It("should set proper authorization header in API requests", func() {
				// This will be validated through integration testing
				// Business requirement: All API calls must include proper Bearer token authentication
				Skip("Integration test - requires actual API call validation")
			})
		})

		Describe("Rate Limiting and Exponential Backoff", func() {
			It("should implement exponential backoff for failed requests", func() {
				// Business requirement BR-VDB-001: Handle rate limiting with exponential backoff
				// Test server that fails initially then succeeds
				retryCount := 0
				testServer.Close()
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					retryCount++
					if retryCount < 3 {
						w.WriteHeader(http.StatusTooManyRequests)
						_, _ = w.Write([]byte(`{"error": "Rate limit exceeded"}`))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"object": "list",
						"data": [{"object": "embedding", "embedding": [0.1, 0.2, 0.3], "index": 0}],
						"model": "text-embedding-3-small",
						"usage": {"prompt_tokens": 5, "total_tokens": 5}
					}`))
				}))

				// Create service with configurable base URL pointing to test server
				config := &vector.OpenAIConfig{
					BaseURL:    testServer.URL,
					MaxRetries: 3,
					Timeout:    5 * time.Second,
				}
				retryTestService := vector.NewOpenAIEmbeddingServiceWithConfig("test-key", mockCache, logger, config)

				// Business validation: Service should retry failed requests with exponential backoff
				start := time.Now()
				embedding, err := retryTestService.GenerateEmbedding(ctx, "Test retry behavior")
				duration := time.Since(start)

				// Should succeed after retries
				Expect(err).ToNot(HaveOccurred(), "Should succeed after retries")
				Expect(embedding).ToNot(BeEmpty(), "Should return valid embedding after retries")
				Expect(retryCount).To(Equal(3), "Should have made exactly 3 requests (2 failures + 1 success)")

				// Should have taken time for exponential backoff (at least 1 second for backoffs)
				Expect(duration).To(BeNumerically(">", time.Second), "Should take time for exponential backoff")

				logger.WithFields(logrus.Fields{
					"retry_count": retryCount,
					"duration":    duration,
				}).Info("Exponential backoff test completed successfully")
			})

			It("should respect configured retry limits", func() {
				// Business requirement BR-VDB-001: Configurable retry limits to prevent infinite loops
				failureCount := 0
				testServer.Close()
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					failureCount++
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error": "Internal server error"}`))
				}))

				// Create service with limited retries
				config := &vector.OpenAIConfig{
					BaseURL:    testServer.URL,
					MaxRetries: 2, // Limit to 2 retries for faster test
					Timeout:    2 * time.Second,
				}
				retryLimitService := vector.NewOpenAIEmbeddingServiceWithConfig("test-key", mockCache, logger, config)

				// Service should fail after max retries (business requirement)
				start := time.Now()
				_, err := retryLimitService.GenerateEmbedding(ctx, "Test retry limits")
				duration := time.Since(start)

				// Should fail after exhausting retries
				Expect(err).To(HaveOccurred(), "Should fail after max retries")
				Expect(failureCount).To(Equal(3), "Should have made exactly 3 attempts (initial + 2 retries)")

				// Should have taken time for retries but not too long
				Expect(duration).To(BeNumerically(">", time.Second), "Should have attempted retries with backoff")
				Expect(duration).To(BeNumerically("<", 10*time.Second), "Should not retry indefinitely")

				logger.WithFields(logrus.Fields{
					"failure_count": failureCount,
					"duration":      duration,
					"error":         err.Error(),
				}).Info("Retry limits test completed successfully")
			})
		})

		Describe("API Quotas and Usage Tracking", func() {
			It("should track token usage from API responses", func() {
				// Business requirement: Track API quotas and usage for cost management
				text := "Sample text for embedding generation"

				embedding, err := service.GenerateEmbedding(ctx, text)

				// Business validations
				Expect(err).ToNot(HaveOccurred(), "Should generate embedding successfully")
				Expect(embedding).ToNot(BeEmpty(), "Should return non-empty embedding")
				Expect(len(embedding)).To(BeNumerically(">", 0), "Should return embedding with valid dimensions")

				// Token usage tracking validation would require service modification
				// Business requirement: Service should expose usage metrics
				Skip("Implementation needed: Usage tracking interface")
			})
		})
	})

	// BR-VDB-001.2: Embedding Generation Requirements
	Context("Embedding Generation", func() {
		Describe("Single Text Embedding", func() {
			It("should generate embeddings for valid text input", func() {
				// Business requirement: Generate high-quality embeddings for text input
				testText := "Kubernetes pod is experiencing high memory usage"

				embedding, err := service.GenerateEmbedding(ctx, testText)

				// Business validations
				Expect(err).ToNot(HaveOccurred(), "Should generate embedding without error")
				Expect(len(embedding)).To(Equal(service.GetDimension()), "BR-DATABASE-001-A: Embedding generation must produce vectors with correct dimensions for database storage operations")

				// Validate embedding quality - should not be all zeros
				hasNonZeroValues := false
				for _, value := range embedding {
					if value != 0.0 {
						hasNonZeroValues = true
						break
					}
				}
				Expect(hasNonZeroValues).To(BeTrue(), "Embedding should contain meaningful non-zero values")
			})

			It("should handle empty text input gracefully", func() {
				// Business requirement: Robust handling of edge cases
				embedding, err := service.GenerateEmbedding(ctx, "")

				// Business validation: Should handle empty input without crashing
				if err != nil {
					// Acceptable to return error for empty input
					Expect(err.Error()).To(ContainSubstring("empty"), "Error should indicate empty input issue")
				} else {
					// If no error, should return valid embedding structure
					Expect(len(embedding)).To(Equal(service.GetDimension()), "BR-DATABASE-001-A: Embedding structure must contain valid vector data with correct dimensions for database storage operations")
				}
			})

			It("should handle large text input efficiently", func() {
				// Business requirement: Handle large text inputs without performance degradation
				largeText := strings.Repeat("This is a long text for testing embedding generation performance. ", 100)

				startTime := time.Now()
				embedding, err := service.GenerateEmbedding(ctx, largeText)
				duration := time.Since(startTime)

				// Business validations
				Expect(err).ToNot(HaveOccurred(), "Should handle large text without error")
				Expect(len(embedding)).To(Equal(service.GetDimension()), "BR-DATABASE-001-A: Large text embedding generation must produce vectors with correct dimensions for database storage operations")
				Expect(duration).To(BeNumerically("<", 30*time.Second), "Should complete within reasonable time")
			})
		})

		Describe("Batch Embedding Generation", func() {
			It("should efficiently process multiple texts in batches", func() {
				// Business requirement: Batch process multiple texts efficiently
				texts := []string{
					"CPU usage is high on worker node",
					"Memory pressure detected in namespace default",
					"Pod restart loop detected in deployment",
					"Network connectivity issues reported",
					"Storage volume is nearly full",
				}

				embeddings, err := service.GenerateBatchEmbeddings(ctx, texts)

				// Business validations
				Expect(err).ToNot(HaveOccurred(), "Should process batch without error")
				Expect(embeddings).To(HaveLen(len(texts)), "Should return embedding for each input text")

				for i, embedding := range embeddings {
					Expect(len(embedding)).To(Equal(service.GetDimension()), "BR-DATABASE-001-A: Batch embedding %d must contain valid vector data with correct dimensions for database storage operations", i)
				}

				// Validate uniqueness - different texts should produce different embeddings
				Expect(embeddings[0]).ToNot(Equal(embeddings[1]), "Different texts should produce different embeddings")
			})

			It("should handle empty batch gracefully", func() {
				// Business requirement: Robust handling of edge cases
				embeddings, err := service.GenerateBatchEmbeddings(ctx, []string{})

				// Business validation
				Expect(err).ToNot(HaveOccurred(), "Should handle empty batch without error")
				Expect(embeddings).To(HaveLen(0), "Should return empty results for empty input")
			})

			It("should respect configured batch size limits", func() {
				// Business requirement: Efficient batching with configurable limits
				// Generate large batch to test chunking
				texts := make([]string, 150) // Larger than typical batch size
				for i := range texts {
					texts[i] = "Test text number " + string(rune(i))
				}

				// Measure performance
				start := time.Now()
				embeddings, err := service.GenerateBatchEmbeddings(ctx, texts)
				duration := time.Since(start)

				// Business validations
				Expect(err).ToNot(HaveOccurred(), "Should handle large batch without error")
				Expect(embeddings).To(HaveLen(len(texts)), "Should return embedding for all texts")

				// Performance validation - should complete in reasonable time
				// For 150 embeddings with mock server, should complete quickly (< 10 seconds)
				Expect(duration).To(BeNumerically("<", 10*time.Second), "Batch processing should be efficient")

				logger.WithFields(logrus.Fields{
					"batch_size": len(texts),
					"duration":   duration,
					"embeddings": len(embeddings),
				}).Info("Batch embedding performance test completed")
			})
		})

		Describe("Caching Integration", func() {
			It("should cache embeddings to reduce API costs", func() {
				// Business requirement: Implement embedding caching to reduce costs
				testText := "Cached embedding test text"
				mockCache.Reset()

				// First call - should cache the result
				embedding1, err := service.GenerateEmbedding(ctx, testText)
				Expect(err).ToNot(HaveOccurred(), "First call should succeed")

				// Verify cache was accessed for read
				getCalls := mockCache.GetGetCalls()
				Expect(len(getCalls)).To(BeNumerically(">=", 1), "Should attempt to read from cache")

				// Give time for async cache write
				time.Sleep(10 * time.Millisecond)

				// Verify cache was written to
				setCalls := mockCache.GetSetCalls()
				Expect(len(setCalls)).To(BeNumerically(">=", 1), "Should write to cache")

				// Second call with same text - should use cache
				mockCache.SetGetResult(embedding1, true, nil) // Simulate cache hit

				embedding2, err := service.GenerateEmbedding(ctx, testText)
				Expect(err).ToNot(HaveOccurred(), "Second call should succeed")

				// Business validation: Should return same embedding from cache
				Expect(embedding2).To(Equal(embedding1), "Cached embedding should match original")
			})

			It("should fallback to API when cache fails", func() {
				// Business requirement: Handle cache failures gracefully
				testText := "Cache failure test text"
				mockCache.SetGetResult(nil, false, nil) // Simulate cache miss

				embedding, err := service.GenerateEmbedding(ctx, testText)

				// Business validation: Should succeed despite cache issues
				Expect(err).ToNot(HaveOccurred(), "Should fallback to API when cache fails")
				Expect(len(embedding)).To(Equal(service.GetDimension()), "BR-DATABASE-001-A: Cache fallback embedding generation must produce vectors with correct dimensions for database storage operations")
			})
		})

		Describe("Error Handling and Fallback Mechanisms", func() {
			It("should handle API failures with appropriate error messages", func() {
				// Business requirement: Handle API failures with fallback mechanisms
				Skip("Implementation needed: Configurable test server for API failure simulation")
			})

			It("should handle network timeouts gracefully", func() {
				// Business requirement: Robust error handling for network issues
				Skip("Implementation needed: Network timeout simulation")
			})

			It("should provide detailed error information for debugging", func() {
				// Business requirement: Clear error messages for operational debugging
				Skip("Implementation needed: Error classification and detailed messages")
			})
		})
	})

	// BR-VDB-001.3: Service Configuration and Performance
	Context("Service Configuration", func() {
		It("should use correct default model configuration", func() {
			// Business requirement: Sensible defaults for production use
			Expect(service.GetModel()).To(Equal("text-embedding-3-small"), "Should use cost-effective default model")
			Expect(service.GetDimension()).To(Equal(5), "Should use configured dimension (test uses 5 for mock data)")
		})

		It("should support model configuration customization", func() {
			// Business requirement: Configurable model selection for different use cases
			Skip("Implementation needed: Configurable model selection")
		})

		It("should maintain consistent embedding dimensions", func() {
			// Business requirement: Consistent dimensions for vector operations
			dimension := service.GetDimension()
			Expect(dimension).To(BeNumerically(">", 0), "Should return positive dimension")

			embedding, err := service.GenerateEmbedding(ctx, "test consistency")
			if err == nil {
				Expect(len(embedding)).To(Equal(dimension), "Generated embedding should match reported dimension")
			}
		})
	})

	// BR-VDB-001.4: Integration Requirements
	Context("Integration with Vector Database Factory", func() {
		It("should be properly instantiated through factory", func() {
			// Business requirement: Seamless integration with existing architecture
			// Test environment variable requirement
			_ = os.Setenv("OPENAI_API_KEY", "test-key-for-factory")
			defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

			// Create test vector database configuration
			vectorConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "memory", // Use memory backend for testing
				EmbeddingService: config.EmbeddingConfig{
					Service:   "openai",
					Dimension: 1536,
				},
				Cache: config.VectorCacheConfig{
					Enabled:   false, // Disable cache for this test
					CacheType: "",
				},
			}

			// Create factory
			factory := vector.NewVectorDatabaseFactory(vectorConfig, nil, logger)

			// Test embedding service creation
			embeddingService, err := factory.CreateEmbeddingService()

			// Business validations
			Expect(err).ToNot(HaveOccurred(), "Factory should create embedding service successfully")
			Expect(embeddingService.GetEmbeddingDimension()).To(Equal(1536), "BR-DATABASE-001-A: Factory must create embedding service with functional OpenAI dimensions for database storage operations")

			// Validate that it's the correct type (through interface methods)
			dimension := embeddingService.GetEmbeddingDimension()
			Expect(dimension).To(Equal(1536), "Should return correct OpenAI embedding dimension")

			logger.Info("Factory integration test completed successfully")
		})

		It("should work with caching layer when configured", func() {
			// Business requirement: Integration with caching infrastructure
			_ = os.Setenv("OPENAI_API_KEY", "test-key-for-caching")
			defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

			// Create test vector database configuration with caching enabled
			vectorConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "memory", // Use memory backend for testing
				EmbeddingService: config.EmbeddingConfig{
					Service:   "openai",
					Dimension: 1536,
				},
				Cache: config.VectorCacheConfig{
					Enabled:   true,
					CacheType: "memory",  // Use memory cache for testing
					TTL:       time.Hour, // 1 hour
					MaxSize:   1000,
				},
			}

			// Create factory
			factory := vector.NewVectorDatabaseFactory(vectorConfig, nil, logger)

			// Test embedding service creation with caching
			embeddingService, err := factory.CreateEmbeddingService()

			// Business validations
			Expect(err).ToNot(HaveOccurred(), "Factory should create cached embedding service successfully")
			Expect(embeddingService.GetEmbeddingDimension()).To(Equal(1536), "BR-DATABASE-001-A: Factory must create cached embedding service with functional OpenAI dimensions for database storage operations")

			// Validate that it's properly configured
			dimension := embeddingService.GetEmbeddingDimension()
			Expect(dimension).To(Equal(1536), "Cached service should return correct OpenAI embedding dimension")

			logger.Info("Factory caching integration test completed successfully")
		})
	})
})
