//go:build unit
// +build unit

package vector

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// BR-VECTOR-SEARCH-QUALITY-001: Comprehensive Vector Search Quality Business Logic Testing
// Business Impact: Validates vector search accuracy and performance for AI-driven pattern matching
// Stakeholder Value: Ensures reliable vector-based similarity search for automated decision making
var _ = Describe("BR-VECTOR-SEARCH-QUALITY-001: Comprehensive Vector Search Quality Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockLogger *logrus.Logger

		// Use REAL business logic components - PYRAMID APPROACH
		memoryVectorDB       *vector.MemoryVectorDatabase
		realEmbeddingService *vector.LocalEmbeddingService

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business vector database and embedding service - PYRAMID APPROACH
		memoryVectorDB = vector.NewMemoryVectorDatabase(mockLogger)
		realEmbeddingService = vector.NewLocalEmbeddingService(384, mockLogger)
	})

	AfterEach(func() {
		cancel()
	})

	// COMPREHENSIVE scenario testing for vector search quality business logic
	DescribeTable("BR-VECTOR-SEARCH-QUALITY-001: Should handle all vector search quality scenarios",
		func(scenarioName string, setupFn func() ([]*vector.ActionPattern, []float64, float64), expectedAccuracy float64, expectedSuccess bool) {
			// Setup test data
			patterns, queryVector, threshold := setupFn()

			// Store patterns in REAL business vector database - PYRAMID APPROACH
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-001: Pattern storage must succeed for %s", scenarioName)
			}

			// Test REAL business vector search quality
			searchResults, err := memoryVectorDB.SearchByVector(ctx, queryVector, 10, threshold)

			// Validate REAL business vector search quality outcomes
			if expectedSuccess {
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-001: Vector search must succeed for %s", scenarioName)
				Expect(searchResults).ToNot(BeEmpty(),
					"BR-VECTOR-SEARCH-QUALITY-001: Must return search results for %s", scenarioName)

				// Validate search quality metrics
				relevantResults := 0
				for _, result := range searchResults {
					// Calculate similarity to determine relevance
					similarity := calculateCosineSimilarity(queryVector, result.Embedding)
					if similarity >= threshold {
						relevantResults++
					}
				}

				accuracy := float64(relevantResults) / float64(len(searchResults))
				Expect(accuracy).To(BeNumerically(">=", expectedAccuracy),
					"BR-VECTOR-SEARCH-QUALITY-001: Search accuracy must meet threshold for %s", scenarioName)

				// Validate result ordering (should be by similarity descending)
				if len(searchResults) > 1 {
					for i := 1; i < len(searchResults); i++ {
						prevSimilarity := calculateCosineSimilarity(queryVector, searchResults[i-1].Embedding)
						currSimilarity := calculateCosineSimilarity(queryVector, searchResults[i].Embedding)
						Expect(prevSimilarity).To(BeNumerically(">=", currSimilarity),
							"BR-VECTOR-SEARCH-QUALITY-001: Results must be ordered by similarity for %s", scenarioName)
					}
				}
			} else {
				Expect(err).To(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-001: Invalid scenarios must fail gracefully for %s", scenarioName)
			}
		},
		Entry("High similarity patterns", "high_similarity", func() ([]*vector.ActionPattern, []float64, float64) {
			return createHighSimilarityTestData()
		}, 0.90, true),
		Entry("Medium similarity patterns", "medium_similarity", func() ([]*vector.ActionPattern, []float64, float64) {
			return createMediumSimilarityTestData()
		}, 0.75, true),
		Entry("Low similarity patterns", "low_similarity", func() ([]*vector.ActionPattern, []float64, float64) {
			return createLowSimilarityTestData()
		}, 0.60, true),
		Entry("Mixed similarity patterns", "mixed_similarity", func() ([]*vector.ActionPattern, []float64, float64) {
			return createMixedSimilarityTestData()
		}, 0.70, true),
		Entry("Identical patterns", "identical_patterns", func() ([]*vector.ActionPattern, []float64, float64) {
			return createIdenticalPatternsTestData()
		}, 1.0, true),
		Entry("Diverse patterns", "diverse_patterns", func() ([]*vector.ActionPattern, []float64, float64) {
			return createDiversePatternsTestData()
		}, 0.65, true),
		Entry("Large dataset patterns", "large_dataset", func() ([]*vector.ActionPattern, []float64, float64) {
			return createLargeDatasetTestData()
		}, 0.80, true),
		Entry("Empty vector query", "empty_vector", func() ([]*vector.ActionPattern, []float64, float64) {
			return createEmptyVectorTestData()
		}, 0.0, false),
		Entry("Invalid threshold", "invalid_threshold", func() ([]*vector.ActionPattern, []float64, float64) {
			return createInvalidThresholdTestData()
		}, 0.0, true), // Should succeed but return no results
	)

	// COMPREHENSIVE vector search accuracy business logic testing
	Context("BR-VECTOR-SEARCH-QUALITY-002: Vector Search Accuracy Business Logic", func() {
		It("should achieve high accuracy with well-defined patterns", func() {
			// Test REAL business logic for vector search accuracy
			patterns := createWellDefinedPatternsTestData()
			queryVector := []float64{0.8, 0.6, 0.4, 0.2, 0.1}
			threshold := 0.7

			// Store patterns in REAL business vector database
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-002: Pattern storage must succeed")
			}

			// Test REAL business vector search accuracy
			searchResults, err := memoryVectorDB.SearchByVector(ctx, queryVector, 5, threshold)

			// Validate REAL business vector search accuracy outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-VECTOR-SEARCH-QUALITY-002: Vector search must succeed")
			Expect(len(searchResults)).To(BeNumerically(">", 0),
				"BR-VECTOR-SEARCH-QUALITY-002: Must return relevant results")

			// Calculate precision and recall
			relevantResults := 0
			totalRelevant := 0

			// Count relevant results in search results
			for _, result := range searchResults {
				similarity := calculateCosineSimilarity(queryVector, result.Embedding)
				if similarity >= threshold {
					relevantResults++
				}
			}

			// Count total relevant patterns in database
			for _, pattern := range patterns {
				similarity := calculateCosineSimilarity(queryVector, pattern.Embedding)
				if similarity >= threshold {
					totalRelevant++
				}
			}

			// Validate precision (relevant results / total results)
			precision := float64(relevantResults) / float64(len(searchResults))
			Expect(precision).To(BeNumerically(">=", 0.8),
				"BR-VECTOR-SEARCH-QUALITY-002: Search precision must be >= 80%")

			// Validate recall (relevant results / total relevant)
			if totalRelevant > 0 {
				recall := float64(relevantResults) / float64(totalRelevant)
				Expect(recall).To(BeNumerically(">=", 0.7),
					"BR-VECTOR-SEARCH-QUALITY-002: Search recall must be >= 70%")
			}
		})

		It("should handle semantic search with business context", func() {
			// Test REAL business logic for semantic search quality
			patterns := createSemanticPatternsTestData()
			query := "high CPU usage deployment scaling"

			// Store patterns in REAL business vector database
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-002: Pattern storage must succeed")
			}

			// Test REAL business semantic search
			searchResults, err := memoryVectorDB.SearchBySemantics(ctx, query, 5)

			// Validate REAL business semantic search outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-VECTOR-SEARCH-QUALITY-002: Semantic search must succeed")
			Expect(len(searchResults)).To(BeNumerically(">", 0),
				"BR-VECTOR-SEARCH-QUALITY-002: Must return semantically relevant results")

			// Validate semantic relevance
			cpuRelatedResults := 0
			scalingRelatedResults := 0

			for _, result := range searchResults {
				if containsSemanticMatch(result.ActionType, []string{"cpu", "memory", "resource"}) {
					cpuRelatedResults++
				}
				if containsSemanticMatch(result.ActionType, []string{"scale", "replicas", "deployment"}) {
					scalingRelatedResults++
				}
			}

			// At least 60% of results should be semantically relevant
			semanticRelevance := float64(cpuRelatedResults+scalingRelatedResults) / float64(len(searchResults))
			Expect(semanticRelevance).To(BeNumerically(">=", 0.6),
				"BR-VECTOR-SEARCH-QUALITY-002: Semantic relevance must be >= 60%")
		})
	})

	// COMPREHENSIVE vector search performance business logic testing
	Context("BR-VECTOR-SEARCH-QUALITY-003: Vector Search Performance Business Logic", func() {
		It("should maintain performance with large datasets", func() {
			// Test REAL business logic for vector search performance
			patterns := createLargeDatasetTestData()
			queryVector := []float64{0.5, 0.5, 0.5, 0.5, 0.5}
			threshold := 0.6

			// Store large dataset in REAL business vector database
			startTime := time.Now()
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-003: Pattern storage must succeed")
			}
			storageTime := time.Since(startTime)

			// Test REAL business vector search performance
			searchStartTime := time.Now()
			searchResults, err := memoryVectorDB.SearchByVector(ctx, queryVector, 10, threshold)
			searchTime := time.Since(searchStartTime)

			// Validate REAL business vector search performance outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-VECTOR-SEARCH-QUALITY-003: Vector search must succeed")
			Expect(len(searchResults)).To(BeNumerically(">", 0),
				"BR-VECTOR-SEARCH-QUALITY-003: Must return results from large dataset")

			// Validate performance metrics
			Expect(storageTime).To(BeNumerically("<", 5*time.Second),
				"BR-VECTOR-SEARCH-QUALITY-003: Storage time must be reasonable")
			Expect(searchTime).To(BeNumerically("<", 1*time.Second),
				"BR-VECTOR-SEARCH-QUALITY-003: Search time must be fast")

			// Validate memory efficiency (results should be limited)
			Expect(len(searchResults)).To(BeNumerically("<=", 10),
				"BR-VECTOR-SEARCH-QUALITY-003: Results must respect limit parameter")
		})

		It("should handle concurrent search operations", func() {
			// Test REAL business logic for concurrent vector search
			patterns := createConcurrentTestData()
			queryVectors := [][]float64{
				{0.9, 0.1, 0.1, 0.1, 0.1},
				{0.1, 0.9, 0.1, 0.1, 0.1},
				{0.1, 0.1, 0.9, 0.1, 0.1},
				{0.1, 0.1, 0.1, 0.9, 0.1},
				{0.1, 0.1, 0.1, 0.1, 0.9},
			}

			// Store patterns in REAL business vector database
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-003: Pattern storage must succeed")
			}

			// Test REAL business concurrent vector search
			results := make([][]*vector.ActionPattern, len(queryVectors))
			errors := make([]error, len(queryVectors))

			// Execute concurrent searches
			done := make(chan bool, len(queryVectors))
			for i, queryVector := range queryVectors {
				go func(idx int, qv []float64) {
					defer func() { done <- true }()
					searchResults, err := memoryVectorDB.SearchByVector(ctx, qv, 5, 0.5)
					results[idx] = searchResults
					errors[idx] = err
				}(i, queryVector)
			}

			// Wait for all searches to complete
			for i := 0; i < len(queryVectors); i++ {
				<-done
			}

			// Validate REAL business concurrent search outcomes
			for i, err := range errors {
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-003: Concurrent search %d must succeed", i)
				Expect(len(results[i])).To(BeNumerically(">", 0),
					"BR-VECTOR-SEARCH-QUALITY-003: Concurrent search %d must return results", i)
			}
		})
	})

	// COMPREHENSIVE vector search edge cases business logic testing
	Context("BR-VECTOR-SEARCH-QUALITY-004: Vector Search Edge Cases Business Logic", func() {
		It("should handle edge cases gracefully", func() {
			// Test REAL business logic for vector search edge cases
			edgeCases := []struct {
				name        string
				queryVector []float64
				threshold   float64
				expectError bool
			}{
				{
					name:        "Empty vector",
					queryVector: []float64{},
					threshold:   0.5,
					expectError: true,
				},
				{
					name:        "Single dimension vector",
					queryVector: []float64{1.0},
					threshold:   0.5,
					expectError: false,
				},
				{
					name:        "Zero vector",
					queryVector: []float64{0.0, 0.0, 0.0, 0.0, 0.0},
					threshold:   0.5,
					expectError: false,
				},
				{
					name:        "Negative threshold",
					queryVector: []float64{0.5, 0.5, 0.5, 0.5, 0.5},
					threshold:   -0.1,
					expectError: false, // Should work but return no results
				},
				{
					name:        "Very high threshold",
					queryVector: []float64{0.5, 0.5, 0.5, 0.5, 0.5},
					threshold:   1.1,
					expectError: false, // Should work but return no results
				},
			}

			// Store some patterns for edge case testing
			patterns := createBasicPatternsTestData()
			for _, pattern := range patterns {
				err := memoryVectorDB.StoreActionPattern(ctx, pattern)
				Expect(err).ToNot(HaveOccurred(),
					"BR-VECTOR-SEARCH-QUALITY-004: Pattern storage must succeed")
			}

			// Test REAL business edge case handling
			for _, edgeCase := range edgeCases {
				By(edgeCase.name)

				searchResults, err := memoryVectorDB.SearchByVector(ctx, edgeCase.queryVector, 5, edgeCase.threshold)

				// Validate REAL business edge case handling outcomes
				if edgeCase.expectError {
					Expect(err).To(HaveOccurred(),
						"BR-VECTOR-SEARCH-QUALITY-004: Edge case '%s' must produce error", edgeCase.name)
				} else {
					Expect(err).ToNot(HaveOccurred(),
						"BR-VECTOR-SEARCH-QUALITY-004: Edge case '%s' must handle gracefully", edgeCase.name)

					// Results may be empty for some edge cases, which is acceptable
					Expect(searchResults).ToNot(BeNil(),
						"BR-VECTOR-SEARCH-QUALITY-004: Edge case '%s' must return non-nil results", edgeCase.name)
				}
			}
		})
	})
})

