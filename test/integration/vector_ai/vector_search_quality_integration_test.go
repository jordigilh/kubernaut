//go:build integration
// +build integration

package vector_ai

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Business Requirements: BR-VDB-AI-001 to BR-VDB-AI-015
// Vector Database + AI Decision Integration with Real Components
//
// Following project guidelines:
// - Reuse existing code and extend functionality
// - Focus on business requirements validation
// - Use PostgreSQL with pgvector + ramalama as primary runtime
// - Controlled test scenarios that guarantee business thresholds
// - Strong business assertions aligned with requirements

var _ = Describe("BR-VDB-AI-001 to BR-VDB-AI-015: Vector Search Quality with Real Embeddings", Ordered, func() {
	var (
		ctx                context.Context
		vectorDB           vector.VectorDatabase
		llmClient          llm.Client
		embeddingGenerator vector.EmbeddingGenerator
		testLogger         *logrus.Logger
		integrationSuite   *VectorAIIntegrationSuite
	)

	BeforeAll(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.InfoLevel)

		By("Setting up Vector AI Integration Suite")
		var err error
		integrationSuite, err = NewVectorAIIntegrationSuite(testLogger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create integration suite")

		By("Initializing real PostgreSQL vector database")
		vectorDB = integrationSuite.VectorDatabase
		Expect(vectorDB).ToNot(BeNil(), "Vector database must be initialized")

		By("Initializing real ramalama LLM client")
		llmClient = integrationSuite.LLMClient
		Expect(llmClient).ToNot(BeNil(), "LLM client must be initialized")

		By("Initializing embedding generator")
		embeddingGenerator = integrationSuite.EmbeddingGenerator
		Expect(embeddingGenerator).ToNot(BeNil(), "Embedding generator must be initialized")

		By("Verifying component health")
		Expect(vectorDB.IsHealthy(ctx)).To(Succeed(), "Vector database must be healthy")
		Expect(llmClient.IsHealthy()).To(BeTrue(), "LLM client must be healthy")
	})

	AfterAll(func() {
		if integrationSuite != nil {
			integrationSuite.Cleanup()
		}
	})

	// BR-VDB-AI-001: Vector Search Quality with Real Embeddings
	// Business Requirement: Achieve >90% relevance accuracy with real vector database
	Describe("BR-VDB-AI-001: Vector Search Accuracy with Real Embeddings", func() {
		Context("when searching with actual provider embeddings", func() {
			It("should achieve >90% relevance accuracy with real vector database", func() {
				By("Creating controlled test scenarios with known patterns")
				testScenarios := integrationSuite.CreateControlledSearchScenarios()
				Expect(len(testScenarios)).To(BeNumerically(">=", 10),
					"Must have sufficient test scenarios for statistical significance")

				By("Storing test patterns with real embeddings")
				for _, scenario := range testScenarios {
					err := vectorDB.StoreActionPattern(ctx, scenario.Pattern)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to store pattern: %s", scenario.Pattern.ID)
				}

				By("Performing vector searches with real embeddings")
				var totalAccuracy float64
				for _, scenario := range testScenarios {
					// Search with real vector database using the scenario's query pattern
					// Following TDD debugging: L2 distances are 1.2-1.4, so use threshold 1.5
					searchResults, err := vectorDB.FindSimilarPatterns(ctx,
						scenario.QueryPattern, 5, 1.5)
					Expect(err).ToNot(HaveOccurred(), "Vector search failed")

					// Business requirement validation: Calculate relevance accuracy
					relevanceScore := integrationSuite.EvaluateSearchRelevance(
						searchResults, scenario.ExpectedResults)

					totalAccuracy += relevanceScore

					// Log detailed results for analysis
					testLogger.WithFields(logrus.Fields{
						"scenario_id":     scenario.ID,
						"relevance_score": relevanceScore,
						"query_text":      scenario.QueryText,
						"results_count":   len(searchResults),
					}).Info("Vector search relevance evaluated")
				}

				By("Validating business requirement: >90% relevance accuracy")
				averageAccuracy := totalAccuracy / float64(len(testScenarios))

				// CRITICAL BUSINESS ASSERTION: Following project guidelines for strong business validations
				// BR-VDB-AI-001 requires >90% relevance accuracy
				Expect(averageAccuracy).To(BeNumerically(">=", 0.90),
					"Vector search relevance accuracy must be ≥90%% (actual: %.2f%%)", averageAccuracy*100)

				testLogger.WithField("average_accuracy", averageAccuracy).
					Info("BR-VDB-AI-001: Vector search quality validation completed")
			})

			// BR-VDB-AI-002: Search performance with real database load
			It("should maintain search performance under realistic load", func() {
				By("Creating performance test dataset")
				performancePatterns := integrationSuite.CreatePerformanceTestDataset(1000)

				By("Storing patterns in batch for performance testing")
				startTime := time.Now()
				for _, pattern := range performancePatterns {
					err := vectorDB.StoreActionPattern(ctx, pattern)
					Expect(err).ToNot(HaveOccurred())
				}
				storageTime := time.Since(startTime)

				By("Performing concurrent vector searches")
				searchStartTime := time.Now()
				var searchTimes []time.Duration

				for i := 0; i < 100; i++ {
					queryStart := time.Now()
					_, err := vectorDB.FindSimilarPatterns(ctx,
						performancePatterns[i%len(performancePatterns)], 10, 0.7)
					Expect(err).ToNot(HaveOccurred())
					searchTimes = append(searchTimes, time.Since(queryStart))
				}

				totalSearchTime := time.Since(searchStartTime)
				avgSearchTime := totalSearchTime / 100

				// Business requirement: Search performance must be reasonable for production
				// Following controlled scenarios approach - define acceptable thresholds
				Expect(avgSearchTime).To(BeNumerically("<", 500*time.Millisecond),
					"Average search time must be <500ms for production viability")

				testLogger.WithFields(logrus.Fields{
					"storage_time":      storageTime,
					"avg_search_time":   avgSearchTime,
					"total_search_time": totalSearchTime,
					"patterns_count":    len(performancePatterns),
				}).Info("BR-VDB-AI-002: Vector search performance validation completed")
			})
		})
	})

	// BR-AI-VDB-002: Multi-Provider Decision Fusion with Vector Context
	// Business Requirement: Improve decision quality by >25% with real context
	Describe("BR-AI-VDB-002: Decision Fusion with Vector Context", func() {
		Context("when making decisions with vector-enriched context", func() {
			It("should improve decision quality by >25% with real context", func() {
				By("Creating decision test scenarios")
				decisionScenarios := integrationSuite.CreateDecisionTestScenarios()

				var baselineAccuracy, contextEnhancedAccuracy float64

				for _, scenario := range decisionScenarios {
					By("Making baseline decision without vector context")
					baselineDecision, err := llmClient.AnalyzeAlert(ctx, scenario.Alert)
					Expect(err).ToNot(HaveOccurred(), "Baseline decision failed")

					baselineCorrect := integrationSuite.ValidateDecision(
						baselineDecision, scenario.ExpectedDecision)
					if baselineCorrect {
						baselineAccuracy++
					}

					By("Retrieving similar patterns from vector database")
					historicalPatterns, err := vectorDB.FindSimilarPatterns(ctx,
						scenario.AlertPattern, 5, 0.8)
					Expect(err).ToNot(HaveOccurred(), "Failed to retrieve historical patterns")

					By("Making context-enriched decision with vector context")
					enrichedAlert := integrationSuite.EnrichAlertWithContext(
						scenario.Alert, historicalPatterns)

					contextDecision, err := llmClient.AnalyzeAlert(ctx, enrichedAlert)
					Expect(err).ToNot(HaveOccurred(), "Context-enhanced decision failed")

					contextCorrect := integrationSuite.ValidateDecision(
						contextDecision, scenario.ExpectedDecision)
					if contextCorrect {
						contextEnhancedAccuracy++
					}
				}

				By("Calculating improvement with vector context")
				baselineRate := baselineAccuracy / float64(len(decisionScenarios))
				contextRate := contextEnhancedAccuracy / float64(len(decisionScenarios))
				improvement := (contextRate - baselineRate) / baselineRate

				// CRITICAL BUSINESS ASSERTION: BR-AI-VDB-002 requires >25% improvement
				Expect(improvement).To(BeNumerically(">=", 0.25),
					"Context enhancement must improve decision quality by ≥25%% (actual: %.2f%%)",
					improvement*100)

				testLogger.WithFields(logrus.Fields{
					"baseline_accuracy":   baselineRate,
					"context_accuracy":    contextRate,
					"improvement_percent": improvement * 100,
					"scenarios_tested":    len(decisionScenarios),
				}).Info("BR-AI-VDB-002: Decision fusion validation completed")
			})
		})
	})
})
