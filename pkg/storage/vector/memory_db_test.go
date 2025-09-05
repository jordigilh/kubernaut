package vector_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

var _ = Describe("MemoryVectorDatabase", func() {
	var (
		db     *vector.MemoryVectorDatabase
		logger *logrus.Logger
		ctx    context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		db = vector.NewMemoryVectorDatabase(logger)
		ctx = context.Background()
	})

	Describe("NewMemoryVectorDatabase", func() {
		It("should create a new memory vector database", func() {
			db := vector.NewMemoryVectorDatabase(logger)
			Expect(db).NotTo(BeNil())
			Expect(db.GetPatternCount()).To(Equal(0))
		})
	})

	Describe("StoreActionPattern", func() {
		Context("when storing a valid pattern", func() {
			It("should store the pattern successfully", func() {
				pattern := createTestPattern("test-1", "scale_deployment", "HighMemoryUsage")

				err := db.StoreActionPattern(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())
				Expect(db.GetPatternCount()).To(Equal(1))
			})

			It("should update the pattern timestamps", func() {
				pattern := createTestPattern("test-2", "restart_pod", "PodCrashing")
				originalCreatedAt := pattern.CreatedAt

				err := db.StoreActionPattern(ctx, pattern)

				Expect(err).NotTo(HaveOccurred())

				// Get the stored pattern back
				storedPattern, err := db.GetPattern("test-2")
				Expect(err).NotTo(HaveOccurred())
				Expect(storedPattern.CreatedAt).To(Equal(originalCreatedAt))
				Expect(storedPattern.UpdatedAt).To(BeTemporally(">=", originalCreatedAt))
			})
		})

		Context("when pattern ID is empty", func() {
			It("should return an error", func() {
				pattern := createTestPattern("", "scale_deployment", "HighMemoryUsage")

				err := db.StoreActionPattern(ctx, pattern)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pattern ID cannot be empty"))
			})
		})

		Context("when pattern embedding is empty", func() {
			It("should return an error", func() {
				pattern := createTestPattern("test-3", "scale_deployment", "HighMemoryUsage")
				pattern.Embedding = []float64{} // Empty embedding

				err := db.StoreActionPattern(ctx, pattern)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pattern embedding cannot be empty"))
			})
		})
	})

	Describe("FindSimilarPatterns", func() {
		BeforeEach(func() {
			// Store test patterns with different embeddings
			patterns := []*vector.ActionPattern{
				createTestPatternWithEmbedding("pattern-1", "scale_deployment", "HighMemoryUsage", []float64{1.0, 0.5, 0.0}, 0.9),
				createTestPatternWithEmbedding("pattern-2", "scale_deployment", "HighMemoryUsage", []float64{0.9, 0.4, 0.1}, 0.8),
				createTestPatternWithEmbedding("pattern-3", "restart_pod", "PodCrashing", []float64{0.1, 0.9, 0.5}, 0.7),
				createTestPatternWithEmbedding("pattern-4", "scale_deployment", "HighCpuUsage", []float64{0.8, 0.6, 0.2}, 0.85),
			}

			for _, pattern := range patterns {
				err := db.StoreActionPattern(ctx, pattern)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("when finding similar patterns with valid query", func() {
			It("should return similar patterns ordered by similarity", func() {
				queryPattern := createTestPatternWithEmbedding("query", "scale_deployment", "HighMemoryUsage", []float64{0.95, 0.45, 0.05}, 0.0)

				similarPatterns, err := db.FindSimilarPatterns(ctx, queryPattern, 3, 0.5)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(similarPatterns)).To(BeNumerically(">=", 2)) // Should find at least 2 similar patterns

				// Check that results are ordered by similarity (descending)
				for i := 1; i < len(similarPatterns); i++ {
					Expect(similarPatterns[i-1].Similarity).To(BeNumerically(">=", similarPatterns[i].Similarity))
				}

				// Check that ranks are assigned correctly
				for i, pattern := range similarPatterns {
					Expect(pattern.Rank).To(Equal(i + 1))
				}
			})

			It("should respect the similarity threshold", func() {
				queryPattern := createTestPatternWithEmbedding("query", "restart_pod", "PodCrashing", []float64{0.0, 1.0, 0.0}, 0.0)

				// Use a high threshold that should filter out most patterns
				similarPatterns, err := db.FindSimilarPatterns(ctx, queryPattern, 10, 0.9)

				Expect(err).NotTo(HaveOccurred())
				for _, pattern := range similarPatterns {
					Expect(pattern.Similarity).To(BeNumerically(">=", 0.9))
				}
			})

			It("should respect the limit parameter", func() {
				queryPattern := createTestPatternWithEmbedding("query", "scale_deployment", "HighMemoryUsage", []float64{1.0, 0.5, 0.0}, 0.0)

				similarPatterns, err := db.FindSimilarPatterns(ctx, queryPattern, 2, 0.0)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(similarPatterns)).To(BeNumerically("<=", 2))
			})

			It("should exclude the same pattern from results", func() {
				// Store the query pattern first
				queryPattern := createTestPatternWithEmbedding("same-pattern", "scale_deployment", "HighMemoryUsage", []float64{1.0, 0.5, 0.0}, 0.9)
				err := db.StoreActionPattern(ctx, queryPattern)
				Expect(err).NotTo(HaveOccurred())

				similarPatterns, err := db.FindSimilarPatterns(ctx, queryPattern, 10, 0.0)

				Expect(err).NotTo(HaveOccurred())
				// Should not include the same pattern in results
				for _, pattern := range similarPatterns {
					Expect(pattern.Pattern.ID).NotTo(Equal("same-pattern"))
				}
			})
		})

		Context("when query pattern has empty embedding", func() {
			It("should return an error", func() {
				queryPattern := createTestPattern("query", "scale_deployment", "HighMemoryUsage")
				queryPattern.Embedding = []float64{} // Empty embedding

				_, err := db.FindSimilarPatterns(ctx, queryPattern, 5, 0.5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("query pattern embedding cannot be empty"))
			})
		})
	})

	Describe("UpdatePatternEffectiveness", func() {
		BeforeEach(func() {
			pattern := createTestPattern("update-test", "scale_deployment", "HighMemoryUsage")
			err := db.StoreActionPattern(ctx, pattern)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when updating existing pattern", func() {
			It("should update the effectiveness score", func() {
				err := db.UpdatePatternEffectiveness(ctx, "update-test", 0.95)

				Expect(err).NotTo(HaveOccurred())

				// Verify the update
				pattern, err := db.GetPattern("update-test")
				Expect(err).NotTo(HaveOccurred())
				Expect(pattern.EffectivenessData.Score).To(Equal(0.95))
				Expect(pattern.EffectivenessData.LastAssessed).To(BeTemporally("~", time.Now(), time.Second))
			})

			It("should create effectiveness data if it doesn't exist", func() {
				// Store a pattern without effectiveness data
				pattern := createTestPattern("no-effectiveness", "restart_pod", "PodCrashing")
				pattern.EffectivenessData = nil
				err := db.StoreActionPattern(ctx, pattern)
				Expect(err).NotTo(HaveOccurred())

				err = db.UpdatePatternEffectiveness(ctx, "no-effectiveness", 0.75)

				Expect(err).NotTo(HaveOccurred())

				// Verify effectiveness data was created
				updatedPattern, err := db.GetPattern("no-effectiveness")
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedPattern.EffectivenessData).NotTo(BeNil())
				Expect(updatedPattern.EffectivenessData.Score).To(Equal(0.75))
			})
		})

		Context("when pattern does not exist", func() {
			It("should return an error", func() {
				err := db.UpdatePatternEffectiveness(ctx, "non-existent", 0.8)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pattern with ID non-existent not found"))
			})
		})
	})

	Describe("SearchBySemantics", func() {
		BeforeEach(func() {
			// Store diverse patterns for semantic search
			patterns := []*vector.ActionPattern{
				createTestPattern("memory-1", "scale_deployment", "HighMemoryUsage"),
				createTestPattern("memory-2", "increase_resources", "MemoryPressure"),
				createTestPattern("cpu-1", "scale_deployment", "HighCpuUsage"),
				createTestPattern("pod-1", "restart_pod", "PodCrashing"),
				createTestPattern("network-1", "restart_network", "NetworkIssue"),
			}

			for _, pattern := range patterns {
				pattern.ResourceType = "deployment"
				if pattern.ActionType == "restart_pod" {
					pattern.ResourceType = "pod"
				}
				err := db.StoreActionPattern(ctx, pattern)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("when searching for memory-related patterns", func() {
			It("should find patterns related to memory", func() {
				results, err := db.SearchBySemantics(ctx, "memory", 10)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(results)).To(BeNumerically(">=", 2))

				// Check that results contain memory-related patterns
				foundMemoryPattern := false
				for _, pattern := range results {
					if pattern.AlertName == "HighMemoryUsage" || pattern.AlertName == "MemoryPressure" {
						foundMemoryPattern = true
						break
					}
				}
				Expect(foundMemoryPattern).To(BeTrue())
			})
		})

		Context("when searching for deployment actions", func() {
			It("should find deployment-related patterns", func() {
				results, err := db.SearchBySemantics(ctx, "deployment", 10)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(results)).To(BeNumerically(">=", 1))

				// Results should be sorted by effectiveness
				if len(results) > 1 {
					for i := 1; i < len(results); i++ {
						prevScore := 0.0
						currScore := 0.0
						if results[i-1].EffectivenessData != nil {
							prevScore = results[i-1].EffectivenessData.Score
						}
						if results[i].EffectivenessData != nil {
							currScore = results[i].EffectivenessData.Score
						}
						Expect(prevScore).To(BeNumerically(">=", currScore))
					}
				}
			})
		})

		Context("when searching with limit", func() {
			It("should respect the limit parameter", func() {
				results, err := db.SearchBySemantics(ctx, "scale", 2)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(results)).To(BeNumerically("<=", 2))
			})
		})

		Context("when no patterns match", func() {
			It("should return empty results", func() {
				results, err := db.SearchBySemantics(ctx, "nonexistent", 10)

				Expect(err).NotTo(HaveOccurred())
				Expect(results).To(BeEmpty())
			})
		})
	})

	Describe("DeletePattern", func() {
		BeforeEach(func() {
			pattern := createTestPattern("delete-test", "scale_deployment", "HighMemoryUsage")
			err := db.StoreActionPattern(ctx, pattern)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when deleting existing pattern", func() {
			It("should remove the pattern", func() {
				err := db.DeletePattern(ctx, "delete-test")

				Expect(err).NotTo(HaveOccurred())
				Expect(db.GetPatternCount()).To(Equal(0))

				// Verify pattern is deleted
				_, err = db.GetPattern("delete-test")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when pattern does not exist", func() {
			It("should return an error", func() {
				err := db.DeletePattern(ctx, "non-existent")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pattern with ID non-existent not found"))
			})
		})
	})

	Describe("GetPatternAnalytics", func() {
		BeforeEach(func() {
			// Store diverse patterns for analytics
			patterns := []*vector.ActionPattern{
				createTestPatternWithEmbedding("analytics-1", "scale_deployment", "critical", []float64{1.0, 0.0, 0.0}, 0.9),
				createTestPatternWithEmbedding("analytics-2", "scale_deployment", "warning", []float64{0.0, 1.0, 0.0}, 0.8),
				createTestPatternWithEmbedding("analytics-3", "restart_pod", "critical", []float64{0.0, 0.0, 1.0}, 0.7),
				createTestPatternWithEmbedding("analytics-4", "increase_resources", "warning", []float64{0.5, 0.5, 0.0}, 0.6),
				createTestPatternWithEmbedding("analytics-5", "scale_deployment", "critical", []float64{0.3, 0.3, 0.4}, 0.95),
			}

			for _, pattern := range patterns {
				pattern.AlertSeverity = pattern.AlertName // Use alert name as severity for testing
				err := db.StoreActionPattern(ctx, pattern)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("when generating analytics", func() {
			It("should return comprehensive analytics", func() {
				analytics, err := db.GetPatternAnalytics(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(analytics).NotTo(BeNil())
				Expect(analytics.TotalPatterns).To(Equal(5))
				Expect(analytics.PatternsByActionType).To(HaveKey("scale_deployment"))
				Expect(analytics.PatternsByActionType).To(HaveKey("restart_pod"))
				Expect(analytics.PatternsByActionType).To(HaveKey("increase_resources"))
				Expect(analytics.PatternsBySeverity).To(HaveKey("critical"))
				Expect(analytics.PatternsBySeverity).To(HaveKey("warning"))
			})

			It("should calculate correct averages", func() {
				analytics, err := db.GetPatternAnalytics(ctx)

				Expect(err).NotTo(HaveOccurred())
				// Average of 0.9, 0.8, 0.7, 0.6, 0.95 = 3.95/5 = 0.79
				Expect(analytics.AverageEffectiveness).To(BeNumerically("~", 0.79, 0.01))
			})

			It("should categorize effectiveness properly", func() {
				analytics, err := db.GetPatternAnalytics(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(analytics.EffectivenessDistribution).To(HaveKey("excellent")) // 0.95
				Expect(analytics.EffectivenessDistribution).To(HaveKey("very_good")) // 0.9, 0.8
				Expect(analytics.EffectivenessDistribution).To(HaveKey("good"))      // 0.7
				Expect(analytics.EffectivenessDistribution).To(HaveKey("fair"))      // 0.6
			})

			It("should return top performing patterns", func() {
				analytics, err := db.GetPatternAnalytics(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(analytics.TopPerformingPatterns)).To(BeNumerically(">=", 1))

				// First pattern should be the highest scoring
				if len(analytics.TopPerformingPatterns) > 0 {
					topPattern := analytics.TopPerformingPatterns[0]
					Expect(topPattern.EffectivenessData.Score).To(Equal(0.95))
				}
			})

			It("should return recent patterns", func() {
				analytics, err := db.GetPatternAnalytics(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(len(analytics.RecentPatterns)).To(BeNumerically(">=", 1))

				// Check that recent patterns are ordered by creation time
				if len(analytics.RecentPatterns) > 1 {
					for i := 1; i < len(analytics.RecentPatterns); i++ {
						prev := analytics.RecentPatterns[i-1].CreatedAt
						curr := analytics.RecentPatterns[i].CreatedAt
						Expect(prev.After(curr) || prev.Equal(curr)).To(BeTrue())
					}
				}
			})
		})
	})

	Describe("IsHealthy", func() {
		It("should report healthy status", func() {
			err := db.IsHealthy(ctx)

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Clear", func() {
		BeforeEach(func() {
			// Store some patterns
			patterns := []*vector.ActionPattern{
				createTestPattern("clear-1", "scale_deployment", "HighMemoryUsage"),
				createTestPattern("clear-2", "restart_pod", "PodCrashing"),
			}

			for _, pattern := range patterns {
				err := db.StoreActionPattern(ctx, pattern)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should remove all patterns", func() {
			Expect(db.GetPatternCount()).To(Equal(2))

			db.Clear()

			Expect(db.GetPatternCount()).To(Equal(0))
		})
	})

	Describe("Concurrent Access", func() {
		It("should handle concurrent reads and writes safely", func() {
			// This tests thread safety
			done := make(chan bool, 3)

			// Concurrent writes
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					pattern := createTestPattern(fmt.Sprintf("concurrent-write-%d", i), "scale_deployment", "HighMemoryUsage")
					err := db.StoreActionPattern(ctx, pattern)
					Expect(err).NotTo(HaveOccurred())
				}
				done <- true
			}()

			// Concurrent reads
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					_ = db.GetPatternCount()
					_, _ = db.GetPatternAnalytics(ctx)
				}
				done <- true
			}()

			// Concurrent searches
			go func() {
				defer GinkgoRecover()
				queryPattern := createTestPattern("concurrent-query", "scale_deployment", "HighMemoryUsage")
				for i := 0; i < 10; i++ {
					_, _ = db.FindSimilarPatterns(ctx, queryPattern, 5, 0.3)
				}
				done <- true
			}()

			// Wait for all goroutines
			<-done
			<-done
			<-done

			// Should have some patterns stored
			Expect(db.GetPatternCount()).To(BeNumerically(">", 0))
		})
	})
})

// Helper functions

func createTestPattern(id, actionType, alertName string) *vector.ActionPattern {
	return &vector.ActionPattern{
		ID:            id,
		ActionType:    actionType,
		AlertName:     alertName,
		AlertSeverity: "warning",
		Namespace:     "test-namespace",
		ResourceType:  "deployment",
		ResourceName:  "test-resource",
		ActionParameters: map[string]interface{}{
			"replicas": 3,
			"reason":   "testing",
		},
		ContextLabels: map[string]string{
			"app":     "test-app",
			"version": "1.0.0",
		},
		PreConditions: map[string]interface{}{
			"alert_severity": "warning",
		},
		PostConditions: map[string]interface{}{
			"execution_status": "completed",
		},
		EffectivenessData: &vector.EffectivenessData{
			Score:                0.8,
			SuccessCount:         1,
			FailureCount:         0,
			AverageExecutionTime: 30 * time.Second,
			SideEffectsCount:     0,
			RecurrenceRate:       0.0,
			ContextualFactors: map[string]float64{
				"hour_of_day": 0.5,
				"day_of_week": 0.3,
			},
			LastAssessed: time.Now(),
		},
		Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5}, // Simple test embedding
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
		Metadata: map[string]interface{}{
			"test": true,
		},
	}
}

func createTestPatternWithEmbedding(id, actionType, alertName string, embedding []float64, effectiveness float64) *vector.ActionPattern {
	pattern := createTestPattern(id, actionType, alertName)
	pattern.Embedding = embedding
	pattern.EffectivenessData.Score = effectiveness
	return pattern
}