// Helper functions to create test data for various vector search quality scenarios

func createHighSimilarityTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "high-sim-1",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.9, 0.8, 0.7, 0.6, 0.5},
		},
		{
			ID:         "high-sim-2",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.85, 0.75, 0.65, 0.55, 0.45},
		},
		{
			ID:         "high-sim-3",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.95, 0.85, 0.75, 0.65, 0.55},
		},
	}
	queryVector := []float64{0.9, 0.8, 0.7, 0.6, 0.5}
	threshold := 0.8
	return patterns, queryVector, threshold
}

func createMediumSimilarityTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "med-sim-1",
			ActionType: "restart_pod",
			Embedding:  []float64{0.6, 0.5, 0.4, 0.3, 0.2},
		},
		{
			ID:         "med-sim-2",
			ActionType: "update_configmap",
			Embedding:  []float64{0.7, 0.6, 0.5, 0.4, 0.3},
		},
		{
			ID:         "med-sim-3",
			ActionType: "check_health",
			Embedding:  []float64{0.5, 0.4, 0.3, 0.2, 0.1},
		},
	}
	queryVector := []float64{0.6, 0.5, 0.4, 0.3, 0.2}
	threshold := 0.6
	return patterns, queryVector, threshold
}

func createLowSimilarityTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "low-sim-1",
			ActionType: "delete_resource",
			Embedding:  []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
		{
			ID:         "low-sim-2",
			ActionType: "create_backup",
			Embedding:  []float64{0.2, 0.3, 0.4, 0.5, 0.6},
		},
		{
			ID:         "low-sim-3",
			ActionType: "network_policy",
			Embedding:  []float64{0.3, 0.4, 0.5, 0.6, 0.7},
		},
	}
	queryVector := []float64{0.8, 0.7, 0.6, 0.5, 0.4}
	threshold := 0.3
	return patterns, queryVector, threshold
}

func createMixedSimilarityTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "mixed-1",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.9, 0.8, 0.7, 0.6, 0.5}, // High similarity
		},
		{
			ID:         "mixed-2",
			ActionType: "restart_pod",
			Embedding:  []float64{0.6, 0.5, 0.4, 0.3, 0.2}, // Medium similarity
		},
		{
			ID:         "mixed-3",
			ActionType: "delete_resource",
			Embedding:  []float64{0.1, 0.2, 0.3, 0.4, 0.5}, // Low similarity
		},
		{
			ID:         "mixed-4",
			ActionType: "update_deployment",
			Embedding:  []float64{0.85, 0.75, 0.65, 0.55, 0.45}, // High similarity
		},
	}
	queryVector := []float64{0.9, 0.8, 0.7, 0.6, 0.5}
	threshold := 0.5
	return patterns, queryVector, threshold
}

func createIdenticalPatternsTestData() ([]*vector.ActionPattern, []float64, float64) {
	embedding := []float64{0.8, 0.6, 0.4, 0.2, 0.1}
	patterns := []*vector.ActionPattern{
		{
			ID:         "identical-1",
			ActionType: "scale_deployment",
			Embedding:  embedding,
		},
		{
			ID:         "identical-2",
			ActionType: "scale_deployment",
			Embedding:  embedding,
		},
		{
			ID:         "identical-3",
			ActionType: "scale_deployment",
			Embedding:  embedding,
		},
	}
	queryVector := embedding
	threshold := 0.9
	return patterns, queryVector, threshold
}

func createDiversePatternsTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "diverse-1",
			ActionType: "cpu_scaling",
			Embedding:  []float64{1.0, 0.0, 0.0, 0.0, 0.0},
		},
		{
			ID:         "diverse-2",
			ActionType: "memory_optimization",
			Embedding:  []float64{0.0, 1.0, 0.0, 0.0, 0.0},
		},
		{
			ID:         "diverse-3",
			ActionType: "network_troubleshooting",
			Embedding:  []float64{0.0, 0.0, 1.0, 0.0, 0.0},
		},
		{
			ID:         "diverse-4",
			ActionType: "storage_management",
			Embedding:  []float64{0.0, 0.0, 0.0, 1.0, 0.0},
		},
		{
			ID:         "diverse-5",
			ActionType: "security_policy",
			Embedding:  []float64{0.0, 0.0, 0.0, 0.0, 1.0},
		},
	}
	queryVector := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
	threshold := 0.1
	return patterns, queryVector, threshold
}

func createLargeDatasetTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := make([]*vector.ActionPattern, 100)
	for i := 0; i < 100; i++ {
		patterns[i] = &vector.ActionPattern{
			ID:         fmt.Sprintf("large-dataset-%d", i),
			ActionType: fmt.Sprintf("action_type_%d", i%10),
			Embedding:  generateRandomEmbedding(5, float64(i)/100.0),
		}
	}
	queryVector := []float64{0.5, 0.5, 0.5, 0.5, 0.5}
	threshold := 0.3
	return patterns, queryVector, threshold
}

func createEmptyVectorTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "empty-test-1",
			ActionType: "test_action",
			Embedding:  []float64{0.5, 0.5, 0.5, 0.5, 0.5},
		},
	}
	queryVector := []float64{} // Empty vector
	threshold := 0.5
	return patterns, queryVector, threshold
}

func createInvalidThresholdTestData() ([]*vector.ActionPattern, []float64, float64) {
	patterns := []*vector.ActionPattern{
		{
			ID:         "invalid-threshold-1",
			ActionType: "test_action",
			Embedding:  []float64{0.5, 0.5, 0.5, 0.5, 0.5},
		},
	}
	queryVector := []float64{0.5, 0.5, 0.5, 0.5, 0.5}
	threshold := 2.0 // Invalid threshold > 1.0
	return patterns, queryVector, threshold
}

func createWellDefinedPatternsTestData() []*vector.ActionPattern {
	return []*vector.ActionPattern{
		{
			ID:         "well-defined-1",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.9, 0.8, 0.7, 0.6, 0.5},
		},
		{
			ID:         "well-defined-2",
			ActionType: "scale_deployment",
			Embedding:  []float64{0.85, 0.75, 0.65, 0.55, 0.45},
		},
		{
			ID:         "well-defined-3",
			ActionType: "restart_pod",
			Embedding:  []float64{0.3, 0.4, 0.5, 0.6, 0.7},
		},
		{
			ID:         "well-defined-4",
			ActionType: "update_configmap",
			Embedding:  []float64{0.2, 0.3, 0.4, 0.5, 0.6},
		},
		{
			ID:         "well-defined-5",
			ActionType: "check_health",
			Embedding:  []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
	}
}

