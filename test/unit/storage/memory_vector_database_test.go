package storage

import (
	"testing"
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	. "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("Memory Vector Database Unit Tests", func() {
	var (
		memoryDB *vector.MemoryVectorDatabase
		logger   *logrus.Logger
		ctx      context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
		memoryDB = vector.NewMemoryVectorDatabase(logger)
		ctx = context.Background()
	})

	Context("Database Creation and Initialization", func() {
		It("should create database with empty pattern store", func() {
			Expect(memoryDB).ToNot(BeNil())

			count := memoryDB.GetPatternCount()
			Expect(count).To(Equal(0), "New database should have zero patterns")
		})

		It("should pass health check for new database", func() {
			err := memoryDB.IsHealthy(ctx)
			Expect(err).ToNot(HaveOccurred(), "New database should be healthy")
		})
	})

	Context("Pattern Storage Operations", func() {
		var testPattern *vector.ActionPattern

		BeforeEach(func() {
			testPattern = CreateTestActionPattern("test-pattern-1")
		})

		It("should store pattern successfully with valid data", func() {
			err := memoryDB.StoreActionPattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred(), "Should store valid pattern without error")

			count := memoryDB.GetPatternCount()
			Expect(count).To(Equal(1), "Pattern count should increase after storing")

			stored, err := memoryDB.GetPattern(testPattern.ID)
			Expect(err).ToNot(HaveOccurred(), "Should retrieve stored pattern")
			Expect(stored.ID).To(Equal(testPattern.ID), "Stored pattern should have correct ID")
			Expect(stored.ActionType).To(Equal(testPattern.ActionType), "Should preserve action type")
		})

		It("should reject pattern with empty ID", func() {
			invalidPattern := CreateTestActionPattern("")
			invalidPattern.ID = ""

			err := memoryDB.StoreActionPattern(ctx, invalidPattern)
			Expect(err).To(HaveOccurred(), "Should reject pattern with empty ID")
			Expect(err.Error()).To(ContainSubstring("pattern ID cannot be empty"))

			count := memoryDB.GetPatternCount()
			Expect(count).To(Equal(0), "Should not store invalid pattern")
		})

		It("should reject pattern with empty embedding", func() {
			invalidPattern := CreateTestActionPattern("test-invalid")
			invalidPattern.Embedding = []float64{}

			err := memoryDB.StoreActionPattern(ctx, invalidPattern)
			Expect(err).To(HaveOccurred(), "Should reject pattern with empty embedding")
			Expect(err.Error()).To(ContainSubstring("pattern embedding cannot be empty"))
		})

		It("should update timestamp when storing pattern", func() {
			beforeStore := time.Now()

			err := memoryDB.StoreActionPattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred())

			stored, err := memoryDB.GetPattern(testPattern.ID)
			Expect(err).ToNot(HaveOccurred())

			Expect(stored.UpdatedAt).To(BeTemporally(">=", beforeStore), "Should update timestamp during storage")
			if stored.CreatedAt.IsZero() {
				Expect(stored.CreatedAt).To(Equal(stored.UpdatedAt), "Should set created timestamp if not present")
			}
		})

		It("should overwrite existing pattern with same ID", func() {
			// Store initial pattern
			err := memoryDB.StoreActionPattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred())

			// Modify and store again
			modifiedPattern := *testPattern
			modifiedPattern.ActionType = "restart"
			modifiedPattern.EffectivenessData.Score = 0.95

			err = memoryDB.StoreActionPattern(ctx, &modifiedPattern)
			Expect(err).ToNot(HaveOccurred())

			// Verify overwrite
			count := memoryDB.GetPatternCount()
			Expect(count).To(Equal(1), "Should not duplicate patterns with same ID")

			stored, err := memoryDB.GetPattern(testPattern.ID)
			Expect(err).ToNot(HaveOccurred())
			Expect(stored.ActionType).To(Equal("restart"), "Should update action type")
			Expect(stored.EffectivenessData.Score).To(Equal(0.95), "Should update effectiveness score")
		})
	})

	Context("Pattern Retrieval Operations", func() {
		BeforeEach(func() {
			// Store test patterns
			patterns := []*vector.ActionPattern{
				CreateTestActionPattern("pattern-1"),
				CreateTestActionPattern("pattern-2"),
				CreateTestActionPattern("pattern-3"),
			}

			for _, pattern := range patterns {
				err := memoryDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should retrieve existing pattern by ID", func() {
			pattern, err := memoryDB.GetPattern("pattern-1")
			Expect(err).ToNot(HaveOccurred(), "Should retrieve existing pattern")
			Expect(pattern.ID).To(Equal("pattern-1"))
		})

		It("should return error for non-existent pattern ID", func() {
			_, err := memoryDB.GetPattern("non-existent")
			Expect(err).To(HaveOccurred(), "Should return error for non-existent pattern")
			Expect(err.Error()).To(ContainSubstring("pattern with ID non-existent not found"))
		})

		It("should return copy of pattern to prevent external modification", func() {
			originalPattern, err := memoryDB.GetPattern("pattern-1")
			Expect(err).ToNot(HaveOccurred())

			// Modify retrieved pattern
			originalEffectiveness := originalPattern.EffectivenessData.Score
			originalPattern.EffectivenessData.Score = 0.99

			// Verify original is unchanged
			retrievedAgain, err := memoryDB.GetPattern("pattern-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(retrievedAgain.EffectivenessData.Score).To(Equal(originalEffectiveness),
				"Database should return immutable copies")
		})
	})

	Context("Pattern Similarity Search", func() {
		var queryPattern *vector.ActionPattern

		BeforeEach(func() {
			// Store patterns with different embeddings for similarity testing
			pattern1 := CreateTestActionPattern("similar-1")
			pattern1.Embedding = []float64{1.0, 0.0, 0.0, 0.0, 0.0} // High similarity to query

			pattern2 := CreateTestActionPattern("similar-2")
			pattern2.Embedding = []float64{0.8, 0.2, 0.0, 0.0, 0.0} // Medium similarity

			pattern3 := CreateTestActionPattern("different-1")
			pattern3.Embedding = []float64{0.0, 0.0, 1.0, 0.0, 0.0} // Low similarity

			patterns := []*vector.ActionPattern{pattern1, pattern2, pattern3}
			for _, pattern := range patterns {
				err := memoryDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}

			// Create query pattern similar to pattern1
			queryPattern = CreateTestActionPattern("query-pattern")
			queryPattern.Embedding = []float64{0.9, 0.1, 0.0, 0.0, 0.0}
		})

		It("should find similar patterns above threshold", func() {
			similar, err := memoryDB.FindSimilarPatterns(ctx, queryPattern, 10, 0.5)
			Expect(err).ToNot(HaveOccurred(), "Should execute similarity search without error")

			// Should find at least the most similar patterns
			Expect(len(similar)).To(BeNumerically(">=", 1), "Should find similar patterns above threshold")

			// Results should be ordered by similarity descending
			if len(similar) > 1 {
				Expect(similar[0].Similarity).To(BeNumerically(">=", similar[1].Similarity),
					"Results should be ordered by similarity")
			}

			// All results should be above threshold
			for _, result := range similar {
				Expect(result.Similarity).To(BeNumerically(">=", 0.5),
					"All results should meet similarity threshold")
			}
		})

		It("should exclude query pattern from results", func() {
			// Store the query pattern
			err := memoryDB.StoreActionPattern(ctx, queryPattern)
			Expect(err).ToNot(HaveOccurred())

			similar, err := memoryDB.FindSimilarPatterns(ctx, queryPattern, 10, 0.0)
			Expect(err).ToNot(HaveOccurred())

			// Should not include the query pattern itself
			for _, result := range similar {
				Expect(result.Pattern.ID).ToNot(Equal(queryPattern.ID),
					"Should exclude query pattern from results")
			}
		})

		It("should respect limit parameter", func() {
			limit := 2
			similar, err := memoryDB.FindSimilarPatterns(ctx, queryPattern, limit, 0.0)
			Expect(err).ToNot(HaveOccurred())

			Expect(len(similar)).To(BeNumerically("<=", limit), "Should respect limit parameter")
		})

		It("should assign correct ranks to results", func() {
			similar, err := memoryDB.FindSimilarPatterns(ctx, queryPattern, 10, 0.0)
			Expect(err).ToNot(HaveOccurred())

			for i, result := range similar {
				expectedRank := i + 1
				Expect(result.Rank).To(Equal(expectedRank), "Should assign sequential ranks")
			}
		})

		It("should reject query with empty embedding", func() {
			emptyQueryPattern := CreateTestActionPattern("empty-query")
			emptyQueryPattern.Embedding = []float64{}

			_, err := memoryDB.FindSimilarPatterns(ctx, emptyQueryPattern, 10, 0.5)
			Expect(err).To(HaveOccurred(), "Should reject query with empty embedding")
			Expect(err.Error()).To(ContainSubstring("query pattern embedding cannot be empty"))
		})
	})

	Context("Pattern Effectiveness Updates", func() {
		var testPattern *vector.ActionPattern

		BeforeEach(func() {
			testPattern = CreateTestActionPattern("effectiveness-test")
			err := memoryDB.StoreActionPattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should update effectiveness score for existing pattern", func() {
			newScore := 0.92

			err := memoryDB.UpdatePatternEffectiveness(ctx, testPattern.ID, newScore)
			Expect(err).ToNot(HaveOccurred(), "Should update effectiveness without error")

			updated, err := memoryDB.GetPattern(testPattern.ID)
			Expect(err).ToNot(HaveOccurred())

			Expect(updated.EffectivenessData.Score).To(Equal(newScore), "Should update effectiveness score")
			Expect(updated.EffectivenessData.LastAssessed).To(BeTemporally(">=", time.Now().Add(-time.Minute)),
				"Should update last assessed timestamp")
		})

		It("should initialize effectiveness data if nil", func() {
			// Create pattern without effectiveness data
			patternWithoutEffectiveness := CreateTestActionPattern("no-effectiveness")
			patternWithoutEffectiveness.EffectivenessData = nil
			err := memoryDB.StoreActionPattern(ctx, patternWithoutEffectiveness)
			Expect(err).ToNot(HaveOccurred())

			newScore := 0.75
			err = memoryDB.UpdatePatternEffectiveness(ctx, patternWithoutEffectiveness.ID, newScore)
			Expect(err).ToNot(HaveOccurred())

			updated, err := memoryDB.GetPattern(patternWithoutEffectiveness.ID)
			Expect(err).ToNot(HaveOccurred())

			Expect(updated.EffectivenessData.Score).To(BeNumerically(">=", 0), "BR-DATABASE-001-A: Effectiveness data must contain valid scoring metrics for vector pattern analysis")
			Expect(updated.EffectivenessData.Score).To(Equal(newScore), "Should set correct score")
		})

		It("should return error for non-existent pattern", func() {
			err := memoryDB.UpdatePatternEffectiveness(ctx, "non-existent", 0.8)
			Expect(err).To(HaveOccurred(), "Should return error for non-existent pattern")
			Expect(err.Error()).To(ContainSubstring("pattern with ID non-existent not found"))
		})
	})

	Context("Semantic Search Operations", func() {
		BeforeEach(func() {
			// Store patterns with different characteristics for semantic search
			patterns := []*vector.ActionPattern{
				{
					ID:           "cpu-scale",
					ActionType:   "scale",
					AlertName:    "HighCPUUsage",
					ResourceType: "deployment",
					Embedding:    []float64{0.1, 0.2},
				},
				{
					ID:           "memory-restart",
					ActionType:   "restart",
					AlertName:    "HighMemoryUsage",
					ResourceType: "pod",
					Embedding:    []float64{0.3, 0.4},
				},
				{
					ID:           "disk-cleanup",
					ActionType:   "cleanup",
					AlertName:    "LowDiskSpace",
					ResourceType: "node",
					Embedding:    []float64{0.5, 0.6},
				},
			}

			for _, pattern := range patterns {
				pattern.EffectivenessData = &vector.EffectivenessData{Score: 0.8}
				err := memoryDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should find patterns matching action type query", func() {
			results, err := memoryDB.SearchBySemantics(ctx, "scale", 10)
			Expect(err).ToNot(HaveOccurred(), "Should execute semantic search without error")

			Expect(len(results)).To(BeNumerically(">=", 1), "Should find patterns matching action type")

			// Verify at least one result contains the search term
			found := false
			for _, result := range results {
				if result.ActionType == "scale" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should find pattern with matching action type")
		})

		It("should find patterns matching alert name query", func() {
			results, err := memoryDB.SearchBySemantics(ctx, "memory", 10)
			Expect(err).ToNot(HaveOccurred())

			found := false
			for _, result := range results {
				if result.AlertName == "HighMemoryUsage" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Should find pattern with matching alert name")
		})

		It("should sort results by effectiveness score", func() {
			// Update effectiveness scores to test sorting
			err := memoryDB.UpdatePatternEffectiveness(ctx, "cpu-scale", 0.95)
			Expect(err).ToNot(HaveOccurred())

			err = memoryDB.UpdatePatternEffectiveness(ctx, "memory-restart", 0.75)
			Expect(err).ToNot(HaveOccurred())

			results, err := memoryDB.SearchBySemantics(ctx, "", 10) // Search all
			Expect(err).ToNot(HaveOccurred())

			if len(results) > 1 {
				// Verify descending order by effectiveness
				for i := 0; i < len(results)-1; i++ {
					score1 := 0.0
					score2 := 0.0

					if results[i].EffectivenessData != nil {
						score1 = results[i].EffectivenessData.Score
					}
					if results[i+1].EffectivenessData != nil {
						score2 = results[i+1].EffectivenessData.Score
					}

					Expect(score1).To(BeNumerically(">=", score2),
						"Results should be sorted by effectiveness descending")
				}
			}
		})

		It("should respect limit parameter", func() {
			limit := 1
			results, err := memoryDB.SearchBySemantics(ctx, "", limit)
			Expect(err).ToNot(HaveOccurred())

			Expect(len(results)).To(BeNumerically("<=", limit), "Should respect limit parameter")
		})

		It("should handle empty query gracefully", func() {
			results, err := memoryDB.SearchBySemantics(ctx, "", 10)
			Expect(err).ToNot(HaveOccurred(), "Should handle empty query without error")

			// Empty query might return all patterns or none, but should not error
			Expect(len(results)).To(BeNumerically(">=", 0), "Should return non-negative number of results")
		})
	})

	Context("Pattern Deletion Operations", func() {
		var testPattern *vector.ActionPattern

		BeforeEach(func() {
			testPattern = CreateTestActionPattern("delete-test")
			err := memoryDB.StoreActionPattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should delete existing pattern successfully", func() {
			initialCount := memoryDB.GetPatternCount()

			err := memoryDB.DeletePattern(ctx, testPattern.ID)
			Expect(err).ToNot(HaveOccurred(), "Should delete existing pattern without error")

			finalCount := memoryDB.GetPatternCount()
			Expect(finalCount).To(Equal(initialCount-1), "Pattern count should decrease after deletion")

			_, err = memoryDB.GetPattern(testPattern.ID)
			Expect(err).To(HaveOccurred(), "Should not be able to retrieve deleted pattern")
		})

		It("should return error when deleting non-existent pattern", func() {
			err := memoryDB.DeletePattern(ctx, "non-existent")
			Expect(err).To(HaveOccurred(), "Should return error for non-existent pattern")
			Expect(err.Error()).To(ContainSubstring("pattern with ID non-existent not found"))
		})
	})

	Context("Pattern Analytics Operations", func() {
		BeforeEach(func() {
			// Clear any existing patterns to ensure clean state
			memoryDB.Clear()

			// Store patterns with diverse characteristics for analytics
			patterns := []*vector.ActionPattern{
				{
					ID:                "analytics-1",
					ActionType:        "scale",
					AlertSeverity:     "critical",
					Embedding:         []float64{0.1, 0.2},
					EffectivenessData: &vector.EffectivenessData{Score: 0.9},
					CreatedAt:         time.Now().Add(-2 * time.Hour),
				},
				{
					ID:                "analytics-2",
					ActionType:        "restart",
					AlertSeverity:     "warning",
					Embedding:         []float64{0.3, 0.4},
					EffectivenessData: &vector.EffectivenessData{Score: 0.7},
					CreatedAt:         time.Now().Add(-time.Hour),
				},
				{
					ID:                "analytics-3",
					ActionType:        "scale",
					AlertSeverity:     "critical",
					Embedding:         []float64{0.5, 0.6},
					EffectivenessData: &vector.EffectivenessData{Score: 0.8},
					CreatedAt:         time.Now().Add(-3 * time.Hour),
				},
			}

			for _, pattern := range patterns {
				err := memoryDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should generate comprehensive analytics", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred(), "Should generate analytics without error")

			Expect(analytics.TotalPatterns).To(Equal(3), "BR-DATABASE-001-A: Pattern analytics must provide measurable pattern counts for vector database operations")
			Expect(analytics.GeneratedAt).To(BeTemporally(">=", time.Now().Add(-time.Minute)),
				"Should set recent generation timestamp")
		})

		It("should categorize patterns by action type", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(analytics.PatternsByActionType).To(HaveKey("scale"))
			Expect(analytics.PatternsByActionType).To(HaveKey("restart"))
			Expect(analytics.PatternsByActionType["scale"]).To(Equal(2), "Should count scale actions")
			Expect(analytics.PatternsByActionType["restart"]).To(Equal(1), "Should count restart actions")
		})

		It("should categorize patterns by severity", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(analytics.PatternsBySeverity).To(HaveKey("critical"))
			Expect(analytics.PatternsBySeverity).To(HaveKey("warning"))
			Expect(analytics.PatternsBySeverity["critical"]).To(Equal(2), "Should count critical alerts")
			Expect(analytics.PatternsBySeverity["warning"]).To(Equal(1), "Should count warning alerts")
		})

		It("should calculate correct average effectiveness", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			expectedAverage := (0.9 + 0.7 + 0.8) / 3.0
			Expect(analytics.AverageEffectiveness).To(BeNumerically("~", expectedAverage, 0.001),
				"Should calculate correct average effectiveness")
		})

		It("should provide effectiveness distribution", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(analytics.EffectivenessDistribution).ToNot(BeEmpty(), "Should provide effectiveness distribution")

			// Verify buckets make sense
			for bucket, count := range analytics.EffectivenessDistribution {
				Expect(count).To(BeNumerically(">", 0), "Distribution counts should be positive")
				Expect(bucket).To(BeElementOf([]string{"excellent", "very_good", "good", "fair", "poor", "very_poor"}),
					"Should use standard effectiveness buckets")
			}
		})

		It("should identify top performing patterns", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(analytics.TopPerformingPatterns).ToNot(BeEmpty(), "Should identify top performers")

			// Verify sorted by effectiveness
			if len(analytics.TopPerformingPatterns) > 1 {
				first := analytics.TopPerformingPatterns[0]
				second := analytics.TopPerformingPatterns[1]

				Expect(first.EffectivenessData.Score).To(BeNumerically(">=", second.EffectivenessData.Score),
					"Top performers should be sorted by effectiveness")
			}
		})

		It("should identify recent patterns", func() {
			analytics, err := memoryDB.GetPatternAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(analytics.RecentPatterns).ToNot(BeEmpty(), "Should identify recent patterns")

			// Verify sorted by creation time
			if len(analytics.RecentPatterns) > 1 {
				first := analytics.RecentPatterns[0]
				second := analytics.RecentPatterns[1]

				Expect(first.CreatedAt).To(BeTemporally(">=", second.CreatedAt),
					"Recent patterns should be sorted by creation time")
			}
		})
	})

	Context("Database Maintenance Operations", func() {
		BeforeEach(func() {
			// Store some test patterns
			for i := 0; i < 3; i++ {
				pattern := CreateTestActionPattern(fmt.Sprintf("maintenance-test-%d", i))
				err := memoryDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should clear all patterns when requested", func() {
			initialCount := memoryDB.GetPatternCount()
			Expect(initialCount).To(BeNumerically(">", 0), "Should have patterns before clearing")

			memoryDB.Clear()

			finalCount := memoryDB.GetPatternCount()
			Expect(finalCount).To(Equal(0), "Should have no patterns after clearing")
		})

		It("should remain healthy after clearing", func() {
			memoryDB.Clear()

			err := memoryDB.IsHealthy(ctx)
			Expect(err).ToNot(HaveOccurred(), "Should remain healthy after clearing")
		})
	})

	Context("Concurrent Operations", func() {
		It("should handle concurrent pattern storage safely", func() {
			const goroutines = 10
			const patternsPerGoroutine = 5

			done := make(chan bool, goroutines)

			for g := 0; g < goroutines; g++ {
				go func(goroutineID int) {
					defer func() { done <- true }()

					for i := 0; i < patternsPerGoroutine; i++ {
						pattern := CreateTestActionPattern(fmt.Sprintf("concurrent-%d-%d", goroutineID, i))
						err := memoryDB.StoreActionPattern(ctx, pattern)
						Expect(err).ToNot(HaveOccurred())
					}
				}(g)
			}

			// Wait for all goroutines
			for i := 0; i < goroutines; i++ {
				<-done
			}

			finalCount := memoryDB.GetPatternCount()
			expectedCount := goroutines * patternsPerGoroutine
			Expect(finalCount).To(Equal(expectedCount), "Should store all patterns safely in concurrent environment")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUmemoryUvectorUdatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UmemoryUvectorUdatabase Suite")
}
