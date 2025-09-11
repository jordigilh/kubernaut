package testutil

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
)

// StorageAssertions provides standardized assertion helpers for storage tests
type StorageAssertions struct{}

// NewStorageAssertions creates a new storage assertions helper
func NewStorageAssertions() *StorageAssertions {
	return &StorageAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *StorageAssertions) AssertNoError(err error, context string) {
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *StorageAssertions) AssertErrorContains(err error, expectedText string) {
	gomega.Expect(err).To(gomega.HaveOccurred(), "Expected an error containing '%s'", expectedText)
	gomega.Expect(err.Error()).To(gomega.ContainSubstring(expectedText))
}

// AssertValidActionPattern verifies action pattern has required fields
func (a *StorageAssertions) AssertValidActionPattern(pattern *vector.ActionPattern) {
	gomega.Expect(pattern).NotTo(gomega.BeNil())
	gomega.Expect(pattern.ID).NotTo(gomega.BeEmpty())
	gomega.Expect(pattern.ActionType).NotTo(gomega.BeEmpty())
	gomega.Expect(pattern.AlertName).NotTo(gomega.BeEmpty())
	gomega.Expect(pattern.Embedding).NotTo(gomega.BeEmpty())
	gomega.Expect(pattern.CreatedAt).NotTo(gomega.BeZero())
}

// AssertValidActionPatterns verifies a slice of action patterns
func (a *StorageAssertions) AssertValidActionPatterns(patterns []*vector.ActionPattern) {
	gomega.Expect(patterns).NotTo(gomega.BeEmpty())
	for _, pattern := range patterns {
		a.AssertValidActionPattern(pattern)
	}
}

// AssertPatternSimilarity verifies similarity scores are within expected range
func (a *StorageAssertions) AssertPatternSimilarity(results []*vector.SimilarPattern, minSimilarity float64) {
	gomega.Expect(results).NotTo(gomega.BeEmpty())
	for _, result := range results {
		gomega.Expect(result.Similarity).To(gomega.BeNumerically(">=", minSimilarity))
		gomega.Expect(result.Similarity).To(gomega.BeNumerically("<=", 1.0))
		a.AssertValidActionPattern(result.Pattern)
	}
}

// AssertSimilarityResultsOrdered verifies similarity results are in descending order
func (a *StorageAssertions) AssertSimilarityResultsOrdered(results []*vector.SimilarPattern) {
	for i := 1; i < len(results); i++ {
		gomega.Expect(results[i].Similarity).To(gomega.BeNumerically("<=", results[i-1].Similarity),
			"Similarity results should be in descending order")
	}
}

// AssertNormalizedEmbedding verifies embedding is normalized (L2 norm ≈ 1.0)
func (a *StorageAssertions) AssertNormalizedEmbedding(embedding []float64, tolerance float64) {
	gomega.Expect(embedding).NotTo(gomega.BeEmpty())

	var sumSquares float64
	for _, val := range embedding {
		sumSquares += val * val
	}

	gomega.Expect(sumSquares).To(gomega.BeNumerically("~", 1.0, tolerance),
		"Embedding should be normalized with L2 norm ≈ 1.0")
}

// AssertEmbeddingDimension verifies embedding has expected dimension
func (a *StorageAssertions) AssertEmbeddingDimension(embedding []float64, expectedDim int) {
	gomega.Expect(embedding).To(gomega.HaveLen(expectedDim),
		"Embedding should have dimension %d", expectedDim)
}

// AssertEmbeddingsAreDifferent verifies two embeddings are not identical
func (a *StorageAssertions) AssertEmbeddingsAreDifferent(embedding1, embedding2 []float64) {
	gomega.Expect(embedding1).To(gomega.HaveLen(len(embedding2)))

	different := false
	for i := 0; i < len(embedding1); i++ {
		if embedding1[i] != embedding2[i] {
			different = true
			break
		}
	}

	gomega.Expect(different).To(gomega.BeTrue(), "Embeddings should be different")
}

