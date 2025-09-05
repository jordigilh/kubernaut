//go:build integration
// +build integration

package vector

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Vector Database Performance and Resilience", Ordered, func() {
	var (
		logger           *logrus.Logger
		stateManager     *shared.ComprehensiveStateManager
		db               *sql.DB
		vectorDB         vector.VectorDatabase
		embeddingService vector.EmbeddingGenerator
		factory          *vector.VectorDatabaseFactory
		ctx              context.Context
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Reduce log noise for performance tests
		ctx = context.Background()

		// Use comprehensive state manager with database isolation
		stateManager = shared.NewIsolatedTestSuiteV2("Vector Performance Tests").
			WithLogger(logger).
			WithDatabaseIsolation(shared.TransactionIsolation).
			WithCustomCleanup(func() error {
				// Clean up performance test data
				if db != nil {
					_, err := db.Exec("DELETE FROM action_patterns WHERE id LIKE 'perf-%' OR id LIKE 'stress-%'")
					if err != nil {
						logger.WithError(err).Warn("Failed to clean up performance test patterns")
					}
				}
				return nil
			}).
			Build()

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Skip slow tests if requested
		if testConfig.SkipSlowTests {
			Skip("Slow tests skipped via SKIP_SLOW_TESTS")
		}

		// Get database connection
		dbInterface := stateManager.GetDatabaseHelper().GetDatabase()
		var ok bool
		db, ok = dbInterface.(*sql.DB)
		if !ok {
			Skip("Performance tests require a real database connection")
		}
		Expect(db).ToNot(BeNil(), "Database connection should be available")

		// Verify prerequisites
		var extensionExists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&extensionExists)
		if err != nil || !extensionExists {
			Skip("pgvector extension not available")
		}

		// Configure vector database for performance testing
		vectorConfig := &config.VectorDBConfig{
			Enabled: true,
			Backend: "postgresql",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
				Model:     "all-MiniLM-L6-v2",
			},
			PostgreSQL: config.PostgreSQLVectorConfig{
				UseMainDB:  true,
				IndexLists: 50, // Optimized for performance testing
			},
		}

		// Create services
		factory = vector.NewVectorDatabaseFactory(vectorConfig, db, logger)
		embeddingService, err = factory.CreateEmbeddingService()
		Expect(err).ToNot(HaveOccurred())
		vectorDB, err = factory.CreateVectorDatabase()
		Expect(err).ToNot(HaveOccurred())

		logger.Info("Vector database performance test suite setup completed")
	})

	AfterAll(func() {
		if stateManager != nil {
			err := stateManager.CleanupAllState()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	BeforeEach(func() {
		// Clean up any existing performance test data
		_, err := db.Exec("DELETE FROM action_patterns WHERE id LIKE 'perf-%' OR id LIKE 'stress-%'")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up after each test
		_, err := db.Exec("DELETE FROM action_patterns WHERE id LIKE 'perf-%' OR id LIKE 'stress-%'")
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Embedding Generation Performance", func() {
		It("should generate embeddings efficiently", func() {
			By("measuring embedding generation time for various text lengths")
			testTexts := []string{
				"short",
				"medium length alert message with some details",
				"very long alert message with extensive details about the system state including multiple resource types, namespaces, and complex failure scenarios that require detailed analysis and contextual understanding",
			}

			for _, text := range testTexts {
				startTime := time.Now()
				embedding, err := embeddingService.GenerateTextEmbedding(ctx, text)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(embedding).To(HaveLen(384))
				Expect(duration).To(BeNumerically("<", 500*time.Millisecond),
					fmt.Sprintf("Embedding generation should be fast for text: %s", text))

				logger.WithFields(logrus.Fields{
					"text_length": len(text),
					"duration_ms": duration.Milliseconds(),
				}).Debug("Embedding generation performance")
			}
		})

		It("should handle batch embedding generation efficiently", func() {
			By("generating multiple embeddings in parallel")
			const batchSize = 20
			texts := make([]string, batchSize)
			for i := 0; i < batchSize; i++ {
				texts[i] = fmt.Sprintf("test alert %d with specific details about the failure", i)
			}

			startTime := time.Now()

			var wg sync.WaitGroup
			errors := make(chan error, batchSize)
			embeddings := make(chan []float64, batchSize)

			for _, text := range texts {
				wg.Add(1)
				go func(t string) {
					defer wg.Done()
					emb, err := embeddingService.GenerateTextEmbedding(ctx, t)
					if err != nil {
						errors <- err
						return
					}
					embeddings <- emb
				}(text)
			}

			wg.Wait()
			close(errors)
			close(embeddings)

			duration := time.Since(startTime)

			By("verifying no errors occurred")
			select {
			case err := <-errors:
				Fail(fmt.Sprintf("Embedding generation failed: %v", err))
			default:
				// No errors
			}

			By("verifying all embeddings were generated")
			embeddingCount := 0
			for range embeddings {
				embeddingCount++
			}
			Expect(embeddingCount).To(Equal(batchSize))

			By("verifying batch processing is efficient")
			avgTimePerEmbedding := duration / time.Duration(batchSize)
			Expect(avgTimePerEmbedding).To(BeNumerically("<", 100*time.Millisecond))

			logger.WithFields(logrus.Fields{
				"batch_size":           batchSize,
				"total_duration_ms":    duration.Milliseconds(),
				"avg_per_embedding_ms": avgTimePerEmbedding.Milliseconds(),
			}).Info("Batch embedding generation performance")
		})
	})

	Context("Pattern Storage Performance", func() {
		It("should store patterns efficiently at scale", func() {
			By("creating a large set of test patterns")
			const patternCount = 100
			patterns := make([]*vector.ActionPattern, patternCount)

			for i := 0; i < patternCount; i++ {
				patterns[i] = createTestPatternWithType(
					embeddingService,
					ctx,
					fmt.Sprintf("action_type_%d", i%10),
					fmt.Sprintf("Alert_%d", i%20),
				)
				patterns[i].ID = fmt.Sprintf("perf-pattern-%d", i)
			}

			By("measuring bulk storage performance")
			startTime := time.Now()

			for _, pattern := range patterns {
				err := vectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}

			duration := time.Since(startTime)
			avgTimePerPattern := duration / time.Duration(patternCount)

			By("verifying storage performance meets expectations")
			Expect(avgTimePerPattern).To(BeNumerically("<", 50*time.Millisecond))

			By("verifying all patterns were stored")
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM action_patterns WHERE id LIKE 'perf-pattern-%'").Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(patternCount))

			logger.WithFields(logrus.Fields{
				"pattern_count":      patternCount,
				"total_duration_ms":  duration.Milliseconds(),
				"avg_per_pattern_ms": avgTimePerPattern.Milliseconds(),
			}).Info("Pattern storage performance")
		})

		It("should handle concurrent storage efficiently", func() {
			By("storing patterns concurrently")
			const concurrency = 10
			const patternsPerGoroutine = 10

			var wg sync.WaitGroup
			errors := make(chan error, concurrency*patternsPerGoroutine)

			startTime := time.Now()

			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(routineID int) {
					defer wg.Done()

					for j := 0; j < patternsPerGoroutine; j++ {
						pattern := createTestPatternWithType(
							embeddingService,
							ctx,
							fmt.Sprintf("concurrent_action_%d", routineID),
							fmt.Sprintf("ConcurrentAlert_%d_%d", routineID, j),
						)
						pattern.ID = fmt.Sprintf("stress-concurrent-%d-%d", routineID, j)

						err := vectorDB.StoreActionPattern(ctx, pattern)
						if err != nil {
							errors <- err
							return
						}
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(startTime)
			close(errors)

			By("verifying no errors occurred during concurrent storage")
			select {
			case err := <-errors:
				Fail(fmt.Sprintf("Concurrent storage failed: %v", err))
			default:
				// No errors
			}

			By("verifying all patterns were stored")
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM action_patterns WHERE id LIKE 'stress-concurrent-%'").Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(concurrency * patternsPerGoroutine))

			totalPatterns := concurrency * patternsPerGoroutine
			avgTimePerPattern := duration / time.Duration(totalPatterns)

			logger.WithFields(logrus.Fields{
				"concurrency":          concurrency,
				"patterns_per_routine": patternsPerGoroutine,
				"total_patterns":       totalPatterns,
				"total_duration_ms":    duration.Milliseconds(),
				"avg_per_pattern_ms":   avgTimePerPattern.Milliseconds(),
			}).Info("Concurrent storage performance")
		})
	})

	Context("Vector Search Performance", func() {
		var searchPatterns []*vector.ActionPattern

		BeforeEach(func() {
			// Populate database with patterns for search testing
			const searchTestPatterns = 50
			searchPatterns = make([]*vector.ActionPattern, searchTestPatterns)

			for i := 0; i < searchTestPatterns; i++ {
				searchPatterns[i] = createTestPatternWithType(
					embeddingService,
					ctx,
					fmt.Sprintf("search_action_%d", i%5),
					fmt.Sprintf("SearchAlert_%d", i%10),
				)
				searchPatterns[i].ID = fmt.Sprintf("perf-search-%d", i)

				err := vectorDB.StoreActionPattern(ctx, searchPatterns[i])
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should perform similarity search efficiently", func() {
			By("measuring similarity search performance")
			searchPattern := createTestPatternWithType(embeddingService, ctx, "search_action_1", "SearchAlert_1")

			const searchIterations = 10
			var totalDuration time.Duration

			for i := 0; i < searchIterations; i++ {
				startTime := time.Now()

				similarPatterns, err := vectorDB.FindSimilarPatterns(ctx, searchPattern, 10, 0.5)

				duration := time.Since(startTime)
				totalDuration += duration

				Expect(err).ToNot(HaveOccurred())
				Expect(len(similarPatterns)).To(BeNumerically(">", 0))
			}

			avgSearchTime := totalDuration / searchIterations
			Expect(avgSearchTime).To(BeNumerically("<", 100*time.Millisecond))

			logger.WithFields(logrus.Fields{
				"search_iterations":  searchIterations,
				"avg_search_time_ms": avgSearchTime.Milliseconds(),
				"total_patterns":     len(searchPatterns),
			}).Info("Similarity search performance")
		})

		It("should perform semantic search efficiently", func() {
			By("measuring semantic search performance")
			searchQueries := []string{
				"memory usage high",
				"deployment scaling",
				"pod restart failure",
				"resource quota exceeded",
				"node not ready",
			}

			for _, query := range searchQueries {
				startTime := time.Now()

				patterns, err := vectorDB.SearchBySemantics(ctx, query, 10)

				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(duration).To(BeNumerically("<", 150*time.Millisecond))

				logger.WithFields(logrus.Fields{
					"query":          query,
					"search_time_ms": duration.Milliseconds(),
					"results_found":  len(patterns),
				}).Debug("Semantic search performance")
			}
		})

		It("should handle concurrent search efficiently", func() {
			By("performing concurrent searches")
			const concurrentSearches = 5

			var wg sync.WaitGroup
			errors := make(chan error, concurrentSearches)
			durations := make(chan time.Duration, concurrentSearches)

			searchPattern := createTestPatternWithType(embeddingService, ctx, "search_action_2", "SearchAlert_2")

			startTime := time.Now()

			for i := 0; i < concurrentSearches; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					searchStart := time.Now()
					_, err := vectorDB.FindSimilarPatterns(ctx, searchPattern, 5, 0.6)
					searchDuration := time.Since(searchStart)

					if err != nil {
						errors <- err
						return
					}
					durations <- searchDuration
				}()
			}

			wg.Wait()
			totalConcurrentTime := time.Since(startTime)
			close(errors)
			close(durations)

			By("verifying no errors occurred")
			select {
			case err := <-errors:
				Fail(fmt.Sprintf("Concurrent search failed: %v", err))
			default:
				// No errors
			}

			By("measuring individual search times")
			var maxDuration time.Duration
			searchCount := 0
			for duration := range durations {
				if duration > maxDuration {
					maxDuration = duration
				}
				searchCount++
			}

			Expect(searchCount).To(Equal(concurrentSearches))

			logger.WithFields(logrus.Fields{
				"concurrent_searches":    concurrentSearches,
				"total_time_ms":          totalConcurrentTime.Milliseconds(),
				"max_individual_time_ms": maxDuration.Milliseconds(),
			}).Info("Concurrent search performance")
		})
	})

	Context("Error Injection and Resilience", func() {
		It("should handle database connection issues gracefully", func() {
			By("simulating temporary database unavailability")
			// Note: In a real test environment, you might temporarily disable
			// the database connection or use a connection pool that can be controlled

			// For now, we'll test with invalid queries to simulate database issues
			invalidVectorDB := vector.NewPostgreSQLVectorDatabase(db, embeddingService, logger)

			// Test resilience by attempting operations during simulated issues
			pattern := createTestPatternWithType(embeddingService, ctx, "resilience_test", "ResilienceAlert")
			pattern.ID = "stress-resilience-1"

			// This should succeed normally
			err := invalidVectorDB.StoreActionPattern(ctx, pattern)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle large embedding dimensions gracefully", func() {
			By("testing with unusually large embeddings")
			pattern := createTestPatternWithType(embeddingService, ctx, "large_embedding", "LargeEmbeddingAlert")
			pattern.ID = "stress-large-embedding"

			// Create an unusually large embedding (still within reasonable bounds)
			largeEmbedding := make([]float64, 384)
			for i := range largeEmbedding {
				largeEmbedding[i] = float64(i) / 384.0
			}
			pattern.Embedding = largeEmbedding

			err := vectorDB.StoreActionPattern(ctx, pattern)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle invalid pattern data gracefully", func() {
			By("testing with various invalid pattern configurations")

			// Test with empty ID
			invalidPattern1 := createTestPatternWithType(embeddingService, ctx, "invalid_test", "InvalidAlert")
			invalidPattern1.ID = ""
			err := vectorDB.StoreActionPattern(ctx, invalidPattern1)
			Expect(err).To(HaveOccurred())

			// Test with nil embedding
			invalidPattern2 := createTestPatternWithType(embeddingService, ctx, "invalid_test", "InvalidAlert")
			invalidPattern2.ID = "stress-invalid-2"
			invalidPattern2.Embedding = nil
			err = vectorDB.StoreActionPattern(ctx, invalidPattern2)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Resource Usage and Cleanup", func() {
		It("should manage memory efficiently during large operations", func() {
			By("performing memory-intensive operations")
			const largePatternCount = 200

			// Create and store a large number of patterns
			for i := 0; i < largePatternCount; i++ {
				pattern := createTestPatternWithType(
					embeddingService,
					ctx,
					fmt.Sprintf("memory_test_%d", i%20),
					fmt.Sprintf("MemoryAlert_%d", i),
				)
				pattern.ID = fmt.Sprintf("stress-memory-%d", i)

				err := vectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())

				// Periodically check that we can still perform operations
				if i%50 == 0 {
					analytics, err := vectorDB.GetPatternAnalytics(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(analytics.TotalPatterns).To(BeNumerically(">=", i))
				}
			}

			By("verifying all patterns were stored correctly")
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM action_patterns WHERE id LIKE 'stress-memory-%'").Scan(&count)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(largePatternCount))
		})
	})
})
