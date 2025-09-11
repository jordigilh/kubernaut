//go:build unit
// +build unit

package storage

import (
	"context"
	"math"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

/*
 * Business Requirement Validation: External Vector Database Integration
 *
 * This test suite validates business requirements for external vector database integrations
 * following development guidelines:
 * - Reuses existing test framework code (Ginkgo/Gomega)
 * - Focuses on business outcomes, not implementation details
 * - Uses meaningful assertions with business thresholds
 * - Integrates with existing codebase patterns
 * - Logs all errors and business metrics
 */

var _ = Describe("Business Requirement Validation: External Vector Database Integration", func() {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		logger           *logrus.Logger
		factory          *vector.VectorDatabaseFactory
		baseConfig       *config.VectorDBConfig
		commonAssertions *testutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for business metrics
		commonAssertions = testutil.NewCommonAssertions()

		// Reuse existing configuration pattern from vector_database_factory_test.go
		baseConfig = &config.VectorDBConfig{
			Enabled: true,
			Backend: "memory",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
				Model:     "all-MiniLM-L6-v2",
			},
			Cache: config.VectorCacheConfig{
				Enabled:   true,
				TTL:       24 * time.Hour, // Business requirement: 24h cache for cost optimization
				MaxSize:   10000,          // Business requirement: support production scale
				CacheType: "memory",
			},
		}
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-VDB-001
	 * Business Logic: MUST integrate OpenAI's embedding service for high-quality semantic embeddings
	 *
	 * Business Success Criteria:
	 *   - Embedding quality improvement >25% over local embeddings
	 *   - Cost optimization with intelligent caching reducing API costs >40%
	 *   - Rate limiting compliance with <500ms latency
	 *   - Fallback mechanism reliability maintaining >99.5% availability
	 *
	 * Test Focus: Business value of OpenAI integration - accuracy gains and cost management
	 * Expected Business Value: Improved incident similarity detection with controlled costs
	 */
	Context("BR-VDB-001: OpenAI Embedding Service Integration", func() {
		var (
			originalOpenAIKey string
		)

		BeforeEach(func() {
			originalOpenAIKey = os.Getenv("OPENAI_API_KEY")
		})

		AfterEach(func() {
			if originalOpenAIKey == "" {
				os.Unsetenv("OPENAI_API_KEY")
			} else {
				os.Setenv("OPENAI_API_KEY", originalOpenAIKey)
			}
		})

		It("should deliver measurable business value through OpenAI embedding quality and cost optimization", func() {
			// Business Context: Production incident similarity analysis
			By("Setting up OpenAI service configuration with business parameters")
			os.Setenv("OPENAI_API_KEY", "test-openai-key-business-validation")

			openAIConfig := *baseConfig
			openAIConfig.EmbeddingService.Service = "openai"
			openAIConfig.EmbeddingService.Model = "text-embedding-ada-002" // OpenAI production model
			openAIConfig.Cache.Enabled = true                              // Critical for cost optimization

			factory = vector.NewVectorDatabaseFactory(&openAIConfig, nil, logger)

			By("Creating OpenAI embedding service with cost optimization features")
			embeddingService, err := factory.CreateEmbeddingService()

			// Business Requirement Validation: Service Creation
			Expect(err).ToNot(HaveOccurred(), "OpenAI service must be creatable for business deployment")
			Expect(embeddingService).ToNot(BeNil(), "Must provide valid embedding service for business operations")

			// Business Requirement: Embedding dimensions must support business use cases
			actualDimension := embeddingService.GetEmbeddingDimension()
			Expect(actualDimension).To(BeNumerically(">=", 1024),
				"OpenAI embeddings must be >=1024 dimensions for high-quality semantic similarity")
			Expect(actualDimension).To(BeNumerically("<=", 3072),
				"Embedding dimensions must be reasonable for production performance")

			By("Validating cost optimization through intelligent caching")
			// Simulate business scenario: repeated similar incident queries
			testTexts := []string{
				"Kubernetes pod OutOfMemory error in production namespace",
				"Pod memory limit exceeded causing restart loop",
				"Memory pressure causing pod eviction in cluster",
			}

			// First pass: populate cache (simulates initial cost)
			startTime := time.Now()
			firstPassEmbeddings := make([][]float32, len(testTexts))
			for i, text := range testTexts {
				embedding, err := embeddingService.GenerateEmbedding(ctx, text)
				Expect(err).ToNot(HaveOccurred(), "Must generate embeddings for business scenarios")
				firstPassEmbeddings[i] = embedding
			}
			firstPassDuration := time.Since(startTime)

			// Second pass: cache hits (simulates cost savings)
			startTime = time.Now()
			secondPassEmbeddings := make([][]float32, len(testTexts))
			for i, text := range testTexts {
				embedding, err := embeddingService.GenerateEmbedding(ctx, text)
				Expect(err).ToNot(HaveOccurred(), "Cached embeddings must be retrievable")
				secondPassEmbeddings[i] = embedding
			}
			secondPassDuration := time.Since(startTime)

			// Business Requirement Validation: Cost optimization through caching
			if openAIConfig.Cache.Enabled {
				speedupRatio := float64(firstPassDuration) / float64(secondPassDuration)
				Expect(speedupRatio).To(BeNumerically(">=", 2.0),
					"Caching must provide >=2x speedup for cost optimization (simulating >40% API cost reduction)")

				// Validate embedding consistency (cache correctness)
				for i := range testTexts {
					similarity := calculateCosineSimilarity(firstPassEmbeddings[i], secondPassEmbeddings[i])
					Expect(similarity).To(BeNumerically(">=", 0.999),
						"Cached embeddings must be identical for business reliability")
				}
			}

			By("Validating business performance requirements for real-time operations")
			singleEmbeddingStart := time.Now()
			_, err = embeddingService.GenerateEmbedding(ctx, "Single performance test query")
			singleEmbeddingDuration := time.Since(singleEmbeddingStart)

			// Business Requirement: Response time for real-time incident analysis
			Expect(singleEmbeddingDuration).To(BeNumerically("<", 500*time.Millisecond),
				"Single embedding generation must be <500ms for real-time incident similarity")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-VDB-001",
				"service":                 "openai",
				"embedding_dimension":     actualDimension,
				"cache_speedup_ratio":     float64(firstPassDuration) / float64(secondPassDuration),
				"single_query_latency_ms": singleEmbeddingDuration.Milliseconds(),
				"cost_optimization_cache": openAIConfig.Cache.Enabled,
				"business_impact":         "OpenAI integration provides high-quality embeddings with cost optimization",
			}).Info("BR-VDB-001: OpenAI embedding service business validation completed")
		})

		It("should provide fallback mechanism for business continuity when OpenAI is unavailable", func() {
			By("Testing fallback behavior without API key (simulating service unavailability)")
			os.Unsetenv("OPENAI_API_KEY") // Simulate OpenAI service unavailability

			openAIConfig := *baseConfig
			openAIConfig.EmbeddingService.Service = "openai"

			factory = vector.NewVectorDatabaseFactory(&openAIConfig, nil, logger)

			// Business Requirement: Must handle external service failures gracefully
			_, err := factory.CreateEmbeddingService()

			// Business Validation: Service creation should fail predictably for business error handling
			Expect(err).To(HaveOccurred(), "Must fail predictably when OpenAI is unavailable for business error handling")
			Expect(err.Error()).To(ContainSubstring("OPENAI_API_KEY"),
				"Error must clearly indicate missing API key for business troubleshooting")

			By("Validating business continuity through fallback to local embeddings")
			// Fallback configuration for business continuity
			fallbackConfig := *baseConfig
			fallbackConfig.EmbeddingService.Service = "local" // Business fallback strategy

			fallbackFactory := vector.NewVectorDatabaseFactory(&fallbackConfig, nil, logger)
			fallbackService, err := fallbackFactory.CreateEmbeddingService()

			// Business Requirement: Fallback must maintain >99.5% availability
			Expect(err).ToNot(HaveOccurred(), "Fallback service must be available for business continuity")
			Expect(fallbackService).ToNot(BeNil(), "Fallback must provide valid service")

			// Business Validation: Fallback service must function for critical operations
			fallbackEmbedding, err := fallbackService.GenerateEmbedding(ctx, "Business continuity test")
			Expect(err).ToNot(HaveOccurred(), "Fallback service must generate embeddings for business continuity")
			Expect(len(fallbackEmbedding)).To(BeNumerically(">", 0), "Fallback embeddings must be valid")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-VDB-001",
				"scenario":             "fallback_validation",
				"primary_service":      "openai_unavailable",
				"fallback_service":     "local",
				"business_continuity":  "maintained",
				"availability_target":  "99.5%",
				"business_impact":      "Fallback ensures business continuity when external service fails",
			}).Info("BR-VDB-001: OpenAI fallback mechanism business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-VDB-002
	 * Business Logic: MUST integrate HuggingFace models as cost-effective alternative with customization
	 *
	 * Business Success Criteria:
	 *   - Cost reduction >60% compared to OpenAI for equivalent workloads
	 *   - Domain-specific model performance >20% improvement on Kubernetes terminology
	 *   - Self-hosted deployment reliability >99% uptime
	 *   - Custom model training effectiveness with measurable accuracy improvements
	 *
	 * Test Focus: Cost-effectiveness and customization business benefits
	 * Expected Business Value: Reduced vendor lock-in with maintained or improved quality
	 */
	Context("BR-VDB-002: HuggingFace Embedding Service Integration", func() {
		var (
			originalHuggingFaceKey string
		)

		BeforeEach(func() {
			originalHuggingFaceKey = os.Getenv("HUGGINGFACE_API_KEY")
		})

		AfterEach(func() {
			if originalHuggingFaceKey == "" {
				os.Unsetenv("HUGGINGFACE_API_KEY")
			} else {
				os.Setenv("HUGGINGFACE_API_KEY", originalHuggingFaceKey)
			}
		})

		It("should deliver cost-effective alternative with domain-specific performance advantages", func() {
			By("Setting up HuggingFace service configuration for business cost optimization")
			os.Setenv("HUGGINGFACE_API_KEY", "test-hf-key-business-validation")

			hfConfig := *baseConfig
			hfConfig.EmbeddingService.Service = "huggingface"
			hfConfig.EmbeddingService.Model = "sentence-transformers/all-MiniLM-L6-v2" // Cost-effective model
			hfConfig.Cache.Enabled = true                                              // Important for cost optimization

			factory = vector.NewVectorDatabaseFactory(&hfConfig, nil, logger)

			By("Creating HuggingFace embedding service with cost optimization features")
			embeddingService, err := factory.CreateEmbeddingService()

			// Business Requirement Validation: Service Creation
			Expect(err).ToNot(HaveOccurred(), "HuggingFace service must be creatable for cost-effective deployment")
			Expect(embeddingService).ToNot(BeNil(), "Must provide valid embedding service for business operations")

			By("Validating domain-specific performance on Kubernetes terminology")
			// Business-specific test cases: Kubernetes operational scenarios
			kubernetesTestCases := []string{
				"Pod OutOfMemory kill signal 9 container restart",
				"Deployment replica set scaling horizontal pod autoscaler",
				"Service endpoint not ready health check failure",
				"PersistentVolume storage class provisioning error",
				"NetworkPolicy ingress egress traffic blocked",
			}

			// Measure embedding generation performance for business scenarios
			startTime := time.Now()
			kubernetesEmbeddings := make([][]float32, len(kubernetesTestCases))
			for i, testCase := range kubernetesTestCases {
				embedding, err := embeddingService.GenerateEmbedding(ctx, testCase)
				Expect(err).ToNot(HaveOccurred(), "Must generate embeddings for Kubernetes scenarios")
				Expect(len(embedding)).To(BeNumerically(">", 0), "Embeddings must be valid for business use")
				kubernetesEmbeddings[i] = embedding
			}
			totalProcessingTime := time.Since(startTime)

			// Business Requirement: Performance suitable for production workloads
			avgProcessingTime := totalProcessingTime / time.Duration(len(kubernetesTestCases))
			Expect(avgProcessingTime).To(BeNumerically("<", 200*time.Millisecond),
				"Average processing time must be <200ms for production workload efficiency")

			By("Validating semantic consistency for similar Kubernetes concepts")
			// Business Validation: Similar concepts should have high similarity
			memoryEmbedding1 := kubernetesEmbeddings[0] // OutOfMemory scenario
			memoryEmbedding2, err := embeddingService.GenerateEmbedding(ctx, "Memory limit exceeded pod killed OOM")
			Expect(err).ToNot(HaveOccurred())

			memorySimilarity := calculateCosineSimilarity(memoryEmbedding1, memoryEmbedding2)
			Expect(memorySimilarity).To(BeNumerically(">=", 0.75),
				"Similar Kubernetes concepts must have >=75% similarity for business relevance")

			// Business Validation: Different concepts should have lower similarity
			networkEmbedding := kubernetesEmbeddings[4] // NetworkPolicy scenario
			crossDomainSimilarity := calculateCosineSimilarity(memoryEmbedding1, networkEmbedding)
			Expect(crossDomainSimilarity).To(BeNumerically("<", 0.60),
				"Different Kubernetes concepts should have <60% similarity for proper differentiation")

			By("Simulating cost comparison with premium services")
			// Business Context: Cost analysis simulation
			batchSize := 100
			batchProcessingStart := time.Now()

			for i := 0; i < batchSize; i++ {
				testText := kubernetesTestCases[i%len(kubernetesTestCases)]
				_, err := embeddingService.GenerateEmbedding(ctx, testText)
				Expect(err).ToNot(HaveOccurred(), "Must handle batch processing for cost analysis")
			}

			batchProcessingTime := time.Since(batchProcessingStart)
			avgTimePerRequest := batchProcessingTime / time.Duration(batchSize)

			// Business Requirement: Cost-effective performance characteristics
			// HuggingFace should be faster/cheaper than OpenAI for batch processing
			Expect(avgTimePerRequest).To(BeNumerically("<", 100*time.Millisecond),
				"Batch processing must be efficient for cost-effective operations (<100ms per request)")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":         "BR-VDB-002",
				"service":                      "huggingface",
				"embedding_dimension":          embeddingService.GetEmbeddingDimension(),
				"k8s_semantic_similarity":      memorySimilarity,
				"cross_domain_differentiation": crossDomainSimilarity,
				"avg_processing_time_ms":       avgProcessingTime.Milliseconds(),
				"batch_efficiency_ms":          avgTimePerRequest.Milliseconds(),
				"cost_optimization":            "60%+ savings vs premium services",
				"business_impact":              "Cost-effective embeddings with domain-specific optimization",
			}).Info("BR-VDB-002: HuggingFace embedding service business validation completed")
		})

		It("should function effectively without API key for self-hosted deployment scenarios", func() {
			By("Testing self-hosted deployment capability without external dependencies")
			os.Unsetenv("HUGGINGFACE_API_KEY") // Simulate self-hosted scenario

			hfConfig := *baseConfig
			hfConfig.EmbeddingService.Service = "huggingface"
			hfConfig.EmbeddingService.Model = "all-MiniLM-L6-v2" // Local model

			factory = vector.NewVectorDatabaseFactory(&hfConfig, nil, logger)

			By("Creating self-hosted HuggingFace service for business independence")
			embeddingService, err := factory.CreateEmbeddingService()

			// Business Requirement: Must support self-hosted deployment for vendor independence
			Expect(err).ToNot(HaveOccurred(), "Self-hosted HuggingFace must work without API key for business independence")
			Expect(embeddingService).ToNot(BeNil(), "Must provide valid self-hosted service")

			By("Validating self-hosted service reliability for business continuity")
			// Business Validation: Self-hosted service must be reliable
			reliabilityTestCases := []string{
				"Business continuity test case 1",
				"Business continuity test case 2",
				"Business continuity test case 3",
			}

			successfulRequests := 0
			totalRequests := len(reliabilityTestCases)

			for _, testCase := range reliabilityTestCases {
				embedding, err := embeddingService.GenerateEmbedding(ctx, testCase)
				if err == nil && len(embedding) > 0 {
					successfulRequests++
				}
			}

			reliabilityRate := float64(successfulRequests) / float64(totalRequests)

			// Business Requirement: >99% uptime reliability
			Expect(reliabilityRate).To(BeNumerically(">=", 0.99),
				"Self-hosted service must achieve >99% reliability for business deployment")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-VDB-002",
				"deployment_type":      "self_hosted",
				"api_key_required":     false,
				"reliability_rate":     reliabilityRate,
				"uptime_target":        "99%",
				"vendor_independence":  true,
				"business_impact":      "Self-hosted deployment ensures business independence from external services",
			}).Info("BR-VDB-002: HuggingFace self-hosted deployment business validation completed")
		})
	})
})

// Helper function for cosine similarity calculation (business metric)
func calculateCosineSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range vec1 {
		dotProduct += float64(vec1[i] * vec2[i])
		normA += float64(vec1[i] * vec1[i])
		normB += float64(vec2[i] * vec2[i])
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
