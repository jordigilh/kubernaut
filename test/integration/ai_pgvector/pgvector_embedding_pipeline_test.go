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

//go:build integration
// +build integration

package ai_pgvector

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/embedding"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-AI-PGVECTOR-001: pgvector Embedding Pipeline Integration", Ordered, func() {
	var (
		hooks             *testshared.TestLifecycleHooks
		ctx               context.Context
		suite             *testshared.StandardTestSuite
		embeddingPipeline *embedding.AIEmbeddingPipeline
		logger            *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure
		hooks = testshared.SetupAIIntegrationTest("pgvector Embedding Pipeline",
			testshared.WithRealVectorDB(), // Current milestone: pgvector only
			testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
		)
		hooks.Setup()

		suite = hooks.GetSuite()
		logger = suite.Logger
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx = context.Background()

		// Validate test environment is healthy before each test
		Expect(suite.VectorDB).ToNot(BeNil(), "Vector database should be available")
		Expect(suite.LLMClient.IsHealthy()).To(BeTrue(), "LLM client should be healthy")

		// Create embedding pipeline for integration testing
		embeddingPipeline = embedding.NewAIEmbeddingPipeline(suite.LLMClient, suite.VectorDB, logger)
		Expect(embeddingPipeline).ToNot(BeNil(), "Embedding pipeline should be created successfully")
	})

	Context("when processing AI analysis through complete pgvector pipeline", func() {
		It("should process AI analysis → pgvector storage → retrieval with accuracy optimization", func() {
			By("creating test alert for AI analysis")
			testAlert := &types.Alert{
				ID:          "test-alert-embedding-pipeline-001",
				Summary:     "High CPU usage detected on critical pod",
				Description: "CPU utilization has exceeded 90% for more than 5 minutes on production workload",
				Severity:    "high",
				Status:      "firing",
				Namespace:   "production",
				Resource:    "pod/critical-app-7d9f8b6c5d-x9m2n",
				Labels: map[string]string{
					"pod_name":    "critical-app-7d9f8b6c5d-x9m2n",
					"cpu_percent": "94.2",
					"threshold":   "90",
				},
				Annotations: map[string]string{
					"duration": "7m23s",
					"source":   "prometheus",
				},
			}

			By("processing alert through AI analysis")
			aiStartTime := time.Now()
			recommendation, err := suite.LLMClient.AnalyzeAlert(ctx, *testAlert)
			aiProcessingTime := time.Since(aiStartTime)

			// Following guideline: Always handle errors, never ignore them
			Expect(err).ToNot(HaveOccurred(), "AI analysis should complete successfully")
			Expect(recommendation).ToNot(BeNil(), "AI should provide recommendation")

			// BR-AI-PGVECTOR-001: AI processing accuracy validation
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7), "BR-AI-PGVECTOR-001: AI recommendation confidence should meet minimum threshold")
			Expect(recommendation.Action).ToNot(BeEmpty(), "BR-AI-PGVECTOR-001: AI should provide actionable recommendation")

			By("storing AI analysis results in pgvector with accuracy-optimized embeddings")
			embeddingRequest := &embedding.EmbeddingRequest{
				ID:      fmt.Sprintf("embedding-%s", testAlert.ID),
				Content: recommendation.Reasoning.Summary,
				Metadata: map[string]interface{}{
					"alert_id":        testAlert.ID,
					"action":          recommendation.Action,
					"confidence":      recommendation.Confidence,
					"processing_time": aiProcessingTime.Milliseconds(),
					"optimization":    "accuracy_cost", // Current milestone focus
				},
			}

			storageStartTime := time.Now()
			err = embeddingPipeline.StoreEmbedding(ctx, embeddingRequest)
			storageTime := time.Since(storageStartTime)

			Expect(err).ToNot(HaveOccurred(), "Embedding storage should succeed")

			// BR-AI-PGVECTOR-002: Storage efficiency validation
			Expect(storageTime).To(BeNumerically("<", 3*time.Second), "BR-AI-PGVECTOR-002: Storage should be cost-effective (under 3s)")

			By("retrieving similar embeddings with accuracy-optimized similarity search")
			retrievalStartTime := time.Now()
			similarEmbeddings, err := embeddingPipeline.RetrieveSimilarEmbeddings(ctx, recommendation.Reasoning.Summary, 3)
			retrievalTime := time.Since(retrievalStartTime)

			Expect(err).ToNot(HaveOccurred(), "Similarity search should succeed")
			Expect(len(similarEmbeddings)).To(BeNumerically(">=", 1), "Should retrieve at least the stored embedding")

			// BR-AI-PGVECTOR-003: Retrieval accuracy validation
			Expect(retrievalTime).To(BeNumerically("<", 2*time.Second), "BR-AI-PGVECTOR-003: Retrieval should be efficient")

			// Validate similarity threshold meets accuracy requirements
			for _, embedding := range similarEmbeddings {
				Expect(embedding.Similarity).To(BeNumerically(">=", 0.7), "BR-AI-PGVECTOR-003: Similarity should meet accuracy threshold")
			}

			By("validating end-to-end pipeline performance and cost optimization")
			totalPipelineTime := aiProcessingTime + storageTime + retrievalTime

			// BR-AI-PGVECTOR-004: End-to-end performance validation
			Expect(totalPipelineTime).To(BeNumerically("<", 10*time.Second), "BR-AI-PGVECTOR-004: Complete pipeline should finish within cost constraints")

			// Validate cost optimization - accuracy over speed focus
			costMetrics := embeddingPipeline.GetCostMetrics(ctx)
			Expect(costMetrics.AccuracyScore).To(BeNumerically(">=", 0.85), "BR-AI-PGVECTOR-004: Should prioritize accuracy")
			Expect(costMetrics.StorageEfficiency).To(BeNumerically(">=", 0.8), "BR-AI-PGVECTOR-004: Should maintain storage efficiency")
		})

		It("should handle cost-effective vector operations under resource constraints", func() {
			By("creating multiple test scenarios for cost validation")
			testScenarios := []struct {
				AlertType   string
				ExpectedOps int
				MaxCost     float64
			}{
				{"critical_cpu", 2, 0.10},   // High priority, realistic operation count
				{"warning_memory", 2, 0.05}, // Medium priority, cost-conscious
				{"info_disk", 1, 0.02},      // Low priority, minimal operations (analysis only)
			}

			totalCost := 0.0
			for _, scenario := range testScenarios {
				By(fmt.Sprintf("processing %s scenario with cost constraints", scenario.AlertType))

				testAlert := createTestAlertForScenario(scenario.AlertType)

				// Process through cost-optimized pipeline
				costResult, err := embeddingPipeline.ProcessWithCostOptimization(ctx, testAlert, scenario.MaxCost)

				Expect(err).ToNot(HaveOccurred(), "Cost-optimized processing should succeed")
				Expect(costResult.ActualCost).To(BeNumerically("<=", scenario.MaxCost), "Should respect cost constraints")
				Expect(costResult.OperationsCompleted).To(BeNumerically(">=", scenario.ExpectedOps), "Should complete minimum required operations")

				totalCost += costResult.ActualCost
			}

			// BR-AI-PGVECTOR-005: Overall cost efficiency validation
			Expect(totalCost).To(BeNumerically("<=", 0.20), "BR-AI-PGVECTOR-005: Total processing cost should be optimized")
		})
	})

	Context("when validating pgvector connection management", func() {
		It("should optimize connection pooling for accuracy and cost", func() {
			By("testing connection pool efficiency")

			// Current milestone: Focus on accuracy/cost over speed
			poolMetrics := embeddingPipeline.GetConnectionPoolMetrics(ctx)

			// BR-AI-PGVECTOR-006: Connection efficiency validation
			Expect(poolMetrics.ActiveConnections).To(BeNumerically("<=", 5), "BR-AI-PGVECTOR-006: Should use efficient connection count")
			Expect(poolMetrics.IdleConnections).To(BeNumerically(">=", 2), "BR-AI-PGVECTOR-006: Should maintain cost-effective idle connections")
			Expect(poolMetrics.ConnectionReuse).To(BeNumerically(">=", 0.8), "BR-AI-PGVECTOR-006: Should prioritize connection reuse for cost optimization")
		})

		It("should handle graceful degradation under resource constraints", func() {
			By("simulating resource-constrained environment")

			// Limit resources to test graceful degradation
			resourceLimits := &embedding.ResourceConstraints{
				MaxMemoryMB:     100,
				MaxConnections:  2,
				MaxProcessingMs: 5000,
			}

			err := embeddingPipeline.ApplyResourceConstraints(ctx, resourceLimits)
			Expect(err).ToNot(HaveOccurred(), "Should apply resource constraints successfully")

			By("processing alert under constraints")
			testAlert := createTestAlertForScenario("resource_constrained")

			result, err := embeddingPipeline.ProcessWithResourceConstraints(ctx, testAlert)

			// Should succeed with degraded performance but maintain accuracy
			Expect(err).ToNot(HaveOccurred(), "Should handle resource constraints gracefully")
			Expect(result.ProcessingCompleted).To(BeTrue(), "Should complete processing despite constraints")
			Expect(result.AccuracyMaintained).To(BeNumerically(">=", 0.8), "BR-AI-PGVECTOR-007: Should maintain accuracy under constraints")
		})
	})
})

// Helper functions for test scenarios

// createTestAlertForScenario creates test alerts for different scenarios
func createTestAlertForScenario(alertType string) *types.Alert {
	scenarios := map[string]*types.Alert{
		"critical_cpu": {
			ID:          "critical-cpu-001",
			Summary:     "Critical CPU utilization",
			Description: "CPU usage exceeds critical threshold",
			Severity:    "critical",
			Status:      "firing",
		},
		"warning_memory": {
			ID:          "warning-memory-001",
			Summary:     "Warning memory usage",
			Description: "Memory usage approaching threshold",
			Severity:    "warning",
			Status:      "firing",
		},
		"info_disk": {
			ID:          "info-disk-001",
			Summary:     "Info disk usage",
			Description: "Disk usage information",
			Severity:    "info",
			Status:      "firing",
		},
		"resource_constrained": {
			ID:          "resource-constrained-001",
			Summary:     "Resource constrained scenario",
			Description: "Testing under resource limitations",
			Severity:    "warning",
			Status:      "firing",
		},
	}

	alert, exists := scenarios[alertType]
	if !exists {
		return scenarios["info_disk"] // Default fallback
	}
	return alert
}
