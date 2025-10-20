package contextapi

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

var _ = Describe("Vector Search Integration Tests", func() {
	var (
		testCtx        context.Context
		cancel         context.CancelFunc
		cachedExecutor *query.CachedExecutor
		testEmbedding  []float32
	)

	BeforeEach(func() {
		testCtx, cancel = context.WithTimeout(ctx, 30*time.Second)

		// BR-CONTEXT-003: Semantic similarity search setup via CachedExecutor
		cacheConfig := &cache.Config{
			RedisAddr:  "localhost:6379",
			LRUSize:    1000,
			DefaultTTL: 5 * time.Minute,
		}
		cacheManager, err := cache.NewCacheManager(cacheConfig, logger)
		Expect(err).ToNot(HaveOccurred())

		executorCfg := &query.Config{
			DB:    sqlxDB,
			Cache: cacheManager,
			TTL:   5 * time.Minute,
		}
		cachedExecutor, err = query.NewCachedExecutor(executorCfg)
		Expect(err).ToNot(HaveOccurred())

		// Create test embedding (384 dimensions per validation)
		testEmbedding = CreateTestEmbedding(384)

		// Setup test data with embeddings
		for i := 0; i < 10; i++ {
			incident := CreateIncidentWithEmbedding(int64(i+1), "default")
			if i < 5 {
				// First 5 incidents have similar embeddings
				incident.Embedding = CreateSimilarEmbedding(testEmbedding, 0.9)
			} else {
				// Last 5 have different embeddings
				incident.Embedding = CreateTestEmbedding(384)
			}
			err := InsertTestIncident(sqlxDB, incident)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		defer cancel()

		// Clean up test data
		_, err := db.ExecContext(testCtx, "TRUNCATE TABLE remediation_audit")
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Basic Vector Search", func() {
		It("should return similar incidents based on embedding similarity", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Semantic similarity search
			limit := 5
			threshold := float32(0.8)

			incidents, scores, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, limit, threshold)

			Expect(err).ToNot(HaveOccurred(), "Vector search should succeed")
			Expect(incidents).ToNot(BeEmpty(), "Should return similar incidents")
			Expect(scores).ToNot(BeEmpty(), "Should return similarity scores")
			Expect(len(incidents)).To(Equal(len(scores)), "Incidents and scores should match")
			Expect(len(incidents)).To(BeNumerically("<=", limit), "Should respect limit")

			// Verify all scores meet threshold
			for _, score := range scores {
				Expect(score).To(BeNumerically(">=", threshold), "Scores should meet threshold")
			}
		})
	})

	Context("Similarity Threshold", func() {
		It("should filter results by similarity score", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Similarity threshold filtering
			// High threshold (0.9) - should return fewer, more similar results
			incidentsHigh, scoresHigh, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, 10, 0.9)
			Expect(err).ToNot(HaveOccurred())

			// Low threshold (0.5) - should return more results
			incidentsLow, scoresLow, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, 10, 0.5)
			Expect(err).ToNot(HaveOccurred())

			// Low threshold should return more or equal results
			Expect(len(incidentsLow)).To(BeNumerically(">=", len(incidentsHigh)))

			// All high-threshold scores should be >= 0.9
			for _, score := range scoresHigh {
				Expect(score).To(BeNumerically(">=", 0.9))
			}

			// All low-threshold scores should be >= 0.5
			for _, score := range scoresLow {
				Expect(score).To(BeNumerically(">=", 0.5))
			}
		})
	})

	Context("Limit Parameter", func() {
		It("should respect result count limit", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Result limiting
			limits := []int{1, 3, 5, 10}

			for _, limit := range limits {
				incidents, _, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, limit, 0.5)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(incidents)).To(BeNumerically("<=", limit),
					fmt.Sprintf("Should return at most %d results", limit))
			}
		})
	})

	Context("Empty Embedding", func() {
		It("should handle empty embedding gracefully", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Input validation
			emptyEmbedding := []float32{}

			_, _, err := cachedExecutor.SemanticSearch(testCtx, emptyEmbedding, 10, 0.8)

			// Should return error for invalid embedding
			Expect(err).To(HaveOccurred(), "Empty embedding should return error")
		})
	})

	Context("Zero Results", func() {
		It("should return empty array when no results match threshold", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Empty result set handling
			// Use very high threshold (0.99) with different embedding
			differentEmbedding := CreateTestEmbedding(384)

			incidents, scores, err := cachedExecutor.SemanticSearch(testCtx, differentEmbedding, 10, 0.99)

			Expect(err).ToNot(HaveOccurred(), "Query should succeed")
			Expect(incidents).To(BeEmpty(), "Should return empty array")
			Expect(scores).To(BeEmpty(), "Should return empty scores")
		})
	})

	Context("High-Dimensional Embeddings", func() {
		It("should handle high-dimensional embeddings correctly", func() {
			// Day 8 DO-GREEN: Test activated

			// BR-CONTEXT-003: Support for various embedding dimensions
			// Test with 384-dimensional embedding (standard for Context API)
			embedding384 := CreateTestEmbedding(384)

			incidents, _, err := cachedExecutor.SemanticSearch(testCtx, embedding384, 5, 0.8)

			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).ToNot(BeNil())
		})
	})

	Context("Distance Metrics", func() {
		It("should use cosine similarity for distance calculation", func() {
			// Day 8 DO-REFACTOR: Test activated (Batch 8)

			// BR-CONTEXT-003: Cosine similarity metric
			// pgvector supports: <-> (L2), <#> (negative inner product), <=> (cosine distance)
			// We use cosine distance (<=> operator) for semantic similarity

			incidents, scores, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, 5, 0.8)

			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).ToNot(BeNil())
			Expect(scores).ToNot(BeEmpty())

			// Scores should be between 0 and 1 for cosine similarity
			for _, score := range scores {
				Expect(score).To(BeNumerically(">=", 0.0))
				Expect(score).To(BeNumerically("<=", 1.0))
			}
		})
	})

	Context("Score Ordering", func() {
		It("should return results ordered by similarity score (highest first)", func() {
			// Day 8 DO-REFACTOR: Test activated (Batch 8)

			// BR-CONTEXT-003: Result ordering by similarity
			incidents, scores, err := cachedExecutor.SemanticSearch(testCtx, testEmbedding, 10, 0.5)

			Expect(err).ToNot(HaveOccurred())
			Expect(incidents).ToNot(BeNil())
			Expect(scores).ToNot(BeEmpty())

			// Verify scores are in descending order
			for i := 1; i < len(scores); i++ {
				Expect(scores[i]).To(BeNumerically("<=", scores[i-1]),
					"Scores should be in descending order")
			}
		})
	})
})
