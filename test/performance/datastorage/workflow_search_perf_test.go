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
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/pgvector/pgvector-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// Performance Tests for Workflow Search with Hybrid Weighted Scoring
//
// Business Requirements:
// - BR-STORAGE-013: Semantic search with hybrid weighted scoring must be performant
//
// Performance Targets (Local Testing):
// - P50 Latency: <100ms
// - P95 Latency: <200ms
// - P99 Latency: <500ms
// - Concurrent Queries: 10 QPS sustained
//
// Test Scales:
// - 1K workflows: Typical production catalog size
// - 5K workflows: Large production catalog
// - 10K workflows: Stress test
//
// Note: These are local performance tests using Podman PostgreSQL.
// Production-scale testing (100K+ workflows, high concurrency) deferred to V1.1+

var _ = Describe("Workflow Search Performance", Label("performance"), func() {
	var (
		testCtx context.Context
		testID  string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testID = fmt.Sprintf("perf-%d", time.Now().UnixNano())
	})

	Context("with 1K workflows", func() {
		var workflowCount int = 1000

		BeforeEach(func() {
			logger.Info("📦 Seeding workflow catalog...", zap.Int("count", workflowCount))
			seedWorkflows(testCtx, workflowCount, testID)
			logger.Info("✅ Workflow catalog seeded")
		})

		AfterEach(func() {
			logger.Info("🧹 Cleaning up test workflows...")
			cleanupWorkflows(testCtx, testID)
			logger.Info("✅ Cleanup complete")
		})

		It("should achieve P50 latency <100ms", func() {
			// ARRANGE: Prepare search request
			embedding := generateTestEmbedding("OOMKilled critical gitops argocd production")
			embeddingVec := pgvector.NewVector(embedding)
			filters := &models.WorkflowSearchFilters{
				SignalType:         "OOMKilled",
				Severity:           "critical",
				ResourceManagement: stringPtr("gitops"),
				GitOpsTool:         stringPtr("argocd"),
			}
			searchReq := &models.WorkflowSearchRequest{
				Query:     "OOMKilled critical with GitOps ArgoCD",
				Embedding: &embeddingVec,
				Filters:   filters,
				TopK:      10,
			}

			// ACT: Run 100 searches and measure latency
			latencies := make([]time.Duration, 100)
			for i := 0; i < 100; i++ {
				start := time.Now()
				_, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)
				latencies[i] = time.Since(start)
				Expect(err).ToNot(HaveOccurred())
			}

			// ASSERT: Calculate P50 and verify
			p50 := calculatePercentile(latencies, 50)
			logger.Info("Performance Results (1K workflows)",
				zap.Duration("P50", p50),
				zap.String("target", "<100ms"))

			Expect(p50).To(BeNumerically("<", 100*time.Millisecond),
				fmt.Sprintf("P50 latency (%v) should be <100ms", p50))
		})

		It("should achieve P95 latency <200ms", func() {
			// ARRANGE: Prepare search request
			embedding := generateTestEmbedding("OOMKilled critical gitops argocd production")
			embeddingVec := pgvector.NewVector(embedding)
			filters := &models.WorkflowSearchFilters{
				SignalType:         "OOMKilled",
				Severity:           "critical",
				ResourceManagement: stringPtr("gitops"),
			}
			searchReq := &models.WorkflowSearchRequest{
				Query:     "OOMKilled critical with GitOps",
				Embedding: &embeddingVec,
				Filters:   filters,
				TopK:      10,
			}

			// ACT: Run 100 searches and measure latency
			latencies := make([]time.Duration, 100)
			for i := 0; i < 100; i++ {
				start := time.Now()
				_, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)
				latencies[i] = time.Since(start)
				Expect(err).ToNot(HaveOccurred())
			}

			// ASSERT: Calculate P95 and verify
			p95 := calculatePercentile(latencies, 95)
			logger.Info("Performance Results (1K workflows)",
				zap.Duration("P95", p95),
				zap.String("target", "<200ms"))

			Expect(p95).To(BeNumerically("<", 200*time.Millisecond),
				fmt.Sprintf("P95 latency (%v) should be <200ms", p95))
		})

		It("should achieve P99 latency <500ms", func() {
			// ARRANGE: Prepare search request
			embedding := generateTestEmbedding("OOMKilled critical gitops argocd production")
			embeddingVec := pgvector.NewVector(embedding)
			filters := &models.WorkflowSearchFilters{
				SignalType: "OOMKilled",
				Severity:   "critical",
			}
			searchReq := &models.WorkflowSearchRequest{
				Query:     "OOMKilled critical",
				Embedding: &embeddingVec,
				Filters:   filters,
				TopK:      10,
			}

			// ACT: Run 100 searches and measure latency
			latencies := make([]time.Duration, 100)
			for i := 0; i < 100; i++ {
				start := time.Now()
				_, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)
				latencies[i] = time.Since(start)
				Expect(err).ToNot(HaveOccurred())
			}

			// ASSERT: Calculate P99 and verify
			p99 := calculatePercentile(latencies, 99)
			logger.Info("Performance Results (1K workflows)",
				zap.Duration("P99", p99),
				zap.String("target", "<500ms"))

			Expect(p99).To(BeNumerically("<", 500*time.Millisecond),
				fmt.Sprintf("P99 latency (%v) should be <500ms", p99))
		})

		It("should handle 10 concurrent queries (10 QPS)", func() {
			// ARRANGE: Prepare search requests
			embedding := generateTestEmbedding("OOMKilled critical gitops production")
			embeddingVec := pgvector.NewVector(embedding)
			filters := &models.WorkflowSearchFilters{
				SignalType: "OOMKilled",
				Severity:   "critical",
			}
			searchReq := &models.WorkflowSearchRequest{
				Query:     "OOMKilled critical",
				Embedding: &embeddingVec,
				Filters:   filters,
				TopK:      10,
			}

			// ACT: Run 10 concurrent searches
			start := time.Now()
			done := make(chan error, 10)
			for i := 0; i < 10; i++ {
				go func() {
					_, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)
					done <- err
				}()
			}

			// Wait for all searches to complete
			for i := 0; i < 10; i++ {
				err := <-done
				Expect(err).ToNot(HaveOccurred())
			}
			duration := time.Since(start)

			// ASSERT: Verify QPS
			qps := float64(10) / duration.Seconds()
			logger.Info("Concurrent Query Performance (1K workflows)",
				zap.Float64("QPS", qps),
				zap.Duration("total_duration", duration),
				zap.String("target", "≥10 QPS"))

			Expect(qps).To(BeNumerically(">=", 10.0),
				fmt.Sprintf("QPS (%.2f) should be ≥10", qps))
		})

		It("should provide latency distribution summary", func() {
			// ARRANGE: Prepare search request
			embedding := generateTestEmbedding("OOMKilled critical gitops argocd production")
			embeddingVec := pgvector.NewVector(embedding)
			filters := &models.WorkflowSearchFilters{
				SignalType:         "OOMKilled",
				Severity:           "critical",
				ResourceManagement: stringPtr("gitops"),
				GitOpsTool:         stringPtr("argocd"),
			}
			searchReq := &models.WorkflowSearchRequest{
				Query:     "OOMKilled critical with GitOps ArgoCD",
				Embedding: &embeddingVec,
				Filters:   filters,
				TopK:      10,
			}

			// ACT: Run 100 searches and measure latency
			latencies := make([]time.Duration, 100)
			for i := 0; i < 100; i++ {
				start := time.Now()
				_, err := workflowRepo.SearchByEmbedding(testCtx, searchReq)
				latencies[i] = time.Since(start)
				Expect(err).ToNot(HaveOccurred())
			}

			// ASSERT: Calculate all percentiles
			p50 := calculatePercentile(latencies, 50)
			p95 := calculatePercentile(latencies, 95)
			p99 := calculatePercentile(latencies, 99)
			min := latencies[0]
			max := latencies[0]
			var sum time.Duration
			for _, lat := range latencies {
				if lat < min {
					min = lat
				}
				if lat > max {
					max = lat
				}
				sum += lat
			}
			avg := sum / time.Duration(len(latencies))

			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("Latency Distribution Summary (1K workflows, 100 queries)")
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			logger.Info("Latency Metrics:",
				zap.Duration("Min", min),
				zap.Duration("Avg", avg),
				zap.Duration("P50", p50),
				zap.Duration("P95", p95),
				zap.Duration("P99", p99),
				zap.Duration("Max", max))
			logger.Info("Performance Targets:",
				zap.String("P50_target", "<100ms"),
				zap.String("P95_target", "<200ms"),
				zap.String("P99_target", "<500ms"))
			logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Verify all targets
			Expect(p50).To(BeNumerically("<", 100*time.Millisecond), "P50 target")
			Expect(p95).To(BeNumerically("<", 200*time.Millisecond), "P95 target")
			Expect(p99).To(BeNumerically("<", 500*time.Millisecond), "P99 target")
		})
	})
})

