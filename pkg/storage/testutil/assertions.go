<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

>>>>>>> crd_implementation
package testutil

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// StorageAssertions provides standardized assertion helpers for storage tests
type StorageAssertions struct{}

// NewStorageAssertions creates a new storage assertions helper
func NewStorageAssertions() *StorageAssertions {
	return &StorageAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *StorageAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *StorageAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertValidActionPattern verifies action pattern has required fields
func (a *StorageAssertions) AssertValidActionPattern(pattern *vector.ActionPattern) {
	Expect(pattern).NotTo(BeNil())
	Expect(pattern.ID).NotTo(BeEmpty())
	Expect(pattern.ActionType).NotTo(BeEmpty())
	Expect(pattern.AlertName).NotTo(BeEmpty())
	Expect(pattern.Embedding).NotTo(BeEmpty())
	Expect(pattern.CreatedAt).NotTo(BeZero())
}

// AssertValidActionPatterns verifies a slice of action patterns
func (a *StorageAssertions) AssertValidActionPatterns(patterns []*vector.ActionPattern) {
	Expect(patterns).NotTo(BeEmpty())
	for _, pattern := range patterns {
		a.AssertValidActionPattern(pattern)
	}
}

// AssertPatternSimilarity verifies similarity scores are within expected range
func (a *StorageAssertions) AssertPatternSimilarity(results []*vector.SimilarPattern, minSimilarity float64) {
	Expect(results).NotTo(BeEmpty())
	for _, result := range results {
		Expect(result.Similarity).To(BeNumerically(">=", minSimilarity))
		Expect(result.Similarity).To(BeNumerically("<=", 1.0))
		a.AssertValidActionPattern(result.Pattern)
	}
}

// AssertSimilarityResultsOrdered verifies similarity results are in descending order
func (a *StorageAssertions) AssertSimilarityResultsOrdered(results []*vector.SimilarPattern) {
	for i := 1; i < len(results); i++ {
		Expect(results[i].Similarity).To(BeNumerically("<=", results[i-1].Similarity),
			"Similarity results should be in descending order")
	}
}

// AssertNormalizedEmbedding verifies embedding is normalized (L2 norm ≈ 1.0)
func (a *StorageAssertions) AssertNormalizedEmbedding(embedding []float64, tolerance float64) {
	Expect(embedding).NotTo(BeEmpty())

	var sumSquares float64
	for _, val := range embedding {
		sumSquares += val * val
	}

	Expect(sumSquares).To(BeNumerically("~", 1.0, tolerance),
		"Embedding should be normalized with L2 norm ≈ 1.0")
}

// AssertEmbeddingDimension verifies embedding has expected dimension
func (a *StorageAssertions) AssertEmbeddingDimension(embedding []float64, expectedDim int) {
	Expect(embedding).To(HaveLen(expectedDim),
		"Embedding should have dimension %d", expectedDim)
}

// AssertEmbeddingsAreDifferent verifies two embeddings are not identical
func (a *StorageAssertions) AssertEmbeddingsAreDifferent(embedding1, embedding2 []float64) {
	Expect(embedding1).To(HaveLen(len(embedding2)))

	different := false
	for i := 0; i < len(embedding1); i++ {
		if embedding1[i] != embedding2[i] {
			different = true
			break
		}
	}

	Expect(different).To(BeTrue(), "Embeddings should be different")
}

// AssertPatternCount verifies database contains expected number of patterns
func (a *StorageAssertions) AssertPatternCount(db interface{ GetPatternCount() int }, expectedCount int) {
	Expect(db.GetPatternCount()).To(Equal(expectedCount))
}

// AssertPatternExists verifies a pattern exists in the database
func (a *StorageAssertions) AssertPatternExists(db interface {
	GetPattern(string) (*vector.ActionPattern, error)
}, patternID string) {
	pattern, err := db.GetPattern(patternID)
	a.AssertNoError(err, "pattern retrieval")
	a.AssertValidActionPattern(pattern)
	Expect(pattern.ID).To(Equal(patternID))
}

// AssertPatternNotExists verifies a pattern does not exist in the database
func (a *StorageAssertions) AssertPatternNotExists(db interface {
	GetPattern(string) (*vector.ActionPattern, error)
}, patternID string) {
	_, err := db.GetPattern(patternID)
	Expect(err).To(HaveOccurred())
	a.AssertErrorContains(err, "not found")
}

// AssertRecentTimestamp verifies timestamp is within expected time range
func (a *StorageAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	Expect(timestamp).To(BeTemporally(">=", time.Now().Add(-maxAge)))
	Expect(timestamp).To(BeTemporally("<=", time.Now().Add(time.Minute))) // Small buffer for test execution time
}

// AssertTimestampOrder verifies timestamps are in expected order
func (a *StorageAssertions) AssertTimestampOrder(earlier, later time.Time) {
	Expect(later).To(BeTemporally(">=", earlier))
}

// AssertConnectionPoolHealth verifies connection pool is healthy
func (a *StorageAssertions) AssertConnectionPoolHealth(pool interface{ IsHealthy() bool }) {
	Expect(pool.IsHealthy()).To(BeTrue())
}

// AssertRetryBehavior verifies retry attempts and success
func (a *StorageAssertions) AssertRetryBehavior(attempts int, maxAttempts int, finalSuccess bool) {
	Expect(attempts).To(BeNumerically("<=", maxAttempts))
	if finalSuccess {
		Expect(attempts).To(BeNumerically(">", 0))
	}
}

// AssertSearchResultsRelevance verifies search results are relevant to query
func (a *StorageAssertions) AssertSearchResultsRelevance(results []*vector.SimilarPattern, queryContext string) {
	Expect(results).NotTo(BeEmpty())

	for _, result := range results {
		a.AssertValidActionPattern(result.Pattern)
		Expect(result.Similarity).To(BeNumerically(">", 0))

		// Additional relevance checks can be added based on query context
		if queryContext != "" {
			// Verify pattern is somehow related to query context
			// This is a placeholder for more sophisticated relevance checking
			Expect(result.Pattern.AlertName).NotTo(BeEmpty())
		}
	}
}

// AssertEffectivenessData verifies effectiveness data is properly structured
func (a *StorageAssertions) AssertEffectivenessData(data *vector.EffectivenessData) {
	Expect(data).NotTo(BeNil())
	Expect(data.Score).To(BeNumerically(">=", 0.0))
	Expect(data.Score).To(BeNumerically("<=", 1.0))
	Expect(data.SuccessCount).To(BeNumerically(">=", 0))
	Expect(data.FailureCount).To(BeNumerically(">=", 0))
	Expect(data.RecurrenceRate).To(BeNumerically(">=", 0.0))
	Expect(data.RecurrenceRate).To(BeNumerically("<=", 1.0))
}
