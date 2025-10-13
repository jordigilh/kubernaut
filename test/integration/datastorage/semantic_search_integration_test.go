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

package datastorage

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/database/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

var _ = Describe("BR-STORAGE-012: Semantic Search Integration", func() {
	var (
		queryService *query.Service
		testSchema   string
		testCtx      context.Context
	)

	BeforeEach(func() {
		testCtx = context.Background()

		// Create unique test schema
		testSchema = fmt.Sprintf("test_semantic_%d", GinkgoRandomSeed())
		_, err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", testSchema))
		Expect(err).ToNot(HaveOccurred())

		// Set search path to test schema
		_, err = db.Exec(fmt.Sprintf("SET search_path TO %s", testSchema))
		Expect(err).ToNot(HaveOccurred())

		// Initialize schema in test schema
		initializer := schema.NewInitializer(db, logger)
		err = initializer.Initialize(testCtx)
		Expect(err).ToNot(HaveOccurred())

		// Seed database with test data including embeddings
		seedSemanticSearchData()

		// Create query service
		queryService = query.NewService(sqlxDB, logger)
	})

	AfterEach(func() {
		// Reset search path
		_, _ = db.Exec("SET search_path TO public")

		// Drop test schema
		if testSchema != "" {
			_, _ = db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", testSchema))
		}
	})

	It("should perform semantic search with embeddings", func() {
		// BR-STORAGE-012: Semantic search capability
		results, err := queryService.SemanticSearch(ctx, "pod restart failure")

		Expect(err).ToNot(HaveOccurred())
		Expect(results).ToNot(BeEmpty(), "semantic search should return results with embeddings")

		// Verify results have similarity scores
		for _, result := range results {
			Expect(result.Similarity).To(BeNumerically(">", 0.0))
			Expect(result.Similarity).To(BeNumerically("<=", 1.0))
		}
	})

	It("should return results ordered by similarity (highest first)", func() {
		// BR-STORAGE-012: Results ordered by similarity
		results, err := queryService.SemanticSearch(ctx, "deployment scaling issue")

		Expect(err).ToNot(HaveOccurred())
		Expect(results).To(HaveLen(10), "should limit to 10 results")

		// Verify results are ordered by similarity (descending)
		for i := 0; i < len(results)-1; i++ {
			Expect(results[i].Similarity).To(BeNumerically(">=", results[i+1].Similarity),
				"results should be ordered by similarity (highest first)")
		}
	})

	It("should handle queries with no matching embeddings", func() {
		// BR-STORAGE-012: Graceful handling of no results
		_, err := queryService.SemanticSearch(ctx, "completely unrelated gibberish xyz123")

		Expect(err).ToNot(HaveOccurred())
		// Results may be empty or have low similarity scores
		// This is acceptable behavior for semantic search
	})
})

// seedSemanticSearchData creates test audit data with embeddings for semantic search
func seedSemanticSearchData() {
	baseTime := time.Now()

	// Create 10 audits with varied embeddings to test similarity search
	testCases := []struct {
		name        string
		description string
		seed        float32
	}{
		{"pod-restart-001", "pod restart failure in production", 0.1},
		{"pod-restart-002", "pod restart issue with memory", 0.15},
		{"deployment-scale-001", "deployment scaling issue", 0.2},
		{"deployment-scale-002", "deployment failed to scale", 0.25},
		{"memory-leak-001", "memory leak detected", 0.3},
		{"memory-leak-002", "memory leak in pod", 0.35},
		{"network-issue-001", "network connectivity problem", 0.4},
		{"network-issue-002", "network timeout error", 0.45},
		{"disk-space-001", "disk space exhausted", 0.5},
		{"disk-space-002", "disk quota exceeded", 0.55},
	}

	for i, tc := range testCases {
		embedding := generateTestEmbedding(tc.seed)

		// Convert embedding to pgvector format
		embeddingStr := embeddingToString(embedding)

		_, err := db.Exec(`
			INSERT INTO remediation_audit (
				name, namespace, phase, action_type, status,
				start_time, remediation_request_id, alert_fingerprint,
				severity, environment, cluster_name, target_resource,
				metadata, embedding, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::vector, $15, $16)
		`,
			tc.name,
			"default",
			"completed",
			"scale_deployment",
			"success",
			baseTime.Add(-time.Duration(i)*time.Hour),
			"req-semantic-"+tc.name,
			"alert-semantic",
			"high",
			"production",
			"prod-cluster",
			"deployment/app",
			`{"description": "`+tc.description+`"}`,
			embeddingStr,
			baseTime,
			baseTime,
		)
		Expect(err).ToNot(HaveOccurred())
	}
}

// embeddingToString converts a float32 slice to pgvector string format '[x,y,z,...]'
func embeddingToString(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}

	result := "["
	for i, val := range embedding {
		if i > 0 {
			result += ","
		}
		result += formatFloat(val)
	}
	result += "]"
	return result
}

// formatFloat formats a float32 for pgvector
func formatFloat(f float32) string {
	return fmt.Sprintf("%f", f)
}