// Helper functions

func seedWorkflows(ctx context.Context, count int, testID string) {
	signalTypes := []string{"OOMKilled", "CrashLoopBackOff", "ImagePullBackOff", "NodeNotReady", "PodEvicted"}
	severities := []string{"critical", "high", "medium", "low"}
	resourceMgmt := []string{"gitops", "manual", "helm", "terraform", "operator"}
	gitopsTools := []string{"argocd", "flux", ""}
	environments := []string{"production", "staging", "development"}
	businessCats := []string{"revenue-critical", "customer-facing", "internal"}
	priorities := []string{"P0", "P1", "P2", "P3"}
	riskTols := []string{"low", "medium", "high"}

	for i := 0; i < count; i++ {
		// Generate random labels
		signalType := signalTypes[rand.Intn(len(signalTypes))]
		severity := severities[rand.Intn(len(severities))]
		resourceMgmtVal := resourceMgmt[rand.Intn(len(resourceMgmt))]
		gitopsTool := gitopsTools[rand.Intn(len(gitopsTools))]
		environment := environments[rand.Intn(len(environments))]
		businessCat := businessCats[rand.Intn(len(businessCats))]
		priority := priorities[rand.Intn(len(priorities))]
		riskTol := riskTols[rand.Intn(len(riskTols))]

		// Generate random embedding
		embedding := generateTestEmbedding(fmt.Sprintf("%s %s %s %s", signalType, severity, resourceMgmtVal, environment))
		embeddingVec := pgvector.NewVector(embedding)

		// Marshal labels to JSON
		labelsMap := map[string]interface{}{
			"signal_type":         signalType,
			"severity":            severity,
			"resource_management": resourceMgmtVal,
			"gitops_tool":         gitopsTool,
			"environment":         environment,
			"business_category":   businessCat,
			"priority":            priority,
			"risk_tolerance":      riskTol,
		}
		labelsJSON, err := json.Marshal(labelsMap)
		Expect(err).ToNot(HaveOccurred())

		// Create workflow
		workflow := &models.RemediationWorkflow{
			WorkflowID:       fmt.Sprintf("perf-wf-%s-%d", testID, i),
			Version:          "1.0.0",
			Name:             fmt.Sprintf("Performance Test Workflow %d", i),
			Description:      fmt.Sprintf("Test workflow for performance testing: %s %s", signalType, severity),
			Content:          fmt.Sprintf("# Workflow content for performance test %d", i),
			ContentHash:      fmt.Sprintf("%064d", i), // Dummy hash for testing
			Labels:           labelsJSON,
			Embedding:        &embeddingVec,
			Status:           "active",
			IsLatestVersion:  true,
		}

		err = workflowRepo.Create(ctx, workflow)
		Expect(err).ToNot(HaveOccurred())

		// Progress indicator
		if (i+1)%100 == 0 {
			logger.Info(fmt.Sprintf("  Seeded %d/%d workflows...", i+1, count))
		}
	}
}

func cleanupWorkflows(ctx context.Context, testID string) {
	query := fmt.Sprintf("DELETE FROM remediation_workflow_catalog WHERE workflow_id LIKE 'perf-wf-%s-%%'", testID)
	_, err := db.ExecContext(ctx, query)
	Expect(err).ToNot(HaveOccurred())
}

func generateTestEmbedding(text string) []float32 {
	// Generate a deterministic 384-dimensional embedding
	embedding := make([]float32, 384)
	hash := 0
	for _, c := range text {
		hash = (hash*31 + int(c)) % 1000
	}

	// Fill embedding with deterministic values based on hash
	for i := range embedding {
		embedding[i] = float32((hash+i)%100) / 100.0
	}

	return embedding
}

func calculatePercentile(latencies []time.Duration, percentile int) time.Duration {
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func stringPtr(s string) *string {
	return &s
}