// AssertPatternCount verifies database contains expected number of patterns
func (a *StorageAssertions) AssertPatternCount(db interface{ GetPatternCount() int }, expectedCount int) {
	gomega.Expect(db.GetPatternCount()).To(gomega.Equal(expectedCount))
}

// AssertPatternExists verifies a pattern exists in the database
func (a *StorageAssertions) AssertPatternExists(db interface {
	GetPattern(string) (*vector.ActionPattern, error)
}, patternID string) {
	pattern, err := db.GetPattern(patternID)
	a.AssertNoError(err, "pattern retrieval")
	a.AssertValidActionPattern(pattern)
	gomega.Expect(pattern.ID).To(gomega.Equal(patternID))
}

// AssertPatternNotExists verifies a pattern does not exist in the database
func (a *StorageAssertions) AssertPatternNotExists(db interface {
	GetPattern(string) (*vector.ActionPattern, error)
}, patternID string) {
	_, err := db.GetPattern(patternID)
	gomega.Expect(err).To(gomega.HaveOccurred())
	a.AssertErrorContains(err, "not found")
}

// AssertRecentTimestamp verifies timestamp is within expected time range
func (a *StorageAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	gomega.Expect(timestamp).To(BeTemporally(">=", time.Now().Add(-maxAge)))
	gomega.Expect(timestamp).To(BeTemporally("<=", time.Now().Add(time.Minute))) // Small buffer for test execution time
}

// AssertTimestampOrder verifies timestamps are in expected order
func (a *StorageAssertions) AssertTimestampOrder(earlier, later time.Time) {
	gomega.Expect(later).To(BeTemporally(">=", earlier))
}

// AssertConnectionPoolHealth verifies connection pool is healthy
func (a *StorageAssertions) AssertConnectionPoolHealth(pool interface{ IsHealthy() bool }) {
	gomega.Expect(pool.IsHealthy()).To(gomega.BeTrue())
}

// AssertRetryBehavior verifies retry attempts and success
func (a *StorageAssertions) AssertRetryBehavior(attempts int, maxAttempts int, finalSuccess bool) {
	gomega.Expect(attempts).To(gomega.BeNumerically("<=", maxAttempts))
	if finalSuccess {
		gomega.Expect(attempts).To(gomega.BeNumerically(">", 0))
	}
}

// AssertSearchResultsRelevance verifies search results are relevant to query
func (a *StorageAssertions) AssertSearchResultsRelevance(results []*vector.SimilarPattern, queryContext string) {
	gomega.Expect(results).NotTo(gomega.BeEmpty())

	for _, result := range results {
		a.AssertValidActionPattern(result.Pattern)
		gomega.Expect(result.Similarity).To(gomega.BeNumerically(">", 0))

		// Additional relevance checks can be added based on query context
		if queryContext != "" {
			// Verify pattern is somehow related to query context
			// This is a placeholder for more sophisticated relevance checking
			gomega.Expect(result.Pattern.AlertName).NotTo(gomega.BeEmpty())
		}
	}
}

// AssertEffectivenessData verifies effectiveness data is properly structured
func (a *StorageAssertions) AssertEffectivenessData(data *vector.EffectivenessData) {
	gomega.Expect(data).NotTo(gomega.BeNil())
	gomega.Expect(data.Score).To(gomega.BeNumerically(">=", 0.0))
	gomega.Expect(data.Score).To(gomega.BeNumerically("<=", 1.0))
	gomega.Expect(data.SuccessCount).To(gomega.BeNumerically(">=", 0))
	gomega.Expect(data.FailureCount).To(gomega.BeNumerically(">=", 0))
	gomega.Expect(data.RecurrenceRate).To(gomega.BeNumerically(">=", 0.0))
	gomega.Expect(data.RecurrenceRate).To(gomega.BeNumerically("<=", 1.0))
}