func createSemanticPatternsTestData() []*vector.ActionPattern {
	return []*vector.ActionPattern{
		{
			ID:         "semantic-1",
			ActionType: "cpu_scale_deployment",
			Embedding:  []float64{0.9, 0.1, 0.1, 0.1, 0.1},
		},
		{
			ID:         "semantic-2",
			ActionType: "memory_scale_deployment",
			Embedding:  []float64{0.8, 0.2, 0.1, 0.1, 0.1},
		},
		{
			ID:         "semantic-3",
			ActionType: "network_troubleshoot",
			Embedding:  []float64{0.1, 0.9, 0.1, 0.1, 0.1},
		},
		{
			ID:         "semantic-4",
			ActionType: "storage_cleanup",
			Embedding:  []float64{0.1, 0.1, 0.9, 0.1, 0.1},
		},
		{
			ID:         "semantic-5",
			ActionType: "replica_scaling",
			Embedding:  []float64{0.7, 0.1, 0.1, 0.2, 0.1},
		},
	}
}

func createConcurrentTestData() []*vector.ActionPattern {
	return []*vector.ActionPattern{
		{
			ID:         "concurrent-1",
			ActionType: "action_1",
			Embedding:  []float64{1.0, 0.0, 0.0, 0.0, 0.0},
		},
		{
			ID:         "concurrent-2",
			ActionType: "action_2",
			Embedding:  []float64{0.0, 1.0, 0.0, 0.0, 0.0},
		},
		{
			ID:         "concurrent-3",
			ActionType: "action_3",
			Embedding:  []float64{0.0, 0.0, 1.0, 0.0, 0.0},
		},
		{
			ID:         "concurrent-4",
			ActionType: "action_4",
			Embedding:  []float64{0.0, 0.0, 0.0, 1.0, 0.0},
		},
		{
			ID:         "concurrent-5",
			ActionType: "action_5",
			Embedding:  []float64{0.0, 0.0, 0.0, 0.0, 1.0},
		},
	}
}

func createBasicPatternsTestData() []*vector.ActionPattern {
	return []*vector.ActionPattern{
		{
			ID:         "basic-1",
			ActionType: "basic_action",
			Embedding:  []float64{0.5, 0.5, 0.5, 0.5, 0.5},
		},
		{
			ID:         "basic-2",
			ActionType: "basic_action_2",
			Embedding:  []float64{0.6, 0.4, 0.3, 0.2, 0.1},
		},
	}
}

// Helper functions for vector search quality calculations

func calculateCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func containsSemanticMatch(text string, keywords []string) bool {
	lowerText := strings.ToLower(text)
	for _, keyword := range keywords {
		if strings.Contains(lowerText, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func generateRandomEmbedding(dimensions int, seed float64) []float64 {
	embedding := make([]float64, dimensions)
	for i := 0; i < dimensions; i++ {
		embedding[i] = seed + float64(i)*0.1
	}
	return embedding
}
